// tls_client_cffi_src provides and manages a CFFI (C Foreign Function Interface) which allows code in other languages to interact with the module.
package tls_client_cffi_src

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bogdanfinn/tls-client/profiles"
	"golang.org/x/net/html/charset"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/fhttp/cookiejar"
	"github.com/bogdanfinn/fhttp/http2"
	tls_client "github.com/bogdanfinn/tls-client"
	tls "github.com/bogdanfinn/utls"
	"github.com/google/uuid"
)

var clientsLock = sync.RWMutex{}

// sessionLifecycleLock lets independent sessions build in parallel while
// making ClearSessionCache an atomic boundary for all session operations.
var sessionLifecycleLock sync.RWMutex

type sessionLockEntry struct {
	lock sync.Mutex
	refs int
}

var (
	sessionLocksLock sync.Mutex
	sessionLocks     = make(map[string]*sessionLockEntry)
)

// clients contains all registered clients, mapped by their individual IDs.
// Metadata is used for bounded LRU and optional idle-TTL eviction.
var clients = make(map[string]*sessionClientEntry)

// RemoveSession deletes the client with the given sessionId from the client session storage.
func RemoveSession(sessionId string) {
	sessionLifecycleLock.RLock()
	unlockSession := lockSession(sessionId)

	clientsLock.Lock()
	entry, ok := clients[sessionId]
	if ok {
		delete(clients, sessionId)
	}
	clientsLock.Unlock()

	unlockSession()
	sessionLifecycleLock.RUnlock()

	if ok {
		entry.client.CloseIdleConnections()
	}
}

// ClearSessionCache empties the client session storage.
func ClearSessionCache() {
	sessionLifecycleLock.Lock()
	clientsLock.Lock()
	oldClients := clients
	clients = make(map[string]*sessionClientEntry)
	sessionCacheNextPrune.Store(0)
	clientsLock.Unlock()
	sessionLifecycleLock.Unlock()

	for _, entry := range oldClients {
		entry.client.CloseIdleConnections()
	}
}

// GetClient returns the client with the given sessionId from the client session storage.
// If there is no client with the given sessionId, it returns an error.
func GetClient(sessionId string) (tls_client.HttpClient, error) {
	client, ok := getCachedSessionClient(sessionId, false)
	if !ok {
		return nil, fmt.Errorf("no client found for sessionId: %s", sessionId)
	}

	return client, nil
}

// GetClientForSessionOperation leases a session for a non-request CFFI
// operation such as reading or updating cookies. The returned release function
// is idempotent and must be called on every path.
func GetClientForSessionOperation(sessionId string) (tls_client.HttpClient, func(), error) {
	release := acquireSessionOperation(sessionId)
	client, ok := getCachedSessionClient(sessionId, true)
	if !ok {
		release()
		return nil, nil, fmt.Errorf("no client found for sessionId: %s", sessionId)
	}
	return client, release, nil
}

// CreateClient creates a new client from a given RequestInput.
//
// The RequestInput should only contain a TLSClientIdentifier or a CustomTlsClient. If both are provided, an error will be returned.
func CreateClient(requestInput RequestInput) (client tls_client.HttpClient, sessionID string, withSession bool, clientErr *TLSClientError) {
	client, sessionID, withSession, releaseSession, clientErr := createClient(requestInput, false)
	if releaseSession != nil {
		releaseSession()
	}
	return client, sessionID, withSession, clientErr
}

// CreateClientForRequest keeps an existing session exclusively leased until
// releaseSession is called. CFFI request entrypoints use this to keep dynamic
// proxy and redirect changes attached to the request that requested them.
func CreateClientForRequest(requestInput RequestInput) (client tls_client.HttpClient, sessionID string, withSession bool, releaseSession func(), clientErr *TLSClientError) {
	return createClient(requestInput, true)
}

func createClient(requestInput RequestInput, holdSession bool) (client tls_client.HttpClient, sessionID string, withSession bool, releaseSession func(), clientErr *TLSClientError) {
	useSession := true
	sessionId := requestInput.SessionId

	newSessionId := uuid.New().String()
	if sessionId != nil && *sessionId != "" {
		newSessionId = *sessionId
	} else {
		useSession = false
	}

	if requestInput.TLSClientIdentifier != "" && requestInput.CustomTlsClient != nil {
		clientErr := NewTLSClientError(fmt.Errorf("cannot build client out of client identifier and custom tls client information. Please provide only one of them"))

		return nil, newSessionId, useSession, nil, clientErr
	}

	if requestInput.TimeoutSeconds != 0 && requestInput.TimeoutMilliseconds != 0 {
		clientErr := NewTLSClientError(fmt.Errorf("cannot build client with both defined timeout in seconds and timeout in milliseconds. Please provide only one of them"))

		return nil, newSessionId, useSession, nil, clientErr
	}

	if useSession {
		releaseSession = acquireSessionOperation(newSessionId)
	}
	releaseBeforeReturn := releaseSession
	defer func() {
		if releaseBeforeReturn != nil {
			releaseBeforeReturn()
		}
	}()

	tlsClient, err := getTlsClient(requestInput, newSessionId, useSession)
	if err != nil {
		clientErr := NewTLSClientError(fmt.Errorf("failed to build client out of request input: %w", err))

		return nil, newSessionId, useSession, nil, clientErr
	}

	if holdSession {
		releaseBeforeReturn = nil
	} else {
		releaseSession = nil
	}

	return tlsClient, newSessionId, useSession, releaseSession, nil
}

// BuildRequest constructs a HTTP request from a given RequestInput.
func BuildRequest(input RequestInput) (*http.Request, *TLSClientError) {
	var tlsReq *http.Request
	var err error

	if input.RequestMethod == "" || input.RequestUrl == "" {
		return nil, NewTLSClientError(fmt.Errorf("no request url or request method provided"))
	}

	if input.RequestBody != nil && *input.RequestBody != "" {
		var requestBodyBytes []byte
		if input.IsByteRequest {
			requestBodyBytes, err = base64.StdEncoding.DecodeString(*input.RequestBody)

			if err != nil {
				return nil, NewTLSClientError(fmt.Errorf("failed to base64 decode request body: %w", err))
			}
		} else {
			requestBodyBytes = []byte(*input.RequestBody)
		}

		requestBody := bytes.NewReader(requestBodyBytes)
		tlsReq, err = http.NewRequest(input.RequestMethod, input.RequestUrl, requestBody)
	} else {
		tlsReq, err = http.NewRequest(input.RequestMethod, input.RequestUrl, nil)
	}

	if err != nil {
		return nil, NewTLSClientError(fmt.Errorf("failed to create request object: %w", err))
	}

	if input.RequestHostOverride != nil {
		tlsReq.Host = *input.RequestHostOverride
	}

	headers := http.Header{}

	for key, value := range input.Headers {
		headers[key] = []string{value}
	}

	headers[http.HeaderOrderKey] = input.HeaderOrder

	tlsReq.Header = headers

	return tlsReq, nil
}

func readAllBodyWithStreamToFile(respBody io.Reader, input RequestInput) ([]byte, error) {
	var respBodyBytes []byte
	var err error
	var bodyLen int64
	blockSize := 32 * 1024
	if input.StreamOutputBlockSize != nil {
		blockSize = *input.StreamOutputBlockSize
	}
	if blockSize <= 0 {
		return nil, fmt.Errorf("stream output block size must be greater than zero")
	}
	if blockSize > 16*1024*1024 {
		return nil, fmt.Errorf("stream output block size must not exceed 16777216 bytes")
	}

	f, err := os.OpenFile(*input.StreamOutputPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			fmt.Printf("failed to close file: %v\n", closeErr)
		}
	}()

	buf := make([]byte, blockSize)
	// Read the response body
	for {
		n, readErr := respBody.Read(buf)
		if input.MaxResponseBodyBytes > 0 && int64(n) > input.MaxResponseBodyBytes-bodyLen {
			return nil, fmt.Errorf("response body exceeds configured limit of %d bytes", input.MaxResponseBodyBytes)
		}
		bodyLen += int64(n)
		if input.WithDebug {
			fmt.Printf("Reading at: %d\n", bodyLen)
		}
		if n > 0 {
			_, err = f.Write(buf[:n])
		}
		if err != nil {
			if input.WithDebug {
				fmt.Printf("Append stream output error: %+v\n", err)
			}

			return nil, err
		}

		if readErr == io.EOF {
			if input.StreamOutputEOFSymbol != nil {
				if _, err = f.Write([]byte(*input.StreamOutputEOFSymbol)); err != nil {
					return nil, fmt.Errorf("failed to append stream EOF symbol: %w", err)
				}
			}

			break
		} else if readErr != nil {
			if input.WithDebug {
				fmt.Printf("Reading Response Body error: %+v\n", readErr)
			}
			return nil, readErr
		}
	}

	return respBodyBytes, nil
}

func readResponseBody(reader io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return io.ReadAll(reader)
	}

	readLimit := maxBytes
	if maxBytes < int64(1<<63-1) {
		readLimit++
	}
	body, err := io.ReadAll(io.LimitReader(reader, readLimit))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("response body exceeds configured limit of %d bytes", maxBytes)
	}
	return body, nil
}

// BuildResponse constructs a client response from a given HTTP response. The client response can then be sent to the interface consumer.
func BuildResponse(sessionId string, withSession bool, resp *http.Response, cookies []*http.Cookie, input RequestInput) (Response, *TLSClientError) {
	if input.MaxResponseBodyBytes < 0 {
		return Response{}, NewTLSClientError(errors.New("max response body bytes must not be negative"))
	}
	if resp == nil {
		return Response{}, NewTLSClientError(errors.New("response must not be nil"))
	}
	if resp.Body == nil {
		resp.Body = io.NopCloser(bytes.NewReader(nil))
	}
	defer resp.Body.Close()

	isByteResponse := input.IsByteResponse

	ce := resp.Header.Get("Content-Encoding")
	ct := resp.Header.Get("Content-Type")

	var respBodyBytes []byte
	var bodyReader io.Reader

	if !resp.Uncompressed {
		resp.Body = http.DecompressBodyByType(resp.Body, ce)
	}

	bodyReader = resp.Body

	if !isByteResponse {
		// Try to preview a single byte of the body reader to prevent EOF caused by empty bodies.
		// This is probably the best way of reliably detecting empty response bodies,
		// especially when the content-length response header is not present.
		firstByte := make([]byte, 1)
		n, err := io.ReadFull(resp.Body, firstByte)
		if err != nil && !errors.Is(err, io.EOF) {
			return Response{}, NewTLSClientError(err)
		}

		if n == 0 {
			respBodyBytes = nil
		} else {
			bodyReader = io.MultiReader(bytes.NewReader(firstByte[:n]), resp.Body)
			// Automatically detect the charset for non-byte responses
			bodyReader, err = charset.NewReader(bodyReader, ct)
			if err != nil {
				return Response{}, NewTLSClientError(err)
			}

			if input.StreamOutputPath != nil {
				respBodyBytes, err = readAllBodyWithStreamToFile(bodyReader, input)
			} else {
				respBodyBytes, err = readResponseBody(bodyReader, input.MaxResponseBodyBytes)
			}
			if err != nil {
				return Response{}, NewTLSClientError(err)
			}
		}
	} else {
		var err error
		if input.StreamOutputPath != nil {
			respBodyBytes, err = readAllBodyWithStreamToFile(bodyReader, input)
		} else {
			respBodyBytes, err = readResponseBody(bodyReader, input.MaxResponseBodyBytes)
		}

		if err != nil {
			return Response{}, NewTLSClientError(err)
		}
	}

	var finalResponse string
	if isByteResponse {
		finalResponse = encodeByteResponse(respBodyBytes)
	} else {
		finalResponse = string(respBodyBytes)
	}

	response := Response{
		Id:           uuid.New().String(),
		Status:       resp.StatusCode,
		UsedProtocol: resp.Proto,
		Body:         finalResponse,
		Headers:      resp.Header,
		Target:       "",
		Cookies:      cookiesToMap(cookies),
	}

	if resp.Request != nil && resp.Request.URL != nil {
		response.Target = resp.Request.URL.String()
	}

	if withSession {
		response.SessionId = sessionId
	}

	return response, nil
}

func encodeByteResponse(body []byte) string {
	prefix := "data:" + http.DetectContentType(body) + ";base64,"
	var encoded strings.Builder
	encoded.Grow(len(prefix) + base64.StdEncoding.EncodedLen(len(body)))
	encoded.WriteString(prefix)
	encoder := base64.NewEncoder(base64.StdEncoding, &encoded)
	_, _ = encoder.Write(body)
	_ = encoder.Close()
	return encoded.String()
}

func getTlsClient(requestInput RequestInput, sessionId string, withSession bool) (tls_client.HttpClient, error) {
	tlsClientIdentifier := requestInput.TLSClientIdentifier

	// Resolve from integer ProfileID if string identifier is empty.
	if tlsClientIdentifier == "" && requestInput.ProfileID > 0 {
		tlsClientIdentifier = profiles.ProfileID(requestInput.ProfileID).String()
	}

	proxyUrl := requestInput.ProxyUrl

	var resolvedIdentifierProfile profiles.ClientProfile
	if tlsClientIdentifier != "" {
		var err error
		resolvedIdentifierProfile, err = getTlsClientProfile(tlsClientIdentifier)
		if err != nil {
			return nil, err
		}
	}

	client, ok := getCachedSessionClient(sessionId, withSession)

	if ok && withSession {
		modifiedClient, _, err := handleModification(client, proxyUrl, requestInput.FollowRedirects, requestInput.IsRotatingProxy)
		if err != nil {
			return nil, fmt.Errorf("failed to modify existing client: %w", err)
		}

		return modifiedClient, nil
	}

	clientProfile := profiles.DefaultClientProfile

	if requestInput.CustomTlsClient != nil {
		clientHelloId, h2Settings, h2SettingsOrder, pseudoHeaderOrder, connectionFlow, priorityFrames, headerPriority, streamId, allowHttp, h3Settings, h3SettingsOrder, h3PriorityParam, h3PseudoHeaderOrder, http3SendGreaseFrames, err := getCustomTlsClientProfile(requestInput.CustomTlsClient)
		if err != nil {
			return nil, fmt.Errorf("can not build http client out of custom tls client information: %w", err)
		}

		clientProfile = profiles.NewClientProfile(
			clientHelloId,
			profilesSettingMap(h2Settings),
			profilesSettingSlice(h2SettingsOrder),
			pseudoHeaderOrder, connectionFlow,
			profilesPriorities(priorityFrames),
			profilesPriorityParams(headerPriority),
			streamId, allowHttp,
			h3Settings, h3SettingsOrder, h3PriorityParam, h3PseudoHeaderOrder, http3SendGreaseFrames,
		)
	}

	if tlsClientIdentifier != "" {
		clientProfile = resolvedIdentifierProfile
	}

	timeoutOption := tls_client.WithTimeoutSeconds(tls_client.DefaultTimeoutSeconds)

	if requestInput.TimeoutSeconds != 0 {
		timeoutOption = tls_client.WithTimeoutSeconds(requestInput.TimeoutSeconds)
	}

	if requestInput.TimeoutMilliseconds != 0 {
		timeoutOption = tls_client.WithTimeoutMilliseconds(requestInput.TimeoutMilliseconds)
	}

	options := []tls_client.HttpClientOption{
		timeoutOption,
		tls_client.WithClientProfile(clientProfile),
	}

	if requestInput.WithRandomTLSExtensionOrder {
		options = append(options, tls_client.WithRandomTLSExtensionOrder())
	}

	if requestInput.ForceHttp1 {
		options = append(options, tls_client.WithForceHttp1())
	}

	if requestInput.DisableHttp3 {
		options = append(options, tls_client.WithDisableHttp3())
	}

	if requestInput.WithProtocolRacing {
		options = append(options, tls_client.WithProtocolRacing())
	}

	if requestInput.DisableIPV6 {
		options = append(options, tls_client.WithDisableIPV6())
	}

	if requestInput.DisableIPV4 {
		options = append(options, tls_client.WithDisableIPV4())
	}

	if requestInput.TransportOptions != nil {
		transportOptions := &tls_client.TransportOptions{
			DisableKeepAlives:         requestInput.TransportOptions.DisableKeepAlives,
			DisableCompression:        requestInput.TransportOptions.DisableCompression,
			MaxIdleConns:              requestInput.TransportOptions.MaxIdleConns,
			MaxIdleConnsPerHost:       requestInput.TransportOptions.MaxIdleConnsPerHost,
			MaxConnsPerHost:           requestInput.TransportOptions.MaxConnsPerHost,
			MaxCachedTransports:       requestInput.TransportOptions.MaxCachedTransports,
			TLSClientSessionCacheSize: requestInput.TransportOptions.TLSClientSessionCacheSize,
			ProtocolRacingHTTP2Delay:  requestInput.TransportOptions.ProtocolRacingHTTP2Delay,
			ProtocolRacingTimeout:     requestInput.TransportOptions.ProtocolRacingTimeout,
			MaxResponseHeaderBytes:    requestInput.TransportOptions.MaxResponseHeaderBytes,
			WriteBufferSize:           requestInput.TransportOptions.WriteBufferSize,
			ReadBufferSize:            requestInput.TransportOptions.ReadBufferSize,
			IdleConnTimeout:           requestInput.TransportOptions.IdleConnTimeout,
			// RootCAs:                requestInput.TransportOptions.RootCAs,
		}

		options = append(options, tls_client.WithTransportOptions(transportOptions))
	}

	if requestInput.LocalAddress != nil {
		localAddr, err := net.ResolveTCPAddr("", *requestInput.LocalAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve tcp address from local %s address: %w", *requestInput.LocalAddress, err)
		}

		options = append(options, tls_client.WithLocalAddr(*localAddr))
	}

	if requestInput.CatchPanics {
		options = append(options, tls_client.WithCatchPanics())
	}

	if len(requestInput.CertificatePinningHosts) > 0 {
		options = append(options, tls_client.WithCertificatePinning(requestInput.CertificatePinningHosts, nil))
	}

	if requestInput.WithDebug {
		options = append(options, tls_client.WithDebug())
	}

	if !requestInput.WithoutCookieJar {
		var jarOptions []tls_client.CookieJarOption
		if requestInput.WithDebug {
			jarOptions = append(jarOptions, tls_client.WithDebugLogger())
		}

		jar, _ := cookiejar.New(nil)
		if requestInput.WithCustomCookieJar {
			jar := tls_client.NewCookieJar(jarOptions...)
			options = append(options, tls_client.WithCookieJar(jar))
		} else {
			options = append(options, tls_client.WithCookieJar(jar))
		}
	}

	if !requestInput.FollowRedirects {
		options = append(options, tls_client.WithNotFollowRedirects())
	}

	if requestInput.InsecureSkipVerify {
		options = append(options, tls_client.WithInsecureSkipVerify())
	}

	if len(requestInput.DefaultHeaders) != 0 {
		options = append(options, tls_client.WithDefaultHeaders(requestInput.DefaultHeaders))
	}

	if len(requestInput.ConnectHeaders) != 0 {
		options = append(options, tls_client.WithConnectHeaders(requestInput.ConnectHeaders))
	}

	if requestInput.ServerNameOverwrite != nil && *requestInput.ServerNameOverwrite != "" {
		options = append(options, tls_client.WithServerNameOverwrite(*requestInput.ServerNameOverwrite))
	}

	proxy := proxyUrl

	if proxy != nil && *proxy != "" {
		options = append(options, tls_client.WithProxyUrl(*proxy))
	}

	tlsClient, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)

	if err == nil && withSession {
		evicted := storeCachedSessionClient(sessionId, tlsClient)
		closeSessionClients(evicted)
	}

	return tlsClient, err
}

func lockSession(sessionId string) func() {
	sessionLocksLock.Lock()
	entry := sessionLocks[sessionId]
	if entry == nil {
		entry = &sessionLockEntry{}
		sessionLocks[sessionId] = entry
	}
	entry.refs++
	sessionLocksLock.Unlock()

	entry.lock.Lock()
	return func() {
		entry.lock.Unlock()

		sessionLocksLock.Lock()
		entry.refs--
		if entry.refs == 0 && sessionLocks[sessionId] == entry {
			delete(sessionLocks, sessionId)
		}
		sessionLocksLock.Unlock()
	}
}

func acquireSessionOperation(sessionId string) func() {
	sessionLifecycleLock.RLock()
	unlockSession := lockSession(sessionId)

	var once sync.Once
	return func() {
		once.Do(func() {
			touchCachedSessionClient(sessionId)
			unlockSession()
			evicted := pruneSessionCacheIfNeeded(time.Now(), "")
			sessionLifecycleLock.RUnlock()
			closeSessionClients(evicted)
		})
	}
}

func getCustomTlsClientProfile(customClientDefinition *CustomTlsClient) (tls.ClientHelloID, map[http2.SettingID]uint32, []http2.SettingID, []string, uint32, []http2.Priority, *http2.PriorityParam, uint32, bool, map[uint64]uint64, []uint64, uint32, []string, bool, error) {
	specFactory, err := tls_client.GetSpecFactoryFromJa3String(customClientDefinition.Ja3String, customClientDefinition.SupportedSignatureAlgorithms, customClientDefinition.SupportedDelegatedCredentialsAlgorithms, customClientDefinition.SupportedVersions, customClientDefinition.KeyShareCurves, customClientDefinition.ALPNProtocols, customClientDefinition.ALPSProtocols, customClientDefinition.ECHCandidateCipherSuites.Translate(), customClientDefinition.ECHCandidatePayloads, customClientDefinition.CertCompressionAlgos, customClientDefinition.RecordSizeLimit)
	if err != nil {
		return tls.ClientHelloID{}, nil, nil, nil, 0, nil, nil, 0, false, nil, nil, 0, nil, false, err
	}

	resolvedH2Settings := make(map[http2.SettingID]uint32)
	for key, value := range customClientDefinition.H2Settings {
		resolvedKey, ok := tls_client.H2SettingsMap[key]
		if !ok {
			continue
		}

		resolvedH2Settings[resolvedKey] = value
	}

	var resolvedH2SettingsOrder []http2.SettingID
	for _, order := range customClientDefinition.H2SettingsOrder {
		resolvedKey, ok := tls_client.H2SettingsMap[order]
		if !ok {
			continue
		}

		resolvedH2SettingsOrder = append(resolvedH2SettingsOrder, resolvedKey)
	}

	pseudoHeaderOrder := customClientDefinition.PseudoHeaderOrder
	connectionFlow := customClientDefinition.ConnectionFlow

	var priorityFrames []http2.Priority
	for _, priority := range customClientDefinition.PriorityFrames {
		priorityFrames = append(priorityFrames, http2.Priority{
			StreamID: priority.StreamID,
			PriorityParam: http2.PriorityParam{
				StreamDep: priority.PriorityParam.StreamDep,
				Exclusive: priority.PriorityParam.Exclusive,
				Weight:    priority.PriorityParam.Weight,
			},
		})
	}

	var headerPriority *http2.PriorityParam

	if customClientDefinition.HeaderPriority != nil {
		headerPriority = &http2.PriorityParam{
			StreamDep: customClientDefinition.HeaderPriority.StreamDep,
			Exclusive: customClientDefinition.HeaderPriority.Exclusive,
			Weight:    customClientDefinition.HeaderPriority.Weight,
		}
	}

	clientHelloId := tls.ClientHelloID{
		Client:      "Custom",
		Version:     "1",
		Seed:        nil,
		SpecFactory: specFactory,
	}

	// Process HTTP/3 settings
	resolvedH3Settings := make(map[uint64]uint64)
	for key, value := range customClientDefinition.H3Settings {
		resolvedKey, ok := tls_client.H3SettingsMap[key]
		if !ok {
			continue
		}

		resolvedH3Settings[resolvedKey] = value
	}

	var resolvedH3SettingsOrder []uint64
	for _, order := range customClientDefinition.H3SettingsOrder {
		resolvedKey, ok := tls_client.H3SettingsMap[order]
		if !ok {
			continue
		}

		resolvedH3SettingsOrder = append(resolvedH3SettingsOrder, resolvedKey)
	}

	h3PseudoHeaderOrder := customClientDefinition.H3PseudoHeaderOrder
	h3PriorityParam := customClientDefinition.H3PriorityParam
	http3SendGreaseFrames := customClientDefinition.H3SendGreaseFrames

	return clientHelloId, resolvedH2Settings, resolvedH2SettingsOrder, pseudoHeaderOrder, connectionFlow, priorityFrames, headerPriority, customClientDefinition.StreamId, customClientDefinition.AllowHttp, resolvedH3Settings, resolvedH3SettingsOrder, h3PriorityParam, h3PseudoHeaderOrder, http3SendGreaseFrames, nil
}

func getTlsClientProfile(tlsClientIdentifier string) (profiles.ClientProfile, error) {
	clientProfile, err := profiles.ResolveClientProfileStrict(tlsClientIdentifier)
	if err != nil {
		return profiles.ClientProfile{}, fmt.Errorf("invalid tls client identifier: %w", err)
	}
	return clientProfile, nil
}

func handleModification(client tls_client.HttpClient, proxyUrl *string, followRedirects bool, isRotatingProxy bool) (tls_client.HttpClient, bool, error) {
	changed := false

	if client == nil {
		return client, false, fmt.Errorf("no tls client for modification check")
	}

	if proxyUrl != nil {
		if client.GetProxy() != *proxyUrl || isRotatingProxy {
			err := client.SetProxy(*proxyUrl)
			if err != nil {
				return nil, false, fmt.Errorf("failed to change proxy url of client: %w", err)
			}

			changed = true
		}
	}

	if client.GetFollowRedirect() != followRedirects {
		client.SetFollowRedirect(followRedirects)
		changed = true
	}

	return client, changed, nil
}

func cookiesToMap(cookies []*http.Cookie) map[string]string {
	ret := make(map[string]string, 0)

	for _, c := range cookies {
		ret[c.Name] = c.Value
	}

	return ret
}

// Conversion helpers for fhttp/http2 → profiles H2 types.
func profilesSettingMap(m map[http2.SettingID]uint32) map[profiles.SettingID]uint32 {
	out := make(map[profiles.SettingID]uint32, len(m))
	for k, v := range m { out[profiles.SettingID(k)] = v }
	return out
}

func profilesSettingSlice(s []http2.SettingID) []profiles.SettingID {
	out := make([]profiles.SettingID, len(s))
	for i, v := range s { out[i] = profiles.SettingID(v) }
	return out
}

func profilesPriorities(pp []http2.Priority) []profiles.Priority {
	out := make([]profiles.Priority, len(pp))
	for i, p := range pp {
		out[i] = profiles.Priority{
			StreamID:      p.StreamID,
			PriorityParam: profiles.PriorityParam(p.PriorityParam),
		}
	}
	return out
}

func profilesPriorityParams(p *http2.PriorityParam) *profiles.PriorityParam {
	if p == nil { return nil }
	return &profiles.PriorityParam{
		StreamDep: p.StreamDep,
		Exclusive: p.Exclusive,
		Weight:    p.Weight,
	}
}
