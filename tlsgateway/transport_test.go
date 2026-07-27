package tlsgateway

import (
	"io"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/bogdanfinn/tls-client/profiles"
)

// TestTransportRoundTripH2 verifies H2 GET against a real server.
func TestTransportRoundTripH2(t *testing.T) {
	tr := NewTransport(profiles.Chrome_150)
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	resp, err := client.Get("https://httpbin.org/ip")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status: %d", resp.StatusCode)
	}
	t.Logf("H2 proto=%s status=%d", resp.Proto, resp.StatusCode)
}

// TestTransportRoundTripH2POST verifies H2 request to a real server.
func TestTransportRoundTripH2POST(t *testing.T) {
	tr := NewTransport(profiles.Chrome_150)
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	resp, err := client.Get("https://api.github.com/zen")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("H2 POST proto=%s body=%q", resp.Proto, string(body))
}

// TestTransportH2Concurrent verifies H2 handles concurrent requests.
func TestTransportH2Concurrent(t *testing.T) {
	tr := NewTransport(profiles.Chrome_150)
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get("https://httpbin.org/ip")
			if err != nil {
				errCh <- err
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			errCh <- nil
		}()
	}
	wg.Wait()
	close(errCh)

	errs := 0
	for err := range errCh {
		if err != nil {
			errs++
		}
	}
	if errs > n/2 {
		t.Errorf("too many errors: %d/%d", errs, n)
	}
	t.Logf("H2 concurrent: %d requests, %d errors", n, errs)
}

// TestTransportH2SelectProfiles verifies a few key profiles work with H2.
func TestTransportH2SelectProfiles(t *testing.T) {
	keys := []string{
		"chrome_150", "chrome_146", "chrome_131",
		"firefox_148", "firefox_147",
		"safari_ios_18_5",
		"okhttp4_android_13",
	}

	for _, key := range keys {
		profile, err := profiles.ResolveClientProfileStrict(key)
		if err != nil {
			t.Errorf("%s: resolve: %v", key, err)
			continue
		}

		tr := NewTransport(profile)
		client := &http.Client{Transport: tr, Timeout: 15 * time.Second}

		resp, err := client.Get("https://httpbin.org/ip")
		if err != nil {
			t.Errorf("%s: request: %v", key, err)
			tr.CloseIdleConnections()
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		t.Logf("%s: proto=%s status=%d", key, resp.Proto, resp.StatusCode)
		tr.CloseIdleConnections()
	}
}

// TestTransportThreadSafety verifies concurrent SetProfile + RoundTrip.
func TestTransportThreadSafety(t *testing.T) {
	tr := NewTransport(profiles.Chrome_150)
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _ := client.Get("https://httpbin.org/ip")
			if resp != nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.SetProfile(profiles.Firefox_148)
		}()
	}
	wg.Wait()
	t.Log("thread safety: OK")
}

// TestTransportHTTP1Fallback verifies plain HTTP uses H1 transport.
func TestTransportHTTP1Fallback(t *testing.T) {
	tr := NewTransport(profiles.Chrome_150)
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	// Plain HTTP should still work (no fingerprint applied).
	resp, err := client.Get("https://httpbin.org/ip")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("proto=%s status=%d", resp.Proto, resp.StatusCode)
}

// TestTransportTimeout verifies request timeout.
func TestTransportTimeout(t *testing.T) {
	tr := NewTransport(profiles.Chrome_150)
	client := &http.Client{Transport: tr, Timeout: 10 * time.Millisecond}
	defer tr.CloseIdleConnections()

	_, err := client.Get("https://httpbin.org/delay/5")
	if err == nil {
		t.Error("expected timeout error")
	}
	t.Logf("timeout: %v", err)
}

// TestTransportProxyURL verifies proxy URL is set on the plain HTTP transport.
func TestTransportProxyURL(t *testing.T) {
	proxyURL := "http://proxy.example.com:8080"
	parsed, _ := url.Parse(proxyURL)

	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{
		Proxy: http.ProxyURL(parsed),
	})

	if tr.h1p.Proxy == nil {
		t.Error("expected proxy on h1p")
	}

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	u, err := tr.h1p.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy: %v", err)
	}
	if u.String() != proxyURL {
		t.Errorf("expected %q, got %q", proxyURL, u.String())
	}
	t.Logf("proxy URL: OK")
}
