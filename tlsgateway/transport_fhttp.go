//go:build fhttp

package tlsgateway

import (
	"fmt"
	"net"
	"net/http"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	fhttp2 "github.com/bogdanfinn/fhttp/http2"
	utls "github.com/bogdanfinn/utls"

	"github.com/bogdanfinn/tls-client/profiles"
)

// FhttpTransport provides Akamai-level HTTP/2 fingerprint customization.
//
// Activated with: go build -tags fhttp
//
// Uses bogdanfinn/fhttp — a fork of net/http that replaces crypto/tls with
// uTLS and exposes full H2 frame control (SETTINGS, stream IDs, pseudo-header
// order, PRIORITY frames). Required when the target fingerprints H2 handshakes
// beyond the TLS ClientHello (Akamai).
type FhttpTransport struct {
	transport *fhttp2.Transport
}

// FhttpOptions configures the fhttp transport.
type FhttpOptions struct {
	InsecureSkipVerify   bool
	ServerNameOverwrite  string
	RandomExtensionOrder bool
}

// NewFhttpTransport creates a transport with browser-identical H2 frames.
func NewFhttpTransport(profile profiles.ClientProfile, opts FhttpOptions) *FhttpTransport {
	utlsCfg := &utls.Config{
		InsecureSkipVerify: opts.InsecureSkipVerify,
		OmitEmptyPsk:       true,
		ClientSessionCache: utls.NewLRUClientSessionCache(32),
	}

	h2Transport := &fhttp2.Transport{
		DialTLS: func(network, addr string, _ *utls.Config) (net.Conn, error) {
			return dialTLSFhttp(network, addr, profile, utlsCfg, opts.RandomExtensionOrder, opts.ServerNameOverwrite)
		},
		TLSClientConfig: utlsCfg,
		IdleConnTimeout: 90 * time.Second,
	}

	// Profile H2 frame customization.
	if s := profile.GetSettings(); len(s) > 0 {
		h2Transport.Settings = make(map[fhttp2.SettingID]uint32, len(s)+2)
		for k, v := range s {
			h2Transport.Settings[fhttp2.SettingID(k)] = v
		}
	} else {
		h2Transport.Settings = make(map[fhttp2.SettingID]uint32, 6)
	}

	// Chrome always sends these SETTINGS. Add if profile doesn't include them.
	chromeDefaults := map[fhttp2.SettingID]uint32{
		3: 1000,   // SETTINGS_MAX_CONCURRENT_STREAMS
		5: 16384,  // SETTINGS_MAX_FRAME_SIZE
	}
	for id, val := range chromeDefaults {
		if _, ok := h2Transport.Settings[id]; !ok {
			h2Transport.Settings[id] = val
		}
	}

	if o := profile.GetSettingsOrder(); len(o) > 0 {
		h2Transport.SettingsOrder = make([]fhttp2.SettingID, 0, len(o)+2)
		for _, id := range o {
			h2Transport.SettingsOrder = append(h2Transport.SettingsOrder, fhttp2.SettingID(id))
		}
		// Insert missing Chrome SETTINGS at their correct positions.
		// Chrome order: 1, 2, 3, 4, 5, 6
		h2Transport.SettingsOrder = insertMissing(h2Transport.SettingsOrder, fhttp2.SettingID(3), 2) // after #2
		h2Transport.SettingsOrder = insertMissing(h2Transport.SettingsOrder, fhttp2.SettingID(5), 4) // after #4
	} else {
		h2Transport.SettingsOrder = []fhttp2.SettingID{1, 2, 3, 4, 5, 6}
	}

	h2Transport.InitialStreamID = profile.GetStreamID()
	if h2Transport.InitialStreamID == 0 {
		// Chrome uses 3, Firefox uses 1. Default to 3 (Chrome family
		// = majority of profiles). Go's default (1) is detected by Akamai.
		h2Transport.InitialStreamID = 3
	}
	h2Transport.ConnectionFlow = profile.GetConnectionFlow()

	if hp := profile.GetHeaderPriority(); hp != nil {
		h2Transport.HeaderPriority = &fhttp2.PriorityParam{
			StreamDep: hp.StreamDep,
			Exclusive: hp.Exclusive,
			Weight:    hp.Weight,
		}
	}

	h2Transport.PseudoHeaderOrder = profile.GetPseudoHeaderOrder()

	if prios := profile.GetPriorities(); len(prios) > 0 {
		h2Transport.Priorities = make([]fhttp2.Priority, len(prios))
		for i, p := range prios {
			h2Transport.Priorities[i] = fhttp2.Priority{
				StreamID: p.StreamID,
				PriorityParam: fhttp2.PriorityParam{
					StreamDep: p.PriorityParam.StreamDep,
					Exclusive: p.PriorityParam.Exclusive,
					Weight:    p.PriorityParam.Weight,
				},
			}
		}
	}

	return &FhttpTransport{transport: h2Transport}
}

// RoundTrip implements http.RoundTripper.
func (ft *FhttpTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	freq, err := fhttp.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), req.Body)
	if err != nil {
		return nil, fmt.Errorf("fhttp: %w", err)
	}
	freq.Header = fhttp.Header(req.Header)

	fresp, err := ft.transport.RoundTrip(freq)
	if err != nil {
		return nil, err
	}

	return &http.Response{
		Status:        fresp.Status,
		StatusCode:    fresp.StatusCode,
		Proto:         fresp.Proto,
		ProtoMajor:    fresp.ProtoMajor,
		ProtoMinor:    fresp.ProtoMinor,
		Header:        http.Header(fresp.Header),
		Body:          fresp.Body,
		ContentLength: fresp.ContentLength,
		Request:       req,
	}, nil
}

// CloseIdleConnections closes idle connections.
func (ft *FhttpTransport) CloseIdleConnections() {
	if ft.transport != nil {
		ft.transport.CloseIdleConnections()
	}
}

// insertMissing inserts id into ids at position insertAfter if not already present.
func insertMissing(ids []fhttp2.SettingID, id fhttp2.SettingID, insertAfter fhttp2.SettingID) []fhttp2.SettingID {
	for _, existing := range ids {
		if existing == id {
			return ids // already present
		}
	}
	result := make([]fhttp2.SettingID, 0, len(ids)+1)
	for _, existing := range ids {
		result = append(result, existing)
		if existing == insertAfter {
			result = append(result, id)
		}
	}
	return result
}

func dialTLSFhttp(network, addr string, profile profiles.ClientProfile, cfg *utls.Config, randomOrder bool, sniOverride string) (net.Conn, error) {
	dialer := &net.Dialer{}
	rawConn, err := dialer.Dial(network, addr)
	if err != nil {
		return nil, fmt.Errorf("tlsgateway: fhttp dial: %w", err)
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("tlsgateway: fhttp split: %w", err)
	}
	if sniOverride != "" {
		host = sniOverride
	}
	cfg.ServerName = host

	uconn := utls.UClient(rawConn, cfg, profile.GetClientHelloId(), randomOrder, false, false)
	if err := uconn.Handshake(); err != nil {
		uconn.Close()
		return nil, fmt.Errorf("tlsgateway: fhttp handshake: %w", err)
	}
	return uconn, nil
}
