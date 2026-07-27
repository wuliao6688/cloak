package tls_client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/fhttp/httputil"
	"github.com/bogdanfinn/tls-client/bandwidth"
	"github.com/bogdanfinn/tls-client/profiles"
	"golang.org/x/net/proxy"
)

var defaultRedirectFunc = func(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}

// TLSDialerFunc is a function that dials a TLS connection to the given address.
// It's used for WebSocket connections to ensure they use the same TLS fingerprinting
// as regular HTTP requests.
type TLSDialerFunc func(ctx context.Context, network, addr string) (net.Conn, error)

type HttpClient interface {
	GetCookies(u *url.URL) []*http.Cookie
	SetCookies(u *url.URL, cookies []*http.Cookie)
	SetCookieJar(jar http.CookieJar)
	GetCookieJar() http.CookieJar
	SetProxy(proxyUrl string) error
	GetProxy() string
	SetFollowRedirect(followRedirect bool)
	GetFollowRedirect() bool
	CloseIdleConnections()
	Do(req *http.Request) (*http.Response, error)
	Get(url string) (resp *http.Response, err error)
	Head(url string) (resp *http.Response, err error)
	Post(url, contentType string, body io.Reader) (resp *http.Response, err error)

	GetBandwidthTracker() bandwidth.BandwidthTracker
	GetDialer() proxy.ContextDialer
	GetTLSDialer() TLSDialerFunc

	AddPreRequestHook(hook PreRequestHookFunc)
	AddPostResponseHook(hook PostResponseHookFunc)
	ResetPreHooks()
	ResetPostHooks()
}

// Interface guards are a cheap way to make sure all methods are implemented, this is a static check and does not affect runtime performance.
var _ HttpClient = (*httpClient)(nil)

type httpClient struct {
	client           *http.Client
	clientState      atomic.Pointer[http.Client]
	logger           Logger
	debugLogs        bool
	bandwidthTracker bandwidth.BandwidthTracker
	config           *httpClientConfig
	stateLck         sync.RWMutex
	proxyMutationLck sync.Mutex
	dialer           proxy.ContextDialer

	preHooksLck  sync.RWMutex
	postHooksLck sync.RWMutex
	preHooks     []PreRequestHookFunc
	postHooks    []PostResponseHookFunc
}

var DefaultTimeoutSeconds = 30

const DefaultDebugBodyLimit int64 = 64 * 1024

var DefaultOptions = []HttpClientOption{
	WithTimeoutSeconds(DefaultTimeoutSeconds),
	WithClientProfile(profiles.DefaultClientProfile),
	WithRandomTLSExtensionOrder(),
	WithNotFollowRedirects(),
}

func ProvideDefaultClient(logger Logger) (HttpClient, error) {
	jar := NewCookieJar()
	options := make([]HttpClientOption, 0, len(DefaultOptions)+1)
	options = append(options, DefaultOptions...)
	options = append(options, WithCookieJar(jar))
	return NewHttpClient(logger, options...)
}

// NewHttpClient constructs a new HTTP client with the given logger and client options.
func NewHttpClient(logger Logger, options ...HttpClientOption) (HttpClient, error) {
	config := &httpClientConfig{
		followRedirects:    true,
		badPinHandler:      nil,
		customRedirectFunc: nil,
		defaultHeaders:     make(http.Header),
		connectHeaders:     make(http.Header),
		clientProfile:      profiles.DefaultClientProfile,
		timeout:            time.Duration(DefaultTimeoutSeconds) * time.Second,
		debugBodyLimit:     DefaultDebugBodyLimit,
	}

	for _, opt := range options {
		opt(config)
	}

	if err := validateConfig(config); err != nil {
		return nil, err
	}

	if config.debug {
		if logger == nil {
			logger = NewLogger()
		}

		logger = NewDebugLogger(logger)
	}

	if logger == nil {
		logger = NewNoopLogger()
	}

	client, dialer, bandwidthTracker, clientProfile, err := buildFromConfig(logger, config)
	if err != nil {
		return nil, err
	}

	config.clientProfile = clientProfile

	result := &httpClient{
		client:           client,
		logger:           logger,
		debugLogs:        loggerDebugEnabled(logger),
		config:           config,
		stateLck:         sync.RWMutex{},
		proxyMutationLck: sync.Mutex{},
		bandwidthTracker: bandwidthTracker,
		dialer:           dialer,
		preHooksLck:      sync.RWMutex{},
		postHooksLck:     sync.RWMutex{},
		preHooks:         append([]PreRequestHookFunc{}, config.preHooks...),
		postHooks:        append([]PostResponseHookFunc{}, config.postHooks...),
	}
	result.clientState.Store(client)
	return result, nil
}

func validateConfig(config *httpClientConfig) error {
	if config.enableProtocolRacing && config.disableHttp3 {
		return fmt.Errorf("%w: HTTP/3 racing cannot be enabled when HTTP/3 is disabled", ErrRacingNotSupported)
	}

	if config.enableProtocolRacing && config.forceHttp1 {
		return fmt.Errorf("%w: HTTP/3 racing cannot be enabled when HTTP/1 is forced", ErrRacingNotSupported)
	}

	if config.enableProtocolRacing && (config.proxyUrl != "" || config.proxyDialerFactory != nil) {
		return fmt.Errorf("%w: HTTP/3 racing cannot be combined with a TCP proxy because the HTTP/3 attempt would bypass it", ErrRacingNotSupported)
	}

	if config.enableProtocolRacing && config.dialContext != nil {
		return fmt.Errorf("%w: HTTP/3 racing cannot be combined with a custom TCP DialContext", ErrRacingNotSupported)
	}

	if config.enableProtocolRacing && config.localAddr != nil {
		return fmt.Errorf("%w: HTTP/3 racing cannot enforce WithLocalAddr on the QUIC attempt", ErrRacingNotSupported)
	}

	if config.enableProtocolRacing && (config.disableIPV4 || config.disableIPV6) {
		return fmt.Errorf("%w: HTTP/3 racing cannot enforce IP-family restrictions on the QUIC attempt", ErrRacingNotSupported)
	}

	if config.enableProtocolRacing && len(config.certificatePins) > 0 {
		return fmt.Errorf("%w: HTTP/3 racing cannot be combined with certificate pinning until QUIC pin verification is configured", ErrRacingNotSupported)
	}

	if config.enableProtocolRacing && config.enabledBandwidthTracker {
		return fmt.Errorf("%w: HTTP/3 racing cannot be combined with bandwidth tracking until QUIC traffic is tracked", ErrRacingNotSupported)
	}

	if config.debugBodyLimit < 0 {
		return fmt.Errorf("invalid config: debug body limit must not be negative")
	}
	if config.transportOptions != nil && config.transportOptions.MaxCachedTransports < -1 {
		return fmt.Errorf("invalid config: max cached transports must be -1, zero, or a positive value")
	}
	if config.transportOptions != nil && config.transportOptions.TLSClientSessionCacheSize < 0 {
		return fmt.Errorf("invalid config: TLS client session cache size must be zero or a positive value")
	}
	if config.transportOptions != nil && config.transportOptions.ProtocolRacingHTTP2Delay != nil && *config.transportOptions.ProtocolRacingHTTP2Delay < 0 {
		return fmt.Errorf("invalid config: protocol racing HTTP/2 delay must not be negative")
	}
	if config.transportOptions != nil && config.transportOptions.ProtocolRacingTimeout != nil && *config.transportOptions.ProtocolRacingTimeout <= 0 {
		return fmt.Errorf("invalid config: protocol racing timeout must be positive")
	}

	if config.disableIPV4 && config.disableIPV6 {
		return fmt.Errorf("invalid config: cannot disable both IPv4 and IPv6")
	}

	if len(config.certificatePins) > 0 && config.insecureSkipVerify {
		return fmt.Errorf("invalid config: certificate pinning cannot be used with insecure skip verify")
	}

	if config.proxyUrl != "" && config.proxyDialerFactory != nil {
		return fmt.Errorf("invalid config: cannot set both proxy URL and custom proxy dialer factory (only one will be used)")
	}

	if config.dialContext != nil && (config.proxyUrl != "" || config.proxyDialerFactory != nil) {
		return fmt.Errorf("invalid config: WithDialContext overrides the built-in proxy logic; if you use a custom dialer, you must handle the proxy connection (CONNECT handshake) yourself inside that dialer")
	}

	return nil
}

type customContextDialer struct {
	dialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

func (c *customContextDialer) Dial(network, addr string) (net.Conn, error) {
	return c.dialContext(context.Background(), network, addr)
}

func (c *customContextDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return c.dialContext(ctx, network, addr)
}

func buildFromConfig(logger Logger, config *httpClientConfig) (*http.Client, proxy.ContextDialer, bandwidth.BandwidthTracker, profiles.ClientProfile, error) {
	var dialer proxy.ContextDialer
	dialer = newDirectDialer(config.timeout, config.localAddr, config.dialer)

	if config.proxyUrl != "" && config.proxyDialerFactory == nil {
		proxyDialer, err := newConnectDialer(config.proxyUrl, config.timeout, config.localAddr, config.dialer, config.connectHeaders, logger)
		if err != nil {
			return nil, nil, nil, profiles.ClientProfile{}, err
		}

		dialer = proxyDialer
	}

	if config.proxyDialerFactory != nil {
		proxyDialer, err := config.proxyDialerFactory(config.proxyUrl, config.timeout, config.localAddr, config.connectHeaders, logger)
		if err != nil {
			return nil, nil, nil, profiles.ClientProfile{}, err
		}

		dialer = proxyDialer
	}

	// If a custom DialContext is provided, it takes precedence over everything.
	// This allows the user to have full control over the TCP connection (ZeroDNS, socket tracking, etc).
	if config.dialContext != nil {
		dialer = &customContextDialer{
			dialContext: config.dialContext,
		}
	}

	var redirectFunc func(req *http.Request, via []*http.Request) error
	if !config.followRedirects {
		redirectFunc = defaultRedirectFunc
	} else {
		redirectFunc = nil

		if config.customRedirectFunc != nil {
			redirectFunc = config.customRedirectFunc
		}
	}

	var bandwidthTracker bandwidth.BandwidthTracker
	if config.enabledBandwidthTracker {
		bandwidthTracker = bandwidth.NewTracker()
	} else {
		bandwidthTracker = bandwidth.NewNopeTracker()
	}

	clientProfile := config.clientProfile

	transport, err := newRoundTripper(clientProfile, config.transportOptions, config.serverNameOverwrite, config.insecureSkipVerify, config.withRandomTlsExtensionOrder, config.forceHttp1, config.disableHttp3, config.enableProtocolRacing, config.certificatePins, config.badPinHandler, config.disableIPV6, config.disableIPV4, bandwidthTracker, dialer)
	if err != nil {
		return nil, nil, nil, clientProfile, err
	}

	client := &http.Client{
		Timeout:       config.timeout,
		Transport:     transport,
		CheckRedirect: redirectFunc,
	}

	if config.cookieJar != nil {
		client.Jar = config.cookieJar
	}

	return client, dialer, bandwidthTracker, clientProfile, nil
}

// CloseIdleConnections closes all idle connections of the underlying http client.
func (c *httpClient) CloseIdleConnections() {
	client := c.snapshotClient()
	client.CloseIdleConnections()
}

// GetDialer() returns the underlying Dialer
func (c *httpClient) GetDialer() proxy.ContextDialer {
	c.stateLck.RLock()
	defer c.stateLck.RUnlock()
	return c.dialer
}

// GetTLSDialer returns a TLS dialer function that uses the same TLS fingerprinting
// as regular HTTP requests. This is essential for WebSocket connections to maintain
// consistent fingerprinting.
func (c *httpClient) GetTLSDialer() TLSDialerFunc {
	// Get the roundTripper from the client's transport
	c.stateLck.RLock()
	transport := c.client.Transport
	dialer := c.dialer
	c.stateLck.RUnlock()

	rt, ok := transport.(*roundTripper)
	if !ok {
		// Fallback to a simple TLS dialer if the transport is not a roundTripper
		return func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		}
	}

	// Return a function that uses the roundTripper's dialTLSForWebsocket method
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return rt.dialTLSForWebsocket(ctx, network, addr)
	}
}

// SetFollowRedirect configures the client's HTTP redirect following policy.
func (c *httpClient) SetFollowRedirect(followRedirect bool) {
	c.stateLck.Lock()
	defer c.stateLck.Unlock()

	if c.debugLogs {
		c.logger.Debug("set follow redirect from %v to %v", c.config.followRedirects, followRedirect)
	}

	c.config.followRedirects = followRedirect
	c.applyFollowRedirect()
}

// GetFollowRedirect returns the client's HTTP redirect following policy.
func (c *httpClient) GetFollowRedirect() bool {
	c.stateLck.RLock()
	defer c.stateLck.RUnlock()
	return c.config.followRedirects
}

func (c *httpClient) applyFollowRedirect() {
	var redirectFunc func(req *http.Request, via []*http.Request) error
	if c.config.followRedirects {
		if c.debugLogs {
			c.logger.Debug("automatic redirect following is enabled")
		}
	} else {
		if c.debugLogs {
			c.logger.Debug("automatic redirect following is disabled")
		}
		redirectFunc = defaultRedirectFunc
	}

	if c.config.customRedirectFunc != nil && c.config.followRedirects {
		redirectFunc = c.config.customRedirectFunc
	}

	currentClient := c.client
	c.client = &http.Client{
		Timeout:       currentClient.Timeout,
		Transport:     currentClient.Transport,
		CheckRedirect: redirectFunc,
		Jar:           currentClient.Jar,
	}
	c.clientState.Store(c.client)
}

// SetProxy configures the client to use the given proxy URL.
//
// proxyUrl should be formatted as:
//
//	"http://user:pass@host:port"
func (c *httpClient) SetProxy(proxyUrl string) error {
	c.proxyMutationLck.Lock()
	defer c.proxyMutationLck.Unlock()

	c.stateLck.RLock()
	currentProxy := c.config.proxyUrl
	enableProtocolRacing := c.config.enableProtocolRacing
	hasCustomDialContext := c.config.dialContext != nil
	c.stateLck.RUnlock()

	if currentProxy == proxyUrl {
		return nil
	}
	if proxyUrl != "" && enableProtocolRacing {
		return ErrRacingNotSupported
	}
	if proxyUrl != "" && hasCustomDialContext {
		return errors.New("a proxy cannot be applied dynamically when WithDialContext is configured")
	}

	if c.debugLogs {
		c.logger.Debug("set proxy from %s to %s", currentProxy, proxyUrl)
	}
	dialer, transport, err := c.buildProxyTransport(proxyUrl)
	if err != nil {
		c.logger.Error("failed to apply new proxy. keeping previous proxy: %v", err)
		return err
	}

	c.stateLck.Lock()
	oldTransport := c.client.Transport
	currentClient := c.client
	c.client = &http.Client{
		Timeout:       currentClient.Timeout,
		Transport:     transport,
		CheckRedirect: currentClient.CheckRedirect,
		Jar:           currentClient.Jar,
	}
	c.dialer = dialer
	c.config.proxyUrl = proxyUrl
	c.clientState.Store(c.client)
	c.stateLck.Unlock()

	closeIdleTransport(oldTransport)
	return nil
}

// GetProxy returns the proxy URL used by the client.
func (c *httpClient) GetProxy() string {
	c.stateLck.RLock()
	defer c.stateLck.RUnlock()
	return c.config.proxyUrl
}

func (c *httpClient) buildProxyTransport(proxyUrl string) (proxy.ContextDialer, http.RoundTripper, error) {
	var dialer proxy.ContextDialer
	dialer = newDirectDialer(c.config.timeout, c.config.localAddr, c.config.dialer)

	if proxyUrl != "" && c.config.proxyDialerFactory == nil {
		if c.debugLogs {
			c.logger.Debug("proxy url %s supplied - using proxy connect dialer", proxyUrl)
		}
		proxyDialer, err := newConnectDialer(proxyUrl, c.config.timeout, c.config.localAddr, c.config.dialer, c.config.connectHeaders, c.logger)
		if err != nil {
			c.logger.Error("failed to create proxy connect dialer: %s", err.Error())
			return nil, nil, err
		}

		dialer = proxyDialer
	}

	if c.config.proxyDialerFactory != nil {
		if c.debugLogs {
			c.logger.Debug("using custom proxy connect dialer")
		}
		proxyDialer, err := c.config.proxyDialerFactory(proxyUrl, c.config.timeout, c.config.localAddr, c.config.connectHeaders, c.logger)
		if err != nil {
			c.logger.Error("failed to create proxy connect dialer: %s", err.Error())
			return nil, nil, err
		}

		dialer = proxyDialer
	}

	if c.config.dialContext != nil {
		dialer = &customContextDialer{
			dialContext: c.config.dialContext,
		}
	}

	transport, err := newRoundTripper(c.config.clientProfile, c.config.transportOptions, c.config.serverNameOverwrite, c.config.insecureSkipVerify, c.config.withRandomTlsExtensionOrder, c.config.forceHttp1, c.config.disableHttp3, c.config.enableProtocolRacing, c.config.certificatePins, c.config.badPinHandler, c.config.disableIPV6, c.config.disableIPV4, c.bandwidthTracker, dialer)
	if err != nil {
		return nil, nil, err
	}

	return dialer, transport, nil
}

// GetCookies returns the cookies in the client's cookie jar for a given URL.
func (c *httpClient) GetCookies(u *url.URL) []*http.Cookie {
	if u == nil {
		c.logger.Warn("cannot get cookies for a nil URL")
		return nil
	}
	if c.debugLogs {
		c.logger.Debug("get cookies for url: %s", u.String())
	}
	jar := c.snapshotClient().Jar
	if jar == nil {
		c.logger.Warn("you did not setup a cookie jar")
		return nil
	}

	return jar.Cookies(u)
}

// SetCookies sets a list of cookies for a given URL in the client's cookie jar.
func (c *httpClient) SetCookies(u *url.URL, cookies []*http.Cookie) {
	if u == nil {
		c.logger.Warn("cannot set cookies for a nil URL")
		return
	}
	if c.debugLogs {
		c.logger.Debug("set cookies for url: %s", u.String())
	}

	jar := c.snapshotClient().Jar
	if jar == nil {
		c.logger.Warn("you did not setup a cookie jar")
		return
	}

	jar.SetCookies(u, cookies)
}

// SetCookieJar sets a jar as the clients cookie jar. This is the recommended way when you want to "clear" the existing cookiejar
func (c *httpClient) SetCookieJar(jar http.CookieJar) {
	c.stateLck.Lock()
	defer c.stateLck.Unlock()
	currentClient := c.client
	c.client = &http.Client{
		Timeout:       currentClient.Timeout,
		Transport:     currentClient.Transport,
		CheckRedirect: currentClient.CheckRedirect,
		Jar:           jar,
	}
	c.config.cookieJar = jar
	c.clientState.Store(c.client)
}

// GetCookieJar returns the jar the client is currently using
func (c *httpClient) GetCookieJar() http.CookieJar {
	return c.snapshotClient().Jar
}

// GetBandwidthTracker returns the bandwidth tracker
func (c *httpClient) GetBandwidthTracker() bandwidth.BandwidthTracker {
	return c.bandwidthTracker
}

// AddPreRequestHook adds a pre-request hook that is called before each request is sent.
// Multiple hooks can be added and they will be executed in the order they were added.
// If any hook returns an error, the request is aborted and subsequent hooks are not called.
// This method is thread-safe.
func (c *httpClient) AddPreRequestHook(hook PreRequestHookFunc) {
	c.preHooksLck.Lock()
	defer c.preHooksLck.Unlock()
	c.preHooks = append(c.preHooks, hook)
}

// AddPostResponseHook adds a post-response hook that is called after each request completes.
// Multiple hooks can be added and they will be executed in the order they were added.
// All hooks are always executed, even if the request failed or a previous hook panicked.
// This method is thread-safe.
func (c *httpClient) AddPostResponseHook(hook PostResponseHookFunc) {
	c.postHooksLck.Lock()
	defer c.postHooksLck.Unlock()
	c.postHooks = append(c.postHooks, hook)
}

func (c *httpClient) ResetPreHooks() {
	c.preHooksLck.Lock()
	defer c.preHooksLck.Unlock()
	c.preHooks = []PreRequestHookFunc{}
}

func (c *httpClient) ResetPostHooks() {
	c.postHooksLck.Lock()
	defer c.postHooksLck.Unlock()
	c.postHooks = []PostResponseHookFunc{}
}

// executePreHooks runs all registered pre-request hooks in order.
// Returns an error if any hook returns an error or panics, aborting subsequent hooks.
// If a hook returns an error wrapping ErrContinueHooks, the error is logged and
// execution continues to the next hook.
func (c *httpClient) executePreHooks(req *http.Request) error {
	c.preHooksLck.RLock()
	hooks := c.preHooks
	c.preHooksLck.RUnlock()

	for _, hook := range hooks {
		if err := c.runPreHook(hook, req); err != nil {
			if errors.Is(err, ErrContinueHooks) {
				c.logger.Warn("pre-request hook error (continuing): %v", err)
				continue
			}
			return err
		}
	}
	return nil
}

func (c *httpClient) runPreHook(hook PreRequestHookFunc, req *http.Request) (err error) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("panic in pre-request hook: %v", r)
			err = fmt.Errorf("panic in pre-request hook: %v", r)
		}
	}()
	return hook(req)
}

// executePostHooks runs all registered post-response hooks in order.
// If any hook returns an error or panics, subsequent hooks are not called,
// unless the error wraps ErrContinueHooks.
func (c *httpClient) executePostHooks(originalReq *http.Request, resp *http.Response, requestErr error) {
	c.postHooksLck.RLock()
	hooks := c.postHooks
	c.postHooksLck.RUnlock()

	if len(hooks) == 0 {
		return
	}

	ctx := &PostResponseContext{
		Request:  originalReq,
		Response: resp,
		Error:    requestErr,
	}

	for _, hook := range hooks {
		if err := c.runPostHook(hook, ctx); err != nil {
			if errors.Is(err, ErrContinueHooks) {
				c.logger.Warn("post-response hook error (continuing): %v", err)
				continue
			}
			c.logger.Error("post-response hook error: %v", err)
			return
		}
	}
}

func (c *httpClient) runPostHook(hook PostResponseHookFunc, ctx *PostResponseContext) (err error) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("panic in post-response hook: %v", r)
			err = fmt.Errorf("panic in post-response hook: %v", r)
		}
	}()
	return hook(ctx)
}

// Do issues a given HTTP request and returns the corresponding response.
//
// If the returned error is nil, the response contains a non-nil body, which the user is expected to close.
func (c *httpClient) Do(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, ErrRequestNil
	}

	if err := c.executePreHooks(req); err != nil {
		closeRequestBody(req)
		return nil, err
	}

	resp, err := c.do(req)

	c.executePostHooks(req, resp, err)

	return resp, err
}

func (c *httpClient) do(req *http.Request) (resp *http.Response, requestErr error) {
	if c.config.catchPanics {
		defer func() {
			recovered := recover()

			if recovered != nil {
				closeRequestBody(req)
				if resp != nil && resp.Body != nil {
					_ = resp.Body.Close()
				}
				resp = nil
				requestErr = fmt.Errorf("panic occurred in tls client request handling: %v", recovered)
			}

			if recovered != nil && c.config.debug {
				c.logger.Debug(requestErr.Error())
			}

			if recovered != nil && !c.config.debug {
				c.logger.Info("critical error during request handling")
			}
		}()
	}

	if req.Header == nil {
		req.Header = make(http.Header)
	}

	// Merge profile/client defaults field by field. Request-specific values
	// always win; setting one header must not discard every other default.
	mergeDefaultHeaders(req.Header, c.config.defaultHeaders)

	// Header order must be lowercase. Avoid inserting an empty ordering key and
	// only allocate a replacement slice if an element actually needs changing.
	if order, ok := req.Header[http.HeaderOrderKey]; ok {
		req.Header[http.HeaderOrderKey] = allToLower(order)
	}

	if c.config.debug {
		if err := c.logDebugRequest(req); err != nil {
			closeRequestBody(req)
			return nil, err
		}
	}

	client := c.snapshotClient()
	resp, err := client.Do(req)
	if err != nil {
		if c.debugLogs {
			c.logger.Debug("failed to do request: %s", err.Error())
		}
		return nil, err
	}

	if c.debugLogs {
		c.logger.Debug("headers on request:\n%v", req.Header)
		if resp.Request != nil {
			c.logger.Debug("cookies on request:\n%v", resp.Request.Cookies())
		}
		c.logger.Debug("headers on response:\n%v", resp.Header)
		c.logger.Debug("cookies on response:\n%v", resp.Cookies())
		c.logger.Debug("requested %s : status %d", req.URL.String(), resp.StatusCode)
	}

	if c.config.debug {
		responseBytes, err := httputil.DumpResponse(resp, false)
		if err != nil {
			if resp.Body != nil {
				_ = resp.Body.Close()
			}
			return nil, err
		}

		if resp.Body != nil && c.config.debugBodyLimit > 0 {
			resp.Body = newDebugPreviewBody(resp.Body, c.logger, c.config.debugBodyLimit)
		}

		c.logger.Debug("raw response header bytes received: %d (%d kb)", len(responseBytes), len(responseBytes)/1024)
	}

	return resp, nil
}

func (c *httpClient) logDebugRequest(req *http.Request) error {
	debugReq := req.Clone(context.Background())
	debugReq.Body = nil
	debugReq.GetBody = nil
	debugReq.ContentLength = 0

	requestBytes, err := httputil.DumpRequestOut(debugReq, false)
	if err != nil {
		return err
	}
	c.logger.Debug("raw request header bytes sent: %d (%d kb)", len(requestBytes), len(requestBytes)/1024)

	if req.Body == nil || c.config.debugBodyLimit == 0 {
		return nil
	}
	if req.GetBody == nil {
		c.logger.Debug("request body preview omitted: body is not replayable")
		return nil
	}

	body, err := req.GetBody()
	if err != nil {
		return fmt.Errorf("failed to open request body for debug preview: %w", err)
	}
	defer body.Close()

	preview, truncated, err := readBodyPreview(body, c.config.debugBodyLimit)
	if err != nil {
		return fmt.Errorf("failed to read request body debug preview: %w", err)
	}
	logBodyPreview(c.logger, "request", preview, truncated)
	return nil
}

func readBodyPreview(reader io.Reader, limit int64) ([]byte, bool, error) {
	if reader == nil || limit <= 0 {
		return nil, false, nil
	}

	readLimit := limit
	if limit < maxInt64Value {
		readLimit++
	}
	preview, err := io.ReadAll(io.LimitReader(reader, readLimit))
	if err != nil {
		return nil, false, err
	}
	if int64(len(preview)) <= limit {
		return preview, false, nil
	}
	return preview[:int(limit)], true, nil
}

type debugPreviewBody struct {
	body      io.ReadCloser
	logger    Logger
	limit     int64
	preview   []byte
	truncated bool
	once      sync.Once
}

func newDebugPreviewBody(body io.ReadCloser, logger Logger, limit int64) io.ReadCloser {
	return &debugPreviewBody{
		body:    body,
		logger:  logger,
		limit:   limit,
		preview: make([]byte, 0, previewCapacity(limit)),
	}
}

func (b *debugPreviewBody) Read(buffer []byte) (int, error) {
	read, err := b.body.Read(buffer)
	if read > 0 {
		remaining := b.limit - int64(len(b.preview))
		if remaining > 0 {
			capture := int64(read)
			if capture > remaining {
				capture = remaining
			}
			b.preview = append(b.preview, buffer[:capture]...)
		}
		if int64(read) > remaining {
			b.truncated = true
		}
	}
	if err != nil {
		b.log()
	}
	return read, err
}

func (b *debugPreviewBody) Close() error {
	err := b.body.Close()
	b.log()
	return err
}

func (b *debugPreviewBody) log() {
	b.once.Do(func() {
		logBodyPreview(b.logger, "response", b.preview, b.truncated)
	})
}

func logBodyPreview(logger Logger, direction string, preview []byte, truncated bool) {
	suffix := ""
	if truncated {
		suffix = " (truncated)"
	}
	logger.Debug("%s body preview%s: %s", direction, suffix, string(preview))
}

const maxInt64Value = int64(1<<63 - 1)

func previewCapacity(limit int64) int {
	if limit <= 0 {
		return 0
	}
	if limit > 4096 {
		return 4096
	}
	return int(limit)
}

func (c *httpClient) snapshotClient() *http.Client {
	if client := c.clientState.Load(); client != nil {
		return client
	}
	c.stateLck.RLock()
	defer c.stateLck.RUnlock()
	return c.client
}

func closeIdleTransport(transport http.RoundTripper) {
	if transport == nil {
		return
	}
	if closeIdler, ok := transport.(interface{ CloseIdleConnections() }); ok {
		closeIdler.CloseIdleConnections()
	}
}

func (c *httpClient) Get(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return c.Do(req)
}

func (c *httpClient) Head(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return nil, err
	}

	return c.Do(req)
}

func (c *httpClient) Post(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)

	return c.Do(req)
}

func allToLower(list []string) []string {
	var lower []string
	for i, elem := range list {
		normalized := strings.ToLower(elem)
		if normalized == elem {
			continue
		}
		if lower == nil {
			lower = append([]string(nil), list...)
		}
		lower[i] = normalized
	}
	if lower == nil {
		return list
	}
	return lower
}

func headerContainsFold(headers http.Header, key string) bool {
	if _, ok := headers[key]; ok {
		return true
	}
	for existingKey := range headers {
		if strings.EqualFold(existingKey, key) {
			return true
		}
	}
	return false
}

func mergeDefaultHeaders(headers, defaults http.Header) {
	if len(defaults) == 0 {
		return
	}

	// A linear EqualFold scan avoids building a normalization map for the small
	// header sets used by normal requests. Fall back to O(n) map construction
	// for unusually large request/default combinations.
	useLinearScan := len(headers) == 0 || len(headers) <= 64/len(defaults)
	if useLinearScan {
		for key, values := range defaults {
			if headerContainsFold(headers, key) {
				continue
			}
			headers[key] = append([]string(nil), values...)
		}
		return
	}

	existingHeaders := make(map[string]struct{}, len(headers)+len(defaults))
	for key := range headers {
		existingHeaders[strings.ToLower(key)] = struct{}{}
	}
	for key, values := range defaults {
		normalizedKey := strings.ToLower(key)
		if _, exists := existingHeaders[normalizedKey]; exists {
			continue
		}
		headers[key] = append([]string(nil), values...)
		existingHeaders[normalizedKey] = struct{}{}
	}
}
