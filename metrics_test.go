package tls_client

import (
	"sync"
	"testing"

	"github.com/bogdanfinn/tls-client/profiles"
)

func TestTransportMetricsConcurrent(t *testing.T) {
	m := NewTransportMetrics()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Connections.Created.Add(1)
			m.Connections.Reused.Add(1)
			m.Racing.Attempted.Add(1)
			m.Racing.H3Wins.Add(1)
			m.Cache.Lookups.Add(1)
			m.Cache.Hits.Add(1)
		}()
	}
	wg.Wait()

	s := m.Snapshot()
	if s.ConnCreated != 100 || s.ConnReused != 100 {
		t.Fatalf("unexpected connection counters: created=%d reused=%d", s.ConnCreated, s.ConnReused)
	}
	if s.RaceAttempts != 100 || s.RaceH3Wins != 100 {
		t.Fatalf("unexpected racing counters: attempts=%d h3wins=%d", s.RaceAttempts, s.RaceH3Wins)
	}
	if s.CacheLookups != 100 || s.CacheHits != 100 {
		t.Fatalf("unexpected cache counters: lookups=%d hits=%d", s.CacheLookups, s.CacheHits)
	}
}

func TestMetricsSnapshotZeroOnInit(t *testing.T) {
	m := NewTransportMetrics()
	s := m.Snapshot()
	if s.ConnCreated != 0 || s.RaceH3Wins != 0 || s.CacheHits != 0 {
		t.Fatal("expected all-zero snapshot on new metrics")
	}
}

func BenchmarkMetricsAdd(b *testing.B) {
	m := NewTransportMetrics()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.Cache.Lookups.Add(1)
			m.Cache.Hits.Add(1)
		}
	})
}

// Verify MetricsCollector interface can be satisfied (TransportMetrics is its own collector).
func (m *TransportMetrics) GetMetrics() *TransportMetrics {
	return m
}

var _ MetricsCollector = (*TransportMetrics)(nil)

func TestMetricsSnapshotIdempotent(t *testing.T) {
	m := NewTransportMetrics()
	m.Cache.Lookups.Add(5)
	m.Cache.Hits.Add(3)

	// Multiple snapshots should return same data (no side effects)
	s1 := m.Snapshot()
	s2 := m.Snapshot()
	if s1.CacheLookups != s2.CacheLookups || s1.CacheHits != s2.CacheHits {
		t.Fatal("snapshot should be idempotent")
	}
}

func TestClientWithDefaultOptionsProfile(t *testing.T) {
	// Ensure default client construction is stable post-refactor
	client, err := NewHttpClient(NewNoopLogger(),
		WithClientProfile(profiles.Chrome_146),
		WithTimeoutSeconds(10),
	)
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	client.CloseIdleConnections()
}
