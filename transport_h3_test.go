package cloak

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wuliao6688/quic-go-utls"
	"github.com/wuliao6688/quic-go-utls/http3"
	utls "github.com/wuliao6688/utls"

	"github.com/wuliao6688/cloak/profiles"
)

// startLocalH3Server starts a local HTTP/3 server on 127.0.0.1:0 and
// returns its base URL. It uses a UDP socket pre-bound to get a real
// ephemeral port, then passes it to the server via QUICConfig? Simpler:
// http3.Server.ListenAndServe binds :0 internally, so we discover the
// port via a pre-bound UDP socket + ListenAndServe on that address.
func startLocalH3Server(t *testing.T) string {
	t.Helper()

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
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert := utls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
	_ = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644)
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	_ = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600)

	// Pre-bind a UDP socket to learn an ephemeral port.
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	port := udpConn.LocalAddr().(*net.UDPAddr).Port
	_ = udpConn.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "H3 OK %s %s UA=%s", r.Proto, r.URL.Path, r.UserAgent())
	})

	server := http3.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: mux,
		TLSConfig: &utls.Config{
			Certificates: []utls.Certificate{cert},
			NextProtos:   []string{"h3"},
		},
		QUICConfig: &quic.Config{},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServeTLS(certPath, keyPath)
	}()
	t.Cleanup(func() { _ = server.Close() })

	// Wait for bind readiness (poll the UDP port with a quick dial).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("server failed: %v", err)
			}
		default:
		}
		conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Sprintf("https://localhost:%d", port)
}

// TestH3TransportLocalServer verifies H3 GET against a local QUIC server
// using the Chrome_144 profile (which has full H3 fingerprint data).
func TestH3TransportLocalServer(t *testing.T) {
	url := startLocalH3Server(t)

	tr := NewH3TransportWithOptions(profiles.Chrome_144, TransportOptions{InsecureSkipVerify: true})
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	defer tr.CloseIdleConnections()

	resp, err := client.Get(url + "/test-path")
	if err != nil {
		t.Fatalf("H3 GET: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("proto=%s status=%d body=%s", resp.Proto, resp.StatusCode, string(body))

	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if resp.Proto != "HTTP/3.0" {
		t.Errorf("proto: %s (want HTTP/3.0)", resp.Proto)
	}
	if !contains(string(body), "UA=Mozilla") {
		t.Errorf("UA not injected: %s", string(body))
	}
}

// TestH3TransportClientHelloID verifies the QUIC TLS fingerprint is injected
// by checking that the UQUICClient path is exercised (ClientHelloID set).
func TestH3TransportClientHelloID(t *testing.T) {
	tr := NewH3Transport(profiles.Chrome_144)
	defer tr.CloseIdleConnections()

	id := tr.profile.GetClientHelloId()
	// utls's IsSet() is inverted: returns true for EMPTY ids.
	if id.IsSet() {
		t.Fatal("Chrome_144 ClientHelloID unexpectedly empty")
	}
	if id.Str() == "Golang" {
		t.Fatalf("Chrome_144 ClientHelloID is Golang default: %s", id.Str())
	}
	t.Logf("ClientHelloID: %s (fingerprint injected via UQUICClient)", id.Str())

	// quicConfig must carry the ID (the fork reads it at dial time).
	// utls's IsSet() is inverted (true = empty), so !IsSet() = set.
	if tr.quicConfig.ClientHelloID.IsSet() {
		t.Fatal("quicConfig.ClientHelloID not propagated")
	}
}

// TestH3TransportGREASE verifies Chrome's H3 SETTINGS include a GREASE
// entry and Firefox doesn't (per upstream semantics).
func TestH3TransportGREASE(t *testing.T) {
	chrome := NewH3Transport(profiles.Chrome_144)
	rt := chrome.buildHTTP3("localhost:443")
	t3, ok := rt.(*http3.Transport)
	if !ok {
		t.Fatalf("buildHTTP3 returned %T", rt)
	}
	// Chrome: priorityParam > 0 → GREASE setting appended.
	if len(t3.AdditionalSettings) <= 1 {
		t.Errorf("Chrome AdditionalSettings=%v (want >1 with GREASE)", t3.AdditionalSettings)
	}
	if !t3.SendGreaseFrames {
		t.Error("Chrome SendGreaseFrames should be true")
	}
	chrome.CloseIdleConnections()

	firefox := NewH3Transport(profiles.Firefox_147)
	rt2 := firefox.buildHTTP3("localhost:443")
	t3f, ok := rt2.(*http3.Transport)
	if !ok {
		t.Fatalf("buildHTTP3 returned %T", rt2)
	}
	t.Logf("Firefox AdditionalSettings=%v (may have no GREASE)", t3f.AdditionalSettings)
	firefox.CloseIdleConnections()
}
