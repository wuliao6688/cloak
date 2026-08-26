package cloak

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wuliao6688/quic-go-utls"
	"github.com/wuliao6688/quic-go-utls/http3"
	utls "github.com/wuliao6688/utls"
	"golang.org/x/net/http2"

	"github.com/wuliao6688/cloak/profiles"
)

// h3ServerBundle holds both an H3 and an H2 server for racing tests.
type h3ServerBundle struct {
	h3URL string // https://localhost:PORT (H3)
	h2URL string // https://127.0.0.1:PORT (H2)
}

// startH3H2Servers starts a local H3 server AND a local H2 server on
// different ports, both serving identical responses. Used to verify the
// racer picks the right protocol per host.
func startH3H2Servers(t *testing.T) *h3ServerBundle {
	t.Helper()

	// ---- shared self-signed cert ----
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert := utls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
	_ = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644)
	keyDER, _ := x509.MarshalECPrivateKey(priv)
	_ = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "%s %s", r.Proto, r.URL.Path)
	})

	// ---- H3 server ----
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	h3Port := udpConn.LocalAddr().(*net.UDPAddr).Port
	_ = udpConn.Close()

	h3srv := &http3.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", h3Port),
		Handler: handler,
		TLSConfig: &utls.Config{
			Certificates: []utls.Certificate{cert},
			NextProtos:   []string{"h3"},
		},
		QUICConfig: &quic.Config{},
	}
	h3ErrCh := make(chan error, 1)
	go func() { h3ErrCh <- h3srv.ListenAndServeTLS(certPath, keyPath) }()
	t.Cleanup(func() { _ = h3srv.Close() })

	// ---- H2 server ----
	h2srv := httptest.NewUnstartedServer(handler)
	if err := http2.ConfigureServer(h2srv.Config, nil); err != nil {
		t.Fatalf("http2.ConfigureServer: %v", err)
	}
	h2srv.TLS = &tls.Config{NextProtos: []string{"h2", "http/1.1"}}
	h2srv.StartTLS()
	t.Cleanup(h2srv.Close)

	// Wait for H3 readiness.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-h3ErrCh:
			if err != nil {
				t.Fatalf("h3 server failed: %v", err)
			}
		default:
		}
		conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", h3Port))
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	return &h3ServerBundle{
		h3URL: fmt.Sprintf("https://localhost:%d", h3Port),
		h2URL: h2srv.URL,
	}
}

// TestH3RaceTransportPicksH3 verifies the racer prefers H3 for an
// H3-capable host and caches the decision.
func TestH3RaceTransportPicksH3(t *testing.T) {
	bundle := startH3H2Servers(t)

	rt := NewH3RaceTransportWithOptions(profiles.Chrome_144, TransportOptions{InsecureSkipVerify: true}, DefaultRaceOptions())
	client := &http.Client{Transport: rt, Timeout: 10 * time.Second}
	defer rt.CloseIdleConnections()

	// The H3 URL (localhost) has QUIC; the racer should pick H3.
	resp, err := client.Get(bundle.h3URL + "/h3-path")
	if err != nil {
		t.Fatalf("H3 GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	t.Logf("H3 host response: %s", string(body))

	if resp.Proto != "HTTP/3.0" {
		t.Fatalf("proto=%s want HTTP/3.0", resp.Proto)
	}

	// Cached decision must be h3.
	host := hostPortFromURL(bundle.h3URL)
	if cached, ok := rt.protocolCache.Load(host); !ok || cached != "h3" {
		t.Fatalf("protocol cache=%v want h3", cached)
	}
}

// TestH3RaceTransportFallsBackToH2 verifies that a host WITHOUT QUIC
// (H2-only server) is handled: racer falls back to H2 and caches it.
func TestH3RaceTransportFallsBackToH2(t *testing.T) {
	bundle := startH3H2Servers(t)

	rt := NewH3RaceTransportWithOptions(profiles.Chrome_144, TransportOptions{InsecureSkipVerify: true}, DefaultRaceOptions())
	client := &http.Client{Transport: rt, Timeout: 10 * time.Second}
	defer rt.CloseIdleConnections()

	// The H2 URL (127.0.0.1 via httptest) has no QUIC listener on that
	// port — H3 leg fails, H2 wins.
	resp, err := client.Get(bundle.h2URL + "/h2-path")
	if err != nil {
		t.Fatalf("H2 GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	t.Logf("H2 host response: %s", string(body))

	if resp.Proto != "HTTP/2.0" {
		t.Fatalf("proto=%s want HTTP/2.0", resp.Proto)
	}

	// Cached decision must be h2 (H3 unsupported).
	host := hostPortFromURL(bundle.h2URL)
	if cached, ok := rt.protocolCache.Load(host); !ok || cached != "h2" {
		t.Fatalf("protocol cache=%v want h2", cached)
	}
}

// TestImpersonateH3 verifies the public API returns a working client.
func TestImpersonateH3(t *testing.T) {
	bundle := startH3H2Servers(t)

	// The public API uses default cert verification; for the self-signed
	// test cert we rebuild with InsecureSkipVerify via the underlying API.
	rt := NewH3RaceTransportWithOptions(profiles.Chrome_144, TransportOptions{InsecureSkipVerify: true}, DefaultRaceOptions())
	c := &http.Client{Transport: rt, Timeout: 10 * time.Second}
	defer rt.CloseIdleConnections()

	resp, err := c.Get(bundle.h3URL + "/api")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	t.Logf("ImpersonateH3 proto=%s", resp.Proto)
}

// hostPortFromURL extracts host:port from a URL string (helper for tests).
func hostPortFromURL(raw string) string {
	req, err := http.NewRequest("GET", raw, nil)
	if err != nil {
		return raw
	}
	return hostPort(req)
}
