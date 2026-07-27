package tls_client

import (
	"io"
	"strings"
	"sync"
	"testing"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/tls-client/profiles"
)

// Benchmark HTTP client construction (no network).

func BenchmarkNewHttpClient(b *testing.B) {
	b.ReportAllocs()
	jar := NewCookieJar()
	for i := 0; i < b.N; i++ {
		client, err := NewHttpClient(NewNoopLogger(),
			WithClientProfile(profiles.Chrome_146),
			WithTimeoutSeconds(30),
			WithCookieJar(jar),
		)
		if err != nil {
			b.Fatal(err)
		}
		client.CloseIdleConnections()
	}
}

// Benchmark Transport cache lookups (no network).

func BenchmarkGetCachedTransport(b *testing.B) {
	cache := map[string]http.RoundTripper{"example.com:443": &noopRoundTripper{}}
	var lock sync.RWMutex
	meta := &transportCacheMeta{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getCachedTransportEntry(cache, &lock, meta, "example.com:443")
	}
}

// Benchmark cloneRequestForRace (body cloning for protocol racing).

func BenchmarkCloneRequestForRace(b *testing.B) {
	body := "payload"
	req, err := http.NewRequest("GET", "https://example.com", strings.NewReader(body))
	if err != nil {
		b.Fatal(err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(body)), nil
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cloned, err := cloneRequestForRace(req, req.Context())
		if err != nil {
			b.Fatal(err)
		}
		cloned.Body.Close()
	}
}

// Benchmark protocolRacerConfig.toRacer() construction.

func BenchmarkProtocolRacerConstruction(b *testing.B) {
	cfg := &protocolRacerConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.toRacer()
	}
}
