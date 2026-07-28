//go:build h3

package tlsgateway

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/quic-go-utls/http3"
	utls "github.com/bogdanfinn/utls"

	"github.com/bogdanfinn/tls-client/profiles"
)

// H3Transport adds HTTP/3 (QUIC) support.
//
// Activated with: go build -tags h3
//
// NOTE: bogdanfinn/quic-go-utls is deeply coupled to bogdanfinn/fhttp —
// the http3.Transport works with fhttp.Request/fhttp.Response types,
// not net/http. The H3Transport bridges this by converting between
// the two type systems at the boundary.
//
// This is the ONE remaining fork dependency in the tlsgateway layered
// architecture, because QUIC bakes TLS 1.3 into its protocol — there
// is no way to inject uTLS at the connection level like we do for TCP.
type H3Transport struct {
	h3 *http3.Transport
	h2 *Transport

	profileMu sync.RWMutex
	profile   profiles.ClientProfile

	preferH3     bool
	enableRacing bool
	raceDelay    time.Duration
}

// H3Options configures HTTP/3 behavior.
type H3Options struct {
	InsecureSkipVerify bool
	PreferH3           bool
	EnableRacing       bool
	RaceDelay          time.Duration
}

var _ http.RoundTripper = (*H3Transport)(nil)

// NewH3Transport creates a Transport with HTTP/3 support.
// H2 + H1.1 are always available as fallback.
func NewH3Transport(profile profiles.ClientProfile, opts H3Options) *H3Transport {
	t := &H3Transport{
		profile:      profile,
		preferH3:     opts.PreferH3,
		enableRacing: opts.EnableRacing,
		raceDelay:    opts.RaceDelay,
		h2:           NewTransport(profile),
	}
	if t.raceDelay == 0 {
		t.raceDelay = 300 * time.Millisecond
	}

	// quic-go-utls uses uTLS internally for QUIC TLS handshakes.
	// Pass a utls.Config (not crypto/tls.Config) — the fork expects it.
	utlsCfg := &utls.Config{
		InsecureSkipVerify: opts.InsecureSkipVerify,
		OmitEmptyPsk:       true,
		ClientSessionCache: utls.NewLRUClientSessionCache(32),
	}

	settings := profile.GetHttp3Settings()
	settingsOrder := profile.GetHttp3SettingsOrder()

	t.h3 = &http3.Transport{
		TLSClientConfig:      utlsCfg,
		EnableDatagrams:      true,
		AdditionalSettings:   settings,
	}

	if len(settingsOrder) > 0 {
		t.h3.AdditionalSettingsOrder = settingsOrder
	}

	return t
}

// RoundTrip implements http.RoundTripper.
func (t *H3Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return t.h2.RoundTrip(req)
	}

	if t.enableRacing && isRaceEligibleMethod(req.Method) {
		return t.raceH3H2(req)
	}

	if t.preferH3 {
		resp, err := t.roundTripH3(req)
		if err == nil {
			return resp, nil
		}
	}

	return t.h2.RoundTrip(req)
}

// roundTripH3 converts net/http.Request → fhttp.Request → fhttp.Response → net/http.Response.
func (t *H3Transport) roundTripH3(req *http.Request) (*http.Response, error) {
	fReq, err := fhttp.NewRequest(req.Method, req.URL.String(), req.Body)
	if err != nil {
		return nil, err
	}
	fReq.Header = make(fhttp.Header, len(req.Header))
	for k, vs := range req.Header {
		for _, v := range vs {
			fReq.Header.Add(k, v)
		}
	}
	fReq = fReq.WithContext(req.Context())

	fResp, err := t.h3.RoundTrip(fReq)
	if err != nil {
		return nil, err
	}

	// Convert fhttp.Response back to net/http.Response.
	resp := &http.Response{
		Status:     fResp.Status,
		StatusCode: fResp.StatusCode,
		Proto:      fResp.Proto,
		ProtoMajor: fResp.ProtoMajor,
		ProtoMinor: fResp.ProtoMinor,
		Header:     make(http.Header),
		Body:       fResp.Body,
		Request:    req,
	}
	for k, vs := range fResp.Header {
		for _, v := range vs {
			resp.Header.Add(k, v)
		}
	}
	return resp, nil
}

func (t *H3Transport) raceH3H2(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	type result struct {
		resp *http.Response
		err  error
	}

	h3Ch := make(chan result, 1)
	h2Ch := make(chan result, 1)

	go func() {
		resp, err := t.roundTripH3(req)
		h3Ch <- result{resp, err}
	}()

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(t.raceDelay):
		}
		resp, err := t.h2.RoundTrip(req)
		h2Ch <- result{resp, err}
	}()

	var h3Done, h2Done bool
	for !h3Done || !h2Done {
		select {
		case r := <-h3Ch:
			h3Done = true
			if r.err == nil {
				go func() { <-h2Ch }()
				return r.resp, nil
			}
		case r := <-h2Ch:
			h2Done = true
			if r.err == nil {
				go func() { <-h3Ch }()
				return r.resp, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	select {
	case r := <-h2Ch:
		return nil, r.err
	default:
		return nil, ctx.Err()
	}
}

// SetProfile updates the TLS fingerprint profile.
func (t *H3Transport) SetProfile(profile profiles.ClientProfile) {
	t.h2.SetProfile(profile)
	t.profileMu.Lock()
	t.profile = profile
	t.profileMu.Unlock()
}

// CloseIdleConnections closes idle connections.
func (t *H3Transport) CloseIdleConnections() {
	if t.h3 != nil {
		t.h3.Close()
	}
	if t.h2 != nil {
		t.h2.CloseIdleConnections()
	}
}

// DialTLS creates a raw uTLS connection (delegates to H2 base).
func (t *H3Transport) DialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.h2.DialTLS(ctx, network, addr)
}
