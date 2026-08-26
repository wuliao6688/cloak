package cloak

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/wuliao6688/cloak/profiles"
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
	H2Delay time.Duration // wait before starting H1.1 (default: 300ms)
	Timeout time.Duration // max wait for either (default: 10s)
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
//  1. Start H2 immediately.
//  2. After H2Delay, start H1.1 in parallel.
//  3. Return the first successful response.
//  4. The slower goroutine sends to a buffered channel and exits
//     gracefully — no drain goroutines, no leaks.
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

	// Start H2 immediately. Always sends to h2Ch — even on error.
	go func() {
		resp, err := rt.base.h2.RoundTrip(req)
		// Non-blocking send: if the channel is full (we already returned),
		// skip — the result is not needed.
		select {
		case h2Ch <- raceResult{resp, err}:
		default:
			// Already won by H1, discard.
			if resp != nil {
				resp.Body.Close()
			}
		}
	}()

	// Start H1 after the configured delay.
	go func() {
		select {
		case <-ctx.Done():
			// Context cancelled — send error so the select below can proceed.
			select {
			case h1Ch <- raceResult{err: ctx.Err()}:
			default:
			}
			return
		case <-time.After(rt.raceOpts.H2Delay):
		}
		resp, err := rt.base.h1.RoundTrip(req)
		select {
		case h1Ch <- raceResult{resp, err}:
		default:
			if resp != nil {
				resp.Body.Close()
			}
		}
	}()

	// Wait for the first success, or both to complete.
	var h2Done, h1Done bool
	for !h2Done || !h1Done {
		select {
		case r := <-h2Ch:
			h2Done = true
			if r.err == nil {
				return r.resp, nil
			}
		case r := <-h1Ch:
			h1Done = true
			if r.err == nil {
				return r.resp, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Both failed — return H2 error (more informative).
	select {
	case r := <-h2Ch:
		return nil, r.err
	default:
		return nil, ctx.Err()
	}
}

// CloseIdleConnections closes idle connections.
func (rt *RaceTransport) CloseIdleConnections() {
	if rt.base != nil {
		rt.base.CloseIdleConnections()
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

// DialTLS creates a raw uTLS connection (delegates to base).
func (rt *RaceTransport) DialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	return rt.base.DialTLS(ctx, network, addr)
}
