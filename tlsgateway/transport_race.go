package tlsgateway

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/bogdanfinn/tls-client/profiles"
)

// RaceTransport wraps a base Transport with H2-vs-H1 protocol racing.
// On first contact with a target, it races H2 against H1.1 and returns
// whichever responds first. The slower connection stays in the pool for
// future requests.
//
// Similar to Chrome's "Happy Eyeballs" at the protocol level.
// Zero fork dependencies.
type RaceTransport struct {
	base     *Transport
	raceOpts RaceOptions
}

// RaceOptions configures protocol racing behavior.
type RaceOptions struct {
	// H2Delay is how long to wait for H2 before starting H1.1.
	// Chrome uses 300ms. Set to 0 to race both immediately.
	// Default: 300ms.
	H2Delay time.Duration

	// Timeout is the maximum time to wait for either connection.
	// Default: 10s.
	Timeout time.Duration
}

// DefaultRaceOptions returns sensible defaults.
func DefaultRaceOptions() RaceOptions {
	return RaceOptions{
		H2Delay: 300 * time.Millisecond,
		Timeout: 10 * time.Second,
	}
}

var _ http.RoundTripper = (*RaceTransport)(nil)

// NewRaceTransport creates a Transport that races H2 against H1.1.
func NewRaceTransport(profile profiles.ClientProfile, raceOpts RaceOptions) *RaceTransport {
	return &RaceTransport{
		base:     NewTransport(profile),
		raceOpts: raceOpts,
	}
}

// NewRaceTransportWithOptions is like NewRaceTransport but with TransportOptions.
func NewRaceTransportWithOptions(profile profiles.ClientProfile, opts TransportOptions, raceOpts RaceOptions) *RaceTransport {
	return &RaceTransport{
		base:     NewTransportWithOptions(profile, opts),
		raceOpts: raceOpts,
	}
}

// SetProfile updates the fingerprint on the base transport.
func (rt *RaceTransport) SetProfile(profile profiles.ClientProfile) {
	rt.base.SetProfile(profile)
}

// RoundTrip implements http.RoundTripper.
//
// Race strategy:
//  1. Start H2 request immediately.
//  2. After H2Delay, start H1.1 request in parallel.
//  3. Return the first successful response.
//  4. Cancel the slower request; its connection stays in the pool for reuse.
//
// Only races GET/HEAD/OPTIONS — mutating methods must not be sent twice.
func (rt *RaceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" || !isRaceEligibleMethod(req.Method) {
		return rt.base.RoundTrip(req)
	}

	ctx, cancel := context.WithTimeout(req.Context(), rt.raceOpts.Timeout)
	defer cancel()
	req = req.WithContext(ctx)

	type raceResult struct {
		resp *http.Response
		err  error
	}

	h2Ch := make(chan raceResult, 1)
	h1Ch := make(chan raceResult, 1)

	// Start H2 immediately.
	go func() {
		resp, err := rt.base.h2.RoundTrip(req)
		h2Ch <- raceResult{resp, err}
	}()

	// Start H1 after the configured delay.
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(rt.raceOpts.H2Delay):
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		resp, err := rt.base.h1.RoundTrip(req)
		h1Ch <- raceResult{resp, err}
	}()

	// Wait for the first success.
	var h2Done, h1Done bool
	for !h2Done || !h1Done {
		select {
		case r := <-h2Ch:
			h2Done = true
			if r.err == nil {
				go func() { <-h1Ch }()
				return r.resp, nil
			}
		case r := <-h1Ch:
			h1Done = true
			if r.err == nil {
				go func() { <-h2Ch }()
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

func isRaceEligibleMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// CloseIdleConnections closes idle connections.
func (rt *RaceTransport) CloseIdleConnections() {
	if rt.base != nil {
		rt.base.CloseIdleConnections()
	}
}

// DialTLS creates a raw uTLS connection (delegates to base).
func (rt *RaceTransport) DialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	return rt.base.DialTLS(ctx, network, addr)
}
