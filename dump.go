package cloak

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
)

// DumpOptions controls debug output granularity.
type DumpOptions struct {
	Output               io.Writer
	RequestHeader        bool
	RequestBody          bool
	ResponseHeader       bool
	ResponseBody         bool
	Async                bool
}

// DefaultDumpOptions returns options that dump both request and response headers.
func DefaultDumpOptions() *DumpOptions {
	return &DumpOptions{
		Output:         os.Stderr,
		RequestHeader:  true,
		ResponseHeader: true,
	}
}

// FullDumpOptions returns options that dump everything.
func FullDumpOptions() *DumpOptions {
	return &DumpOptions{
		Output:         os.Stderr,
		RequestHeader:  true,
		RequestBody:    true,
		ResponseHeader: true,
		ResponseBody:   true,
	}
}

func (d *DumpOptions) dumpRequest(req *http.Request) {
	if d.Async {
		go d.writeDump(reqDump(req, d.RequestBody))
	} else {
		d.writeDump(reqDump(req, d.RequestBody))
	}
}

func (d *DumpOptions) dumpResponse(resp *Response) {
	if d.Async {
		go d.writeDump(respDump(resp.Response, d.ResponseBody))
	} else {
		d.writeDump(respDump(resp.Response, d.ResponseBody))
	}
}

func (d *DumpOptions) writeDump(s string) {
	if d.Output == nil {
		return
	}
	fmt.Fprint(d.Output, s)
}

func reqDump(req *http.Request, body bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("> %s %s %s\n", req.Method, req.URL.String(), req.Proto))
	for k, vs := range req.Header {
		for _, v := range vs {
			sb.WriteString(fmt.Sprintf("> %s: %s\n", k, v))
		}
	}
	if body && req.Body != nil {
		b, _ := httputil.DumpRequestOut(req, true)
		sb.WriteString(string(b))
	}
	sb.WriteString("\n")
	return sb.String()
}

func respDump(resp *http.Response, body bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("< %s %s\n", resp.Proto, resp.Status))
	for k, vs := range resp.Header {
		for _, v := range vs {
			sb.WriteString(fmt.Sprintf("< %s: %s\n", k, v))
		}
	}
	if body {
		b, _ := httputil.DumpResponse(resp, true)
		sb.WriteString(string(b))
	}
	sb.WriteString("\n")
	return sb.String()
}
