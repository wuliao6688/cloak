package tlsgateway

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
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

	queryParams map[string]string

	commonHeaders      map[string]string // per-request default headers (req: SetCommonHeaders)
	commonQueryParams  map[string]string // per-request default query params
	onRequest          []func(*http.Request) error  // req: RequestMiddleware
	onResponse         []func(*Response) error      // req: ResponseMiddleware
	baseURL     string
	outputFile  string
	output      io.Writer
}

// SetHeader sets a request header.
func (r *Request) SetHeader(key, value string) *Request {
	if r.headers == nil { r.headers = make(map[string]string) }
	r.headers[key] = value
	return r
}

// SetHeaders sets multiple request headers.
func (r *Request) SetHeaders(h map[string]string) *Request {
	for k, v := range h { r.SetHeader(k, v) }
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

// SetQueryParam adds a single query parameter.
func (r *Request) SetQueryParam(key, value string) *Request {
	if r.queryParams == nil { r.queryParams = make(map[string]string) }
	r.queryParams[key] = value
	return r
}

// SetQueryParams sets multiple query parameters.
func (r *Request) SetQueryParams(params map[string]string) *Request {
	for k, v := range params { r.SetQueryParam(k, v) }
	return r
}

// SetCommonQueryParams sets query params applied to all requests from this builder.
func (r *Request) SetCommonQueryParams(params map[string]string) *Request {
	if r.commonQueryParams == nil { r.commonQueryParams = make(map[string]string) }
	for k, v := range params { r.commonQueryParams[k] = v }
	return r
}

// SetCommonHeaders sets headers applied to all requests from this builder.
func (r *Request) SetCommonHeaders(headers map[string]string) *Request {
	if r.commonHeaders == nil { r.commonHeaders = make(map[string]string) }
	for k, v := range headers { r.commonHeaders[k] = v }
	return r
}

// OnRequest registers a callback that runs before the request is sent.
// Equivalent to req's RequestMiddleware.
func (r *Request) OnRequest(fn func(req *http.Request) error) *Request {
	r.onRequest = append(r.onRequest, fn)
	return r
}

// OnResponse registers a callback that runs after the response is received.
// Equivalent to req's ResponseMiddleware.
func (r *Request) OnResponse(fn func(resp *Response) error) *Request {
	r.onResponse = append(r.onResponse, fn)
	return r
}

// SetBaseURL sets the base URL for relative paths.
func (r *Request) SetBaseURL(base string) *Request { r.baseURL = base; return r }

// SetBearerAuthToken sets the Authorization: Bearer header.
func (r *Request) SetBearerAuthToken(token string) *Request {
	return r.SetHeader("Authorization", "Bearer "+token)
}

// SetBasicAuth sets the Authorization: Basic header.
func (r *Request) SetBasicAuth(username, password string) *Request {
	req, _ := http.NewRequest("GET", "/", nil)
	req.SetBasicAuth(username, password)
	return r.SetHeader("Authorization", req.Header.Get("Authorization"))
}

// SetBody sets the request body reader.
func (r *Request) SetBody(body io.Reader) *Request { r.body = body; return r }

// SetBodyString sets the request body from a string.
func (r *Request) SetBodyString(s string) *Request {
	return r.SetBody(strings.NewReader(s))
}

// SetBodyBytes sets the request body from bytes.
func (r *Request) SetBodyBytes(b []byte) *Request {
	return r.SetBody(bytes.NewReader(b))
}

// SetCookies sets cookies on the request.
func (r *Request) SetCookies(cookies ...*http.Cookie) *Request {
	for _, c := range cookies {
		r.SetHeader("Cookie", c.String())
	}
	return r
}

// SetOutputFile saves the response body to a file.
func (r *Request) SetOutputFile(file string) *Request {
	r.outputFile = file
	return r
}

// SetOutput writes the response body to an io.Writer.
func (r *Request) SetOutput(w io.Writer) *Request {
	r.output = w
	return r
}

// buildURL constructs the full URL with base URL and query parameters.
func (r *Request) buildURL() string {
	u := r.url
	if r.baseURL != "" && !strings.HasPrefix(u, "http") {
		u = strings.TrimRight(r.baseURL, "/") + "/" + strings.TrimLeft(u, "/")
	}
	allParams := make(map[string]string)
	for k, v := range r.commonQueryParams { allParams[k] = v }
	for k, v := range r.queryParams { allParams[k] = v }
	if len(allParams) > 0 {
		sep := "?"
		if strings.Contains(u, "?") { sep = "&" }
		for k, v := range allParams {
			u += sep + url.QueryEscape(k) + "=" + url.QueryEscape(v)
			sep = "&"
		}
	}
	return u
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
	fullURL := r.buildURL()

	req, err := http.NewRequestWithContext(context.Background(), r.method, fullURL, r.body)
	if err != nil {
		return nil, err
	}

	// Apply common headers first, then per-request overrides (req: SetCommonHeaders + SetHeader).
	for k, v := range r.commonHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}

	// Run request hooks (req: RequestMiddleware).
	for _, fn := range r.onRequest {
		if err := fn(req); err != nil {
			return nil, err
		}
	}

	ti := newTraceInfo()
	req = req.WithContext(context.WithValue(req.Context(), traceKey{}, ti))

	if r.dumpOpts != nil && r.dumpOpts.RequestHeader {
		r.dumpOpts.dumpRequest(req)
	}

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

	if r.dumpOpts != nil && r.dumpOpts.ResponseHeader {
		r.dumpOpts.dumpResponse(resp)
	}

	resp.autoUnmarshal()

	// Run response hooks (req: ResponseMiddleware).
	for _, fn := range r.onResponse {
		if err := fn(resp); err != nil {
			return resp, err
		}
	}

	// Save to file if requested.
	if r.outputFile != "" {
		body := resp.BodyBytes()
		os.WriteFile(r.outputFile, body, 0644)
	}
	if r.output != nil {
		r.output.Write(resp.BodyBytes())
	}

	return resp, nil
}

type traceKey struct{}

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

var _ = strings.TrimSpace
