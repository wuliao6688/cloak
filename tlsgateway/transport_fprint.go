package tlsgateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	utls "github.com/bogdanfinn/utls"

	"github.com/bogdanfinn/tls-client/internal/http2"
	"github.com/bogdanfinn/tls-client/profiles"
)

// FingerprintTransport is a full HTTP/2 fingerprint Transport that uses
// the forked internal/http2 package with H2 SETTINGS customization.
//
// It supports H2→H1 fallback for servers that don't speak H2 (same as
// the default Transport).
type FingerprintTransport struct {
	h2  *http2.Transport
	h1  *http.Transport // H1 fallback (forceH1 dial)

	mu      sync.RWMutex
	profile profiles.ClientProfile
	opts    FingerprintTransportOptions

	// h2Disabled tracks hosts that don't support H2.
	h2Disabled sync.Map
	// h2ProbeMu serializes H2 capability probing.
	h2ProbeMu sync.Map
}

// FingerprintTransportOptions configures the FingerprintTransport.
type FingerprintTransportOptions struct {
	InsecureSkipVerify bool
}

// NewFingerprintTransport creates a FingerprintTransport using the given profile.
func NewFingerprintTransport(profile profiles.ClientProfile) *FingerprintTransport {
	return NewFingerprintTransportWithOptions(profile, FingerprintTransportOptions{})
}

// NewFingerprintTransportWithOptions creates a FingerprintTransport with options.
func NewFingerprintTransportWithOptions(profile profiles.ClientProfile, opts FingerprintTransportOptions) *FingerprintTransport {
	ft := &FingerprintTransport{
		profile: profile,
		opts:    opts,
	}
	ft.rebuild()
	return ft
}

func (ft *FingerprintTransport) rebuild() {
	profile := ft.profile

	// Convert profile H2 settings to internal/http2.Settings.
	settings := profile.GetSettings()
	var h2settings []http2.Setting
	if len(settings) > 0 {
		for id, val := range settings {
			h2settings = append(h2settings, http2.Setting{
				ID:  http2.SettingID(id),
				Val: val,
			})
		}
		// Sort by SettingID.
		for i := 1; i < len(h2settings); i++ {
			j := i
			for j > 0 && h2settings[j].ID < h2settings[j-1].ID {
				h2settings[j], h2settings[j-1] = h2settings[j-1], h2settings[j]
				j--
			}
		}
	}

	// Default to Chrome H2 settings if profile doesn't provide enough.
	if len(h2settings) == 0 {
		for _, s := range ChromeH2Settings {
			h2settings = append(h2settings, http2.Setting{
				ID:  http2.SettingID(s.ID),
				Val: s.Val,
			})
		}
	}

	// Ensure Chrome-required settings are present.
	h2settings = ensureSetting(h2settings, 3, 1000) // MaxConcurrentStreams

	streamID := profile.GetStreamID()
	if streamID == 0 {
		streamID = 3 // Chrome default
	}

	ft.h2 = &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			return ft.dialTLS(ctx, network, addr, false)
		},
		Settings:        h2settings,
		InitialStreamID: streamID,
		ConnectionFlow:  profile.GetConnectionFlow(),
		MaxHeaderListSize:          262144,
		StrictMaxConcurrentStreams: false,
		IdleConnTimeout:            90 * time.Second,
		ReadIdleTimeout:            30 * time.Second,
		PingTimeout:                15 * time.Second,
		MaxReadFrameSize:           1 << 20,
		MaxDecoderHeaderTableSize:  4096,
		MaxEncoderHeaderTableSize:  4096,
	}

	// H1 fallback transport with forceH1=true ALPN.
	ft.h1 = &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return ft.dialTLS(ctx, network, addr, true)
		},
		ForceAttemptHTTP2:     false,
		TLSNextProto:          make(map[string]func(string, *tls.Conn) http.RoundTripper),
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func (ft *FingerprintTransport) dialTLS(ctx context.Context, network, addr string, forceH1 bool) (net.Conn, error) {
	profile := ft.profile
	insecure := ft.opts.InsecureSkipVerify

	dialer := &net.Dialer{}
	rawConn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("fprint: dial: %w", err)
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("fprint: split hostport: %w", err)
	}

	utlsConfig := &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: insecure,
		OmitEmptyPsk:       true,
		ClientSessionCache: utls.NewLRUClientSessionCache(32),
	}

	clientHelloID := profile.GetClientHelloId()
	uconn := utls.UClient(rawConn, utlsConfig, clientHelloID, false, forceH1, false)
	if err := uconn.HandshakeContext(ctx); err != nil {
		uconn.Close()
		return nil, fmt.Errorf("fprint: handshake: %w", err)
	}
	return uconn, nil
}

// RoundTrip implements http.RoundTripper with H2→H1 fallback.
func (ft *FingerprintTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return http.DefaultTransport.RoundTrip(req)
	}

	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "443"
	}
	key := host + ":" + port

	// Fast path: host known to lack H2.
	if _, disabled := ft.h2Disabled.Load(key); disabled {
		return ft.h1.RoundTrip(req)
	}

	// Single-flight H2 probe.
	muI, _ := ft.h2ProbeMu.LoadOrStore(key, &sync.Mutex{})
	mu := muI.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// Double-check.
	if _, disabled := ft.h2Disabled.Load(key); disabled {
		return ft.h1.RoundTrip(req)
	}

	// Probe H2 with fingerprint settings.
	resp, err := ft.h2.RoundTrip(req)
	if err == nil {
		return resp, nil
	}

	// Fall back to H1 if the server doesn't speak H2.
	if isProtocolError(err) {
		ft.h2Disabled.Store(key, true)
		return ft.h1.RoundTrip(req)
	}

	return nil, err
}

// SetProfile replaces the profile and rebuilds the transport.
func (ft *FingerprintTransport) SetProfile(profile profiles.ClientProfile) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.profile = profile
	ft.h2.CloseIdleConnections()
	ft.h1.CloseIdleConnections()
	ft.h2Disabled.Clear()
	ft.rebuild()
}

// CloseIdleConnections closes idle connections.
func (ft *FingerprintTransport) CloseIdleConnections() {
	if ft.h2 != nil {
		ft.h2.CloseIdleConnections()
	}
	if ft.h1 != nil {
		ft.h1.CloseIdleConnections()
	}
}

// ensureSetting ensures a setting with the given ID exists.
func ensureSetting(settings []http2.Setting, id uint16, val uint32) []http2.Setting {
	for _, s := range settings {
		if uint16(s.ID) == id {
			return settings
		}
	}
	return append(settings, http2.Setting{ID: http2.SettingID(id), Val: val})
}
