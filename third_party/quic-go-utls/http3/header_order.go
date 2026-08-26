package http3

// Magic header keys for controlling request header order. This fork
// uses standard net/http instead of a forked HTTP library.
// The H3 request writer reads these keys and strips them from the wire.
const (
	// HeaderOrderKey is a magic Key for setting the order of request headers.
	HeaderOrderKey = "Header-Order:"
	// PHeaderOrderKey is a magic Key for setting http2 pseudo header order.
	PHeaderOrderKey = "PHeader-Order:"
)
