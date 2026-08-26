package cloak

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	utls "github.com/wuliao6688/utls"
	"golang.org/x/net/http2"

	"github.com/wuliao6688/cloak/profiles"
)

// Transport is an http.RoundTripper that applies a TLS ClientHello
// fingerprint from a profiles.ClientProfile. It is the zero-fork
// default — only depends on utls + x/net/http2 + standard library.
//
// Protocol negotiation:
//   - HTTPS → HTTP/2 (x/net/http2 + uTLS)
//   - If server doesn't support H2 → automatic fallback to HTTP/1.1 over TLS
//   - HTTP  → HTTP/1.1 (net/http)
type Transport struct {
	h2  *http2.Transport // primary: HTTPS with HTTP/2
	h1  *http.Transport   // fallback: HTTP/1.1 over TLS
	h1p *http.Transport   // plain HTTP (no TLS)

	profileMu sync.RWMutex
	profile   profiles.ClientProfile

	randomExtensionOrder bool
	serverNameOverwrite  string
	insecureSkipVerify   bool

	// h2Disabled tracks hosts that don't support H2.
	// Map key: "host:port" → true.
	h2Disabled sync.Map

	// h2ProbeMu serializes H2 capability probing per host.
	// Without this, 20 goroutines hitting a new non-H2 server
	// all load h2Disabled=false, all try H2 simultaneously,
	// and half get "connection force closed" since the H2
	// transport tears down connections during failure.
	h2ProbeMu sync.Map // map[string]*sync.Mutex

	// Debug (optional). When set, TLS handshake and transport
	// decisions are logged.
	debugWriter io.Writer
	debugLog   *log.Logger

	// browserHeaders are injected into every request RoundTrip.
	// Populated from BrowserFingerprint at NewTransport time.
	// Makes Transport self-contained — no external HeaderRoundTripper needed.
	browserHeaders map[string]string
}

var _ http.RoundTripper = (*Transport)(nil)

// TransportOptions configures optional Transport behavior.
type TransportOptions struct {
	RandomExtensionOrder bool
	ServerNameOverwrite  string
	Proxy                func(*http.Request) (*url.URL, error)
	InsecureSkipVerify   bool // default: false (certificates verified)
}

// NewTransport creates a Transport using the given profile.
// TLS certificate verification is enabled by default.
func NewTransport(profile profiles.ClientProfile) *Transport {
	return NewTransportWithOptions(profile, TransportOptions{})
}

// NewTransportWithOptions creates a Transport with options.
func NewTransportWithOptions(profile profiles.ClientProfile, opts TransportOptions) *Transport {
	t := &Transport{
		profile:              profile,
		randomExtensionOrder: opts.RandomExtensionOrder,
		serverNameOverwrite:  opts.ServerNameOverwrite,
		insecureSkipVerify:   opts.InsecureSkipVerify,
		debugLog:             log.New(io.Discard, "", 0),
	}

	// Inject browser headers (req-style: Transport = TLS + HTTP in one object).
	if fp := BrowserFingerprint(profile.GetClientHelloStr()); fp != nil {
		t.browserHeaders = fp.Headers
	}

	// Plain HTTP (no TLS).
	t.h1p = &http.Transport{
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if opts.Proxy != nil {
		t.h1p.Proxy = opts.Proxy
	}

	// HTTPS with HTTP/2 (primary).
	t.h2 = &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			return t.dialTLS(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: t.insecureSkipVerify},
	}

	// HTTPS fallback to HTTP/1.1 (when server doesn't support H2).
	// Uses a SEPARATE dialer that forces HTTP/1.1 ALPN — without this,
	// the H2 ALPN advertised by the uTLS profile would cause the server
	// to send H2 frames that H1 can't parse, breaking the fallback.
	t.h1 = &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return t.dialTLSH1(ctx, network, addr)
		},
		ForceAttemptHTTP2:     false,
		TLSNextProto:          make(map[string]func(string, *tls.Conn) http.RoundTripper),
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return t
}

// SetProfile replaces the TLS fingerprint profile.
// Clears the H2 capability cache since a different fingerprint
// may negotiate H2 differently.
func (t *Transport) SetProfile(profile profiles.ClientProfile) {
	t.profileMu.Lock()
	t.profile = profile
	t.profileMu.Unlock()
	t.h2Disabled.Clear()
}

// SetRandomExtensionOrder enables/disables random TLS extension order.
func (t *Transport) SetRandomExtensionOrder(enabled bool) {
	t.profileMu.Lock()
	t.randomExtensionOrder = enabled
	t.profileMu.Unlock()
}

// RoundTrip implements http.RoundTripper.
//
// Protocol negotiation per-host:
//  1. If host is known to lack H2 → go straight to H1.1.
//  2. Only ONE goroutine probes H2 per host (h2ProbeMu).
//  3. If H2 probe succeeds → future requests use H2.
//  4. If H2 fails with protocol error → mark host disabled, use H1.1.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Inject browser headers (req-style: one object, everything built-in).
	for k, v := range t.browserHeaders {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}

	if req.URL.Scheme != "https" {
		return t.h1p.RoundTrip(req)
	}

	host := hostPort(req)

	// Fast path: host is known to lack H2.
	if _, disabled := t.h2Disabled.Load(host); disabled {
		t.debugf("%s → H1 (cached)", host)
		return t.h1.RoundTrip(req)
	}

	// Get or create a per-host mutex for H2 probing.
	muI, _ := t.h2ProbeMu.LoadOrStore(host, &sync.Mutex{})
	mu := muI.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// Double-check: another goroutine may have probed while we waited.
	if _, disabled := t.h2Disabled.Load(host); disabled {
		t.debugf("%s → H1 (double-check)", host)
		return t.h1.RoundTrip(req)
	}

	// Probe H2.
	resp, err := t.h2.RoundTrip(req)
	if err == nil {
		t.debugf("%s → H2 OK", host)
		return resp, nil
	}

	// H2 failed — check if server doesn't support it.
	if isProtocolError(err) {
		t.debugf("%s → H2 failed (%v), fallback to H1", host, err)
		t.h2Disabled.Store(host, true)
		return t.h1.RoundTrip(req)
	}

	return nil, err
}

// hostPort extracts "host:port" from the request URL.
// For HTTPS, port defaults to 443.
func hostPort(req *http.Request) string {
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "443"
	}
	return host + ":" + port
}

// isProtocolError returns true when the error indicates the server
// doesn't support H2. This covers ALPN mismatches, H1.1 fallback
// responses, and connection establishment failures that indicate
// the host cannot speak H2.
func isProtocolError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case contains(msg, "http2: frame too large"):
		return true
	case contains(msg, "unexpected ALPN"):
		return true
	case contains(msg, "TLS handshake") && contains(msg, "no application protocol"):
		return true
	case contains(msg, "client conn could not be established"):
		return true
	}
	return false
}

func (t *Transport) setInsecureSkipVerify(v bool) {
	t.profileMu.Lock()
	defer t.profileMu.Unlock()
	t.insecureSkipVerify = v
	if t.h2 != nil {
		t.h2.TLSClientConfig.InsecureSkipVerify = v
	}
	if t.h1 != nil && t.h1.TLSClientConfig != nil {
		t.h1.TLSClientConfig.InsecureSkipVerify = v
	}
}

// SetInsecureSkipVerify toggles TLS certificate verification on all
// underlying transports (h2/h1). Public so it can be reached through
// wrapper round trippers (HeaderRoundTripper, customHeaderRoundTripper).
func (t *Transport) SetInsecureSkipVerify(v bool) {
	t.setInsecureSkipVerify(v)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSub(s, substr)
}

func searchSub(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// DialTLS creates a raw uTLS connection (no H2 framing). Exported for
// use by the proxy CONNECT tunnel handler.
func (t *Transport) DialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.dialTLS(ctx, network, addr)
}

func (t *Transport) dialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.dialTLSWithH1(ctx, network, addr, false)
}

// dialTLSH1 is like dialTLS but forces HTTP/1.1 ALPN (no "h2").
// Used by the H1 fallback transport to avoid the server sending H2
// frames to an H1.1 client. Without this, Akamai and similar CDNs
// detect the ALPN mismatch and block the connection.
func (t *Transport) dialTLSH1(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.dialTLSWithH1(ctx, network, addr, true)
}

func (t *Transport) dialTLSWithH1(ctx context.Context, network, addr string, forceH1 bool) (net.Conn, error) {
	t.profileMu.RLock()
	profile := t.profile
	randomOrder := t.randomExtensionOrder
	sniOverride := t.serverNameOverwrite
	insecure := t.insecureSkipVerify
	t.profileMu.RUnlock()

	dialer := &net.Dialer{}
	rawConn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("cloak: dial: %w", err)
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("cloak: split hostport: %w", err)
	}
	if sniOverride != "" {
		host = sniOverride
	}

	utlsConfig := &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: insecure,
		OmitEmptyPsk:       true,
		ClientSessionCache: utls.NewLRUClientSessionCache(32),
	}

	clientHelloID := profile.GetClientHelloId()

	t.debugf("%s → dial TLS profile=%s forceH1=%v", addr, profile.GetClientHelloStr(), forceH1)

	uconn := utls.UClient(rawConn, utlsConfig, clientHelloID, randomOrder, forceH1, false)
	if err := uconn.HandshakeContext(ctx); err != nil {
		uconn.Close()
		t.debugf("%s → handshake FAIL: %v", addr, err)
		return nil, fmt.Errorf("cloak: handshake: %w", err)
	}
	t.debugf("%s → handshake OK negotiated=%s", addr, uconn.ConnectionState().NegotiatedProtocol)
	return uconn, nil
}

// CloseIdleConnections closes idle connections in all transports.
func (t *Transport) CloseIdleConnections() {
	if t.h1p != nil {
		t.h1p.CloseIdleConnections()
	}
	if t.h1 != nil {
		t.h1.CloseIdleConnections()
	}
	if t.h2 != nil {
		t.h2.CloseIdleConnections()
	}
}
