// brabeus serves a private, git-backed personal record over MCP.
//
// The record is a git repository. This kernel is its only writer, which is what
// makes write conflicts structurally impossible rather than merely unlikely. A
// second repository may be mounted alongside it read-only, as a mirror the record
// can point into without copying.
//
// Design: docs/personal-context-system-v1.md.
//
// This file is composition only. Anything with a decision in it belongs in a
// file that has a test.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	// TZ must work in a minimal image with no zoneinfo, because claim deadlines
	// are compared in it (spec §8.1).
	_ "time/tzdata"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/allanschon/brabeus/internal/block"
	"github.com/allanschon/brabeus/internal/claims"
	"github.com/allanschon/brabeus/internal/identity"
	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/retrieval"
	"github.com/allanschon/brabeus/internal/server"
	"github.com/allanschon/brabeus/internal/store"
	"github.com/allanschon/brabeus/internal/view"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envEither reads the notebook's new name first and its old one second, so
// a deployment configured before the rename keeps working unchanged.
func envEither(newKey, oldKey, def string) string {
	if v := os.Getenv(newKey); v != "" {
		return v
	}
	return env(oldKey, def)
}

// notebookEmbedder gives the notebook the record's embedder only when the
// deployment asked for it.
func notebookEmbedder(e retrieval.Embedder) retrieval.Embedder {
	if os.Getenv("BRABEUS_NOTEBOOK_EMBED") != "1" {
		return nil
	}
	return e
}

func main() {
	branch := env("BRABEUS_BRANCH", "main")
	// Beside the working copies, on the data volume, so a host key accepted
	// once survives the container.
	knownHosts := env("BRABEUS_KNOWN_HOSTS", "/var/lib/brabeus/known_hosts")
	if err := store.PrepareKnownHosts(knownHosts); err != nil {
		log.Fatalf("known_hosts: %v", err)
	}

	// The enabled modules and their manifests. The kernel refuses to start on
	// a set it does not understand (spec §6); a wrong set is a deployment
	// error, and finding it at the first write would be later and quieter.
	set, err := module.Load(env("BRABEUS_MODULES_DIR", "/etc/brabeus/modules"),
		strings.Split(env("BRABEUS_MODULES", "memory"), ","))
	if err != nil {
		log.Fatalf("modules: %v", err)
	}
	log.Printf("modules: %s (profiles %v)", strings.Join(set.Names(), ","), set.Profiles())

	// Parsed here, beside the module set, so a broken summary template is a
	// startup failure rather than a surprise at the first session.
	renderer, err := block.New(set)
	if err != nil {
		log.Fatalf("templates: %v", err)
	}
	consumers := server.ParseConsumers(os.Getenv("BRABEUS_CONSUMERS"))

	// The dense leg is OFF unless a sidecar is named. That is deliberate:
	// the server must remain a working keyword-only store when the sidecar is
	// not deployed, and it reports "unavailable" rather than pretending.
	embedder := newEmbedder()
	if embedder == nil {
		log.Printf("no BRABEUS_EMBED_URL: search is keyword-only")
	} else {
		log.Printf("embedding sidecar %s, model %s", env("BRABEUS_EMBED_URL", ""), env("BRABEUS_EMBED_MODEL", retrieval.DefaultEmbedModel))
	}

	repoURL := os.Getenv("BRABEUS_REPO")
	if repoURL == "" {
		log.Fatalf("BRABEUS_REPO is required (ssh://… or https://… to the private record repository)")
	}
	memory := &store.Store{
		Embedder:    embedder,
		Dir:         env("BRABEUS_DIR", "/var/lib/brabeus/memory"),
		RemoteURL:   repoURL,
		Branch:      branch,
		SSHCommand:  store.SSHCommandFor(env("BRABEUS_SSH_KEY", "/etc/brabeus/deploy_key"), knownHosts),
		CommitName:  env("BRABEUS_COMMIT_NAME", "brabeus"),
		CommitEmail: env("BRABEUS_COMMIT_EMAIL", "brabeus@localhost"),
	}
	if err := memory.Ensure(); err != nil {
		log.Fatalf("memory store: %v", err)
	}
	// Every write is validated against this set from here on. The mirror below
	// gets no set: it is read-only and never validates.
	memory.SetModules(set)

	// A goal's claims are checked against the adapters' argument schemas when
	// it is written, so no goal carries a claim that no run could evaluate
	// (spec §8.1). A backend named but misconfigured stops the kernel here
	// rather than reading no-evidence on every claim for a day.
	memory.ValidateClaim = claims.Validate
	adapters, err := claims.New(claims.FromEnv(os.Getenv), nil)
	if err != nil {
		log.Fatalf("claims: %v", err)
	}
	interval, err := time.ParseDuration(env("BRABEUS_CLAIM_INTERVAL", "24h"))
	if err != nil || interval < 0 {
		log.Fatalf("BRABEUS_CLAIM_INTERVAL %q: a duration such as 24h, or 0 to disable the schedule", os.Getenv("BRABEUS_CLAIM_INTERVAL"))
	}
	// The stamp sits beside the working copy on the data volume, not in the
	// record: when claims last ran is the deployment's fact, not the person's.
	runner := &claims.Runner{Store: memory, Adapters: adapters, Interval: interval, Now: time.Now,
		StatePath: filepath.Join(filepath.Dir(memory.Dir), "claims-last-run")}

	// One-time, explicit, and removed from the environment once it has run:
	// the migration is the only thing that rewrites records nobody asked to
	// change, and it must not be a thing that happens to a restart.
	if os.Getenv("BRABEUS_MIGRATE") == "1" {
		rep, err := memory.Migrate("kernel")
		if err != nil {
			log.Fatalf("migration: %v", err)
		}
		log.Printf("migration: %d retagged (%d given a stamp), %d already tagged, commit %q — unset BRABEUS_MIGRATE", rep.Retagged, rep.Stamped, rep.Tagged, rep.Commit)
	}
	log.Printf("memory store ready at %s", memory.Dir)

	// The notebook is optional: the server is useful without it, and failing
	// to clone it must not take the memory store down too. BRABEUS_NOTEBOOK_*
	// is read first and BRABEUS_MIRROR_* second, so a deployment configured
	// under the older name keeps working unchanged (envEither).
	var projects *store.Store
	if url := envEither("BRABEUS_NOTEBOOK_REPO", "BRABEUS_MIRROR_REPO", ""); url != "" {
		projects = &store.Store{
			// The notebook shares the record's embedder only when
			// BRABEUS_NOTEBOOK_EMBED=1: its cold pass runs minutes, not
			// seconds (227 s for 467 files, measured 2026-09-28), and it
			// builds in the background exactly as the record's own corpus
			// does, so it never delays startup or an answer in flight.
			Embedder:  notebookEmbedder(embedder),
			Dir:       envEither("BRABEUS_NOTEBOOK_DIR", "BRABEUS_MIRROR_DIR", "/var/lib/brabeus/projects"),
			RemoteURL: url,
			Branch:    branch,
			// A DIFFERENT key, read-only on the notebook repository. Reusing
			// the memory key here would have to mean widening it, and the
			// whole design rests on the writer being unable to reach the
			// publishable tree.
			SSHCommand: store.SSHCommandFor(envEither("BRABEUS_NOTEBOOK_SSH_KEY", "BRABEUS_MIRROR_SSH_KEY", "/etc/brabeus/projects_key"), knownHosts),
			ReadOnly:   true,
		}
		if err := projects.Ensure(); err != nil {
			log.Printf("notebook unavailable, continuing without it: %v", err)
			projects = nil
		} else {
			dense := "keyword-only"
			if projects.Embedder != nil {
				dense = "dense on"
			}
			log.Printf("notebook ready at %s (read-only, %s)", projects.Dir, dense)
		}
	}

	addr := env("BRABEUS_LISTEN", "127.0.0.1:8082")
	id, err := newIdentity(addr)
	if err != nil {
		log.Fatalf("identity: %v", err)
	}
	log.Printf("identity: %s", id.Describe())

	deps := server.Deps{Memory: memory, Projects: projects, Set: set, Block: renderer, Now: time.Now,
		LastRun: runner.LastRun, ClaimInterval: interval}

	// Shutdown waits for the claims runner, so a results file it has written
	// is committed before the process exits: one left uncommitted would read
	// as "no change" to every later run and never reach the remote.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	runnerDone := make(chan struct{})
	go func() {
		defer close(runnerDone)
		runner.Start(ctx)
	}()

	go func() {
		for range time.Tick(15 * time.Minute) {
			if err := memory.Sync(); err != nil {
				log.Printf("memory sync: %v", err)
			}
			if projects != nil {
				if err := projects.Sync(); err != nil {
					log.Printf("projects sync: %v", err)
				}
			}
		}
	}()

	// RefuseUnidentified sits closest to each handler (spec §11): a caller the
	// identity mode cannot name never reaches tool dispatch or the block,
	// whether or not auth is also configured.
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		caller, consumer, _ := server.Caller(id, consumers, r)
		return server.New(deps, caller, consumer)
	}, nil)

	authMode, authToken := env("BRABEUS_AUTH_MODE", "none"), os.Getenv("BRABEUS_AUTH_TOKEN")
	guarded, err := server.AuthMiddleware(server.RefuseUnidentified(mcpHandler, id, consumers), authMode, authToken)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}
	// /context serves the same block a session's SessionStart hook wants, so
	// it needs the same identity and auth boundary as /mcp, not a lighter one.
	ctxGuarded, err := server.AuthMiddleware(server.RefuseUnidentified(server.ContextHandler(deps, id, consumers), id, consumers), authMode, authToken)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	insGuarded, err := server.AuthMiddleware(server.RefuseUnidentified(server.InstructionsHandler(deps, id, consumers), id, consumers), authMode, authToken)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	vr, err := view.New(set)
	if err != nil {
		log.Fatalf("view: %v", err)
	}
	viewGuarded, err := server.AuthMiddleware(server.RefuseUnidentified(server.ViewHandler(deps, vr, id, consumers), id, consumers), authMode, authToken)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", guarded)
	mux.Handle("/context", ctxGuarded)
	mux.Handle("/instructions", insGuarded)
	// The view sits behind the same identity and auth as /context (spec §10);
	// its own mux answers every method but GET with 405.
	mux.Handle("/view/", viewGuarded)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, healthz(server.Version, id.Mode(), set, claims.Status(runner), claims.StatusErrors(runner)))
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		// A second signal during the drain kills the process as usual.
		stop()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("brabeus %s listening on %s", server.Version, addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-runnerDone
	log.Printf("brabeus stopped")
}

// newIdentity selects how the calling machine is identified. tailscale is the
// default; token exists for clients that are not on the Tailscale network and
// therefore have no peer identity.
func newIdentity(listen string) (identity.Identity, error) {
	switch mode := env("BRABEUS_IDENTITY_MODE", "tailscale"); mode {
	case "tailscale":
		// Loopback by default. Behind Docker's published port the proxy
		// arrives from the bridge gateway instead, and that address has to
		// be named here or every caller is unknown, which is refused at
		// startup rather than discovered per call.
		trusted, err := identity.ParseTrustedProxies(env("BRABEUS_TRUSTED_PROXIES", "127.0.0.0/8,::1/128"))
		if err != nil {
			return nil, err
		}
		if err := identity.CheckTrustReachable(listen, trusted); err != nil {
			return nil, err
		}
		return identity.NewTailscale(env("BRABEUS_TAILSCALE_SOCK", "/var/run/tailscale/tailscaled.sock"), trusted), nil
	case "token":
		return identity.NewToken(os.Getenv("BRABEUS_TOKENS"))
	default:
		return nil, fmt.Errorf("unknown BRABEUS_IDENTITY_MODE %q: use tailscale or token", mode)
	}
}

// newEmbedder builds the dense leg's client, or nil if no sidecar is named.
//
// RAW, not cached. The Store wraps this for the corpus pass itself and
// leaves the read path uncached on purpose — see Store.denseCache. Wrapping it
// here cached queries too, which made a dead sidecar keep reporting `on` for
// any question already asked.
//
// The notebook gets this embedder too, but only when BRABEUS_NOTEBOOK_EMBED=1
// (notebookEmbedder). It is a read-only mirror of a much larger tree, and its
// cold embedding pass runs minutes rather than seconds; the default is off so
// a deployment that never searches it by meaning does not pay that cost on
// every change, and its lexical search already works without it.
func newEmbedder() retrieval.Embedder {
	base := os.Getenv("BRABEUS_EMBED_URL")
	if base == "" {
		return nil
	}
	return retrieval.NewOllamaEmbedder(base, env("BRABEUS_EMBED_MODEL", retrieval.DefaultEmbedModel))
}
