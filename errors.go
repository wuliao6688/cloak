package tls_client

import "errors"

// Sentinel errors for the tls-client package.
// Callers can use errors.Is / errors.As to check for specific failure modes.

var (
	// ErrRequestNil is returned when a nil *http.Request is passed to Do().
	ErrRequestNil = errors.New("request must not be nil")

	// ErrRacingNotSupported is returned when HTTP/3 racing is configured with
	// incompatible options (proxy, custom dialer, cert pinning, bandwidth tracking, etc.).
	ErrRacingNotSupported = errors.New("HTTP/3 racing is not supported with the current configuration")

	// ErrRacingBothProtocolsFailed is returned when both HTTP/3 and HTTP/2 attempts fail during racing.
	ErrRacingBothProtocolsFailed = errors.New("http3 racing: both protocols failed to connect")

	// ErrRequestBodyNotReplayable is returned when protocol racing requires GetBody
	// but the request's GetBody is nil.
	ErrRequestBodyNotReplayable = errors.New("http3 racing requires a replayable request body")

	// ErrBodyCloneFailed is returned when cloning the request body for protocol racing fails.
	ErrBodyCloneFailed = errors.New("failed to clone request body for protocol racing")

	// ErrTransportInitFailed is returned when transport initialization completes
	// without producing a usable transport.
	ErrTransportInitFailed = errors.New("transport initialization completed without a transport")

	// ErrProxySchemeNotSupported is returned when an unsupported proxy scheme is used.
	ErrProxySchemeNotSupported = errors.New("proxy scheme is not supported")

	// ErrInvalidProxyURL is returned when the proxy URL is malformed.
	ErrInvalidProxyURL = errors.New("invalid proxy URL")
)
