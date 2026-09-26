package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Identity answers "which machine is calling", which is what scope filtering
// turns on.
//
// It is an interface with two implementations on purpose: the Tailscale
// network cannot identify a caller arriving from outside it, so allowing
// clients outside it later is a mode change rather than a rewrite.
type Identity interface {
	Machine(r *http.Request) string
	Mode() string
	// Describe is the startup log line: the mode and what it trusts, so an
	// operator reading "unresolved caller" can see what would have resolved.
	Describe() string
}

// ---------- tailscale identity ----------

// tailscaleIdentity resolves the caller from X-Forwarded-For, which
// `tailscale serve` sets to the peer's Tailscale address.
//
// The header is only as trustworthy as the hop that set it. `serve`
// OVERWRITES it — verified 2026-09-06 by sending a forged X-Forwarded-For and
// receiving the true value back — but the container listens on every
// interface, so anything else on the Docker network can reach the port
// directly and write the header itself. The header is therefore believed only
// when the TCP peer is one of the trusted proxies; from anyone else the
// caller is unknown, and unknown fails closed.
type tailscaleIdentity struct {
	nodes   func() (map[string]string, error)
	trusted []netip.Prefix // peers whose X-Forwarded-For is believed

	mu      sync.Mutex
	cached  map[string]string
	fetched time.Time // when cached was last replaced
	tried   time.Time // when tailscaled was last asked, whatever the answer
}

func (t *tailscaleIdentity) Mode() string { return "tailscale" }

func (t *tailscaleIdentity) Describe() string {
	return fmt.Sprintf("tailscale, believing X-Forwarded-For from %v", t.trusted)
}

// trusts reports whether one of the prefixes contains the address. The one
// membership test for both the startup check and the per-request check, so
// they cannot disagree. A v4 peer on a dual-stack listener shows up as
// ::ffff:a.b.c.d, and a link-local v6 peer carries a zone that
// Prefix.Contains never matches; both are normalised away first.
func trusts(prefixes []netip.Prefix, a netip.Addr) bool {
	a = a.Unmap().WithZone("")
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// fromTrustedProxy reports whether the TCP peer is one we accept the header
// from. An unparseable peer address is not trusted.
func (t *tailscaleIdentity) fromTrustedProxy(r *http.Request) bool {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	return trusts(t.trusted, ap.Addr())
}

// parseTrustedProxies reads a comma-separated list of networks or addresses.
// Nothing is a valid answer only by being said explicitly: an empty list
// would trust no one and hide every machine-scoped memory while looking
// configured.
func parseTrustedProxies(spec string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("BRABEUS_TRUSTED_PROXIES has an empty entry in %q", spec)
		}
		if p, err := netip.ParsePrefix(item); err == nil {
			out = append(out, p)
			continue
		}
		a, err := netip.ParseAddr(item)
		if err != nil {
			return nil, fmt.Errorf("BRABEUS_TRUSTED_PROXIES entry %q is neither a network nor an address", item)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// checkTrustReachable refuses a trust list no peer can ever satisfy. The image
// listens on every interface and the proxy arrives from the Docker bridge, so
// a loopback-only list there would mark every caller unknown and hide every
// machine-scoped memory while /healthz still says ok. Misconfiguration is
// refused at startup, where the operator is looking, not discovered per call.
func checkTrustReachable(listen string, trusted []netip.Prefix) error {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("BRABEUS_LISTEN %q: %w", listen, err)
	}
	// A loopback listener is reached only by loopback peers, of its own
	// family. Trusting the other family's loopback trusts nobody who can
	// actually connect.
	if host == "localhost" {
		if trusts(trusted, netip.MustParseAddr("127.0.0.1")) || trusts(trusted, netip.MustParseAddr("::1")) {
			return nil
		}
		return fmt.Errorf("BRABEUS_LISTEN %s is loopback but BRABEUS_TRUSTED_PROXIES %v trusts no loopback peer", listen, trusted)
	}
	if a, err := netip.ParseAddr(host); err == nil && a.IsLoopback() {
		if trusts(trusted, a) {
			return nil
		}
		return fmt.Errorf("BRABEUS_LISTEN %s is loopback but BRABEUS_TRUSTED_PROXIES %v trusts no peer of that family", listen, trusted)
	}
	for _, p := range trusted {
		if !p.Addr().IsLoopback() {
			return nil
		}
	}
	return fmt.Errorf("BRABEUS_LISTEN %s is reachable from off-host but BRABEUS_TRUSTED_PROXIES %v is loopback only: "+
		"no proxy could ever be trusted and every caller would be unknown", listen, trusted)
}

func (t *tailscaleIdentity) Machine(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if raw == "" {
		return ""
	}
	if !t.fromTrustedProxy(r) {
		// Said only when there was a header to ignore: this is the line that
		// names the peer to add when a proxy has moved.
		log.Printf("X-Forwarded-For from %s ignored: not in BRABEUS_TRUSTED_PROXIES", r.RemoteAddr)
		return ""
	}
	// `serve` overwrites rather than appends, so exactly one address is the
	// well-formed case. A chain means the request did not arrive the way this
	// server's trust depends on; fail closed rather than guess which hop to
	// believe.
	if strings.Contains(raw, ",") {
		return ""
	}
	if _, err := netip.ParseAddr(raw); err != nil {
		return ""
	}
	return t.resolve(raw)
}

// How long the node map is believed complete, and how recently it must have
// been fetched for a miss to be taken as "unknown address" rather than "a
// machine joined since".
const (
	nodesTTL         = 2 * time.Minute
	nodesMissRefresh = 10 * time.Second
)

// resolve maps a Tailscale address to a hostname through a cached node map.
//
// A miss on a map older than a few seconds is reason to ask again: a machine
// that joined a minute ago is not in the map, and leaving it unknown until
// the cache happens to expire is a silent failure. A miss on a map fetched a
// moment ago is just an unknown address, and asking again for every one of
// them would hammer tailscaled. When tailscaled cannot be reached the last
// good map is served, stale beats nothing, and it is not asked again until
// the same window has passed: the dial has a five second timeout and runs
// under the lock, so redialling per request would stall every session.
func (t *tailscaleIdentity) resolve(ip string) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if host, ok := t.cached[ip]; ok && time.Since(t.fetched) < nodesTTL {
		return host
	}
	// A successful fetch is an attempt too; a map from a moment ago is fresh
	// whether or not the last attempt failed.
	lastTry := t.tried
	if t.fetched.After(lastTry) {
		lastTry = t.fetched
	}
	if time.Since(lastTry) < nodesMissRefresh {
		return t.cached[ip]
	}
	t.tried = time.Now()
	m, err := t.nodes()
	if err != nil {
		if t.fetched.IsZero() {
			log.Printf("tailscaled status: %v; no node map yet, every caller is unknown", err)
		} else {
			log.Printf("tailscaled status: %v; using the node map from %s ago", err, time.Since(t.fetched).Round(time.Second))
		}
		return t.cached[ip]
	}
	t.cached, t.fetched = m, time.Now()
	return m[ip]
}

// parseTailscaleStatus turns tailscaled's LocalAPI status into an address to
// hostname map. Reading the live node list rather than a configured table is
// deliberate: a machine joining the Tailscale network must not require anyone
// to remember to update a map.
func parseTailscaleStatus(r io.Reader) (map[string]string, error) {
	var st struct {
		Self *tailscaleNode            `json:"Self"`
		Peer map[string]*tailscaleNode `json:"Peer"`
	}
	if err := json.NewDecoder(r).Decode(&st); err != nil {
		return nil, err
	}
	out := map[string]string{}
	add := func(n *tailscaleNode) {
		if n == nil {
			return
		}
		for _, ip := range n.TailscaleIPs {
			out[ip] = normaliseHost(n.HostName)
		}
	}
	add(st.Self)
	for _, p := range st.Peer {
		add(p)
	}
	if len(out) == 0 {
		// An empty map would silently resolve every caller to "unknown", which
		// looks identical to a working server that nobody is calling.
		return nil, fmt.Errorf("tailscaled reported no nodes")
	}
	return out, nil
}

type tailscaleNode struct {
	HostName     string   `json:"HostName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
}

func newTailscaleIdentity(socket string, trusted []netip.Prefix) *tailscaleIdentity {
	return &tailscaleIdentity{trusted: trusted, nodes: func() (map[string]string, error) {
		cl := &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socket)
				},
			},
		}
		resp, err := cl.Get("http://local-tailscaled.sock/localapi/v0/status")
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("tailscaled localapi: %s", resp.Status)
		}
		return parseTailscaleStatus(resp.Body)
	}}
}

// ---------- token identity ----------

// tokenIdentity maps a bearer token to a machine name, for clients that are
// not on the Tailscale network and therefore have no peer identity.
//
// Whichever mode is not in use gets no exercise from real traffic. Switching
// modes is itself a change that needs proving.
type tokenIdentity struct {
	byToken map[string]string
}

func newTokenIdentity(spec string) (*tokenIdentity, error) {
	m := map[string]string{}
	for _, pair := range strings.Split(spec, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		tok, host, ok := strings.Cut(pair, "=")
		tok, host = strings.TrimSpace(tok), strings.TrimSpace(host)
		if !ok || tok == "" || host == "" {
			return nil, fmt.Errorf("BRABEUS_TOKENS entries must be <token>=<machine>")
		}
		m[tok] = normaliseHost(host)
	}
	if len(m) == 0 {
		return nil, fmt.Errorf("identity mode is token but BRABEUS_TOKENS is empty")
	}
	return &tokenIdentity{byToken: m}, nil
}

func (t *tokenIdentity) Mode() string { return "token" }

func (t *tokenIdentity) Describe() string {
	return fmt.Sprintf("token, %d machines configured", len(t.byToken))
}

func (t *tokenIdentity) Machine(r *http.Request) string {
	header := r.Header.Get("Authorization")
	tok, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return ""
	}
	return t.byToken[strings.TrimSpace(tok)]
}
