package tlsgateway

import (
	"io"
	"context"
	"net/http"
	"strings"
	"time"
)

// Request is a fluent HTTP request builder (req-style).
// Create via ImpersonateRequest(profile).Get(url).
type Request struct {
	client  *http.Client
	method  string
	url     string
	headers map[string]string
	body    io.Reader

	successResult  any
	errorResult    any
	dumpOpts       *DumpOptions
	retryCount     int
	retryCondition RetryConditionFunc
	retryInterval  GetRetryIntervalFunc
}

// SetHeader sets a request header.
func (r *Request) SetHeader(key, value string) *Request {
	if r.headers == nil {
		r.headers = make(map[string]string)
	}
	r.headers[key] = value
	return r
}

// SetHeaders sets multiple request headers.
func (r *Request) SetHeaders(h map[string]string) *Request {
	for k, v := range h {
		r.SetHeader(k, v)
	}
	return r
}

// SetSuccessResult sets the result to auto-unmarshal on 2xx.
func (r *Request) SetSuccessResult(v any) *Request { r.successResult = v; return r }

// SetErrorResult sets the result to auto-unmarshal on non-2xx.
func (r *Request) SetErrorResult(v any) *Request { r.errorResult = v; return r }

// SetDump enables structured dump for this request.
func (r *Request) SetDump(opts *DumpOptions) *Request { r.dumpOpts = opts; return r }

// SetRetry sets retry count, condition, and backoff interval.
func (r *Request) SetRetry(count int, condition RetryConditionFunc, minInterval, maxInterval time.Duration) *Request {
	r.retryCount = count
	r.retryCondition = condition
	r.retryInterval = newBackoffInterval(minInterval, maxInterval)
	return r
}

// Get executes a GET request.
func (r *Request) Get(urlStr string) (*Response, error) {
	r.method = "GET"
	r.url = urlStr
	return r.do()
}

// Post executes a POST request.
func (r *Request) Post(urlStr string) (*Response, error) {
	r.method = "POST"
	r.url = urlStr
	return r.do()
}

func (r *Request) do() (*Response, error) {
	rc, ok := r.client.Transport.(*roundTripperChain)
	_ = ok
	_ = rc

	return r.executeWithRetry()
}

func (r *Request) executeWithRetry() (*Response, error) {
	var lastResp *Response
	var lastErr error

	for attempt := 0; attempt <= r.retryCount; attempt++ {
		if attempt > 0 && r.retryInterval != nil && lastResp != nil {
			time.Sleep(r.retryInterval(lastResp, attempt))
		}

		resp, err := r.execute()
		if err == nil && resp.IsSuccess() {
			return resp, nil
		}
		if r.retryCondition != nil && err == nil {
			if !r.retryCondition(resp, err) {
				return resp, nil
			}
		}
		lastResp = resp
		lastErr = err
	}
	if lastResp != nil {
		return lastResp, lastErr
	}
	return nil, lastErr
}

func (r *Request) execute() (*Response, error) {
	// Build http request.
	req, err := http.NewRequestWithContext(context.Background(), r.method, r.url, r.body)
	if err != nil {
		return nil, err
	}
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}

	// Trace timing.
	ti := newTraceInfo()
	req = req.WithContext(context.WithValue(req.Context(), traceKey{}, ti))

	// Dump request if enabled.
	if r.dumpOpts != nil && r.dumpOpts.RequestHeader {
		r.dumpOpts.dumpRequest(req)
	}

	// Execute.
	httpResp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	ti.finish(httpResp)

	resp := &Response{
		Response:      httpResp,
		Trace:         *ti,
		successResult: r.successResult,
		errorResult:   r.errorResult,
	}

	// Dump response if enabled.
	if r.dumpOpts != nil && r.dumpOpts.ResponseHeader {
		r.dumpOpts.dumpResponse(resp)
	}

	// Auto-unmarshal.
	resp.autoUnmarshal()
	return resp, nil
}

type traceKey struct{}

// roundTripperChain wraps multiple RoundTrippers.
type roundTripperChain struct {
	chain []func(http.RoundTripper) http.RoundTripper
	base  http.RoundTripper
}

func (c *roundTripperChain) RoundTrip(req *http.Request) (*http.Response, error) {
	rt := c.base
	for i := len(c.chain) - 1; i >= 0; i-- {
		rt = c.chain[i](rt)
	}
	return rt.RoundTrip(req)
}

var _ = strings.TrimSpace // ensure import used
