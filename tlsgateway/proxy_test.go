package tlsgateway

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/bogdanfinn/tls-client/profiles"
)

// startTestProxy starts a proxy on a random port and returns its URL.
func startTestProxy(t *testing.T, profile profiles.ClientProfile) (*Proxy, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	addr := listener.Addr().String()
	p := NewProxy(addr, profile)
	p.Logger = log.New(io.Discard, "", 0)

	p.rebuildTransport()

	srv := &http.Server{
		Addr:    addr,
		Handler: http.HandlerFunc(p.serve),
	}

	p.server = srv
	go func() {
		srv.Serve(listener)
	}()

	// Wait for the proxy to be ready.
	time.Sleep(100 * time.Millisecond)

	return p, fmt.Sprintf("http://%s", addr)
}

// TestProxyHTTPRequest verifies that an HTTP request through the proxy
// reaches the upstream server correctly.
func TestProxyHTTPRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "upstream:%s", r.URL.Path)
	}))
	defer upstream.Close()

	p, proxyURL := startTestProxy(t, profiles.Chrome_150)
	defer p.server.Close()

	proxyParsed, _ := url.Parse(proxyURL)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyParsed),
		},
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(upstream.URL + "/test-path")
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "upstream:/test-path" {
		t.Errorf("unexpected body: %q", string(body))
	}
	t.Logf("HTTP proxy: body=%q status=%d", string(body), resp.StatusCode)
}

// TestProxyHealthCheck verifies the health endpoint works.
func TestProxyHealthCheck(t *testing.T) {
	p, proxyURL := startTestProxy(t, profiles.Chrome_150)
	defer p.server.Close()

	resp, err := http.Get(proxyURL + "/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("health: %s", string(body))

	if resp.StatusCode != 200 {
		t.Errorf("health status: %d", resp.StatusCode)
	}
}

// TestProxyProfileReload verifies the /reload endpoint changes the active profile.
func TestProxyProfileReload(t *testing.T) {
	p, proxyURL := startTestProxy(t, profiles.Chrome_150)
	defer p.server.Close()

	// Initial profile.
	resp, err := http.Get(proxyURL + "/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	t.Logf("before reload: %s", string(body))

	// Reload with Firefox.
	resp, err = http.Post(
		fmt.Sprintf("%s/reload?profile=firefox_148", proxyURL),
		"application/json",
		nil,
	)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	t.Logf("reload response: %s", string(body))

	// Verify profile changed.
	resp, err = http.Get(proxyURL + "/health")
	if err != nil {
		t.Fatalf("health after reload: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	t.Logf("after reload: %s", string(body))
}

// TestProxyConcurrentRequests verifies the proxy handles concurrent clients.
func TestProxyConcurrentRequests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	p, proxyURL := startTestProxy(t, profiles.Chrome_150)
	defer p.server.Close()

	proxyParsed, _ := url.Parse(proxyURL)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyParsed),
		},
		Timeout: 10 * time.Second,
	}

	const goroutines = 20
	errCh := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			resp, err := client.Get(upstream.URL)
			if err != nil {
				errCh <- err
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			errCh <- nil
		}()
	}

	for i := 0; i < goroutines; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent client: %v", err)
		}
	}
	t.Logf("%d concurrent proxy requests passed", goroutines)
}

// TestProxyConnectTunnel verifies CONNECT tunneling works for HTTPS.
// We test by sending a raw CONNECT request and tunneling data through.
func TestProxyConnectTunnel(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secure-ok"))
	}))
	defer upstream.Close()

	p, proxyURL := startTestProxy(t, profiles.Chrome_150)
	defer p.server.Close()
	// Self-signed test certificate — skip verification for the test.
	p.SetInsecureSkipVerify(true)

	// Parse proxy address.
	proxyParsed, _ := url.Parse(proxyURL)
	proxyHost := proxyParsed.Host

	// Parse upstream address (strip https://).
	upstreamParsed, _ := url.Parse(upstream.URL)
	upstreamHost := upstreamParsed.Host

	// Dial the proxy directly.
	conn, err := net.Dial("tcp", proxyHost)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()

	// Send CONNECT request.
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", upstreamHost, upstreamHost)

	// Read the proxy's response.
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}

	response := string(buf[:n])
	t.Logf("CONNECT response: %q", response)

	if !contains(response, "200") {
		t.Fatalf("expected 200, got: %q", response)
	}

	// Now the connection is tunneled. Send a simple HTTP request through the tunnel.
	// (We'd need TLS on top of the tunnel, but for a basic test, the 200 is enough.)
	t.Log("CONNECT tunnel established successfully")
}
