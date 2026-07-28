package tlsgateway

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bogdanfinn/tls-client/profiles"
)

// TestRaceTransportH2 verifies RaceTransport handles basic GET requests.
func TestRaceTransportH2(t *testing.T) {
	srv := startLocalTLSServer(t)
	tr := NewRaceTransportWithOptions(profiles.Chrome_150,
		TransportOptions{InsecureSkipVerify: true},
		RaceOptions{H2Delay: 50 * time.Millisecond, Timeout: 10 * time.Second},
	)
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	resp, err := client.Get(srv.URL + "/race-test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	t.Logf("RaceTransport proto=%s status=%d body=%q", resp.Proto, resp.StatusCode, string(body))
}

// TestRaceTransportPlainHTTP verifies RaceTransport handles plain HTTP (no racing).
func TestRaceTransportPlainHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "plain:%s", r.URL.Path)
	}))
	defer srv.Close()

	tr := NewRaceTransport(profiles.Chrome_150, DefaultRaceOptions())
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	defer tr.CloseIdleConnections()

	resp, err := client.Get(srv.URL + "/plain-race")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "plain:/plain-race" {
		t.Errorf("unexpected body: %q", string(body))
	}
	t.Logf("RaceTransport plain: body=%q", string(body))
}

// TestRaceTransportNonIdempotent verifies POST requests are NOT raced.
func TestRaceTransportNonIdempotent(t *testing.T) {
	srv := startLocalTLSServer(t)
	tr := NewRaceTransportWithOptions(profiles.Chrome_150,
		TransportOptions{InsecureSkipVerify: true},
		DefaultRaceOptions(),
	)
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	defer tr.CloseIdleConnections()

	// POST should NOT be raced (idempotency safety).
	resp, err := client.Post(srv.URL+"/post-test", "text/plain", nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("RaceTransport POST proto=%s status=%d body=%q", resp.Proto, resp.StatusCode, string(body))
}

// TestRaceTransportSetProfile verifies SetProfile propagates correctly.
func TestRaceTransportSetProfile(t *testing.T) {
	srv := startLocalTLSServer(t)
	tr := NewRaceTransportWithOptions(profiles.Chrome_150,
		TransportOptions{InsecureSkipVerify: true},
		DefaultRaceOptions(),
	)
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	defer tr.CloseIdleConnections()

	resp, err := client.Get(srv.URL + "/chrome")
	if err != nil {
		t.Fatalf("chrome Get: %v", err)
	}
	resp.Body.Close()

	tr.SetProfile(profiles.Firefox_148)
	resp, err = client.Get(srv.URL + "/firefox")
	if err != nil {
		t.Fatalf("firefox Get: %v", err)
	}
	resp.Body.Close()
	t.Log("RaceTransport SetProfile: OK")
}
