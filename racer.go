package tls_client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/fhttp/http2"
	"github.com/bogdanfinn/tls-client/bandwidth"
	tls "github.com/bogdanfinn/utls"
)

type protocolRacer struct {
	protocolCache   map[string]string
	protocolCacheMu sync.RWMutex
	protocolInit    keyedLockPool

	clientSessionCache  tls.ClientSessionCache
	insecureSkipVerify  bool
	serverNameOverwrite string
	transportOptions    *TransportOptions
	settings            map[http2.SettingID]uint32
	cachedTransports    map[string]http.RoundTripper
	cachedTransportsLck *sync.RWMutex
	transportCache      *transportCacheMeta
	transportInit       *keyedLockPool
	certificatePinner   CertificatePinner
	badPinHandlerFunc   BadPinHandlerFunc
	bandwidthTracker    bandwidth.BandwidthTracker

	// HTTP/3 specific settings
	http3Settings          map[uint64]uint64
	http3SettingsOrder     []uint64
	http3PriorityParam     uint32
	http3PseudoHeaderOrder []string
	http3SendGreaseFrames  bool
}

func newProtocolRacer(
	clientSessionCache tls.ClientSessionCache,
	insecureSkipVerify bool,
	serverNameOverwrite string,
	transportOptions *TransportOptions,
	settings map[http2.SettingID]uint32,
	cachedTransports map[string]http.RoundTripper,
	cachedTransportsLck *sync.RWMutex,
	transportCache *transportCacheMeta,
	transportInit *keyedLockPool,
	certificatePinner CertificatePinner,
	badPinHandlerFunc BadPinHandlerFunc,
	bandwidthTracker bandwidth.BandwidthTracker,
	http3Settings map[uint64]uint64,
	http3SettingsOrder []uint64,
	http3PriorityParam uint32,
	http3PseudoHeaderOrder []string,
	http3SendGreaseFrames bool,
) *protocolRacer {
	return &protocolRacer{
		protocolCache:          make(map[string]string),
		clientSessionCache:     clientSessionCache,
		insecureSkipVerify:     insecureSkipVerify,
		serverNameOverwrite:    serverNameOverwrite,
		transportOptions:       transportOptions,
		settings:               settings,
		cachedTransports:       cachedTransports,
		cachedTransportsLck:    cachedTransportsLck,
		transportCache:         transportCache,
		transportInit:          transportInit,
		certificatePinner:      certificatePinner,
		badPinHandlerFunc:      badPinHandlerFunc,
		bandwidthTracker:       bandwidthTracker,
		http3Settings:          http3Settings,
		http3SettingsOrder:     http3SettingsOrder,
		http3PriorityParam:     http3PriorityParam,
		http3PseudoHeaderOrder: http3PseudoHeaderOrder,
		http3SendGreaseFrames:  http3SendGreaseFrames,
	}
}

// race races HTTP/3 and HTTP/2 connections and uses whichever responds first.
// Similar to Chrome's "Happy Eyeballs" approach.
func (pr *protocolRacer) race(req *http.Request, addr string, getTransportFunc func(*http.Request, string) error) (*http.Response, error) {
	raceEligible := isRaceEligible(req)
	if raceEligible {
		// Eligible requests are always sent through independent clones. The
		// original body will never reach a transport, so this RoundTripper owns
		// closing it before any attempt starts.
		closeRequestBody(req)
	}

	// Try cached protocol first if available
	if resp, usedCache, err := pr.tryUseCachedProtocol(req, addr, getTransportFunc, raceEligible); usedCache {
		if err == nil {
			return resp, nil
		}
		if !raceEligible {
			return nil, err
		}
	}

	if !raceEligible {
		return pr.roundTripHTTP2(req, addr, getTransportFunc)
	}

	release, err := pr.protocolInit.Lock(req.Context(), addr)
	if err != nil {
		return nil, err
	}
	defer release()

	// Another request may have completed the race while this one was waiting.
	if resp, usedCache, err := pr.tryUseCachedProtocol(req, addr, getTransportFunc, raceEligible); usedCache {
		if err == nil {
			return resp, nil
		}
	}

	// No cached protocol or a replayable cached request failed - start racing.
	return pr.startRace(req, addr, getTransportFunc)
}

func (pr *protocolRacer) tryUseCachedProtocol(req *http.Request, addr string, getTransportFunc func(*http.Request, string) error, raceEligible bool) (*http.Response, bool, error) {
	pr.protocolCacheMu.RLock()
	cachedProtocol, found := pr.protocolCache[addr]
	pr.protocolCacheMu.RUnlock()

	if !found {
		return nil, false, nil
	}

	transport, err := pr.getOrCreateTransport(cachedProtocol, addr, req, getTransportFunc)
	if err != nil {
		if !raceEligible {
			closeRequestBody(req)
		}
		pr.handleCachedProtocolError(err, addr, req)
		return nil, true, err
	}

	requestForProtocol := req
	if raceEligible {
		requestForProtocol, err = cloneRequestForRace(req, req.Context())
		if err != nil {
			return nil, true, err
		}
	}

	resp, err := transport.RoundTrip(requestForProtocol)
	if err == nil {
		return resp, true, nil
	}
	closeRequestBody(requestForProtocol)

	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	failedTransport := pr.deleteCachedTransport(pr.getTransportKey(cachedProtocol, addr))
	// Cached transports may still own unrelated active response bodies. Detach
	// and retire them instead of force-closing the whole QUIC connection.
	closeIdleTransport(failedTransport)
	pr.clearProtocolCache(addr)
	return nil, true, err
}

func (pr *protocolRacer) getOrCreateTransport(protocol, addr string, req *http.Request, getTransportFunc func(*http.Request, string) error) (http.RoundTripper, error) {
	transportKey := pr.getTransportKey(protocol, addr)

	if transport, exists := pr.getCachedTransport(transportKey); exists {
		return transport, nil
	}

	release, err := pr.transportInit.Lock(req.Context(), transportKey)
	if err != nil {
		return nil, err
	}
	defer release()

	if transport, exists := pr.getCachedTransport(transportKey); exists {
		return transport, nil
	}

	transport, err := pr.createTransportForProtocol(protocol, addr, req, getTransportFunc)
	if err != nil {
		return nil, err
	}

	pr.setCachedTransport(transportKey, transport)
	return transport, nil
}

func (pr *protocolRacer) createTransportForProtocol(protocol, addr string, req *http.Request, getTransportFunc func(*http.Request, string) error) (http.RoundTripper, error) {
	if protocol == "h3" {
		return buildHTTP3Transport(pr.getHTTP3Config())
	}

	// For HTTP/2, use the standard transport creation
	transportKey := pr.getTransportKey(protocol, addr)
	if err := getTransportFunc(req, transportKey); err != nil {
		return nil, err
	}

	transport, ok := pr.getCachedTransport(transportKey)
	if !ok || transport == nil {
		return nil, fmt.Errorf("HTTP/2 transport initialization completed without a transport for %s", addr)
	}
	return transport, nil
}

func (pr *protocolRacer) getCachedTransport(key string) (http.RoundTripper, bool) {
	return getCachedTransportEntry(pr.cachedTransports, pr.cachedTransportsLck, pr.transportCache, key)
}

func (pr *protocolRacer) setCachedTransport(key string, transport http.RoundTripper) {
	evicted := setCachedTransportEntry(pr.cachedTransports, pr.cachedTransportsLck, pr.transportCache, key, transport)
	for _, entry := range evicted {
		if entry.removed {
			pr.clearProtocolCacheForTransportKey(entry.key)
		}
		closeIdleTransport(entry.transport)
	}
}

func (pr *protocolRacer) deleteCachedTransport(key string) http.RoundTripper {
	return deleteCachedTransportEntry(pr.cachedTransports, pr.cachedTransportsLck, pr.transportCache, key)
}

func (pr *protocolRacer) startRace(req *http.Request, addr string, getTransportFunc func(*http.Request, string) error) (*http.Response, error) {
	resultCh := make(chan racingResult, 2)
	waitCtx, stopWaitTimer := context.WithTimeout(req.Context(), 10*time.Second)
	defer stopWaitTimer()

	h3Ctx, cancelHTTP3 := context.WithCancel(req.Context())
	h2Ctx, cancelHTTP2 := context.WithCancel(req.Context())

	h3Request, err := cloneRequestForRace(req, h3Ctx)
	if err != nil {
		cancelHTTP3()
		cancelHTTP2()
		return nil, err
	}
	h2Request, err := cloneRequestForRace(req, h2Ctx)
	if err != nil {
		cancelHTTP3()
		cancelHTTP2()
		if h3Request.Body != nil {
			_ = h3Request.Body.Close()
		}
		return nil, err
	}

	var attempts sync.WaitGroup
	attempts.Add(2)
	go func() {
		defer attempts.Done()
		pr.attemptHTTP3(h3Request, resultCh)
	}()
	go func() {
		defer attempts.Done()
		pr.attemptHTTP2(h2Ctx, h2Request, addr, getTransportFunc, resultCh)
	}()
	go func() {
		attempts.Wait()
		close(resultCh)
	}()

	return pr.waitForRaceWinner(waitCtx, addr, resultCh, cancelHTTP3, cancelHTTP2)
}

func (pr *protocolRacer) attemptHTTP3(req *http.Request, resultCh chan<- racingResult) {
	h3Transport, err := buildHTTP3Transport(pr.getHTTP3Config())
	if err != nil {
		closeRequestBody(req)
		resultCh <- racingResult{protocol: "h3", err: fmt.Errorf("failed to build HTTP/3 transport: %w", err)}
		return
	}

	resp, err := h3Transport.RoundTrip(req)
	if err != nil {
		closeRacingTransport(h3Transport)
		resultCh <- racingResult{protocol: "h3", err: fmt.Errorf("HTTP/3 request failed: %w", err)}
	} else {
		resultCh <- racingResult{protocol: "h3", response: resp, transport: h3Transport}
	}
}

func (pr *protocolRacer) attemptHTTP2(ctx context.Context, req *http.Request, addr string, getTransportFunc func(*http.Request, string) error, resultCh chan<- racingResult) {
	// Chrome-like 300ms delay before starting HTTP/2
	// https://groups.google.com/a/chromium.org/g/proto-quic/c/igD7dLSct24
	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		closeRequestBody(req)
		resultCh <- racingResult{protocol: "h2", err: ctx.Err()}
		return
	}

	h2Transport, err := pr.getOrCreateTransport("h2", addr, req, getTransportFunc)
	if err != nil {
		closeRequestBody(req)
		resultCh <- racingResult{protocol: "h2", err: err}
		return
	}

	resp, err := h2Transport.RoundTrip(req)
	if err != nil {
		closeRequestBody(req)
	}
	resultCh <- racingResult{protocol: "h2", response: resp, transport: h2Transport, err: err}
}

func (pr *protocolRacer) waitForRaceWinner(ctx context.Context, addr string, resultCh <-chan racingResult, cancelHTTP3, cancelHTTP2 context.CancelFunc) (*http.Response, error) {
	var lastErr error

	for i := 0; i < 2; i++ {
		select {
		case result := <-resultCh:
			if result.err == nil && result.response != nil {
				pr.cacheWinningProtocol(addr, result.protocol, result.transport)
				cancelLosingProtocol(result.protocol, cancelHTTP3, cancelHTTP2)
				attachWinnerCancel(result.protocol, result.response, cancelHTTP3, cancelHTTP2)
				go cleanupRaceResults(resultCh)
				return result.response, nil
			}
			cancelProtocol(result.protocol, cancelHTTP3, cancelHTTP2)
			cleanupRacingResult(result)
			lastErr = result.err

		case <-ctx.Done():
			cancelHTTP3()
			cancelHTTP2()
			go cleanupRaceResults(resultCh)
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, ctx.Err()
		}
	}

	cancelHTTP3()
	cancelHTTP2()
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("http3 racing: both protocols failed to connect")
}

func (pr *protocolRacer) getTransportKey(protocol, addr string) string {
	if protocol == "h3" {
		return addr + ":h3"
	}
	return addr
}

func (pr *protocolRacer) clearProtocolCache(addr string) {
	pr.protocolCacheMu.Lock()
	delete(pr.protocolCache, addr)
	pr.protocolCacheMu.Unlock()
}

func (pr *protocolRacer) clearProtocolCacheForTransportKey(key string) {
	protocol := "h2"
	if len(key) > len(":h3") && key[len(key)-len(":h3"):] == ":h3" {
		key = key[:len(key)-len(":h3")]
		protocol = "h3"
	}
	pr.protocolCacheMu.Lock()
	if pr.protocolCache[key] == protocol {
		delete(pr.protocolCache, key)
	}
	pr.protocolCacheMu.Unlock()
}

func (pr *protocolRacer) resetProtocolCache() {
	pr.protocolCacheMu.Lock()
	pr.protocolCache = make(map[string]string)
	pr.protocolCacheMu.Unlock()
}

func (pr *protocolRacer) cacheWinningProtocol(addr, protocol string, transport http.RoundTripper) {
	// Publish the winning HTTP/3 transport before the protocol selection. This
	// prevents another request from observing an H3 winner whose actual racing
	// transport is not reachable from the cache yet and constructing a duplicate.
	if protocol == "h3" && transport != nil {
		pr.setCachedTransport(addr+":h3", transport)
	}

	pr.protocolCacheMu.Lock()
	pr.protocolCache[addr] = protocol
	pr.protocolCacheMu.Unlock()
}

func (pr *protocolRacer) roundTripHTTP2(req *http.Request, addr string, getTransportFunc func(*http.Request, string) error) (*http.Response, error) {
	transport, err := pr.getOrCreateTransport("h2", addr, req, getTransportFunc)
	if err != nil {
		closeRequestBody(req)
		return nil, err
	}
	return transport.RoundTrip(req)
}

func isRaceEligible(req *http.Request) bool {
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
	default:
		return false
	}
	return req.Body == nil || req.GetBody != nil
}

func cloneRequestForRace(req *http.Request, ctx context.Context) (*http.Request, error) {
	cloned := req.Clone(ctx)
	if req.Body == nil {
		return cloned, nil
	}
	if req.GetBody == nil {
		return nil, errors.New("http3 racing requires a replayable request body")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("failed to clone request body for protocol racing: %w", err)
	}
	cloned.Body = body
	return cloned, nil
}

func cleanupRaceResults(resultCh <-chan racingResult) {
	for result := range resultCh {
		cleanupRacingResult(result)
	}
}

func closeRequestBody(req *http.Request) {
	if req != nil && req.Body != nil {
		_ = req.Body.Close()
	}
}

func cancelLosingProtocol(winner string, cancelHTTP3, cancelHTTP2 context.CancelFunc) {
	if winner == "h3" {
		cancelHTTP2()
		return
	}
	cancelHTTP3()
}

func cancelProtocol(protocol string, cancelHTTP3, cancelHTTP2 context.CancelFunc) {
	if protocol == "h3" {
		cancelHTTP3()
		return
	}
	if protocol == "h2" {
		cancelHTTP2()
	}
}

func attachWinnerCancel(protocol string, response *http.Response, cancelHTTP3, cancelHTTP2 context.CancelFunc) {
	winnerCancel := cancelHTTP2
	if protocol == "h3" {
		winnerCancel = cancelHTTP3
	}
	if response.Body == nil {
		winnerCancel()
		return
	}
	response.Body = &cancelOnCloseBody{
		ReadCloser: response.Body,
		cancel:     winnerCancel,
	}
}

type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (b *cancelOnCloseBody) Read(buffer []byte) (int, error) {
	read, err := b.ReadCloser.Read(buffer)
	if err != nil {
		b.once.Do(b.cancel)
	}
	return read, err
}

func (b *cancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.cancel)
	return err
}

func cleanupRacingResult(result racingResult) {
	if result.response != nil && result.response.Body != nil {
		_ = result.response.Body.Close()
	}
	if result.protocol == "h3" {
		closeRacingTransport(result.transport)
	}
}

func closeRacingTransport(transport http.RoundTripper) {
	if transport == nil {
		return
	}
	if closer, ok := transport.(interface{ Close() error }); ok {
		_ = closer.Close()
		return
	}
	if closeIdler, ok := transport.(interface{ CloseIdleConnections() }); ok {
		closeIdler.CloseIdleConnections()
	}
}

func (pr *protocolRacer) handleCachedProtocolError(err error, addr string, req *http.Request) {
	if errors.Is(err, ErrBadPinDetected) && pr.badPinHandlerFunc != nil {
		pr.badPinHandlerFunc(req)
	}
	pr.clearProtocolCache(addr)
}

func (pr *protocolRacer) getHTTP3Config() *http3Config {
	return &http3Config{
		clientSessionCache:     pr.clientSessionCache,
		insecureSkipVerify:     pr.insecureSkipVerify,
		serverNameOverwrite:    pr.serverNameOverwrite,
		transportOptions:       pr.transportOptions,
		http3Settings:          pr.http3Settings,
		http3SettingsOrder:     pr.http3SettingsOrder,
		http3PriorityParam:     pr.http3PriorityParam,
		http3PseudoHeaderOrder: pr.http3PseudoHeaderOrder,
		http3SendGreaseFrames:  pr.http3SendGreaseFrames,
	}
}

type racingResult struct {
	protocol  string
	response  *http.Response
	transport http.RoundTripper
	err       error
}
