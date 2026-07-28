package tls_client

import (
	stdtls "crypto/tls"
	"errors"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/tls-client/profiles"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type recordingDebugLogger struct {
	debugCalls atomic.Int64
}

func (l *recordingDebugLogger) Debug(string, ...any) { l.debugCalls.Add(1) }
func (l *recordingDebugLogger) Info(string, ...any)  {}
func (l *recordingDebugLogger) Warn(string, ...any)  {}
func (l *recordingDebugLogger) Error(string, ...any) {}

type disabledRecordingDebugLogger struct {
	recordingDebugLogger
}

func (*disabledRecordingDebugLogger) DebugEnabled() bool { return false }

func TestDoConvertsRecoveredPanicToError(t *testing.T) {
	client := &httpClient{
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			panic("boom")
		})},
		logger: NewNoopLogger(),
		config: &httpClientConfig{
			catchPanics:    true,
			defaultHeaders: make(http.Header),
		},
	}

	body := newCloseTrackingBody()
	req, err := http.NewRequest(http.MethodPost, "https://example.com", body)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.do(req)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected recovered panic error, got response=%v error=%v", resp, err)
	}
	if resp != nil {
		t.Fatalf("expected nil response after panic, got %v", resp)
	}
	select {
	case <-body.closed:
	default:
		t.Fatal("recovered panic did not close the request body")
	}
}

func TestDoClosesRequestBodyWhenPreHookRejectsRequest(t *testing.T) {
	body := newCloseTrackingBody()
	client := &httpClient{
		logger: NewNoopLogger(),
		config: &httpClientConfig{defaultHeaders: make(http.Header)},
		preHooks: []PreRequestHookFunc{func(*http.Request) error {
			return errors.New("reject request")
		}},
	}
	req, err := http.NewRequest(http.MethodPost, "https://example.com", body)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.Do(req); err == nil {
		t.Fatal("pre-hook rejection should return an error")
	}
	select {
	case <-body.closed:
	default:
		t.Fatal("pre-hook rejection did not close the request body")
	}
}

func TestDoMergesDefaultHeadersWithoutOverwritingRequest(t *testing.T) {
	var captured http.Header
	client := &httpClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			captured = req.Header.Clone()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})},
		logger: NewNoopLogger(),
		config: &httpClientConfig{
			defaultHeaders: http.Header{
				"Accept":    {"text/html"},
				"X-Default": {"default"},
			},
		},
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request", "request")

	resp, err := client.do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := captured.Get("Accept"); got != "application/json" {
		t.Fatalf("request header should override default: %q", got)
	}
	if got := captured.Get("X-Default"); got != "default" {
		t.Fatalf("missing merged default header: %q", got)
	}
	if got := captured.Get("X-Request"); got != "request" {
		t.Fatalf("missing request header: %q", got)
	}
}

func TestDoAvoidsUnobservableDebugWork(t *testing.T) {
	logger := &disabledRecordingDebugLogger{}
	client := &httpClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Set-Cookie": {"session=value"}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})},
		logger:    logger,
		debugLogs: loggerDebugEnabled(logger),
		config:    &httpClientConfig{defaultHeaders: make(http.Header)},
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if calls := logger.debugCalls.Load(); calls != 0 {
		t.Fatalf("disabled debug logger received %d calls", calls)
	}
}

func TestDoPreservesUnknownLoggerDebugBehavior(t *testing.T) {
	logger := &recordingDebugLogger{}
	client := &httpClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})},
		logger:    logger,
		debugLogs: loggerDebugEnabled(logger),
		config:    &httpClientConfig{defaultHeaders: make(http.Header)},
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if calls := logger.debugCalls.Load(); calls == 0 {
		t.Fatal("unknown logger implementation should retain historical Debug calls")
	}
}

func TestDoOnlyNormalizesPresentHeaderOrderWithoutMutatingInput(t *testing.T) {
	client := &httpClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})},
		logger: NewNoopLogger(),
		config: &httpClientConfig{defaultHeaders: make(http.Header)},
	}

	withoutOrder, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.do(withoutOrder)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if _, exists := withoutOrder.Header[http.HeaderOrderKey]; exists {
		t.Fatal("request without header order gained an empty ordering key")
	}

	originalOrder := []string{"Host", "User-Agent"}
	withOrder, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	withOrder.Header[http.HeaderOrderKey] = originalOrder
	resp, err = client.do(withOrder)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got := strings.Join(withOrder.Header[http.HeaderOrderKey], ","); got != "host,user-agent" {
		t.Fatalf("unexpected normalized header order: %q", got)
	}
	if got := strings.Join(originalOrder, ","); got != "Host,User-Agent" {
		t.Fatalf("normalization mutated caller-owned order slice: %q", got)
	}
}

func TestDoWithDebugPreservesStreamingResponseBody(t *testing.T) {
	client := &httpClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("response-body")),
				Request:    req,
			}, nil
		})},
		logger: NewNoopLogger(),
		config: &httpClientConfig{
			debug:          true,
			debugBodyLimit: 4,
			defaultHeaders: make(http.Header),
		},
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "response-body" {
		t.Fatalf("debug mode consumed or changed response body: %q", body)
	}
}

func TestDoRejectsNilRequest(t *testing.T) {
	client := &httpClient{}
	if _, err := client.Do(nil); err == nil {
		t.Fatal("expected nil request to return an error")
	}
}

func BenchmarkDoHeaderMergeParallel(b *testing.B) {
	client := &httpClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})},
		logger: NewNoopLogger(),
		config: &httpClientConfig{
			defaultHeaders: http.Header{
				"Accept":     {"*/*"},
				"User-Agent": {"tls-client-benchmark"},
			},
		},
	}
	client.clientState.Store(client.client)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
			if err != nil {
				b.Fatal(err)
			}
			resp, err := client.do(req)
			if err != nil {
				b.Fatal(err)
			}
			_ = resp.Body.Close()
		}
	})
}

func BenchmarkClientSnapshotParallel(b *testing.B) {
	underlying := &http.Client{}

	b.Run("atomic", func(b *testing.B) {
		client := &httpClient{client: underlying}
		client.clientState.Store(underlying)
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if client.snapshotClient() != underlying {
					b.Fatal("unexpected client snapshot")
				}
			}
		})
	})
}

func TestDoTreatsHeaderNamesCaseInsensitivelyWhenMergingDefaults(t *testing.T) {
	var captured http.Header
	client := &httpClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			captured = req.Header.Clone()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})},
		logger: NewNoopLogger(),
		config: &httpClientConfig{
			defaultHeaders: http.Header{"User-Agent": {"default-agent"}},
		},
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header["user-agent"] = []string{"request-agent"}

	resp, err := client.do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := captured["user-agent"]; len(got) != 1 || got[0] != "request-agent" {
		t.Fatalf("request header should override differently-cased default: %v", captured)
	}
	if _, duplicated := captured["User-Agent"]; duplicated {
		t.Fatalf("default header was duplicated with different casing: %v", captured)
	}
}

func TestSetFollowRedirectReplacesClientWithoutMutatingActiveSnapshot(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, nil })
	client := &httpClient{
		client: &http.Client{
			Transport: transport,
		},
		logger: NewNoopLogger(),
		config: &httpClientConfig{
			followRedirects: true,
		},
	}

	activeSnapshot := client.snapshotClient()
	client.SetFollowRedirect(false)
	updatedSnapshot := client.snapshotClient()

	if activeSnapshot == updatedSnapshot {
		t.Fatal("dynamic state changes must replace, not mutate, the active http client")
	}
	if activeSnapshot.CheckRedirect != nil {
		t.Fatal("an in-flight snapshot was mutated")
	}
	if updatedSnapshot.CheckRedirect == nil {
		t.Fatal("updated snapshot did not receive redirect policy")
	}
	if updatedSnapshot.Transport == nil {
		t.Fatal("updated snapshot did not preserve transport")
	}
}

func TestSetProxyFailureKeepsPreviousClientState(t *testing.T) {
	client, err := NewHttpClient(nil)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := client.(*httpClient)
	previousClient := httpClient.snapshotClient()
	previousDialer := httpClient.GetDialer()

	if err := httpClient.SetProxy("://invalid-proxy"); err == nil {
		t.Fatal("expected invalid proxy URL to fail")
	}
	if httpClient.snapshotClient() != previousClient {
		t.Fatal("failed proxy update replaced the active client")
	}
	if httpClient.GetDialer() != previousDialer {
		t.Fatal("failed proxy update replaced the active dialer")
	}
	if got := httpClient.GetProxy(); got != "" {
		t.Fatalf("failed proxy update changed configuration to %q", got)
	}
}

func TestPSKProfileResumesTLSAgainstLocalServer(t *testing.T) {
	server := httptest.NewUnstartedServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, req *stdhttp.Request) {
		w.Header().Set("X-TLS-Did-Resume", strconv.FormatBool(req.TLS.DidResume))
		w.Header().Set("X-Remote-Addr", req.RemoteAddr)
		_, _ = w.Write([]byte("ok"))
	}))
	server.TLS = &stdtls.Config{MinVersion: stdtls.VersionTLS13}
	server.StartTLS()
	defer server.Close()

	client, err := NewHttpClient(nil,
		WithClientProfile(profiles.Chrome_150_PSK),
		WithInsecureSkipVerify(),
	)
	if err != nil {
		t.Fatal(err)
	}

	request := func() (resumed, remoteAddr string) {
		t.Helper()
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			t.Fatal(err)
		}
		return resp.Header.Get("X-TLS-Did-Resume"), resp.Header.Get("X-Remote-Addr")
	}

	firstResumed, firstAddr := request()
	if firstResumed != "false" {
		t.Fatalf("first handshake unexpectedly resumed a session: %q", firstResumed)
	}

	client.CloseIdleConnections()
	secondResumed, secondAddr := request()
	if secondAddr == firstAddr {
		t.Fatalf("second request reused the original connection: %q", secondAddr)
	}
	if secondResumed != "true" {
		t.Fatalf("second handshake did not resume the cached TLS session: %q", secondResumed)
	}
}
