package cloak

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"time"
)

// ResultState represents the outcome of a request.
type ResultState int

const (
	ResultUnknown ResultState = iota
	ResultSuccess              // 2xx
	ResultError                // non-2xx or transport error
)

// Response wraps *http.Response with auto-unmarshal support.
type Response struct {
	*http.Response

	Trace TraceInfo

	successResult any
	errorResult   any
	rawBody       []byte
	unmarshalErr  error
}

// TraceInfo holds 7-point timing from request lifecycle.
type TraceInfo struct {
	DNSLookupTime     time.Duration
	TCPConnectTime    time.Duration
	TLSHandshakeTime  time.Duration
	FirstResponseTime time.Duration
	ResponseTime      time.Duration
	TotalTime         time.Duration
	IsConnReused      bool

	start time.Time
	dns   time.Time
	tcp   time.Time
	tls   time.Time
	first time.Time
}

func newTraceInfo() *TraceInfo { return &TraceInfo{start: time.Now()} }

func (t *TraceInfo) finish(resp *http.Response) {
	t.TotalTime = time.Since(t.start)
	if !t.first.IsZero() {
		t.FirstResponseTime = t.first.Sub(t.tls)
	}
	if t.first.IsZero() {
		t.FirstResponseTime = 0
	}
	if !t.tcp.IsZero() {
		t.TCPConnectTime = t.tcp.Sub(t.dns)
	}
	if !t.tls.IsZero() && !t.tcp.IsZero() {
		t.TLSHandshakeTime = t.tls.Sub(t.tcp)
	}
}

// BodyBytes returns the cached response body.
func (r *Response) BodyBytes() []byte {
	if r.rawBody != nil {
		return r.rawBody
	}
	if r.Response == nil || r.Response.Body == nil {
		return nil
	}
	b, _ := io.ReadAll(r.Response.Body)
	r.Response.Body.Close()
	r.rawBody = b
	return b
}

// UnmarshalJson unmarshals the body as JSON into v.
func (r *Response) UnmarshalJson(v any) error {
	body := r.BodyBytes()
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

// UnmarshalXml unmarshals the body as XML into v.
func (r *Response) UnmarshalXml(v any) error {
	body := r.BodyBytes()
	if len(body) == 0 {
		return nil
	}
	return xml.Unmarshal(body, v)
}

// autoUnmarshal triggers the deferred unmarshal on success/error.
func (r *Response) autoUnmarshal() {
	if r.IsSuccess() && r.successResult != nil {
		r.unmarshalErr = r.UnmarshalJson(r.successResult)
	}
	if !r.IsSuccess() && r.errorResult != nil {
		r.unmarshalErr = r.UnmarshalJson(r.errorResult)
	}
}

// IsSuccess returns true for 2xx status codes.
func (r *Response) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// IsError returns true for non-2xx status codes.
func (r *Response) IsError() bool { return !r.IsSuccess() }

// ResultState returns the classification (req: ResultState).
func (r *Response) ResultState() ResultState {
	if r.unmarshalErr != nil { return ResultError }
	if r.Response == nil { return ResultError }
	if r.IsSuccess() { return ResultSuccess }
	return ResultError
}

// SuccessResult returns the auto-unmarshalled success body.
func (r *Response) SuccessResult() any { return r.successResult }

// ErrorResult returns the auto-unmarshalled error body.
func (r *Response) ErrorResult() any { return r.errorResult }

// UnmarshalErr returns any error from auto-unmarshalling.
func (r *Response) UnmarshalErr() error { return r.unmarshalErr }

// String returns the response body as a string, decoded from the
// response charset (Content-Type) to UTF-8 when a non-UTF-8 charset
// is declared (e.g. EUC-KR for Korean sites, GBK for Chinese).
func (r *Response) String() string {
	return string(r.decodedBody())
}

// Bytes returns the response body as bytes.
func (r *Response) Bytes() []byte { return r.BodyBytes() }

// ToString returns the response body as string (charset-decoded),
// with error.
func (r *Response) ToString() (string, error) {
	return string(r.decodedBody()), r.unmarshalErr
}

// decodedBody returns the raw body decoded to UTF-8 per the response
// Content-Type charset. See decodeCharset for supported encodings.
func (r *Response) decodedBody() []byte {
	if r.Response == nil {
		return r.BodyBytes()
	}
	return decodeCharset(r.BodyBytes(), r.Response.Header.Get("Content-Type"))
}
