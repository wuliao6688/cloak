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
	tr            *Transport
	profile       profiles.ClientProfile
	timeout       time.Duration
	ua            string
	debug         io.Writer
	headers       map[string]string
	orderedHdrs   bool
	h2Fingerprint *H2Fingerprint
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

// WithOrderedHeaders enables HTTP/1.1 header ordering to match Chrome's
// canonical header order. Only affects HTTP/1.1; for H2, pseudo-header
// and header ordering requires forking x/net/http2 (like req's internal/http2).
func (cb *ChainBuilder) WithOrderedHeaders() *ChainBuilder {
	cb.orderedHdrs = true
	return cb
}

// WithH2Fingerprint registers H2 fingerprint configuration for use
// when H2 customization is enabled (via internal/http2 fork or similar).
func (cb *ChainBuilder) WithH2Fingerprint(fingerprint *H2Fingerprint) *ChainBuilder {
	cb.h2Fingerprint = fingerprint
	return cb
}

// SetH2Fingerprint sets a custom H2 fingerprint.
func (cb *ChainBuilder) SetH2Fingerprint(fp *H2Fingerprint) *ChainBuilder {
	cb.h2Fingerprint = fp
	return cb
}

// AsChrome sets the full Chrome browser fingerprint.
func (cb *ChainBuilder) AsChrome() *ChainBuilder {
	cb.h2Fingerprint = BrowserFingerprint("chrome")
	return cb
}

// AsFirefox sets the full Firefox browser fingerprint.
func (cb *ChainBuilder) AsFirefox() *ChainBuilder {
	cb.h2Fingerprint = BrowserFingerprint("firefox")
	return cb
}

// AsSafari sets the full Safari browser fingerprint.
func (cb *ChainBuilder) AsSafari() *ChainBuilder {
	cb.h2Fingerprint = BrowserFingerprint("safari")
	return cb
}

// AsEdge sets the full Edge browser fingerprint.
func (cb *ChainBuilder) AsEdge() *ChainBuilder {
	cb.h2Fingerprint = BrowserFingerprint("edge")
	return cb
}

// AsQQ sets the QQ 浏览器 fingerprint.
func (cb *ChainBuilder) AsQQ() *ChainBuilder {
	cb.h2Fingerprint = BrowserFingerprint("qq")
	return cb
}

// As360 sets the 360 浏览器 fingerprint.
func (cb *ChainBuilder) As360() *ChainBuilder {
	cb.h2Fingerprint = BrowserFingerprint("360")
	return cb
}

// AsIOS sets the iOS fingerprint.
func (cb *ChainBuilder) AsIOS() *ChainBuilder {
	cb.h2Fingerprint = BrowserFingerprint("ios")
	return cb
}

// AsAndroid sets the Android fingerprint.
func (cb *ChainBuilder) AsAndroid() *ChainBuilder {
	cb.h2Fingerprint = BrowserFingerprint("android")
	return cb
}

// AsRandom sets a randomly selected browser fingerprint.
func (cb *ChainBuilder) AsRandom() *ChainBuilder {
	cb.h2Fingerprint = RandomFingerprint()
	return cb
}

// Build creates the impersonated http.Client.
func (cb *ChainBuilder) Build() *http.Client {
	var rt http.RoundTripper = NewHeaderRoundTripper(cb.tr, cb.profile)

	// Get browser fingerprint for header defaults.
	name := cb.profile.GetClientHelloStr()
	fp := BrowserFingerprint(name)

	// Apply HTTP/1.1 header ordering if requested.
	if cb.orderedHdrs {
		rt = NewOrderedHeadersRoundTripperFull(rt, fp.HeaderOrder, fp.PseudoHeaderOrder)
	}

	// Inject browser default headers (cache-control, sec-fetch-*, etc.)
	rt = &browserHeadersRoundTripper{
		inner:   rt,
		headers: fp.Headers,
	}

	// Apply custom header overrides on top.
	if cb.ua != "" || len(cb.headers) > 0 {
		rt = &customHeaderRoundTripper{
			inner:   rt,
			ua:      cb.ua,
			headers: cb.headers,
		}
	}

	return &http.Client{Transport: rt, Timeout: cb.timeout}
}

// browserHeadersRoundTripper injects browser-specific default headers
// (Sec-CH-UA, Accept-Language, etc.) without overriding existing values.
type browserHeadersRoundTripper struct {
	inner   http.RoundTripper
	headers map[string]string
}

func (b *browserHeadersRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range b.headers {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
	return b.inner.RoundTrip(req)
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

// ImpersonateRequest creates a fluent Request builder with browser TLS + HTTP headers.
// Supports SetSuccessResult/SetErrorResult auto-unmarshal, retry, and dump.
//
//	var user User
//	resp, err := tlsgateway.ImpersonateRequest(profiles.Chrome_150).
//	    SetSuccessResult(&user).
//	    SetErrorResult(&apiErr).
//	    SetRetry(3, tlsgateway.RetryOnServerError, 1*time.Second, 10*time.Second).
//	    SetDump(tlsgateway.DefaultDumpOptions()).
//	    Get("https://api.example.com/user")
func ImpersonateRequest(profile profiles.ClientProfile) *Request {
	client := Impersonate(profile)
	return &Request{client: client}
}

// DevMode creates an impersonated Client with full debug dump enabled.
// Equivalent to req's req.DevMode().ImpersonateChrome().
func DevMode(profile profiles.ClientProfile) *http.Client {
	return ImpersonateChain(profile).
		SetDebug(os.Stderr).
		WithOrderedHeaders().
		Build()
}
