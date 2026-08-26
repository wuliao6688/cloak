package cloak

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/wuliao6688/cloak/profiles"
)

// H3RaceTransport races HTTP/3 (QUIC) against HTTP/2 for each host and
// remembers which protocol won, mirroring Chrome's "Happy Eyeballs" at
// the protocol level (same idea as Chrome's Happy Eyeballs and similar
// but adapted to this repo's standard net/http Transport).
//
// Behavior:
//   - First contact with a host: fire H3 and H2 in parallel, return the
//     first successful response, cache the winner per host.
//   - Subsequent requests use the cached protocol directly (no re-race).
//   - If H3 fails for a host (UDP blocked, server lacks QUIC, etc.) the
//     host is cached as H2 and H1 fallback still applies.
//   - Only idempotent methods (GET/HEAD/OPTIONS) are raced — mutating
//     methods must never be sent twice.
//
// The H3 leg carries the full browser fingerprint: uTLS ClientHelloID
// injected into the QUIC TLS handshake (UQUICClient) plus H3 SETTINGS /
// Priority / pseudo-header order / GREASE from the profile.
type H3RaceTransport struct {
	base *Transport
	h3   *H3Transport

	// protocolCache remembers per-host winners: "h3" or "h2".
	protocolCache sync.Map // host:port → string

	raceOpts RaceOptions
}

// NewH3RaceTransport creates a Transport that races H3 against H2.
func NewH3RaceTransport(profile profiles.ClientProfile) *H3RaceTransport {
	return NewH3RaceTransportWithOptions(profile, TransportOptions{}, DefaultRaceOptions())
}

// NewH3RaceTransportWithOptions creates an H3-racing transport with options.
func NewH3RaceTransportWithOptions(profile profiles.ClientProfile, opts TransportOptions, raceOpts RaceOptions) *H3RaceTransport {
	if raceOpts.Timeout == 0 {
		raceOpts = DefaultRaceOptions()
	}
	return &H3RaceTransport{
		base:     NewTransportWithOptions(profile, opts),
		h3:       NewH3TransportWithOptions(profile, opts),
		raceOpts: raceOpts,
	}
}

var _ http.RoundTripper = (*H3RaceTransport)(nil)

// SetProfile updates the fingerprint on both transports.
func (rt *H3RaceTransport) SetProfile(profile profiles.ClientProfile) {
	rt.base.SetProfile(profile)
	rt.h3.SetProfile(profile)
	rt.protocolCache = sync.Map{}
}

// SetDebug enables debug logging on both transports.
func (rt *H3RaceTransport) SetDebug(w interface{ Write([]byte) (int, error) }) {
	rt.base.SetDebug(w)
	rt.h3.SetDebug(w)
}

// RoundTrip implements http.RoundTripper.
func (rt *H3RaceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" || !isRaceEligibleMethod(req.Method) {
		return rt.base.RoundTrip(req)
	}

	host := hostPort(req)

	// Fast path: protocol already known for this host.
	if cached, ok := rt.protocolCache.Load(host); ok {
		if cached == "h3" {
			return rt.h3.RoundTrip(req)
		}
		return rt.base.RoundTrip(req)
	}

	// Race H3 vs H2 for hosts we haven't classified yet.
	return rt.race(req, host)
}

// race fires H3 and H2 concurrently for an unclassified host and caches
// the winner. Any H3 success wins; if H3 fails while H2 succeeds, the
// host is cached as H2 (H3 unsupported — UDP blocked, no QUIC, etc.).
func (rt *H3RaceTransport) race(req *http.Request, host string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), rt.raceOpts.Timeout)
	defer cancel()
	req = req.WithContext(ctx)

	type raceResult struct {
		resp *http.Response
		err  error
	}
	h3Ch := make(chan raceResult, 1)
	h2Ch := make(chan raceResult, 1)

	go func() {
		// Clone the request: the H2 leg's RoundTrip mutates req.Header
		// (browser header injection) and racing two goroutines on the
		// same request is a data race.
		resp, err := rt.h3.RoundTrip(req.Clone(req.Context()))
		select {
		case h3Ch <- raceResult{resp, err}:
		default:
			if resp != nil {
				resp.Body.Close()
			}
		}
	}()

	go func() {
		select {
		case <-ctx.Done():
			select {
			case h2Ch <- raceResult{err: ctx.Err()}:
			default:
			}
			return
		case <-time.After(rt.raceOpts.H2Delay):
		}
		resp, err := rt.base.RoundTrip(req.Clone(req.Context()))
		select {
		case h2Ch <- raceResult{resp, err}:
		default:
			if resp != nil {
				resp.Body.Close()
			}
		}
	}()

	var h3Done, h2Done bool
	var h2Err error
	for !h3Done || !h2Done {
		select {
		case r := <-h3Ch:
			h3Done = true
			if r.err == nil {
				rt.protocolCache.Store(host, "h3")
				return r.resp, nil
			}
		case r := <-h2Ch:
			h2Done = true
			if r.err == nil {
				// H2 succeeded — but H3 may still be in flight. We
				// optimistically cache H2 only if H3 already failed or
				// hasn't returned yet; if H3 later wins we'll flip it.
				rt.protocolCache.Store(host, "h2")
				// Drain H3 result if already available.
				select {
				case r3 := <-h3Ch:
					h3Done = true
					if r3.err == nil {
						rt.protocolCache.Store(host, "h3")
						return r3.resp, nil
					}
				default:
				}
				return r.resp, nil
			}
			h2Err = r.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Both failed.
	if h2Err != nil {
		return nil, h2Err
	}
	select {
	case r := <-h3Ch:
		return nil, r.err
	default:
		return nil, ctx.Err()
	}
}

// CloseIdleConnections closes idle connections in both transports.
func (rt *H3RaceTransport) CloseIdleConnections() {
	rt.base.CloseIdleConnections()
	rt.h3.CloseIdleConnections()
}

// DialTLS creates a raw uTLS connection (delegates to base).
func (rt *H3RaceTransport) DialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	return rt.base.DialTLS(ctx, network, addr)
}
