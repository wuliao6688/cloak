package cloak

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/wuliao6688/quic-go-utls"
	"github.com/wuliao6688/quic-go-utls/http3"
	utls "github.com/wuliao6688/utls"

	"github.com/wuliao6688/cloak/profiles"
)

// H3Transport is an http.RoundTripper that speaks HTTP/3 (QUIC) with a
// browser-identical fingerprint set:
//
//   - QUIC TLS 1.3 handshake: uTLS ClientHelloID injected via
//     utls.UQUICClient (browser TLS fingerprint over QUIC)
//   - HTTP/3 SETTINGS: value + order + trailing GREASE from the profile
//   - Priority Param (e.g. 984832 = 0x0F0700 for Chrome)
//   - Pseudo header order (:method, :authority, :scheme, :path)
//   - GREASE frames (Chrome sends them on the control stream)
//
// This goes beyond most other QUIC clients: they use
// tls.QUICClient (Go default ClientHello) for QUIC — we inject the real
// browser fingerprint via UQUICClient.
//
// Depends on the local fork third_party/quic-go-utls (see go.mod replace),
// which adds quic.Config.ClientHelloID support.
type H3Transport struct {
	profile profiles.ClientProfile

	quicConfig *quic.Config
	tlsConfig  *utls.Config

	// http3.Transport per host (like upstream's cachedTransports).
	mu     sync.Mutex
	t3     map[string]http.RoundTripper

	// Debug (optional).
	debugWriter io.Writer
	debugLog   *log.Logger

	// browserHeaders are injected into every request RoundTrip
	// (same as Transport).
	browserHeaders map[string]string
}

// NewH3Transport creates an HTTP/3 transport from a client profile.
// The profile's ClientHelloID is injected into the QUIC TLS handshake,
// and its Http3* fields drive SETTINGS/Priority/pseudo-header/GREASE.
func NewH3Transport(profile profiles.ClientProfile) *H3Transport {
	return NewH3TransportWithOptions(profile, TransportOptions{})
}

// NewH3TransportWithOptions creates an H3 transport with options.
func NewH3TransportWithOptions(profile profiles.ClientProfile, opts TransportOptions) *H3Transport {
	t := &H3Transport{
		profile:     profile,
		t3:          make(map[string]http.RoundTripper),
		debugLog:    log.New(io.Discard, "", 0),
	}

	if fp := BrowserFingerprint(profile.GetClientHelloStr()); fp != nil {
		t.browserHeaders = fp.Headers
	}

	t.quicConfig = &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		EnableDatagrams: true, // Chrome enables H3_DATAGRAM (setting 0x33)
		// ⭐ KEY: inject the browser TLS fingerprint into QUIC handshake.
		ClientHelloID: profile.GetClientHelloId(),
	}

	t.tlsConfig = &utls.Config{
		InsecureSkipVerify: opts.InsecureSkipVerify,
	}
	if opts.ServerNameOverwrite != "" {
		t.tlsConfig.ServerName = opts.ServerNameOverwrite
	}
	return t
}

// SetDebug enables debug logging.
func (t *H3Transport) SetDebug(w io.Writer) {
	t.debugLog = log.New(w, "[h3] ", 0)
}

// SetProfile replaces the fingerprint profile (clears cached transports).
func (t *H3Transport) SetProfile(profile profiles.ClientProfile) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.profile = profile
	t.quicConfig.ClientHelloID = profile.GetClientHelloId()
	t.t3 = make(map[string]http.RoundTripper)
	if fp := BrowserFingerprint(profile.GetClientHelloStr()); fp != nil {
		t.browserHeaders = fp.Headers
	}
}

// CloseIdleConnections closes cached H3 transports.
func (t *H3Transport) CloseIdleConnections() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, rt := range t.t3 {
		if closer, ok := rt.(io.Closer); ok {
			_ = closer.Close()
		}
	}
	t.t3 = make(map[string]http.RoundTripper)
}

// buildHTTP3 creates the per-host http3.Transport from the profile's H3 fields.
func (t *H3Transport) buildHTTP3(host string) http.RoundTripper {
	profile := t.profile

	// H3 SETTINGS (values + order), appending GREASE like Chrome.
	settings := profile.GetHttp3Settings()
	settingsOrder := profile.GetHttp3SettingsOrder()

	// Chrome sends a random GREASE setting at the END of its SETTINGS.
	// Upstream identifies "Chrome-like" by priorityParam > 0.
	if profile.GetHttp3PriorityParam() > 0 {
		greaseID := generateGREASESettingID()
		greaseValue := generateGREASESettingValue()
		if settings == nil {
			settings = make(map[uint64]uint64)
		}
		settings[greaseID] = greaseValue
		if len(settingsOrder) > 0 {
			orderWithGrease := make([]uint64, len(settingsOrder)+1)
			copy(orderWithGrease, settingsOrder)
			orderWithGrease[len(settingsOrder)] = greaseID
			settingsOrder = orderWithGrease
		}
	}

	t3 := &http3.Transport{
		TLSClientConfig:      t.tlsConfig.Clone(),
		QUICConfig:           t.quicConfig.Clone(),
		EnableDatagrams:      true,
		AdditionalSettings:   settings,
		AdditionalSettingsOrder: settingsOrder,
		SendGreaseFrames:     profile.GetHttp3SendGreaseFrames(),
		MaxResponseHeaderBytes: profileDefaultMaxResponseHeaderBytes(profile),
	}
	// Priority Param (Chrome: 984832 = urgency 3, incremental 0, reprioritize 7)
	if pp := profile.GetHttp3PriorityParam(); pp > 0 {
		t3.PriorityParam = pp
	}
	// Pseudo header order (Chrome: :method, :authority, :scheme, :path)
	if order := profile.GetHttp3PseudoHeaderOrder(); len(order) > 0 {
		t3.PseudoHeaderOrder = order
	}

	return t3
}

// RoundTrip implements http.RoundTripper.
func (t *H3Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("h3: unsupported scheme %q (only https)", req.URL.Scheme)
	}

	// Inject browser headers (same as Transport).
	for k, v := range t.browserHeaders {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}

	host := hostPort(req)

	t.mu.Lock()
	rt, ok := t.t3[host]
	if !ok {
		rt = t.buildHTTP3(host)
		t.t3[host] = rt
	}
	t.mu.Unlock()

	t.debugf("%s → H3 (QUIC, TLS fingerprint %s)", host, t.profile.GetClientHelloStr())
	return rt.RoundTrip(req)
}

func (t *H3Transport) debugf(format string, args ...interface{}) {
	t.debugLog.Printf(format, args...)
}

// profileDefaultMaxResponseHeaderBytes mirrors upstream: Chrome sends
// SETTINGS_MAX_FIELD_SECTION_SIZE = 262144; Firefox doesn't (use -1).
func profileDefaultMaxResponseHeaderBytes(profile profiles.ClientProfile) int {
	if profile.GetHttp3PriorityParam() > 0 {
		return 262144 // CHROME_MAX_FIELD_SECTION_SIZE
	}
	return -1
}

// ---------- GREASE helpers (same values as upstream/curl) ----------

// generateGREASESettingID returns a random GREASE setting identifier.
// GREASE values are 0x?a?a patterns (e.g. 0x0a0a, 0x1a1a...).
func generateGREASESettingID() uint64 {
	return uint64(0x0a0a + uint64(time.Now().UnixNano()%15)*0x1010)
}

// generateGREASESettingValue returns a random GREASE value (0x?a?a pattern).
func generateGREASESettingValue() uint64 {
	return uint64(0x0a0a + uint64(time.Now().UnixNano()/1000%15)*0x1010)
}

// ---------- Dial helpers (for future proxy support) ----------

// dialUDP is a placeholder for UDP dialing — currently the http3.Transport
// creates its own UDP socket. Exposed for SOCKS5 UDP-associate (phase 2).
func dialUDP(ctx context.Context, network, addr string) (net.PacketConn, error) {
	return net.ListenPacket("udp", "")
}

var _ http.RoundTripper = (*H3Transport)(nil)
var _ = url.URL{}
