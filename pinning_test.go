package cloak

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wuliao6688/cloak/profiles"
)

// TestPinningExactHost verifies exact-host pinning: correct pin passes,
// wrong pin is rejected (MITM blocked).
func TestPinningExactHost(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pinned-ok")
	}))
	defer srv.Close()

	// Compute the server cert's real pin.
	realPin := certPin(t, srv.Certificate().Raw)

	// 1. Correct pin → request succeeds.
	trOK := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{
		PinningHosts: map[string][]string{"127.0.0.1": {realPin}},
	})
	resp, err := (&http.Client{Transport: trOK, Timeout: 10 * time.Second}).Get(srv.URL)
	trOK.CloseIdleConnections()
	if err != nil {
		t.Fatalf("正确 pin 应通过: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "pinned-ok" {
		t.Fatalf("响应异常: %q", string(body))
	}
	t.Logf("正确 pin 通过: %q", string(body))

	// 2. Wrong pin → handshake fails (MITM blocked).
	wrongPin := base64.StdEncoding.EncodeToString(make([]byte, 32)) // all zeros
	trBad := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{
		PinningHosts: map[string][]string{"127.0.0.1": {wrongPin}},
	})
	_, err = (&http.Client{Transport: trBad, Timeout: 10 * time.Second}).Get(srv.URL)
	trBad.CloseIdleConnections()
	if err == nil {
		t.Fatal("错误 pin 应被拒绝")
	}
	if !strings.Contains(err.Error(), "pinning") {
		t.Fatalf("错误信息应含 pinning, got: %v", err)
	}
	t.Logf("错误 pin 被拒绝: %v", err)
}

// TestPinningWildcard verifies *.example.com matches subdomains.
func TestPinningWildcard(t *testing.T) {
	pins := map[string][]string{
		"*.example.com": {"abc"},
		"api.test.com":  {"def"},
	}
	cases := []struct {
		host string
		want bool
	}{
		{"api.example.com", true},
		{"foo.bar.example.com", true},
		{"example.com", false}, // bare domain not matched by *.example.com
		{"other.com", false},
		{"api.test.com", true},
		{"x.test.com", false},
	}
	for _, c := range cases {
		_, got := pinMatchesHost(pins, c.host)
		if got != c.want {
			t.Errorf("pinMatchesHost(%s) = %v, want %v", c.host, got, c.want)
		}
	}
	t.Log("通配符匹配正确")
}

// TestPinningNoPinsUnchanged verifies hosts without pins behave normally
// (standard cert verification still applies).
func TestPinningNoPinsUnchanged(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	// No pinning config, default verification → self-signed cert rejected.
	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{})
	_, err := (&http.Client{Transport: tr, Timeout: 10 * time.Second}).Get(srv.URL)
	tr.CloseIdleConnections()
	if err == nil {
		t.Fatal("无 pinning + 自签证书应被拒绝(默认验证仍生效)")
	}
	t.Logf("默认验证仍生效: %v", err)
}

// certPin computes the SHA-256 pin of a DER cert (matches pinning format).
func certPin(t *testing.T, der []byte) string {
	t.Helper()
	sum := sha256.Sum256(der)
	return base64.StdEncoding.EncodeToString(sum[:])
}
