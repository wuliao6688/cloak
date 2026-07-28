package tls_client

import (
	"errors"
	"io"
	"reflect"
	"sync"
	"sync/atomic"

	http "github.com/bogdanfinn/fhttp"
)

// DefaultMaxCachedTransports bounds the per-client host/protocol transport
// cache. A TransportOptions value of -1 keeps the historical unlimited mode.
const DefaultMaxCachedTransports = 256

var errHTTP3TransportRetired = errors.New("http/3 transport has been retired from the cache")

// ─── HTTP/3 graceful retirement ────────────────────────────

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

// ─── Sharded transport cache ───────────────────────────────

const transportShardCountLog2 = 3 // 2^3 = 8 shards
const transportShardCount = 1 << transportShardCountLog2

type transportShard struct {
	mu       sync.RWMutex
	items    map[string]http.RoundTripper
	lastUsed map[string]*atomic.Uint64
	sequence atomic.Uint64
	maxSize  int
	init     sync.Once
}

func (s *transportShard) ensureInit() {
	s.init.Do(func() { s.items = make(map[string]http.RoundTripper) })
}

func (s *transportShard) get(key string) (http.RoundTripper, bool) {
	s.mu.RLock()
	s.ensureInit()
	transport, ok := s.items[key]
	if ok && s.lastUsed != nil {
		s.touch(key)
	}
	s.mu.RUnlock()
	return transport, ok
}

// touch records recency. Must be called under at least RLock.
func (s *transportShard) touch(key string) {
	lastUsed := s.lastUsed[key]
	if lastUsed == nil {
		return
	}
	seq := s.sequence.Add(1)
	for {
		prev := lastUsed.Load()
		if prev >= seq || lastUsed.CompareAndSwap(prev, seq) {
			return
		}
	}
}

func (s *transportShard) set(key string, transport http.RoundTripper) []evictedTransport {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInit()

	var evicted []evictedTransport
	if prev := s.items[key]; prev != nil && !sameRoundTripper(prev, transport) {
		evicted = append(evicted, evictedTransport{key: key, transport: prev})
	}
	s.items[key] = transport

	if s.maxSize > 0 {
		if s.lastUsed == nil {
			s.lastUsed = make(map[string]*atomic.Uint64)
		}
		last := s.lastUsed[key]
		if last == nil {
			last = &atomic.Uint64{}
			s.lastUsed[key] = last
		}
		last.Store(s.sequence.Add(1))

		// LRU eviction — O(N) per shard but max 16–64 items
		for len(s.items) > s.maxSize {
			oldestKey := ""
			oldestSeq := ^uint64(0)
			for k := range s.items {
				seq := uint64(0)
				if lu := s.lastUsed[k]; lu != nil {
					seq = lu.Load()
				}
				if seq < oldestSeq {
					oldestKey = k
					oldestSeq = seq
				}
			}
			if oldestKey == "" {
				break
			}
			evicted = append(evicted, evictedTransport{
				key: oldestKey, transport: s.items[oldestKey], removed: true,
			})
			delete(s.items, oldestKey)
			delete(s.lastUsed, oldestKey)
		}
	}

	// Deduplicate: don't close transports still reachable via another key.
	filtered := evicted[:0]
	for _, c := range evicted {
		stillCached := false
		for _, cached := range s.items {
			if sameRoundTripper(cached, c.transport) {
				stillCached = true
				break
			}
		}
		if !stillCached {
			filtered = append(filtered, c)
		} else if c.removed {
			c.transport = nil
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func (s *transportShard) delete(key string) http.RoundTripper {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInit()
	transport := s.items[key]
	delete(s.items, key)
	if s.lastUsed != nil {
		delete(s.lastUsed, key)
	}
	return transport
}

func (s *transportShard) snapshot() map[string]http.RoundTripper {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.ensureInit()
	out := make(map[string]http.RoundTripper, len(s.items))
	for k, v := range s.items {
		out[k] = v
	}
	return out
}

func (s *transportShard) reset() {
	s.mu.Lock()
	s.items = make(map[string]http.RoundTripper)
	s.lastUsed = nil
	s.sequence.Store(0)
	s.init = sync.Once{}
	s.mu.Unlock()
}

// ─── Sharded cache (public API) ─────────────────────────────

// shardedTransportCache is an 8-way sharded transport cache. Each shard has
// its own RWMutex. Shards are lazily allocated — only shards that are
// actually accessed consume memory. LRU eviction is bounded by per-shard
// capacity (maxEntries/8).
type shardedTransportCache struct {
	mu         sync.Mutex
	shards     [transportShardCount]*transportShard
	shardsInit [transportShardCount]sync.Once
	perShard   int
	smallCache bool // true if maxEntries ≤ 32; force all keys to shard 0
}

func newShardedTransportCache(maxEntries int) *shardedTransportCache {
	perShard := 0
	if maxEntries > 0 {
		// For small caches (≤32), use a single effective shard to preserve
		// exact LRU semantics. For larger caches, distribute across shards.
		if maxEntries <= 32 {
			perShard = maxEntries // all entries go to one shard
		} else {
			perShard = maxEntries / transportShardCount
			if perShard < 2 {
				perShard = 2
			}
		}
	}
	return &shardedTransportCache{perShard: perShard, smallCache: maxEntries > 0 && maxEntries <= 32}
}

func (c *shardedTransportCache) shard(key string) *transportShard {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := uint32(0)
	if !c.smallCache {
		idx = fnvHash(key) & (transportShardCount - 1)
	}
	c.shardsInit[idx].Do(func() {
		c.shards[idx] = &transportShard{maxSize: c.perShard}
	})
	return c.shards[idx]
}

func (c *shardedTransportCache) get(key string) (http.RoundTripper, bool) {
	return c.shard(key).get(key)
}

func (c *shardedTransportCache) set(key string, transport http.RoundTripper) []evictedTransport {
	return c.shard(key).set(key, transport)
}

func (c *shardedTransportCache) delete(key string) http.RoundTripper {
	return c.shard(key).delete(key)
}

// all returns all entries across all shards (for iteration).
func (c *shardedTransportCache) all() map[string]http.RoundTripper {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]http.RoundTripper)
	for i := range c.shards {
		s := c.shards[i]
		if s == nil {
			continue
		}
		items := s.snapshot()
		for k, v := range items {
			out[k] = v
		}
	}
	return out
}

func (c *shardedTransportCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.shards {
		if c.shards[i] != nil {
			c.shards[i].reset()
		}
		c.shardsInit[i] = sync.Once{}
	}
}

// fnvHash is a simple FNV-1a 32-bit hash for shard selection.
func fnvHash(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// ─── Legacy helpers (kept for internal/test compatibility) ──

type transportCacheMeta struct {
	lastUsed   map[string]*atomic.Uint64
	sequence   atomic.Uint64
	maxEntries int
}

type evictedTransport struct {
	key       string
	transport http.RoundTripper
	removed   bool
}

// getCachedTransportEntry is the legacy lookup for code that still uses
// map+lock (protocol racer, internal tests).
func getCachedTransportEntry(items map[string]http.RoundTripper, lock *sync.RWMutex, meta *transportCacheMeta, key string) (http.RoundTripper, bool) {
	lock.RLock()
	defer lock.RUnlock()

	transport, ok := items[key]
	if ok && meta != nil {
		touchTransportCacheEntry(meta, key)
	}
	return transport, ok
}

func touchTransportCacheEntry(meta *transportCacheMeta, key string) {
	lastUsed := meta.lastUsed[key]
	if lastUsed == nil {
		return
	}

	sequence := meta.sequence.Add(1)
	for {
		previous := lastUsed.Load()
		if previous >= sequence || lastUsed.CompareAndSwap(previous, sequence) {
			return
		}
	}
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
		lastUsed := meta.lastUsed[key]
		if lastUsed == nil {
			lastUsed = &atomic.Uint64{}
			meta.lastUsed[key] = lastUsed
		}
		lastUsed.Store(meta.sequence.Add(1))
		for meta.maxEntries >= 0 && len(items) > meta.maxEntries {
			oldestKey := ""
			oldestSequence := ^uint64(0)
			for candidate := range items {
				candidateLastUsed := uint64(0)
				if access := meta.lastUsed[candidate]; access != nil {
					candidateLastUsed = access.Load()
				}
				if candidateLastUsed < oldestSequence {
					oldestKey = candidate
					oldestSequence = candidateLastUsed
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
