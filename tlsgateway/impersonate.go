package tlsgateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/bogdanfinn/tls-client/profiles"
)

// Impersonate creates a fully configured http.Client that impersonates
// the given browser profile — TLS fingerprint (uTLS ClientHello),
// HTTP/2 settings, and browser HTTP headers all in one step.
//
// Usage:
//
//	client := tlsgateway.Impersonate(profiles.Chrome_150)
//	resp, _ := client.Get("https://www.example.com")
//
// Equivalent to calling NewTransport + NewHeaderRoundTripper manually.
func Impersonate(profile profiles.ClientProfile) *http.Client {
	tr := NewTransport(profile)
	htr := NewHeaderRoundTripper(tr, profile)
	return &http.Client{Transport: htr, Timeout: 30 * time.Second}
}

// ImpersonateChain returns a builder for chain-based configuration.
func ImpersonateChain(profile profiles.ClientProfile) *ChainBuilder {
	tr := NewTransport(profile)
	return &ChainBuilder{
		tr:      tr,
		profile: profile,
		timeout: 30 * time.Second,
	}
}

// ChainBuilder provides a fluent API for configuring and building
// an impersonated HTTP client.
//
//	client := tlsgateway.ImpersonateChain(profiles.Chrome_150).
//	    SetUserAgent("custom UA").
//	    SetDebug(os.Stderr).
//	    SetTimeout(10 * time.Second).
//	    Build()
type ChainBuilder struct {
	tr      *Transport
	profile profiles.ClientProfile
	timeout time.Duration
	ua      string
	debug   io.Writer
	headers map[string]string
}

// SetUserAgent overrides the browser User-Agent.
func (cb *ChainBuilder) SetUserAgent(ua string) *ChainBuilder {
	cb.ua = ua
	return cb
}

// SetTimeout sets the client timeout.
func (cb *ChainBuilder) SetTimeout(d time.Duration) *ChainBuilder {
	cb.timeout = d
	return cb
}

// SetHeader adds or overrides a header.
func (cb *ChainBuilder) SetHeader(key, value string) *ChainBuilder {
	if cb.headers == nil {
		cb.headers = make(map[string]string)
	}
	cb.headers[key] = value
	return cb
}

// SetDebug enables TLS/connection debug output.
func (cb *ChainBuilder) SetDebug(w io.Writer) *ChainBuilder {
	cb.debug = w
	cb.tr.SetDebug(w)
	return cb
}

// Transport returns the underlying Transport (for middleware wrapping, etc.).
func (cb *ChainBuilder) Transport() *Transport {
	return cb.tr
}

// Build creates the impersonated http.Client.
func (cb *ChainBuilder) Build() *http.Client {
	htr := NewHeaderRoundTripper(cb.tr, cb.profile)

	// Apply custom headers if any.
	if cb.ua != "" || len(cb.headers) > 0 {
		htrWrap := &customHeaderRoundTripper{
			inner:   htr,
			ua:      cb.ua,
			headers: cb.headers,
		}
		return &http.Client{Transport: htrWrap, Timeout: cb.timeout}
	}

	return &http.Client{Transport: htr, Timeout: cb.timeout}
}

// customHeaderRoundTripper allows overriding specific headers on top
// of the profile-based defaults from HeaderRoundTripper.
type customHeaderRoundTripper struct {
	inner   http.RoundTripper
	ua      string
	headers map[string]string
}

func (c *customHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.ua != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.ua)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	return c.inner.RoundTrip(req)
}

// ─── SelfCheck ────────────────────────────────────────────────────────────

// FingerprintInfo holds the result of a TLS fingerprint self-check.
type FingerprintInfo struct {
	JA3     string `json:"ja3"`
	JA3Hash string `json:"ja3_hash"`
	JA4     string `json:"ja4"`
	Profile string `json:"profile"`
	Error   string `json:"error,omitempty"`
}

// SelfCheck connects to an external fingerprint detection service and
// returns the current TLS fingerprint information. Useful for verifying
// that the uTLS profile is being applied correctly.
//
// Uses tls.peet.ws as the detection service.
func (t *Transport) SelfCheck(timeout time.Duration) (*FingerprintInfo, error) {
	client := &http.Client{Transport: t, Timeout: timeout}
	resp, err := client.Get("https://tls.peet.ws/api/all")
	if err != nil {
		return &FingerprintInfo{Profile: t.profile.GetClientHelloStr(), Error: err.Error()}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	if err != nil {
		return &FingerprintInfo{Profile: t.profile.GetClientHelloStr(), Error: err.Error()}, err
	}

	var data struct {
		TLS struct {
			JA3     string `json:"ja3"`
			JA3Hash string `json:"ja3_hash"`
			JA4     string `json:"ja4"`
		} `json:"tls"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return &FingerprintInfo{Profile: t.profile.GetClientHelloStr(), Error: fmt.Sprintf("parse: %v", err)}, err
	}

	return &FingerprintInfo{
		Profile: t.profile.GetClientHelloStr(),
		JA3:     data.TLS.JA3,
		JA3Hash: data.TLS.JA3Hash,
		JA4:     data.TLS.JA4,
	}, nil
}

// SelfCheck is a package-level convenience function.
func SelfCheck(profile profiles.ClientProfile) (*FingerprintInfo, error) {
	tr := NewTransport(profile)
	defer tr.CloseIdleConnections()
	return tr.SelfCheck(15 * time.Second)
}

// ─── Debug ────────────────────────────────────────────────────────────────

// SetDebug enables debug logging of TLS handshake events and transport decisions.
// When set, the Transport will write connection lifecycle events to w.
//
// Output includes:
//   - TLS handshake start/finish
//   - ALPN negotiation result
//   - H2 vs H1.1 path selection
//   - Protocol errors that trigger fallback
func (t *Transport) SetDebug(w io.Writer) {
	t.profileMu.Lock()
	defer t.profileMu.Unlock()
	t.debugWriter = w
	if w != nil {
		t.debugLog = log.New(w, "[tlsgateway] ", log.Ltime|log.Lmicroseconds)
	} else {
		t.debugLog = log.New(io.Discard, "", 0)
	}
}

// GetDebug returns the current debug writer (nil if disabled).
func (t *Transport) GetDebug() io.Writer {
	t.profileMu.RLock()
	defer t.profileMu.RUnlock()
	return t.debugWriter
}

// debugf logs a debug message if debug is enabled.
func (t *Transport) debugf(format string, args ...interface{}) {
	t.profileMu.RLock()
	dl := t.debugLog
	t.profileMu.RUnlock()
	if dl != nil && dl.Writer() != io.Discard {
		dl.Printf(format, args...)
	}
}

// ─── Dump ─────────────────────────────────────────────────────────────────

// DumpFingerprint is a convenience function that self-checks and prints the
// fingerprint to stdout. Useful for CLI tools:
//
//	tlsgateway.DumpFingerprint(profiles.Chrome_150)
//	// Output: Chrome-150  JA3: eeee4c67...  JA4: t13d1516h2_8daaf6152771_...
func DumpFingerprint(profile profiles.ClientProfile) {
	info, err := SelfCheck(profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DumpFingerprint: %v\n", err)
		return
	}
	ja3 := info.JA3Hash
	if ja3 == "" {
		ja3 = info.JA3
	}
	fmt.Printf("%-20s  JA3: %s  JA4: %s\n", info.Profile, trunc(ja3, 40), trunc(info.JA4, 40))
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// forceHTTP1HeaderOrder forces request headers to match Chrome's order.
// Chrome orders headers: :method, :authority, :scheme, :path
// then alphabetically for the rest.
func forceHTTP1HeaderOrder(req *http.Request) {
	// net/http already handles header ordering in HTTP/1.1 (canonical, then
	// insertion order). For HTTP/1.1, the wire order follows the canonical
	// map iteration, which is arbitrary. We can't control it without
	// replacing the header transport.
	//
	// For HTTP/2, pseudo-header order is controlled by the transport.
	// Currently we use Go's default order. In the future, we should set
	// PseudoHeaderOrder via golang.org/x/net/http2.Transport (requires
	// import but no fork).
}

// forceHTTP2PseudoHeaderOrder sets pseudo-header order on the H2 transport.
func forceHTTP2PseudoHeaderOrder(order []string) {
	// golang.org/x/net/http2.Transport doesn't expose PseudoHeaderOrder.
	// This is a known limitation. If strict H2 fingerprint detection is
	// needed, a fork of x/net/http2 or use of fhttp is required.
	_ = order
}
