package tls_client

import (
	"errors"
	"io"
	"reflect"
	"sync"

	http "github.com/bogdanfinn/fhttp"
)

// DefaultMaxCachedTransports bounds the per-client host/protocol transport
// cache. A TransportOptions value of -1 keeps the historical unlimited mode.
const DefaultMaxCachedTransports = 256

var errHTTP3TransportRetired = errors.New("http/3 transport has been retired from the cache")

// retiringHTTP3Transport gives HTTP/3 transports CloseIdleConnections-like
// retirement semantics. quic-go exposes Close, which tears down active
// responses, so LRU eviction waits until every response body reaches EOF or
// Close before closing the underlying QUIC transport.
type retiringHTTP3Transport struct {
	transport http.RoundTripper

	mu       sync.Mutex
	active   int
	retired  bool
	closed   bool
	closeErr error
	closeOne sync.Once
}

func newRetiringHTTP3Transport(transport http.RoundTripper) *retiringHTTP3Transport {
	return &retiringHTTP3Transport{transport: transport}
}

func (t *retiringHTTP3Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	if t.retired || t.closed {
		t.mu.Unlock()
		closeRequestBody(req)
		return nil, errHTTP3TransportRetired
	}
	t.active++
	t.mu.Unlock()
	releaseWithBody := false
	defer func() {
		if !releaseWithBody {
			t.releaseResponse()
		}
	}()

	response, err := t.transport.RoundTrip(req)
	if response == nil || response.Body == nil {
		return response, err
	}
	response.Body = &releaseTransportBody{
		ReadCloser: response.Body,
		release:    t.releaseResponse,
	}
	releaseWithBody = true
	return response, err
}

// CloseIdleConnections retires the transport without interrupting active
// response bodies. The underlying HTTP/3 Close runs as soon as the last body
// is released.
func (t *retiringHTTP3Transport) CloseIdleConnections() {
	t.mu.Lock()
	t.retired = true
	closeNow := t.active == 0 && !t.closed
	if closeNow {
		t.closed = true
	}
	t.mu.Unlock()
	if closeNow {
		_ = t.closeUnderlying()
	}
}

// Close is the explicit force-close path used for failed or losing racing
// attempts. Callers close any returned response body before invoking it.
func (t *retiringHTTP3Transport) Close() error {
	t.mu.Lock()
	t.retired = true
	t.closed = true
	t.mu.Unlock()
	return t.closeUnderlying()
}

func (t *retiringHTTP3Transport) releaseResponse() {
	t.mu.Lock()
	if t.active > 0 {
		t.active--
	}
	closeNow := t.retired && t.active == 0 && !t.closed
	if closeNow {
		t.closed = true
	}
	t.mu.Unlock()
	if closeNow {
		_ = t.closeUnderlying()
	}
}

func (t *retiringHTTP3Transport) closeUnderlying() error {
	t.closeOne.Do(func() {
		if closer, ok := t.transport.(interface{ Close() error }); ok {
			t.closeErr = closer.Close()
			return
		}
		if closeIdler, ok := t.transport.(interface{ CloseIdleConnections() }); ok {
			closeIdler.CloseIdleConnections()
		}
	})
	return t.closeErr
}

type releaseTransportBody struct {
	io.ReadCloser
	release func()
	once    sync.Once
}

func (b *releaseTransportBody) Read(buffer []byte) (int, error) {
	read, err := b.ReadCloser.Read(buffer)
	if err != nil {
		b.once.Do(b.release)
	}
	return read, err
}

func (b *releaseTransportBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
}

type transportCacheMeta struct {
	lastUsed   map[string]uint64
	sequence   uint64
	maxEntries int
}

type evictedTransport struct {
	key       string
	transport http.RoundTripper
	removed   bool
}

func newTransportCacheMeta(maxEntries int) *transportCacheMeta {
	if maxEntries == 0 {
		maxEntries = DefaultMaxCachedTransports
	}
	return &transportCacheMeta{
		lastUsed:   make(map[string]uint64),
		maxEntries: maxEntries,
	}
}

func getCachedTransportEntry(items map[string]http.RoundTripper, lock *sync.RWMutex, meta *transportCacheMeta, key string) (http.RoundTripper, bool) {
	lock.Lock()
	defer lock.Unlock()

	transport, ok := items[key]
	if ok && meta != nil {
		meta.sequence++
		meta.lastUsed[key] = meta.sequence
	}
	return transport, ok
}

func setCachedTransportEntry(items map[string]http.RoundTripper, lock *sync.RWMutex, meta *transportCacheMeta, key string, transport http.RoundTripper) []evictedTransport {
	if transport == nil {
		return nil
	}

	lock.Lock()
	var evicted []evictedTransport
	if previous := items[key]; previous != nil && !sameRoundTripper(previous, transport) {
		evicted = append(evicted, evictedTransport{key: key, transport: previous})
	}
	items[key] = transport

	if meta != nil {
		meta.sequence++
		meta.lastUsed[key] = meta.sequence
		for meta.maxEntries >= 0 && len(items) > meta.maxEntries {
			oldestKey := ""
			oldestSequence := ^uint64(0)
			for candidate := range items {
				lastUsed := meta.lastUsed[candidate]
				if lastUsed < oldestSequence {
					oldestKey = candidate
					oldestSequence = lastUsed
				}
			}
			if oldestKey == "" {
				break
			}
			evicted = append(evicted, evictedTransport{key: oldestKey, transport: items[oldestKey], removed: true})
			delete(items, oldestKey)
			delete(meta.lastUsed, oldestKey)
		}
	}

	// A transport may temporarily be reachable through more than one key.
	// Do not close a displaced value while another cache key still owns it.
	filtered := evicted[:0]
	for _, candidate := range evicted {
		stillCached := false
		for _, cached := range items {
			if sameRoundTripper(cached, candidate.transport) {
				stillCached = true
				break
			}
		}
		if !stillCached {
			filtered = append(filtered, candidate)
		} else if candidate.removed {
			candidate.transport = nil
			filtered = append(filtered, candidate)
		}
	}
	lock.Unlock()
	return filtered
}

func sameRoundTripper(left, right http.RoundTripper) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftValue := reflect.ValueOf(left)
	rightValue := reflect.ValueOf(right)
	return leftValue.Type() == rightValue.Type() && leftValue.Comparable() && leftValue.Interface() == rightValue.Interface()
}

func deleteCachedTransportEntry(items map[string]http.RoundTripper, lock *sync.RWMutex, meta *transportCacheMeta, key string) http.RoundTripper {
	lock.Lock()
	defer lock.Unlock()

	transport := items[key]
	delete(items, key)
	if meta != nil {
		delete(meta.lastUsed, key)
	}
	return transport
}

func resetTransportCacheMeta(meta *transportCacheMeta) {
	if meta == nil {
		return
	}
	meta.lastUsed = make(map[string]uint64)
	meta.sequence = 0
}
