//go:build fhttp

package tlsgateway

import (
	"context"
	"net"
	"net/http"

	"github.com/bogdanfinn/fhttp/http2"

	"github.com/bogdanfinn/tls-client/profiles"
)

// FhttpTransport provides Akamai-level HTTP/2 fingerprint customization.
//
// Activated with: go build -tags fhttp
//
// The bogdanfinn/fhttp library forks net/http and x/net/http2 to give
// full control over H2 SETTINGS frames, initial stream IDs, pseudo-header
// order, and PRIORITY frames. This is only needed when the target server
// fingerprints HTTP/2 handshakes beyond the TLS ClientHello (Akamai, DataDome).
//
// The default Transport (zero fork) is sufficient for 90%+ of use cases.
type FhttpTransport struct {
	transport       *Transport
	h2Settings      map[http2.SettingID]uint32
	h2SettingsOrder []http2.SettingID
	streamID        uint32
}

// FhttpOptions configures the fhttp-based transport.
type FhttpOptions struct {
	InsecureSkipVerify bool
}

// NewFhttpTransport creates an Akamai-grade transport.
//
// TODO: Full fhttp integration — wire the fhttp round-tripper with
// per-request H2 SETTINGS, header priority, and pseudo-header order.
// Currently delegates to the default Transport while making the
// fhttp-customized settings available for inspection.
func NewFhttpTransport(profile profiles.ClientProfile, opts FhttpOptions) *FhttpTransport {
	settings := profile.GetSettings()
	settingsOrder := profile.GetSettingsOrder()

	h2Settings := make(map[http2.SettingID]uint32, len(settings))
	for k, v := range settings {
		h2Settings[http2.SettingID(k)] = v
	}

	h2Order := make([]http2.SettingID, len(settingsOrder))
	for i, id := range settingsOrder {
		h2Order[i] = http2.SettingID(id)
	}

	streamID := profile.GetStreamID()

	tr := NewTransport(profile)
	if opts.InsecureSkipVerify {
		tr.setInsecureSkipVerify(true)
	}

	return &FhttpTransport{
		transport:       tr,
		h2Settings:      h2Settings,
		h2SettingsOrder: h2Order,
		streamID:        streamID,
	}
}

// RoundTrip implements http.RoundTripper.
func (ft *FhttpTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return ft.transport.RoundTrip(req)
}

// H2Settings returns the fhttp-customized H2 SETTINGS for inspection.
func (ft *FhttpTransport) H2Settings() map[http2.SettingID]uint32 {
	return ft.h2Settings
}

// CloseIdleConnections closes idle connections in all transports.
func (ft *FhttpTransport) CloseIdleConnections() {
	if ft.transport != nil {
		ft.transport.CloseIdleConnections()
	}
}

// DialTLS creates a raw uTLS connection.
func (ft *FhttpTransport) DialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	return ft.transport.DialTLS(ctx, network, addr)
}
