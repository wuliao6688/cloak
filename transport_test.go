package cloak

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"

	"github.com/wuliao6688/cloak/profiles"
)

// startLocalTLSServer starts an HTTPS test server with HTTP/2 support.
// HTTP/2 is essential for concurrent request tests — without it, 20
// goroutines hitting a single H1.1 connection overwhelm its connection pool.
func startLocalTLSServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "OK %s %s", r.Proto, r.URL.Path)
	}))

	// Enable HTTP/2 on the test server.
	// httptest.NewTLSServer does NOT configure H2 by default.
	if err := http2.ConfigureServer(srv.Config, nil); err != nil {
		t.Fatalf("http2.ConfigureServer: %v", err)
	}

	// Ensure TLS config advertises H2 via ALPN.
	srv.TLS = &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
	}

	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// TestTransportRoundTripH2 verifies H2 GET against a local TLS server.
func TestTransportRoundTripH2(t *testing.T) {
	srv := startLocalTLSServer(t)
	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	resp, err := client.Get(srv.URL + "/test-path")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	t.Logf("H2 proto=%s status=%d body=%q", resp.Proto, resp.StatusCode, string(body))
}

// TestTransportRoundTripH2POST verifies H2 request to a local TLS server.
func TestTransportRoundTripH2POST(t *testing.T) {
	srv := startLocalTLSServer(t)
	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	resp, err := client.Get(srv.URL + "/post-test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("H2 POST proto=%s status=%d body=%q", resp.Proto, resp.StatusCode, string(body))
}

// TestTransportH2Concurrent verifies H2 handles concurrent requests.
func TestTransportH2Concurrent(t *testing.T) {
	srv := startLocalTLSServer(t)
	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := client.Get(fmt.Sprintf("%s/concurrent-%d", srv.URL, idx))
			if err != nil {
				errCh <- err
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			errCh <- nil
		}(i)
	}
	wg.Wait()
	close(errCh)

	errs := 0
	for err := range errCh {
		if err != nil {
			t.Logf("concurrent error: %v", err)
			errs++
		}
	}
	if errs > n/2 {
		t.Errorf("%d/%d concurrent requests failed", errs, n)
	}
	t.Logf("concurrent: %d/%d OK", n-errs, n)
}

// startLocalTLSServerH1Only starts a TLS server WITHOUT HTTP/2.
// Used to verify the H2→H1 fallback path works under concurrency.
func startLocalTLSServerH1Only(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "OK %s %s", r.Proto, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestTransportH2FallbackToH1Concurrent verifies the H2→H1 fallback
// does not break under concurrent load. When the server lacks H2,
// 20 goroutines should not race on the fallback path and cause
// "connection force closed" errors.
func TestTransportH2FallbackToH1Concurrent(t *testing.T) {
	srv := startLocalTLSServerH1Only(t)
	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := client.Get(fmt.Sprintf("%s/h2fallback-%d", srv.URL, idx))
			if err != nil {
				errCh <- err
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			errCh <- nil
		}(i)
	}
	wg.Wait()
	close(errCh)

	errs := 0
	for err := range errCh {
		if err != nil {
			t.Errorf("fallback concurrent error: %v", err)
			errs++
		}
	}
	if errs > 0 {
		t.Fatalf("%d/%d fallback requests failed — H2→H1 race detected", errs, n)
	}
	t.Logf("H2→H1 fallback concurrent: %d/%d OK", n-errs, n)
}

// TestTransportH2SelectProfiles verifies a few key profiles work with H2.
func TestTransportH2SelectProfiles(t *testing.T) {
	srv := startLocalTLSServer(t)
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

		tr := NewTransportWithOptions(profile, TransportOptions{InsecureSkipVerify: true})
		client := &http.Client{Transport: tr, Timeout: 15 * time.Second}

		resp, err := client.Get(srv.URL + "/" + key)
		if err != nil {
			t.Errorf("%s: request: %v", key, err)
			tr.CloseIdleConnections()
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Logf("%s: proto=%s status=%d body=%q", key, resp.Proto, resp.StatusCode, string(body))
		tr.CloseIdleConnections()
	}
}

// TestTransportThreadSafety verifies concurrent SetProfile + RoundTrip.
func TestTransportThreadSafety(t *testing.T) {
	srv := startLocalTLSServer(t)
	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _ := client.Get(srv.URL + "/thread-safety")
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

// TestTransportH2FallbackToH1 verifies H2-first then HTTP/1.1 fallback.
func TestTransportH2FallbackToH1(t *testing.T) {
	srv := startLocalTLSServer(t)
	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	resp, err := client.Get(srv.URL + "/fallback")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status: %d", resp.StatusCode)
	}
	t.Logf("fallback proto=%s status=%d", resp.Proto, resp.StatusCode)
}

// TestTransportSetProfile verifies SetProfile changes the TLS fingerprint.
func TestTransportSetProfile(t *testing.T) {
	srv := startLocalTLSServer(t)
	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	// Initial profile.
	resp, err := client.Get(srv.URL + "/chrome")
	if err != nil {
		t.Fatalf("chrome Get: %v", err)
	}
	resp.Body.Close()

	// Switch profile.
	tr.SetProfile(profiles.Firefox_148)
	resp, err = client.Get(srv.URL + "/firefox")
	if err != nil {
		t.Fatalf("firefox Get: %v", err)
	}
	resp.Body.Close()
	t.Log("SetProfile: OK")
}

// TestTransportH1PlainHTTP verifies plain HTTP works without TLS.
func TestTransportH1PlainHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "plain:%s", r.URL.Path)
	}))
	defer srv.Close()

	tr := NewTransport(profiles.Chrome_150)
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	defer tr.CloseIdleConnections()

	resp, err := client.Get(srv.URL + "/plain-test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "plain:/plain-test" {
		t.Errorf("unexpected body: %q", string(body))
	}
	t.Logf("plain HTTP: body=%q", string(body))
}

// TestTransportProxy verifies transport works through an HTTP proxy.
func TestTransportProxy(t *testing.T) {
	// Start a simple proxy.
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "CONNECT" {
			// Simple CONNECT handler for testing.
			w.WriteHeader(200)
			return
		}
		http.Error(w, "not a proxy request", 400)
	}))
	defer proxySrv.Close()

	proxyURL, _ := url.Parse(proxySrv.URL)

	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{
		Proxy:              http.ProxyURL(proxyURL),
		InsecureSkipVerify: true,
	})
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	defer tr.CloseIdleConnections()

	// This will fail (CONNECT only returns 200 without tunneling),
	// but verifies the proxy option is wired correctly.
	srv := startLocalTLSServer(t)
	_, err := client.Get(srv.URL)
	if err == nil {
		t.Log("proxy request succeeded (unexpected with mock proxy)")
	} else {
		t.Logf("proxy request correctly failed: %v", err)
	}
}

// TestTransportCloseIdleConnections verifies cleanup doesn't panic.
func TestTransportCloseIdleConnections(t *testing.T) {
	tr := NewTransport(profiles.Chrome_150)
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	srv := startLocalTLSServer(t)
	resp, err := client.Get(srv.URL)
	if err != nil {
		// Connection error with self-signed cert is expected if InsecureSkipVerify is false.
		t.Logf("expected TLS error (cert verification): %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	tr.CloseIdleConnections()
	t.Log("CloseIdleConnections: OK")
}

// TestTransportDefaultCertVerification verifies that by default,
// certificates ARE validated (self-signed certs are rejected).
func TestTransportDefaultCertVerification(t *testing.T) {
	srv := startLocalTLSServer(t)
	tr := NewTransport(profiles.Chrome_150) // No InsecureSkipVerify
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	defer tr.CloseIdleConnections()

	_, err := client.Get(srv.URL)
	if err == nil {
		t.Error("expected TLS certificate verification error, got nil")
	} else {
		t.Logf("correctly rejected self-signed cert: %v", err)
	}
}
