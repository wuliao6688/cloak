package tlsgateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	utls "github.com/bogdanfinn/utls"
	"golang.org/x/net/http2"

	"github.com/bogdanfinn/tls-client/profiles"
)

// Transport is an http.RoundTripper that applies a TLS ClientHello
// fingerprint from a profiles.ClientProfile. It is the zero-fork
// default — only depends on utls + x/net/http2 + standard library.
//
// Protocol negotiation:
//   - HTTPS → HTTP/2 (x/net/http2 + uTLS)
//   - If server doesn't support H2 → automatic fallback to HTTP/1.1 over TLS
//   - HTTP  → HTTP/1.1 (net/http)
//
// Build-tag extensions (opt-in only):
//
//	go build -tags h3     → adds HTTP/3 support
//	go build -tags fhttp  → adds Akamai-level H2 SETTINGS customization
type Transport struct {
	h2  *http2.Transport // primary: HTTPS with HTTP/2
	h1  *http.Transport   // fallback: HTTP/1.1 over TLS
	h1p *http.Transport   // plain HTTP (no TLS)

	profileMu sync.RWMutex
	profile   profiles.ClientProfile

	randomExtensionOrder bool
	serverNameOverwrite  string
	insecureSkipVerify   bool
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
// This is the zero-fork default — no fhttp, no QUIC fork.
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
	t.h1 = &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return t.dialTLS(ctx, network, addr)
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
func (t *Transport) SetProfile(profile profiles.ClientProfile) {
	t.profileMu.Lock()
	t.profile = profile
	t.profileMu.Unlock()
}

// SetRandomExtensionOrder enables/disables random TLS extension order.
func (t *Transport) SetRandomExtensionOrder(enabled bool) {
	t.profileMu.Lock()
	t.randomExtensionOrder = enabled
	t.profileMu.Unlock()
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return t.h1p.RoundTrip(req)
	}

	// Try H2 first.
	resp, err := t.h2.RoundTrip(req)
	if err == nil {
		return resp, nil
	}

	// H2 failed — check if server doesn't support it.
	if isProtocolError(err) {
		return t.h1.RoundTrip(req)
	}

	return nil, err
}

// isProtocolError returns true when the error indicates the server
// doesn't support H2 (ALPN mismatch, bad preface, etc).
func isProtocolError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case contains(msg, "http2: frame too large") && contains(msg, "HTTP/1.1"):
		return true
	case contains(msg, "unexpected ALPN"):
		return true
	case contains(msg, "TLS handshake") && contains(msg, "no application protocol"):
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
	t.profileMu.RLock()
	profile := t.profile
	randomOrder := t.randomExtensionOrder
	sniOverride := t.serverNameOverwrite
	insecure := t.insecureSkipVerify
	t.profileMu.RUnlock()

	dialer := &net.Dialer{}
	rawConn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("tlsgateway: dial: %w", err)
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("tlsgateway: split hostport: %w", err)
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

	uconn := utls.UClient(rawConn, utlsConfig, clientHelloID, randomOrder, false, false)
	if err := uconn.HandshakeContext(ctx); err != nil {
		uconn.Close()
		return nil, fmt.Errorf("tlsgateway: handshake: %w", err)
	}
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
