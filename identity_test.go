package main

import (
	"errors"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func fakeNodes() (map[string]string, error) {
	return map[string]string{
		"192.0.2.1": "beta",
		"192.0.2.2": "gamma",
		"192.0.2.3": "alpha",
	}, nil
}

// httptest requests arrive from 192.0.2.1. Trust that network so the tests
// below exercise the header, not the proxy check.
func testNet(t *testing.T) []netip.Prefix {
	t.Helper()
	p, err := parseTrustedProxies("192.0.2.0/24")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTailscaleIdentityResolvesTheForwardedAddress(t *testing.T) {
	id := &tailscaleIdentity{nodes: fakeNodes, trusted: testNet(t)}
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("X-Forwarded-For", "192.0.2.1")
	if got := id.Machine(r); got != "beta" {
		t.Errorf("Machine() = %q, want beta", got)
	}
}

// tailscale serve overwrites X-Forwarded-For rather than appending, so a
// well-formed request carries exactly one address. More than one means the
// request did not arrive the way this server's trust depends on, and the safe
// answer is "I do not know you".
func TestTailscaleIdentityRefusesAForwardedChain(t *testing.T) {
	id := &tailscaleIdentity{nodes: fakeNodes, trusted: testNet(t)}
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("X-Forwarded-For", "192.0.2.2, 192.0.2.1")
	if got := id.Machine(r); got != "" {
		t.Errorf("Machine() = %q for a forwarded chain, want empty", got)
	}
}

func TestTailscaleIdentityFailsClosed(t *testing.T) {
	id := &tailscaleIdentity{nodes: fakeNodes, trusted: testNet(t)}
	for _, xff := range []string{"", "203.0.113.9", "not-an-address"} {
		r := httptest.NewRequest("POST", "/mcp", nil)
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		if got := id.Machine(r); got != "" {
			t.Errorf("Machine() = %q for X-Forwarded-For %q, want empty", got, xff)
		}
	}
}

// The shape below is what tailscaled's LocalAPI actually returned on
// 2026-09-06, trimmed. Hostnames come back mixed case.
func TestParseTailscaleStatusMapsEveryAddressToALowercaseHostname(t *testing.T) {
	const status = `{
	  "Self": {"HostName": "alpha", "TailscaleIPs": ["192.0.2.3", "fd7a:115c:a1e0::3"]},
	  "Peer": {
	    "nodekey:aaa": {"HostName": "Gamma", "TailscaleIPs": ["192.0.2.2"]},
	    "nodekey:bbb": {"HostName": "Delta", "TailscaleIPs": ["192.0.2.4"]}
	  }
	}`
	got, err := parseTailscaleStatus(strings.NewReader(status))
	if err != nil {
		t.Fatal(err)
	}
	for ip, want := range map[string]string{
		"192.0.2.3":         "alpha",
		"fd7a:115c:a1e0::3": "alpha",
		"192.0.2.2":         "gamma",
		"192.0.2.4":         "delta",
	} {
		if got[ip] != want {
			t.Errorf("address %s mapped to %q, want %q", ip, got[ip], want)
		}
	}
}

// An empty node list means the lookup succeeded but told us nothing, which
// would silently resolve every caller to "unknown". Treat it as an error.
func TestParseTailscaleStatusRejectsAnEmptyNodeList(t *testing.T) {
	if _, err := parseTailscaleStatus(strings.NewReader(`{"Peer":{}}`)); err == nil {
		t.Error("an empty node list must be an error, not an empty map")
	}
}

func TestTokenIdentityMapsATokenToAMachine(t *testing.T) {
	id, err := newTokenIdentity("s3cret=Beta, other=gamma")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Authorization", "Bearer s3cret")
	if got := id.Machine(r); got != "beta" {
		t.Errorf("Machine() = %q, want beta (normalised)", got)
	}
}

func TestTokenIdentityFailsClosed(t *testing.T) {
	id, err := newTokenIdentity("s3cret=beta")
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"", "Bearer wrong", "s3cret"} {
		r := httptest.NewRequest("POST", "/mcp", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		if got := id.Machine(r); got != "" {
			t.Errorf("Machine() = %q for Authorization %q, want empty", got, header)
		}
	}
}

// Token mode with no tokens would authenticate nobody while looking configured.
func TestTokenIdentityRejectsUnusableConfiguration(t *testing.T) {
	for _, spec := range []string{"", "   ", "nomachine", "=beta", "token="} {
		if _, err := newTokenIdentity(spec); err == nil {
			t.Errorf("newTokenIdentity(%q) must fail loudly", spec)
		}
	}
}

// X-Forwarded-For is only as trustworthy as the hop that set it. The container
// listens on every interface, so anything else on the Docker network can reach
// it directly and write the header itself. The header is believed only when
// the TCP peer is a proxy we were told to trust; from anyone else the caller
// is unknown, and unknown fails closed.
func TestTailscaleIdentityIgnoresTheHeaderFromAnUntrustedPeer(t *testing.T) {
	loopback, err := parseTrustedProxies("127.0.0.0/8,::1/128")
	if err != nil {
		t.Fatal(err)
	}
	id := &tailscaleIdentity{nodes: fakeNodes, trusted: loopback}
	r := httptest.NewRequest("POST", "/mcp", nil) // RemoteAddr 192.0.2.1:1234
	r.Header.Set("X-Forwarded-For", "192.0.2.1")
	if got := id.Machine(r); got != "" {
		t.Errorf("Machine() = %q from an untrusted peer, want empty", got)
	}
}

func TestTailscaleIdentityBelievesATrustedProxy(t *testing.T) {
	trusted, err := parseTrustedProxies("127.0.0.0/8, 172.17.0.1")
	if err != nil {
		t.Fatal(err)
	}
	id := &tailscaleIdentity{nodes: fakeNodes, trusted: trusted}
	for _, peer := range []string{"127.0.0.1:40000", "[::ffff:127.0.0.1]:40000", "172.17.0.1:40000"} {
		r := httptest.NewRequest("POST", "/mcp", nil)
		r.RemoteAddr = peer
		r.Header.Set("X-Forwarded-For", "192.0.2.1")
		if got := id.Machine(r); got != "beta" {
			t.Errorf("Machine() = %q from trusted peer %s, want beta", got, peer)
		}
	}
}

// A misspelt network must not quietly trust nothing or everything.
func TestParseTrustedProxiesRejectsGarbage(t *testing.T) {
	for _, spec := range []string{"", "   ", "docker", "172.17.0.0/33", "172.17.0.1,", "10.0.0.0/8 10.1.0.0/16"} {
		if got, err := parseTrustedProxies(spec); err == nil {
			t.Errorf("parseTrustedProxies(%q) = %v, want an error", spec, got)
		}
	}
}

// The image listens on every interface, so the loopback default can never be
// right inside it. Starting anyway would hide every machine-scoped memory from
// every machine while /healthz says ok. Refuse instead, at startup, where the
// operator is looking.
func mustPrefixes(t *testing.T, spec string) []netip.Prefix {
	t.Helper()
	p, err := parseTrustedProxies(spec)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTrustedProxiesMustBeReachableFromTheListenAddress(t *testing.T) {
	loopback, _ := parseTrustedProxies("127.0.0.0/8,::1/128")
	gateway, _ := parseTrustedProxies("127.0.0.0/8,172.17.0.1")
	for _, c := range []struct {
		listen  string
		trusted []netip.Prefix
		ok      bool
	}{
		{"127.0.0.1:8082", loopback, true},
		{"[::1]:8082", loopback, true},
		{"0.0.0.0:8082", loopback, false},
		{":8082", loopback, false},
		{"0.0.0.0:8082", gateway, true},
		{"192.0.2.10:8082", loopback, false},
		// localhost is loopback whether or not it is spelt as an address.
		{"localhost:8082", loopback, true},
		// Loopback of the wrong family: every peer is ::1 and nothing trusts it.
		{"[::1]:8082", mustPrefixes(t, "127.0.0.0/8"), false},
		{"127.0.0.1:8082", mustPrefixes(t, "::1/128"), false},
		// A v4-mapped loopback listener is reached by v4 loopback peers, and
		// the runtime check unmaps them; the startup check must agree.
		{"[::ffff:127.0.0.1]:8082", mustPrefixes(t, "127.0.0.0/8"), true},
	} {
		err := checkTrustReachable(c.listen, c.trusted)
		if c.ok && err != nil {
			t.Errorf("listen %s with %v: unexpected error %v", c.listen, c.trusted, err)
		}
		if !c.ok && err == nil {
			t.Errorf("listen %s with %v: want an error, the proxy can never match", c.listen, c.trusted)
		}
	}
}

// A machine that joined the Tailscale network a minute ago is not in the cached map. A
// miss on a map that is more than a few seconds old is reason to ask again,
// rather than leaving the newcomer unknown until the cache happens to expire.
func TestTailscaleIdentityRefreshesOnAMissForANewMachine(t *testing.T) {
	calls := 0
	id := &tailscaleIdentity{
		trusted: testNet(t),
		cached:  map[string]string{"192.0.2.1": "beta"},
		fetched: time.Now().Add(-30 * time.Second),
		nodes: func() (map[string]string, error) {
			calls++
			return map[string]string{"192.0.2.1": "beta", "192.0.2.4": "delta"}, nil
		},
	}
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("X-Forwarded-For", "192.0.2.4")
	if got := id.Machine(r); got != "delta" {
		t.Errorf("Machine() = %q for a newly joined node, want delta", got)
	}
	if calls != 1 {
		t.Errorf("tailscaled asked %d times, want once", calls)
	}
}

// But a miss on a map fetched a moment ago is just an unknown address, and
// asking tailscaled again for every one of them is a way to hammer it.
func TestTailscaleIdentityDoesNotRefetchForAMissOnAFreshMap(t *testing.T) {
	calls := 0
	id := &tailscaleIdentity{
		trusted: testNet(t),
		cached:  map[string]string{"192.0.2.1": "beta"},
		fetched: time.Now(),
		nodes: func() (map[string]string, error) {
			calls++
			return nil, nil
		},
	}
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("X-Forwarded-For", "192.0.2.4")
	if got := id.Machine(r); got != "" {
		t.Errorf("Machine() = %q, want empty", got)
	}
	if calls != 0 {
		t.Errorf("tailscaled asked %d times for a miss on a fresh map, want none", calls)
	}
}

// tailscaled being briefly unreachable must not turn every caller unknown
// when the last good map is still in hand. Stale beats nothing.
func TestTailscaleIdentityServesTheLastGoodMapWhenTailscaledIsDown(t *testing.T) {
	id := &tailscaleIdentity{
		trusted: testNet(t),
		cached:  map[string]string{"192.0.2.1": "beta"},
		fetched: time.Now().Add(-10 * time.Minute),
		nodes: func() (map[string]string, error) {
			return nil, errors.New("dial unix /var/run/tailscale/tailscaled.sock: no such file")
		},
	}
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("X-Forwarded-For", "192.0.2.1")
	if got := id.Machine(r); got != "beta" {
		t.Errorf("Machine() = %q with tailscaled down and a stale map in hand, want beta", got)
	}
}

// A down tailscaled must not be redialled, with a five second timeout under
// the lock, on every request for as long as it is down. One attempt per
// refresh window; the stale map answers in between.
func TestTailscaleIdentityDoesNotRedialADownTailscaledOnEveryRequest(t *testing.T) {
	calls := 0
	id := &tailscaleIdentity{
		trusted: testNet(t),
		cached:  map[string]string{"192.0.2.1": "beta"},
		fetched: time.Now().Add(-10 * time.Minute),
		nodes: func() (map[string]string, error) {
			calls++
			return nil, errors.New("down")
		},
	}
	for i := 0; i < 3; i++ {
		r := httptest.NewRequest("POST", "/mcp", nil)
		r.Header.Set("X-Forwarded-For", "192.0.2.1")
		if got := id.Machine(r); got != "beta" {
			t.Errorf("call %d: Machine() = %q, want beta from the stale map", i, got)
		}
	}
	if calls != 1 {
		t.Errorf("tailscaled dialled %d times in one refresh window, want once", calls)
	}
}

// An empty map fetched a moment ago is still a fresh map. A miss on it is an
// unknown address, not a reason to ask again.
func TestTailscaleIdentityDoesNotRefetchForAMissOnAFreshEmptyMap(t *testing.T) {
	calls := 0
	id := &tailscaleIdentity{
		trusted: testNet(t),
		cached:  map[string]string{},
		fetched: time.Now(),
		nodes: func() (map[string]string, error) {
			calls++
			return nil, nil
		},
	}
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("X-Forwarded-For", "192.0.2.4")
	id.Machine(r)
	if calls != 0 {
		t.Errorf("tailscaled asked %d times for a miss on a fresh map, want none", calls)
	}
}
