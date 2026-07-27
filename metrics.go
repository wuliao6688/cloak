package tls_client

import "sync/atomic"

// TransportMetrics exposes read-only transport statistics for monitoring.
// All counters are atomically updated and safe for concurrent reads.
type TransportMetrics struct {
	// Connections tracks raw TCP/TLS connections.
	Connections metricsCounters

	// Transports tracks RoundTripper (HTTP/2 and HTTP/3) instances.
	Transports metricsCounters

	// Racing tracks protocol racing outcomes.
	Racing racingMetrics

	// Cache tracks transport cache efficiency.
	Cache cacheMetrics
}

type metricsCounters struct {
	Created   atomic.Int64
	Reused    atomic.Int64
	Closed    atomic.Int64
	Evicted   atomic.Int64
	Errors    atomic.Int64
}

type racingMetrics struct {
	Attempted     atomic.Int64
	H3Wins        atomic.Int64
	H2Wins        atomic.Int64
	BothFailed    atomic.Int64
	CacheHit      atomic.Int64
	CacheMiss     atomic.Int64
	CacheFallback atomic.Int64 // cached protocol failed, re-raced
}

type cacheMetrics struct {
	Lookups atomic.Int64
	Hits    atomic.Int64
	Misses  atomic.Int64
}

// Snapshot returns a point-in-time copy of all metrics.
func (m *TransportMetrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		ConnCreated:   m.Connections.Created.Load(),
		ConnReused:    m.Connections.Reused.Load(),
		ConnClosed:    m.Connections.Closed.Load(),
		ConnEvicted:   m.Connections.Evicted.Load(),
		ConnErrors:    m.Connections.Errors.Load(),
		TpCreated:     m.Transports.Created.Load(),
		TpReused:      m.Transports.Reused.Load(),
		TpClosed:      m.Transports.Closed.Load(),
		TpEvicted:     m.Transports.Evicted.Load(),
		TpErrors:      m.Transports.Errors.Load(),
		RaceAttempts:  m.Racing.Attempted.Load(),
		RaceH3Wins:    m.Racing.H3Wins.Load(),
		RaceH2Wins:    m.Racing.H2Wins.Load(),
		RaceFailed:    m.Racing.BothFailed.Load(),
		RaceCacheHit:  m.Racing.CacheHit.Load(),
		RaceCacheMiss: m.Racing.CacheMiss.Load(),
		CacheLookups:  m.Cache.Lookups.Load(),
		CacheHits:     m.Cache.Hits.Load(),
		CacheMisses:   m.Cache.Misses.Load(),
	}
}

// MetricsSnapshot is a point-in-time snapshot of transport metrics.
type MetricsSnapshot struct {
	ConnCreated, ConnReused, ConnClosed, ConnEvicted, ConnErrors int64
	TpCreated, TpReused, TpClosed, TpEvicted, TpErrors          int64
	RaceAttempts, RaceH3Wins, RaceH2Wins, RaceFailed            int64
	RaceCacheHit, RaceCacheMiss                                  int64
	CacheLookups, CacheHits, CacheMisses                         int64
}

// NewTransportMetrics creates a new TransportMetrics instance.
func NewTransportMetrics() *TransportMetrics {
	return &TransportMetrics{}
}

// MetricsCollector is an optional interface that roundTripper and protocolRacer
// can implement to expose their metrics.
type MetricsCollector interface {
	GetMetrics() *TransportMetrics
}
