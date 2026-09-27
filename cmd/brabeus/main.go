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
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/allanschon/brabeus/internal/identity"
	"github.com/allanschon/brabeus/internal/retrieval"
	"github.com/allanschon/brabeus/internal/server"
	"github.com/allanschon/brabeus/internal/store"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	branch := env("BRABEUS_BRANCH", "main")
	// Beside the working copies, on the data volume, so a host key accepted
	// once survives the container.
	knownHosts := env("BRABEUS_KNOWN_HOSTS", "/var/lib/brabeus/known_hosts")
	if err := store.PrepareKnownHosts(knownHosts); err != nil {
		log.Fatalf("known_hosts: %v", err)
	}

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
	log.Printf("memory store ready at %s", memory.Dir)

	// The mirror is optional: the server is useful without it, and failing to
	// clone it must not take the memory store down too.
	var projects *store.Store
	if url := env("BRABEUS_MIRROR_REPO", ""); url != "" {
		projects = &store.Store{
			Dir:       env("BRABEUS_MIRROR_DIR", "/var/lib/brabeus/projects"),
			RemoteURL: url,
			Branch:    branch,
			// A DIFFERENT key, read-only on the mirror repository. Reusing
			// the memory key here would have to mean widening it, and the
			// whole design rests on the writer being unable to reach the
			// publishable tree.
			SSHCommand: store.SSHCommandFor(env("BRABEUS_MIRROR_SSH_KEY", "/etc/brabeus/projects_key"), knownHosts),
			ReadOnly:   true,
		}
		if err := projects.Ensure(); err != nil {
			log.Printf("projects mirror unavailable, continuing without it: %v", err)
			projects = nil
		} else {
			log.Printf("projects mirror ready at %s (read-only)", projects.Dir)
		}
	}

	addr := env("BRABEUS_LISTEN", "127.0.0.1:8082")
	id, err := newIdentity(addr)
	if err != nil {
		log.Fatalf("identity: %v", err)
	}
	log.Printf("identity: %s", id.Describe())

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

	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		caller := id.Machine(r)
		if caller == "" {
			log.Printf("unresolved caller from %s: machine-scoped memories will be hidden", r.RemoteAddr)
		}
		return server.New(memory, projects, caller)
	}, nil)

	guarded, err := server.AuthMiddleware(mcpHandler, env("BRABEUS_AUTH_MODE", "none"), os.Getenv("BRABEUS_AUTH_TOKEN"))
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", guarded)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok %s identity=%s\n", server.Version, id.Mode())
	})

	log.Printf("brabeus %s listening on %s", server.Version, addr)
	log.Fatal((&http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}).ListenAndServe())
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
// The projects mirror deliberately gets NO embedder. It is a read-only
// mirror of a much larger tree, embedding it would multiply the startup cost
// for a corpus nobody writes, and its lexical search already works.
func newEmbedder() retrieval.Embedder {
	base := os.Getenv("BRABEUS_EMBED_URL")
	if base == "" {
		return nil
	}
	return retrieval.NewOllamaEmbedder(base, env("BRABEUS_EMBED_MODEL", retrieval.DefaultEmbedModel))
}
