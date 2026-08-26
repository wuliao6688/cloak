package cloak

import (
	"net/http"
	"sync"
	"time"

	"github.com/wuliao6688/cloak/profiles"
)

// ─── Transport pooling ───
//
// Impersonate / ImpersonateRequest / ImpersonateChain historically created
// a BRAND-NEW Transport (and thus a brand-new connection pool) on every
// call. Under sustained per-request use this leaked keep-alive connections
// and their goroutines: each new Transport opens its own connections and
// nothing ever closes them (1h stress test: 26K-127K leaked goroutines).
//
// Fix: share one Transport per profile. Connections are reused across
// calls, so nothing accumulates. The shared Transport is reference-counted;
// when the last user releases it, idle connections are closed.
//
// Per-call customization (SetInsecureSkipVerify, Proxy, PinningHosts …)
// is applied on a per-request basis where possible; transport-level options
// that must not leak across users are handled by the caller (see
// ImpersonateChain which still builds a dedicated Transport for explicit
// per-client configuration).

var (
	transportPool   = map[string]*pooledTransport{}
	transportPoolMu sync.Mutex
)

type pooledTransport struct {
	transport http.RoundTripper // HeaderRoundTripper (browser headers + cloak.Transport)
	refs      int
}

// getPooledClient returns a shared *http.Client for the given profile,
// incrementing the pool refcount. Call releasePooledClient when done.
func getPooledClient(profile profiles.ClientProfile) *http.Client {
	key := profile.GetClientHelloStr()

	transportPoolMu.Lock()
	defer transportPoolMu.Unlock()

	pt, ok := transportPool[key]
	if !ok {
		tr := NewTransport(profile)
		htr := NewHeaderRoundTripper(tr, profile)
		pt = &pooledTransport{transport: htr}
		transportPool[key] = pt
	}
	pt.refs++
	return &http.Client{Transport: pt.transport, Timeout: 30 * time.Second}
}

// releasePooledClient decrements the refcount of the pooled transport for
// the profile. The pooled transport itself is never evicted: it lives for
// the process lifetime so concurrent users can always reuse the same
// connection pool. Refcount 0 only means "no current users" — idle
// connections are closed to free resources, but the entry stays.
func releasePooledClient(profile profiles.ClientProfile) {
	key := profile.GetClientHelloStr()

	transportPoolMu.Lock()
	defer transportPoolMu.Unlock()

	pt, ok := transportPool[key]
	if !ok {
		return
	}
	pt.refs--
	if pt.refs <= 0 {
		pt.refs = 0
		// Close idle connections to free resources, but KEEP the entry so
		// a concurrent user reusing this profile gets the same transport.
		// Evicting here caused a create/destroy loop under concurrency
		// (found by 1h stress test: 37K goroutines when closeAllReqs raced
		// with in-flight requests).
		if closer, ok := pt.transport.(interface{ CloseIdleConnections() }); ok {
			closer.CloseIdleConnections()
		}
	}
}

// pooledClientDo is a convenience: run fn with a pooled client and always
// release it afterwards.
func pooledClientDo(profile profiles.ClientProfile, fn func(*http.Client) error) error {
	c := getPooledClient(profile)
	defer releasePooledClient(profile)
	return fn(c)
}
