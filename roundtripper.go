package tls_client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/fhttp/http2"
	"github.com/bogdanfinn/quic-go-utls/http3"
	"github.com/bogdanfinn/tls-client/bandwidth"
	"github.com/bogdanfinn/tls-client/profiles"
	tls "github.com/bogdanfinn/utls"
	"golang.org/x/net/proxy"
)

const defaultIdleConnectionTimeout = 90 * time.Second
const CHROME_MAX_FIELD_SECTION_SIZE = 262144
const DefaultTLSClientSessionCacheSize = 32

var errProtocolNegotiated = errors.New("protocol negotiated")

type roundTripper struct {
	// Embedded groups — field names promoted for backward compatibility.
	// Group order intentionally mirrors the original field layout.
	rtH2Params    // initialStreamID, allowHTTP, settings, headerPriority, etc.
	rtTLSParams   // clientHelloId, certificatePinner, clientSessionCache, etc.
	rtCacheState  // cachedConnections, cachedTransports, transportCache, locks
	rtH2DialState // http2DialContexts, http2DialCancels, http2DialContextSeq
	rtH3Params    // http3Settings, http3SettingsOrder, http3PriorityParam, etc.
	rtProtoFlags  // forceHttp1, disableHttp3, disableIPV4, disableIPV6

	dialer           proxy.ContextDialer
	bandwidthTracker bandwidth.BandwidthTracker
	transportOptions *TransportOptions

	// racer handles HTTP/3 racing (nil if racing is disabled)
	racer *protocolRacer
}

// http3Config contains all parameters needed to build an HTTP/3 transport
type http3Config struct {
	clientSessionCache     tls.ClientSessionCache
	insecureSkipVerify     bool
	serverNameOverwrite    string
	transportOptions       *TransportOptions
	http3Settings          map[uint64]uint64
	http3SettingsOrder     []uint64
	http3PriorityParam     uint32
	http3PseudoHeaderOrder []string
	http3SendGreaseFrames  bool
}

func (rt *roundTripper) CloseIdleConnections() {
	rt.cachedConnectionsLck.Lock()
	connections := make([]net.Conn, 0, len(rt.cachedConnections))
	for _, connection := range rt.cachedConnections {
		connections = append(connections, connection)
	}
	rt.cachedConnections = make(map[string]net.Conn)
	rt.cachedConnectionsLck.Unlock()
	for _, connection := range connections {
		_ = connection.Close()
	}

	rt.cachedTransportsLck.Lock()
	transports := make([]http.RoundTripper, 0, len(rt.cachedTransports))
	for key, transport := range rt.cachedTransports {
		transports = append(transports, transport)
		delete(rt.cachedTransports, key)
	}
	resetTransportCacheMeta(rt.transportCache)
	rt.cachedTransportsLck.Unlock()

	if rt.racer != nil {
		rt.racer.resetProtocolCache()
	}

	for _, transport := range transports {
		closeIdleTransport(transport)
	}
}

func (rt *roundTripper) getCachedTransport(key string) (http.RoundTripper, bool) {
	return getCachedTransportEntry(rt.cachedTransports, &rt.cachedTransportsLck, rt.transportCache, key)
}

func (rt *roundTripper) setCachedTransport(key string, transport http.RoundTripper) {
	evicted := setCachedTransportEntry(rt.cachedTransports, &rt.cachedTransportsLck, rt.transportCache, key, transport)
	for _, entry := range evicted {
		if entry.removed && rt.racer != nil {
			rt.racer.clearProtocolCacheForTransportKey(entry.key)
		}
		closeIdleTransport(entry.transport)
	}
}

func (rt *roundTripper) takeCachedConnection(addr string) net.Conn {
	rt.cachedConnectionsLck.Lock()
	defer rt.cachedConnectionsLck.Unlock()
	conn := rt.cachedConnections[addr]
	delete(rt.cachedConnections, addr)
	return conn
}

func (rt *roundTripper) cacheConnection(addr string, conn net.Conn) {
	rt.cachedConnectionsLck.Lock()
	previous := rt.cachedConnections[addr]
	rt.cachedConnections[addr] = conn
	rt.cachedConnectionsLck.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
}

func (rt *roundTripper) getHttp3Settings() map[uint64]uint64 {
	if len(rt.http3Settings) == 0 {
		return nil
	}

	// Build settings in the correct order
	orderedSettings := make(map[uint64]uint64)
	if len(rt.http3SettingsOrder) > 0 {
		for _, id := range rt.http3SettingsOrder {
			if val, ok := rt.http3Settings[id]; ok {
				orderedSettings[id] = val
			}
		}
		return orderedSettings
	}

	return rt.http3Settings
}

func buildHTTP3Transport(cfg *http3Config) (http.RoundTripper, error) {
	utlsConfig := &tls.Config{
		ClientSessionCache: cfg.clientSessionCache,
		InsecureSkipVerify: cfg.insecureSkipVerify,
		OmitEmptyPsk:       true,
	}
	if cfg.transportOptions != nil {
		utlsConfig.RootCAs = cfg.transportOptions.RootCAs
		utlsConfig.KeyLogWriter = cfg.transportOptions.KeyLogWriter
		utlsConfig.Certificates = cfg.transportOptions.Certificates
	}

	if cfg.serverNameOverwrite != "" {
		utlsConfig.ServerName = cfg.serverNameOverwrite
	}

	t3 := &http3.Transport{
		TLSClientConfig: utlsConfig,
		EnableDatagrams: true, // Chrome enables H3_DATAGRAM (setting 0x33)
	}

	http3Settings := cfg.http3Settings

	if http3Settings != nil {
		settingsCopy := make(map[uint64]uint64, len(http3Settings))
		for k, v := range http3Settings {
			settingsCopy[k] = v
		}
		http3Settings = settingsCopy
	}

	// Add random GREASE setting only for browsers that send it (Chrome)
	// Firefox sends GREASE frames but not random GREASE settings
	// Use priority parameter as identification: Chrome has it, Firefox doesn't
	if cfg.http3PriorityParam > 0 {
		greaseID := generateGREASESettingID()
		greaseValue := generateGREASESettingValue()

		if http3Settings == nil {
			http3Settings = make(map[uint64]uint64)
		}
		http3Settings[greaseID] = greaseValue

		// Set the order if available, and append GREASE at the end
		if len(cfg.http3SettingsOrder) > 0 {
			orderWithGrease := make([]uint64, len(cfg.http3SettingsOrder)+1)
			copy(orderWithGrease, cfg.http3SettingsOrder)
			orderWithGrease[len(cfg.http3SettingsOrder)] = greaseID
			t3.AdditionalSettingsOrder = orderWithGrease
		}
	} else {
		// Just use the settings order as-is without random GREASE
		if len(cfg.http3SettingsOrder) > 0 {
			t3.AdditionalSettingsOrder = cfg.http3SettingsOrder
		}
	}

	t3.AdditionalSettings = http3Settings

	if len(cfg.http3PseudoHeaderOrder) > 0 {
		t3.PseudoHeaderOrder = cfg.http3PseudoHeaderOrder
	}

	// Enable GREASE frames based on profile (Chrome sends GREASE frames, Firefox doesn't)
	t3.SendGreaseFrames = cfg.http3SendGreaseFrames

	t3.PriorityParam = cfg.http3PriorityParam

	if cfg.transportOptions != nil {
		t3.DisableCompression = cfg.transportOptions.DisableCompression

		maxResponseHeaderBytes, convErr := Int64ToInt(cfg.transportOptions.MaxResponseHeaderBytes)
		if convErr != nil {
			return nil, fmt.Errorf("error converting MaxResponseHeaderBytes to int: %w", convErr)
		}

		if maxResponseHeaderBytes > 0 {
			t3.MaxResponseHeaderBytes = maxResponseHeaderBytes
		} else if maxResponseHeaderBytes < 0 {
			// -1 means don't send SETTINGS_MAX_FIELD_SECTION_SIZE (Firefox behavior)
			t3.MaxResponseHeaderBytes = -1
		} else {
			t3.MaxResponseHeaderBytes = profileDefaultMaxResponseHeaderBytes(cfg)
		}
	} else {
		t3.MaxResponseHeaderBytes = profileDefaultMaxResponseHeaderBytes(cfg)
	}

	return newRetiringHTTP3Transport(t3), nil
}

func profileDefaultMaxResponseHeaderBytes(cfg *http3Config) int {
	if cfg.http3PriorityParam > 0 {
		return CHROME_MAX_FIELD_SECTION_SIZE
	}
	return -1
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	addr := rt.getDialTLSAddr(req)

	if rt.racer != nil && !rt.forceHttp1 && !rt.disableHttp3 && strings.ToLower(req.URL.Scheme) == "https" {
		return rt.racer.race(req, addr, rt.getTransport)
	}

	t, err := rt.getOrCreateTransport(req, addr)
	if err != nil {
		closeRequestBody(req)
		if errors.Is(err, ErrBadPinDetected) && rt.badPinHandlerFunc != nil {
			rt.badPinHandlerFunc(req)
		}
		return nil, err
	}

	return t.RoundTrip(req)
}

func (rt *roundTripper) getOrCreateTransport(req *http.Request, addr string) (http.RoundTripper, error) {
	if transport, ok := rt.getCachedTransport(addr); ok {
		return transport, nil
	}

	release, err := rt.transportInit.Lock(req.Context(), addr)
	if err != nil {
		return nil, err
	}
	defer release()

	if transport, ok := rt.getCachedTransport(addr); ok {
		return transport, nil
	}

	if err := rt.getTransport(req, addr); err != nil {
		return nil, err
	}

	transport, ok := rt.getCachedTransport(addr)
	if !ok || transport == nil {
		return nil, fmt.Errorf("transport initialization completed without a transport for %s", addr)
	}
	return transport, nil
}

func (rt *roundTripper) getTransport(req *http.Request, addr string) error {
	switch strings.ToLower(req.URL.Scheme) {
	case "http":
		rt.setCachedTransport(addr, rt.buildHttp1Transport())
		return nil
	case "https":
	default:
		return fmt.Errorf("invalid URL scheme: [%v]", req.URL.Scheme)
	}

	_, err := rt.dialTLS(req.Context(), "tcp", addr)
	switch err {
	case errProtocolNegotiated:
	case nil:
		return errors.New("dialTLS returned no protocol negotiation result")
	default:
		return err
	}

	return nil
}

func (rt *roundTripper) dialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// If we have the connection from when we determined the HTTPS
	// cachedTransports to use, return that.
	if conn := rt.takeCachedConnection(addr); conn != nil {
		return conn, nil
	}

	if network == "tcp" && rt.disableIPV6 {
		network = "tcp4"
	}

	if network == "tcp" && rt.disableIPV4 {
		network = "tcp6"
	}

	rawConn, err := rt.dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	var host string
	if host, _, err = net.SplitHostPort(addr); err != nil {
		host = addr
	}

	if rt.serverNameOverwrite != "" {
		host = rt.serverNameOverwrite
	}

	tlsConfig := &tls.Config{ClientSessionCache: rt.clientSessionCache, ServerName: host, InsecureSkipVerify: rt.insecureSkipVerify, OmitEmptyPsk: true}
	if rt.transportOptions != nil {
		tlsConfig.RootCAs = rt.transportOptions.RootCAs
		tlsConfig.KeyLogWriter = rt.transportOptions.KeyLogWriter
		tlsConfig.Certificates = rt.transportOptions.Certificates
	}

	rawConn = rt.bandwidthTracker.TrackConnection(ctx, rawConn)

	conn := tls.UClient(rawConn, tlsConfig, rt.clientHelloId, rt.withRandomTlsExtensionOrder, rt.forceHttp1, rt.disableHttp3)
	if err = conn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()

		return nil, err
	}

	err = rt.certificatePinner.Pin(conn, host)

	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if _, ok := rt.getCachedTransport(addr); ok {
		return conn, nil
	}

	// No http.Transport constructed yet, create one based on the results
	// of ALPN if no http1 is enforced.

	switch conn.ConnectionState().NegotiatedProtocol {
	case http2.NextProtoTLS:
		utlsConfig := &tls.Config{ClientSessionCache: rt.clientSessionCache, InsecureSkipVerify: rt.insecureSkipVerify, OmitEmptyPsk: true}
		if rt.transportOptions != nil {
			utlsConfig.RootCAs = rt.transportOptions.RootCAs
			utlsConfig.KeyLogWriter = rt.transportOptions.KeyLogWriter
			utlsConfig.Certificates = rt.transportOptions.Certificates
		}

		if rt.serverNameOverwrite != "" {
			utlsConfig.ServerName = rt.serverNameOverwrite
		}

		idleConnectionTimeout := defaultIdleConnectionTimeout

		if rt.transportOptions != nil && rt.transportOptions.IdleConnTimeout != nil {
			idleConnectionTimeout = *rt.transportOptions.IdleConnTimeout
		}

		t2 := http2.Transport{
			DialTLS:         rt.dialTLSHTTP2,
			TLSClientConfig: utlsConfig,
			ConnectionFlow:  rt.connectionFlow,
			HeaderPriority:  rt.headerPriority,
			IdleConnTimeout: idleConnectionTimeout,
			InitialStreamID: rt.initialStreamID,
			AllowHTTP:       rt.allowHTTP,
		}

		if rt.transportOptions != nil {
			t2.DisableCompression = rt.transportOptions.DisableCompression

			t1 := t2.GetT1()
			if t1 != nil {
				t1.DisableKeepAlives = rt.transportOptions.DisableKeepAlives
				t1.DisableCompression = rt.transportOptions.DisableCompression
				t1.MaxIdleConns = rt.transportOptions.MaxIdleConns
				t1.MaxIdleConnsPerHost = rt.transportOptions.MaxIdleConnsPerHost
				t1.MaxConnsPerHost = rt.transportOptions.MaxConnsPerHost
				// Only set MaxResponseHeaderBytes if > 0 (HTTP/1.1 transport doesn't understand -1)
				if rt.transportOptions.MaxResponseHeaderBytes > 0 {
					t1.MaxResponseHeaderBytes = rt.transportOptions.MaxResponseHeaderBytes
				}
				t1.WriteBufferSize = rt.transportOptions.WriteBufferSize
				t1.ReadBufferSize = rt.transportOptions.ReadBufferSize
				t1.IdleConnTimeout = idleConnectionTimeout
			}
		}

		if rt.pseudoHeaderOrder == nil {
			t2.PseudoHeaderOrder = []string{}
		} else {
			t2.PseudoHeaderOrder = rt.pseudoHeaderOrder
		}

		if rt.settings == nil {
			// when we not provide a map of custom http2 settings
			t2.Settings = map[http2.SettingID]uint32{
				http2.SettingMaxConcurrentStreams: 1000,
				http2.SettingMaxFrameSize:         16384,
				http2.SettingInitialWindowSize:    6291456,
				http2.SettingHeaderTableSize:      65536,
			}

			keys := make([]http2.SettingID, len(t2.Settings))

			i := 0
			// attention: the order might be random here for default values!
			for k := range t2.Settings {
				keys[i] = k
				i++
			}

			t2.SettingsOrder = keys
		} else {
			// use custom http2 settings
			t2.Settings = rt.settings
			t2.SettingsOrder = rt.settingsOrder
		}

		t2.Priorities = rt.priorities

		t2.PushHandler = &http2.DefaultPushHandler{}
		rt.setCachedTransport(addr, &contextAwareHTTP2Transport{transport: &t2, owner: rt, addr: addr})
	case http3.NextProtoH3:
		t3, err := buildHTTP3Transport(&http3Config{
			clientSessionCache:     rt.clientSessionCache,
			insecureSkipVerify:     rt.insecureSkipVerify,
			serverNameOverwrite:    rt.serverNameOverwrite,
			transportOptions:       rt.transportOptions,
			http3Settings:          rt.getHttp3Settings(),
			http3SettingsOrder:     rt.http3SettingsOrder,
			http3PriorityParam:     rt.http3PriorityParam,
			http3PseudoHeaderOrder: rt.http3PseudoHeaderOrder,
			http3SendGreaseFrames:  rt.http3SendGreaseFrames,
		})
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		rt.setCachedTransport(addr, t3)
	default:
		rt.setCachedTransport(addr, rt.buildHttp1Transport())
	}

	// Stash the connection just established for use servicing the
	// actual request (should be near-immediate).
	rt.cacheConnection(addr, conn)

	return nil, errProtocolNegotiated
}

func (rt *roundTripper) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	if network == "tcp" && rt.disableIPV6 {
		network = "tcp4"
	}
	if network == "tcp" && rt.disableIPV4 {
		network = "tcp6"
	}
	return rt.dialer.DialContext(ctx, network, addr)
}

func (rt *roundTripper) buildHttp1Transport() *http.Transport {
	utlsConfig := &tls.Config{ClientSessionCache: rt.clientSessionCache, InsecureSkipVerify: rt.insecureSkipVerify, OmitEmptyPsk: true}
	if rt.transportOptions != nil {
		utlsConfig.RootCAs = rt.transportOptions.RootCAs
		utlsConfig.Certificates = rt.transportOptions.Certificates
	}

	if rt.serverNameOverwrite != "" {
		utlsConfig.ServerName = rt.serverNameOverwrite
	}

	idleConnectionTimeout := defaultIdleConnectionTimeout

	if rt.transportOptions != nil && rt.transportOptions.IdleConnTimeout != nil {
		idleConnectionTimeout = *rt.transportOptions.IdleConnTimeout
	}

	t := &http.Transport{DialContext: rt.dial, DialTLSContext: rt.dialTLS, TLSClientConfig: utlsConfig, ConnectionFlow: rt.connectionFlow, IdleConnTimeout: idleConnectionTimeout}

	if rt.transportOptions != nil {
		t.DisableKeepAlives = rt.transportOptions.DisableKeepAlives
		t.DisableCompression = rt.transportOptions.DisableCompression
		t.MaxIdleConns = rt.transportOptions.MaxIdleConns
		t.MaxIdleConnsPerHost = rt.transportOptions.MaxIdleConnsPerHost
		t.MaxConnsPerHost = rt.transportOptions.MaxConnsPerHost
		// Only set MaxResponseHeaderBytes if > 0 (HTTP/1.1 transport doesn't understand -1)
		if rt.transportOptions.MaxResponseHeaderBytes > 0 {
			t.MaxResponseHeaderBytes = rt.transportOptions.MaxResponseHeaderBytes
		}
		t.WriteBufferSize = rt.transportOptions.WriteBufferSize
		t.ReadBufferSize = rt.transportOptions.ReadBufferSize
	}

	return t
}

func (rt *roundTripper) dialTLSHTTP2(network, addr string, _ *tls.Config) (net.Conn, error) {
	ctx, cancel := rt.contextForHTTP2Dial(addr)
	defer cancel()
	return rt.dialTLS(ctx, network, addr)
}

type contextAwareHTTP2Transport struct {
	transport *http2.Transport
	owner     *roundTripper
	addr      string
}

func (t *contextAwareHTTP2Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	registration := t.owner.registerHTTP2DialContext(t.addr, req.Context())
	defer registration.release()
	return t.transport.RoundTrip(req)
}

func (t *contextAwareHTTP2Transport) CloseIdleConnections() {
	if closeIdler, ok := any(t.transport).(interface{ CloseIdleConnections() }); ok {
		closeIdler.CloseIdleConnections()
	}
}

type http2DialContextRegistration struct {
	owner                 *roundTripper
	addr                  string
	id                    uint64
	stopCancellationWatch func() bool
}

func (registration http2DialContextRegistration) release() {
	if registration.stopCancellationWatch != nil {
		registration.stopCancellationWatch()
	}

	rt := registration.owner
	rt.http2DialContextsLck.Lock()
	contexts := rt.http2DialContexts[registration.addr]
	delete(contexts, registration.id)
	if len(contexts) == 0 {
		delete(rt.http2DialContexts, registration.addr)
	}
	cancels := rt.http2DialCancelsWithoutWaitersLocked(registration.addr)
	rt.http2DialContextsLck.Unlock()
	cancelHTTP2Dials(cancels)
}

func (rt *roundTripper) registerHTTP2DialContext(addr string, ctx context.Context) http2DialContextRegistration {
	if ctx == nil {
		ctx = context.Background()
	}

	rt.http2DialContextsLck.Lock()
	if rt.http2DialContexts == nil {
		rt.http2DialContexts = make(map[string]map[uint64]context.Context)
	}
	rt.http2DialContextSeq++
	id := rt.http2DialContextSeq
	contexts := rt.http2DialContexts[addr]
	if contexts == nil {
		contexts = make(map[uint64]context.Context)
		rt.http2DialContexts[addr] = contexts
	}
	contexts[id] = ctx
	rt.http2DialContextsLck.Unlock()

	registration := http2DialContextRegistration{
		owner: rt,
		addr:  addr,
		id:    id,
	}
	if ctx.Done() != nil {
		registration.stopCancellationWatch = context.AfterFunc(ctx, func() {
			rt.cancelHTTP2DialsWithoutWaiters(addr)
		})
	}
	return registration
}

// contextForHTTP2Dial keeps a shared connection attempt alive while at least
// one request currently waiting on that host remains active. Registrations
// added after the dial starts are included, so one canceled request cannot
// tear down a handshake that a later multiplexed request is already waiting
// for.
func (rt *roundTripper) contextForHTTP2Dial(addr string) (context.Context, context.CancelFunc) {
	rt.http2DialContextsLck.Lock()
	if rt.http2DialCancels == nil {
		rt.http2DialCancels = make(map[string]map[uint64]context.CancelFunc)
	}
	ctx, cancel := context.WithCancel(context.Background())
	rt.http2DialContextSeq++
	id := rt.http2DialContextSeq
	cancels := rt.http2DialCancels[addr]
	if cancels == nil {
		cancels = make(map[uint64]context.CancelFunc)
		rt.http2DialCancels[addr] = cancels
	}
	cancels[id] = cancel
	shouldCancel := !rt.hasActiveHTTP2DialWaiterLocked(addr)
	rt.http2DialContextsLck.Unlock()
	if shouldCancel {
		cancel()
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			rt.http2DialContextsLck.Lock()
			if registered := rt.http2DialCancels[addr]; registered != nil {
				delete(registered, id)
				if len(registered) == 0 {
					delete(rt.http2DialCancels, addr)
				}
			}
			rt.http2DialContextsLck.Unlock()
			cancel()
		})
	}
	return ctx, release
}

func (rt *roundTripper) hasActiveHTTP2DialWaiterLocked(addr string) bool {
	for _, ctx := range rt.http2DialContexts[addr] {
		if ctx.Err() == nil {
			return true
		}
	}
	return false
}

func (rt *roundTripper) http2DialCancelsWithoutWaitersLocked(addr string) []context.CancelFunc {
	if rt.hasActiveHTTP2DialWaiterLocked(addr) {
		return nil
	}
	cancels := rt.http2DialCancels[addr]
	result := make([]context.CancelFunc, 0, len(cancels))
	for _, cancel := range cancels {
		result = append(result, cancel)
	}
	return result
}

func (rt *roundTripper) cancelHTTP2DialsWithoutWaiters(addr string) {
	rt.http2DialContextsLck.Lock()
	cancels := rt.http2DialCancelsWithoutWaitersLocked(addr)
	rt.http2DialContextsLck.Unlock()
	cancelHTTP2Dials(cancels)
}

func cancelHTTP2Dials(cancels []context.CancelFunc) {
	for _, cancel := range cancels {
		cancel()
	}
}

// dialTLSForWebsocket establishes a TLS connection for WebSocket use.
// Unlike dialTLS, this method doesn't cache connections or create transports -
// it simply performs the TLS handshake with the same fingerprinting configuration.
func (rt *roundTripper) dialTLSForWebsocket(ctx context.Context, network, addr string) (net.Conn, error) {
	if network == "tcp" && rt.disableIPV6 {
		network = "tcp4"
	}

	if network == "tcp" && rt.disableIPV4 {
		network = "tcp6"
	}

	rawConn, err := rt.dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	var host string
	if host, _, err = net.SplitHostPort(addr); err != nil {
		host = addr
	}

	if rt.serverNameOverwrite != "" {
		host = rt.serverNameOverwrite
	}

	tlsConfig := &tls.Config{
		ClientSessionCache: rt.clientSessionCache,
		ServerName:         host,
		InsecureSkipVerify: rt.insecureSkipVerify,
		OmitEmptyPsk:       true,
	}
	if rt.transportOptions != nil {
		tlsConfig.RootCAs = rt.transportOptions.RootCAs
		tlsConfig.KeyLogWriter = rt.transportOptions.KeyLogWriter
		tlsConfig.Certificates = rt.transportOptions.Certificates
	}

	rawConn = rt.bandwidthTracker.TrackConnection(ctx, rawConn)

	// Force HTTP/1.1 for WebSocket connections (WebSocket doesn't work over HTTP/2)
	conn := tls.UClient(rawConn, tlsConfig, rt.clientHelloId, rt.withRandomTlsExtensionOrder, true, true)
	if err = conn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}

	err = rt.certificatePinner.Pin(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	return conn, nil
}

func (rt *roundTripper) getDialTLSAddr(req *http.Request) string {
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	return net.JoinHostPort(host, "443")
}

func newRoundTripper(clientProfile profiles.ClientProfile, transportOptions *TransportOptions, serverNameOverwrite string, insecureSkipVerify bool, withRandomTlsExtensionOrder bool, forceHttp1 bool, disableHttp3 bool, enableH3Racing bool, certificatePins map[string][]string, badPinHandlerFunc BadPinHandlerFunc, disableIPV6 bool, disableIPV4 bool, bandwidthTracker bandwidth.BandwidthTracker, dialer ...proxy.ContextDialer) (http.RoundTripper, error) {
	transportOptions = cloneTransportOptions(transportOptions)
	pinner, err := NewCertificatePinner(certificatePins)
	if err != nil {
		return nil, fmt.Errorf("can not instantiate certificate pinner: %w", err)
	}

	var clientSessionCache tls.ClientSessionCache

	withSessionResumption := supportsSessionResumption(clientProfile.GetClientHelloId())

	if withSessionResumption {
		clientSessionCache = tls.NewLRUClientSessionCache(tlsClientSessionCacheSize(transportOptions))
	}

	selectedDialer := proxy.ContextDialer(proxy.Direct)
	if len(dialer) > 0 && dialer[0] != nil {
		selectedDialer = dialer[0]
	}

	rt := &roundTripper{
		dialer:           selectedDialer,
		transportOptions: transportOptions,
		bandwidthTracker: bandwidthTracker,
		rtH2Params: rtH2Params{
			initialStreamID:   clientProfile.GetStreamID(),
			allowHTTP:         clientProfile.GetAllowHTTP(),
			settings:          profileSettings(clientProfile.GetSettings()),
			settingsOrder:     profileSettingsOrder(clientProfile.GetSettingsOrder()),
			headerPriority:    profilePriorityParam(clientProfile.GetHeaderPriority()),
			priorities:        profilePriorities(clientProfile.GetPriorities()),
			pseudoHeaderOrder: clientProfile.GetPseudoHeaderOrder(),
			connectionFlow:    clientProfile.GetConnectionFlow(),
		},
		rtTLSParams: rtTLSParams{
			clientHelloId:               clientProfile.GetClientHelloId(),
			certificatePinner:           pinner,
			clientSessionCache:          clientSessionCache,
			serverNameOverwrite:         serverNameOverwrite,
			insecureSkipVerify:          insecureSkipVerify,
			withRandomTlsExtensionOrder: withRandomTlsExtensionOrder,
			badPinHandlerFunc:           badPinHandlerFunc,
		},
		rtCacheState: rtCacheState{
			cachedTransports:  make(map[string]http.RoundTripper),
			transportCache:    newTransportCacheMeta(maxCachedTransports(transportOptions)),
			cachedConnections: make(map[string]net.Conn),
		},
		rtH2DialState: rtH2DialState{
			http2DialContexts: make(map[string]map[uint64]context.Context),
			http2DialCancels:  make(map[string]map[uint64]context.CancelFunc),
		},
		rtH3Params: rtH3Params{
			http3Settings:          clientProfile.GetHttp3Settings(),
			http3SettingsOrder:     clientProfile.GetHttp3SettingsOrder(),
			http3PriorityParam:     clientProfile.GetHttp3PriorityParam(),
			http3PseudoHeaderOrder: clientProfile.GetHttp3PseudoHeaderOrder(),
			http3SendGreaseFrames:  clientProfile.GetHttp3SendGreaseFrames(),
		},
		rtProtoFlags: rtProtoFlags{
			forceHttp1:   forceHttp1,
			disableHttp3: disableHttp3,
			disableIPV4:  disableIPV4,
			disableIPV6:  disableIPV6,
		},
	}

	// Create protocol racer if HTTP/3 racing is enabled
	if enableH3Racing {
		rt.racer = (&protocolRacerConfig{
			clientSessionCache:     clientSessionCache,
			insecureSkipVerify:     insecureSkipVerify,
			serverNameOverwrite:    serverNameOverwrite,
			transportOptions:       transportOptions,
			settings:               profileSettings(clientProfile.GetSettings()),
			cachedTransports:       rt.cachedTransports,
			cachedTransportsLck:    &rt.cachedTransportsLck,
			transportCache:         rt.transportCache,
			transportInit:          &rt.transportInit,
			certificatePinner:      pinner,
			badPinHandlerFunc:      badPinHandlerFunc,
			bandwidthTracker:       bandwidthTracker,
			http3Settings:          clientProfile.GetHttp3Settings(),
			http3SettingsOrder:     clientProfile.GetHttp3SettingsOrder(),
			http3PriorityParam:     clientProfile.GetHttp3PriorityParam(),
			http3PseudoHeaderOrder: clientProfile.GetHttp3PseudoHeaderOrder(),
			http3SendGreaseFrames:  clientProfile.GetHttp3SendGreaseFrames(),
		}).toRacer()
	}

	return rt, nil
}

func supportsSessionResumption(id tls.ClientHelloID) bool {
	spec, err := id.ToSpec()
	if err != nil {
		spec, err = tls.UTLSIdToSpec(id)
		if err != nil {
			return false
		}
	}

	for _, ext := range spec.Extensions {
		if _, ok := ext.(*tls.UtlsPreSharedKeyExtension); ok {
			return true
		}
	}

	return false
}

func maxCachedTransports(options *TransportOptions) int {
	if options == nil {
		return 0
	}
	return options.MaxCachedTransports
}

func tlsClientSessionCacheSize(options *TransportOptions) int {
	if options == nil || options.TLSClientSessionCacheSize == 0 {
		return DefaultTLSClientSessionCacheSize
	}
	return options.TLSClientSessionCacheSize
}
