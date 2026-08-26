package http3

// Magic header keys for controlling request header order (compat with
// bogdanfinn/fhttp, which this fork replaces with standard net/http).
// The H3 request writer reads these keys and strips them from the wire.
const (
	// HeaderOrderKey is a magic Key for setting the order of request headers.
	HeaderOrderKey = "Header-Order:"
	// PHeaderOrderKey is a magic Key for setting http2 pseudo header order.
	PHeaderOrderKey = "PHeader-Order:"
)
