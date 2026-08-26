package cloak

import "net/http"

// ─── TransportMiddleware ─────────────────────────────────────────────────
//
// req-style middleware: chain http.RoundTripper wrappers that can
// hook into both request and response. Unlike RequestMiddleware/
// ResponseMiddleware which only get pre/post hooks, TransportMiddleware
// wraps the full RoundTrip call and can measure, trace, or transform
// the entire request-response cycle.
//
// Usage:
//
//	tr := cloak.NewTransport(p)
//	tr.Wrap(func(rt http.RoundTripper) http.RoundTripper {
//	    return RoundTripFunc(func(req *http.Request) (*http.Response, error) {
//	        // before request
//	        resp, err := rt.RoundTrip(req)
//	        // after response
//	        return resp, err
//	    })
//	})

// RoundTripFunc is an adapter to allow ordinary functions to
// satisfy http.RoundTripper.
type RoundTripFunc func(*http.Request) (*http.Response, error)

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Middleware is a function that wraps a RoundTripper.
type Middleware func(http.RoundTripper) http.RoundTripper

// MiddlewareChain holds a chain of middleware with a base RoundTripper.
type MiddlewareChain struct {
	base        http.RoundTripper
	middlewares []Middleware
	cached      http.RoundTripper // built once
	dirty       bool
}

// NewMiddlewareChain creates a middleware chain wrapping base.
func NewMiddlewareChain(base http.RoundTripper) *MiddlewareChain {
	return &MiddlewareChain{base: base, dirty: true}
}

// Use appends a middleware to the chain. Middleware is applied
// outermost first (last added = innermost).
func (mc *MiddlewareChain) Use(mw Middleware) *MiddlewareChain {
	mc.middlewares = append(mc.middlewares, mw)
	mc.dirty = true
	return mc
}

// Build constructs the final RoundTripper with all middleware applied.
func (mc *MiddlewareChain) Build() http.RoundTripper {
	if !mc.dirty && mc.cached != nil {
		return mc.cached
	}
	result := mc.base
	// Apply in reverse: last middleware wraps around first.
	for i := len(mc.middlewares) - 1; i >= 0; i-- {
		result = mc.middlewares[i](result)
	}
	mc.cached = result
	mc.dirty = false
	return result
}

// RoundTrip implements http.RoundTripper by delegating to the
// built middleware chain.
func (mc *MiddlewareChain) RoundTrip(req *http.Request) (*http.Response, error) {
	return mc.Build().RoundTrip(req)
}

// Wrap is a convenience method for Transport that applies a single
// middleware to the Transport itself, returning a new RoundTripper.
//
//	tr := cloak.NewTransport(p)
//	wrapped := tr.Wrap(tracingMiddleware, metricsMiddleware)
//	client := &http.Client{Transport: wrapped}
func (t *Transport) Wrap(middlewares ...Middleware) http.RoundTripper {
	chain := NewMiddlewareChain(t)
	for _, mw := range middlewares {
		chain.Use(mw)
	}
	return chain.Build()
}

// ─── Pre-built Middleware ────────────────────────────────────────────────

// DebugMiddleware logs request URLs and response status codes.
func DebugMiddleware(logf func(format string, args ...interface{})) Middleware {
	return func(rt http.RoundTripper) http.RoundTripper {
		return RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			logf("→ %s %s", req.Method, req.URL.String()[:min(80, len(req.URL.String()))])
			resp, err := rt.RoundTrip(req)
			if err != nil {
				logf("← %s %s ERROR: %v", req.Method, req.URL.Host, err)
			} else {
				logf("← %d %s", resp.StatusCode, req.URL.Host)
			}
			return resp, err
		})
	}
}

// UserAgentMiddleware forces a User-Agent header on every request.
func UserAgentMiddleware(ua string) Middleware {
	return func(rt http.RoundTripper) http.RoundTripper {
		return RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("User-Agent") == "" {
				req.Header.Set("User-Agent", ua)
			}
			return rt.RoundTrip(req)
		})
	}
}
