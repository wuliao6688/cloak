package cloak

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wuliao6688/cloak/profiles"
)

// TestFallbackToH1WhenNoALPN guards the fix for servers that complete a
// TLS 1.3 handshake but send no ALPN extension (empty NegotiatedProtocol),
// e.g. the WeChat channels edge for some ClientHello profiles. The stock
// x/net/http2 skips its ALPN check when DialTLSContext returns a conn, so
// it would speak HTTP/2 to a server that selected nothing and stall forever.
// The Transport must instead fall back to HTTP/1.1 and succeed.
func TestFallbackToH1WhenNoALPN(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "alpn-h1-ok")
	}))
	// No NextProtos advertised: the server sends no ALPN, matching the
	// WeChat channels edge that withholds h2 for some ClientHellos.
	srv.TLS = &tls.Config{}
	srv.StartTLS()
	defer srv.Close()

	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	done := make(chan error, 1)
	go func() {
		resp, err := client.Get(srv.URL)
		if err != nil {
			done <- err
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "alpn-h1-ok" {
			done <- err
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("空 ALPN 时应回退 HTTP/1.1 并成功，但失败: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("空 ALPN 时未回退 HTTP/1.1，连接挂起（修复失效）")
	}
}
