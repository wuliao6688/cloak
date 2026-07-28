package tls_client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

type contextErrorDialer struct{}

func (*contextErrorDialer) Dial(network, addr string) (net.Conn, error) {
	return nil, errors.New("Dial without context was called")
}

func (*contextErrorDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestRoundTripperClosesRequestBodyWhenTransportInitializationFails(t *testing.T) {
	body := newCloseTrackingBody()
	req, err := http.NewRequest(http.MethodPost, "ftp://example.com/resource", body)
	if err != nil {
		t.Fatal(err)
	}

	rt := &roundTripper{
		rtCacheState: rtCacheState{
			shardedCache: newShardedTransportCache(0),
		},
	}
	if _, err := rt.RoundTrip(req); err == nil {
		t.Fatal("invalid scheme should fail transport initialization")
	}

	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("request body was not closed after transport initialization failed")
	}
}

type closeIdleTrackingTransport struct {
	closed int
}

func (t *closeIdleTrackingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, nil
}

func (t *closeIdleTrackingTransport) CloseIdleConnections() {
	t.closed++
}

type closeTrackingHTTP3Transport struct {
	body   *closeTrackingBody
	closed int
}

func (t *closeTrackingHTTP3Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       t.body,
		Request:    req,
	}, nil
}

func (t *closeTrackingHTTP3Transport) Close() error {
	t.closed++
	return nil
}

func TestRoundTripperCloseIdleConnectionsResetsSharedCaches(t *testing.T) {
	oldTransport := &closeIdleTrackingTransport{}
	shardedCache := newShardedTransportCache(0)
	shardedCache.set("example.com:443", oldTransport)

	racer := &protocolRacer{
		protocolCache: map[string]string{"example.com:443": "h2"},
		shardedCache:  shardedCache,
		transportInit: &keyedLockPool{},
	}
	rt := &roundTripper{
		rtCacheState: rtCacheState{
			shardedCache: shardedCache,
		},
		racer: racer,
	}

	rt.CloseIdleConnections()

	if oldTransport.closed != 1 {
		t.Fatalf("expected detached transport to be closed once, got %d", oldTransport.closed)
	}
	if len(rt.shardedCache.all()) != 0 || len(racer.shardedCache.all()) != 0 {
		t.Fatal("shared transport cache was not cleared")
	}
	if len(racer.protocolCache) != 0 {
		t.Fatal("protocol cache was not reset")
	}
	newTransport := &closeIdleTrackingTransport{}
	racer.setCachedTransport("new.example:443", newTransport)
	if cached, ok := rt.getCachedTransport("new.example:443"); !ok || cached != newTransport {
		t.Fatal("round tripper and racer no longer share the cache after idle cleanup")
	}
}

func TestRoundTripperTransportCacheUsesLRUEviction(t *testing.T) {
	first := &closeIdleTrackingTransport{}
	second := &closeIdleTrackingTransport{}
	third := &closeIdleTrackingTransport{}
	rt := &roundTripper{
		rtCacheState: rtCacheState{
			shardedCache: newShardedTransportCache(2),
		},
	}

	rt.setCachedTransport("first", first)
	rt.setCachedTransport("second", second)
	if _, ok := rt.getCachedTransport("first"); !ok {
		t.Fatal("expected first transport to be cached")
	}
	rt.setCachedTransport("third", third)

	if _, ok := rt.getCachedTransport("second"); ok {
		t.Fatal("least-recently-used transport was not evicted")
	}
	if first.closed != 0 || second.closed != 1 || third.closed != 0 {
		t.Fatalf("unexpected idle-close counts: first=%d second=%d third=%d", first.closed, second.closed, third.closed)
	}
}

func BenchmarkTransportCacheHitParallel(b *testing.B) {
	rt := &roundTripper{
		rtCacheState: rtCacheState{
			shardedCache: newShardedTransportCache(8),
		},
	}
	rt.setCachedTransport("example.com:443", &closeIdleTrackingTransport{})

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, ok := rt.getCachedTransport("example.com:443"); !ok {
				b.Fatal("cached transport disappeared")
			}
		}
	})
}

func BenchmarkHTTP2DialContextRegistrationParallel(b *testing.B) {
	rt := &roundTripper{
		rtH2DialState: rtH2DialState{
			http2DialContexts: make(map[string]map[uint64]context.Context),
			http2DialCancels:  make(map[string]map[uint64]context.CancelFunc),
		},
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			registration := rt.registerHTTP2DialContext("example.com:443", ctx)
			registration.release()
		}
	})
}

func TestHTTP2DialContextRegistrationSkipsCancellationWatchForBackground(t *testing.T) {
	rt := &roundTripper{}
	registration := rt.registerHTTP2DialContext("example.com:443", context.Background())
	if registration.stopCancellationWatch != nil {
		t.Fatal("background context installed an unnecessary cancellation watch")
	}
	registration.release()
}

func TestTransportCacheConcurrentHitsAndEvictions(t *testing.T) {
	rt := &roundTripper{
		rtCacheState: rtCacheState{
			shardedCache: newShardedTransportCache(8),
		},
	}
	for i := 0; i < 8; i++ {
		rt.setCachedTransport(fmt.Sprintf("seed-%d", i), &closeIdleTrackingTransport{})
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			<-start
			for i := 0; i < 1000; i++ {
				_, _ = rt.getCachedTransport(fmt.Sprintf("seed-%d", (i+offset)%8))
			}
		}(worker)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 1000; i++ {
			rt.setCachedTransport(fmt.Sprintf("churn-%d", i), &closeIdleTrackingTransport{})
		}
	}()

	close(start)
	wg.Wait()

	if len(rt.shardedCache.all()) > 8 {
		t.Fatalf("transport cache exceeded its configured capacity: %d", len(rt.shardedCache.all()))
	}
}

func TestHTTP3LRUEvictionWaitsForActiveResponseBody(t *testing.T) {
	underlying := &closeTrackingHTTP3Transport{body: newCloseTrackingBody()}
	http3Transport := newRetiringHTTP3Transport(underlying)
	rt := &roundTripper{
		rtCacheState: rtCacheState{
			shardedCache: newShardedTransportCache(1),
		},
	}
	rt.setCachedTransport("first:h3", http3Transport)

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http3Transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	rt.setCachedTransport("second", &closeIdleTrackingTransport{})
	if underlying.closed != 0 {
		t.Fatal("LRU eviction interrupted an active HTTP/3 response")
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if underlying.closed != 1 {
		t.Fatalf("retired HTTP/3 transport was not closed after body release: %d", underlying.closed)
	}
}

func TestRetiredHTTP3TransportRejectsNewRequestsWhileDraining(t *testing.T) {
	underlying := &closeTrackingHTTP3Transport{body: newCloseTrackingBody()}
	transport := newRetiringHTTP3Transport(underlying)

	firstRequest, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(firstRequest)
	if err != nil {
		t.Fatal(err)
	}

	transport.CloseIdleConnections()
	if underlying.closed != 0 {
		t.Fatal("retirement interrupted the active HTTP/3 response")
	}

	secondBody := newCloseTrackingBody()
	secondRequest, err := http.NewRequest(http.MethodGet, "https://example.com/second", secondBody)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(secondRequest); !errors.Is(err, errHTTP3TransportRetired) {
		t.Fatalf("retired HTTP/3 transport accepted a new request: %v", err)
	}
	select {
	case <-secondBody.closed:
	case <-time.After(time.Second):
		t.Fatal("rejected request body was not closed")
	}
	if underlying.closed != 0 {
		t.Fatal("rejected request force-closed an active HTTP/3 response")
	}

	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if underlying.closed != 1 {
		t.Fatalf("retired HTTP/3 transport was not closed after draining: %d", underlying.closed)
	}
}

func TestHTTP2RedialUsesActiveRequestContext(t *testing.T) {
	rt := &roundTripper{
		rtCacheState: rtCacheState{
			shardedCache: newShardedTransportCache(0),
		},
		dialer: &contextErrorDialer{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	registration := rt.registerHTTP2DialContext("example.com:443", ctx)
	cancel()

	_, err := rt.dialTLSHTTP2("tcp", "example.com:443", nil)
	registration.release()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("HTTP/2 redial did not inherit request cancellation: %v", err)
	}
}

func TestHTTP2SharedDialCancelsOnlyAfterAllWaitingRequests(t *testing.T) {
	rt := &roundTripper{}
	firstContext, cancelFirst := context.WithCancel(context.Background())
	secondContext, cancelSecond := context.WithCancel(context.Background())
	firstRegistration := rt.registerHTTP2DialContext("example.com:443", firstContext)
	secondRegistration := rt.registerHTTP2DialContext("example.com:443", secondContext)
	defer firstRegistration.release()
	defer secondRegistration.release()

	combined, cancelCombined := rt.contextForHTTP2Dial("example.com:443")
	defer cancelCombined()
	cancelFirst()
	select {
	case <-combined.Done():
		t.Fatal("one canceled request terminated a shared HTTP/2 dial")
	case <-time.After(20 * time.Millisecond):
	}

	cancelSecond()
	select {
	case <-combined.Done():
	case <-time.After(time.Second):
		t.Fatal("shared HTTP/2 dial remained active after all requests canceled")
	}
}

func TestHTTP2SharedDialIncludesRequestsRegisteredAfterDialStarts(t *testing.T) {
	rt := &roundTripper{}
	firstContext, cancelFirst := context.WithCancel(context.Background())
	firstRegistration := rt.registerHTTP2DialContext("example.com:443", firstContext)
	defer firstRegistration.release()

	combined, cancelCombined := rt.contextForHTTP2Dial("example.com:443")
	defer cancelCombined()

	secondContext, cancelSecond := context.WithCancel(context.Background())
	secondRegistration := rt.registerHTTP2DialContext("example.com:443", secondContext)
	defer secondRegistration.release()

	cancelFirst()
	select {
	case <-combined.Done():
		t.Fatal("a late active waiter was not attached to the shared HTTP/2 dial")
	case <-time.After(20 * time.Millisecond):
	}

	cancelSecond()
	select {
	case <-combined.Done():
	case <-time.After(time.Second):
		t.Fatal("shared HTTP/2 dial remained active after all dynamic waiters canceled")
	}
}
