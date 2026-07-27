package tls_client

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

type closeTrackingBody struct {
	closed chan struct{}
	once   sync.Once
}

type noopRoundTripper struct{}

func (*noopRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, nil
}

func newCloseTrackingBody() *closeTrackingBody {
	return &closeTrackingBody{closed: make(chan struct{})}
}

func (b *closeTrackingBody) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (b *closeTrackingBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

func TestProtocolRaceEligibility(t *testing.T) {
	get, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !isRaceEligible(get) {
		t.Fatal("GET without a body should be race eligible")
	}

	post, err := http.NewRequest(http.MethodPost, "https://example.com", bytes.NewBufferString("body"))
	if err != nil {
		t.Fatal(err)
	}
	if isRaceEligible(post) {
		t.Fatal("POST must not be duplicated across protocols")
	}

	get.Body = io.NopCloser(bytes.NewBufferString("body"))
	get.GetBody = nil
	if isRaceEligible(get) {
		t.Fatal("request with a non-replayable body must not be raced")
	}
}

func TestProtocolRacingTimingDefaultsAndOverrides(t *testing.T) {
	pr := &protocolRacer{}
	if got := pr.http2Delay(); got != DefaultProtocolRacingHTTP2Delay {
		t.Fatalf("unexpected default HTTP/2 delay: %v", got)
	}
	if got := pr.racingTimeout(); got != DefaultProtocolRacingTimeout {
		t.Fatalf("unexpected default racing timeout: %v", got)
	}

	delay := 25 * time.Millisecond
	timeout := 2 * time.Second
	pr.transportOptions = &TransportOptions{
		ProtocolRacingHTTP2Delay: &delay,
		ProtocolRacingTimeout:    &timeout,
	}
	if got := pr.http2Delay(); got != delay {
		t.Fatalf("unexpected configured HTTP/2 delay: %v", got)
	}
	if got := pr.racingTimeout(); got != timeout {
		t.Fatalf("unexpected configured racing timeout: %v", got)
	}
}

func TestTransportEvictionOnlyClearsMatchingProtocol(t *testing.T) {
	const addr = "example.com:443"
	racer := &protocolRacer{protocolCache: map[string]string{addr: "h3"}}

	racer.clearProtocolCacheForTransportKey(addr)
	if racer.protocolCache[addr] != "h3" {
		t.Fatal("evicting an obsolete HTTP/2 transport cleared the HTTP/3 winner")
	}

	racer.clearProtocolCacheForTransportKey(addr + ":h3")
	if _, ok := racer.protocolCache[addr]; ok {
		t.Fatal("evicting the active HTTP/3 transport did not clear its protocol cache")
	}
}

func TestCloneRequestForRaceUsesIndependentBodies(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.com", bytes.NewBufferString("payload"))
	if err != nil {
		t.Fatal(err)
	}

	first, err := cloneRequestForRace(req, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Body.Close()
	second, err := cloneRequestForRace(req, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Body.Close()

	firstBody, err := io.ReadAll(first.Body)
	if err != nil {
		t.Fatal(err)
	}
	secondBody, err := io.ReadAll(second.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBody) != "payload" || string(secondBody) != "payload" {
		t.Fatalf("cloned bodies differ: %q %q", firstBody, secondBody)
	}
}

func TestRaceCachesWinningTransportAndClosesLoserResponse(t *testing.T) {
	pr := &protocolRacer{
		protocolCache:       make(map[string]string),
		cachedTransports:    make(map[string]http.RoundTripper),
		cachedTransportsLck: &sync.RWMutex{},
		transportInit:       &keyedLockPool{},
	}

	winnerBody := newCloseTrackingBody()
	loserBody := newCloseTrackingBody()
	winnerTransport := &noopRoundTripper{}
	resultCh := make(chan racingResult, 2)
	resultCh <- racingResult{
		protocol:  "h3",
		response:  &http.Response{Body: winnerBody},
		transport: winnerTransport,
	}
	resultCh <- racingResult{
		protocol: "h2",
		response: &http.Response{Body: loserBody},
	}
	close(resultCh)

	ctx := context.Background()
	h3Ctx, cancelHTTP3 := context.WithCancel(ctx)
	h2Ctx, cancelHTTP2 := context.WithCancel(ctx)
	resp, err := pr.waitForRaceWinner(ctx, "example.com:443", resultCh, cancelHTTP3, cancelHTTP2)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.Body == nil {
		t.Fatal("winner response was not returned")
	}

	select {
	case <-h2Ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("losing protocol context was not canceled")
	}
	select {
	case <-h3Ctx.Done():
		t.Fatal("winning protocol context was canceled before its response body closed")
	default:
	}

	select {
	case <-loserBody.closed:
	case <-time.After(time.Second):
		t.Fatal("loser response body was not closed")
	}

	transport, ok := pr.getCachedTransport("example.com:443:h3")
	if !ok || transport != winnerTransport {
		t.Fatal("actual winning HTTP/3 transport was not cached")
	}

	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h3Ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("winning protocol context was not canceled after its response body closed")
	}
}

func TestAttemptHTTP2ClosesBodyWhenCanceledBeforeRoundTrip(t *testing.T) {
	requestBody := newCloseTrackingBody()
	req, err := http.NewRequest(http.MethodGet, "https://example.com", requestBody)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resultCh := make(chan racingResult, 1)
	pr := &protocolRacer{}
	pr.attemptHTTP2(ctx, req, "example.com:443", nil, resultCh)

	result := <-resultCh
	if result.err == nil {
		t.Fatal("canceled attempt should return an error")
	}
	select {
	case <-requestBody.closed:
	case <-time.After(time.Second):
		t.Fatal("body for request that never reached RoundTrip was not closed")
	}
}

func TestCachedProtocolUsesReplayBodyAndClosesOriginal(t *testing.T) {
	originalBody := newCloseTrackingBody()
	req, err := http.NewRequest(http.MethodGet, "https://example.com", originalBody)
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = int64(len("payload"))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("payload")), nil
	}

	var receivedBody string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		receivedBody = string(body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("response")),
			Request:    req,
		}, nil
	})

	const addr = "example.com:443"
	pr := &protocolRacer{
		protocolCache:       map[string]string{addr: "h2"},
		cachedTransports:    map[string]http.RoundTripper{addr: transport},
		cachedTransportsLck: &sync.RWMutex{},
		transportInit:       &keyedLockPool{},
	}

	resp, err := pr.race(req, addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if receivedBody != "payload" {
		t.Fatalf("cached protocol received the wrong replay body: %q", receivedBody)
	}
	select {
	case <-originalBody.closed:
	case <-time.After(time.Second):
		t.Fatal("original request body was not closed")
	}
}
