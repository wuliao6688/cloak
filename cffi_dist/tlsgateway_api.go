// tlsgateway_api.go — 商用级 CFFI API（零 JSON，类型安全）
//
// 设计原则：
//   1. 零 JSON — 调用方不需要构造/解析 JSON
//   2. Session 模型 — 复用连接/Cookie，Thread-safe
//   3. 场景驱动 — 登录流程、Cookie 管理、响应头提取一条龙
//   4. 易语言友好 — 最少的 DLL 声明，整数错误码
//   5. 内存安全 — 显式 free，无泄漏

package main

/*
#include <stdlib.h>

// TgResponse — 单次请求结果（C 堆分配，调用 tg_response_free 释放）
typedef struct {
	int    status;     // HTTP 状态码
	int    errorCode;  // 0=成功, 1=网络错误, 2=HTTP错误, 3=会话错误, 4=超时
	char*  body;       // 响应体（null-terminated）
	int    bodyLen;    // 响应体长度（字节）
	char*  headers;    // 响应头（"Key: Value\\n..." 格式，NULL=无）
	char*  error;      // 错误描述（NULL=成功）
} TgResponse;
*/
import "C"
import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"sync"
	"unsafe"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/google/uuid"
)

// ─── Error Codes ──────────────────────────────────────────

const (
	errOK         = 0
	errNetwork    = 1
	errHTTP       = 2
	errSession    = 3
	errTimeout    = 4
	errProfile    = 5
)

// ─── Session ──────────────────────────────────────────────

type apiSession struct {
	id        string
	profileID int
	client    tls_client.HttpClient
	timeout   int
	proxy     string
	mu        sync.Mutex

	// Anti-detection
	rotateGroup  int // RotateGroup, 0=disabled
	rotateEvery  int // switch profile every N requests
	reqCount     int // total request counter
	tlsRefresh   int // force new TLS handshake every N requests (default 50)

	// Proxy rotation
	proxyList   []string // proxy pool for rotation
	proxyIdx    int      // current index in proxyList
	proxyRotate int      // switch proxy every N requests (0=disabled)
}

var (
	apiSessions   = make(map[string]*apiSession)
	apiSessionsMu sync.RWMutex
)

func (s *apiSession) rebuild() error {
	pid := s.profileID
	p, err := profiles.ResolveProfileID(profiles.ProfileID(pid))
	if err != nil {
		return err
	}

	// Preserve the cookie jar across rebuilds.
	oldJar := s.client.GetCookieJar()

	opts := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(s.timeout),
		tls_client.WithClientProfile(p),
		tls_client.WithCookieJar(oldJar),
		tls_client.WithNotFollowRedirects(),
	}
	if s.proxy != "" {
		opts = append(opts, tls_client.WithProxyUrl(s.proxy))
	}
	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), opts...)
	if err != nil {
		return err
	}
	s.client = client
	return nil
}

// ─── Core API ─────────────────────────────────────────────

//export tg_session_create_int
func tg_session_create_int(profileID C.int, timeoutSeconds C.int, proxyURL *C.char) C.int {
	result := tg_session_create(profileID, timeoutSeconds, proxyURL)
	if result == nil {
		return -1
	}
	sid := C.GoString(result)
	if strings.HasPrefix(sid, "ERR:") {
		return -1
	}
	// Store int→string mapping
	intMu.Lock()
	intSessions[intNext] = sid
	h := intNext
	intNext++
	intMu.Unlock()
	return C.int(h)
}

var (
	intSessions = make(map[int]string)
	intNext     = 1
	intMu       sync.Mutex
)

func intToSessionID(handle C.int) string {
	intMu.Lock()
	defer intMu.Unlock()
	return intSessions[int(handle)]
}

//export tg_session_create
func tg_session_create(profileID C.int, timeoutSeconds C.int, proxyURL *C.char) *C.char {
	id := uuid.New().String()
	pid := int(profileID)
	timeout := int(timeoutSeconds)
	if timeout <= 0 {
		timeout = 30
	}

	p, err := profiles.ResolveProfileID(profiles.ProfileID(pid))
	if err != nil {
		return cString(fmt.Sprintf("ERR:%d: unknown profile %d", errProfile, pid))
	}

	proxy := ""
	if proxyURL != nil {
		proxy = C.GoString(proxyURL)
	}

	jar := tls_client.NewCookieJar()
	opts := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(timeout),
		tls_client.WithClientProfile(p),
		tls_client.WithCookieJar(jar),
		tls_client.WithNotFollowRedirects(),
	}
	if proxy != "" {
		opts = append(opts, tls_client.WithProxyUrl(proxy))
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), opts...)
	if err != nil {
		return cString(fmt.Sprintf("ERR:%d: %v", errSession, err))
	}

	s := &apiSession{
		id:        id,
		profileID: pid,
		client:    client,
		timeout:   timeout,
		proxy:     proxy,
	}

	apiSessionsMu.Lock()
	apiSessions[id] = s
	apiSessionsMu.Unlock()

	return cString(id)
}

//export tg_session_free
func tg_session_free(sessionID *C.char) {
	id := C.GoString(sessionID)
	apiSessionsMu.Lock()
	delete(apiSessions, id)
	apiSessionsMu.Unlock()
}

// ─── Proxy Management ─────────────────────────────────────

//export tg_session_set_proxy
func tg_session_set_proxy(sessionID *C.char, proxyURL *C.char) C.int {
	id := C.GoString(sessionID)
	apiSessionsMu.RLock()
	s, ok := apiSessions[id]
	apiSessionsMu.RUnlock()
	if !ok {
		return errSession
	}
	proxy := C.GoString(proxyURL)
	s.mu.Lock()
	defer s.mu.Unlock()

	if proxy == "" {
		// Clear proxy: rebuild client without proxy
		s.proxy = ""
		s.proxyList = nil
		s.proxyIdx = 0
		s.proxyRotate = 0
		return errOrOK(s.rebuild())
	}

	s.proxy = proxy
	s.proxyList = nil
	s.proxyRotate = 0
	return errOrOK(s.rebuild())
}

//export tg_session_get_proxy
func tg_session_get_proxy(sessionID *C.char) *C.char {
	id := C.GoString(sessionID)
	apiSessionsMu.RLock()
	s, ok := apiSessions[id]
	apiSessionsMu.RUnlock()
	if !ok {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return cString(s.proxy)
}

//export tg_session_set_proxy_list
func tg_session_set_proxy_list(sessionID *C.char, proxyList *C.char, rotateEveryN C.int) C.int {
	id := C.GoString(sessionID)
	apiSessionsMu.RLock()
	s, ok := apiSessions[id]
	apiSessionsMu.RUnlock()
	if !ok {
		return errSession
	}

	listStr := C.GoString(proxyList)
	if listStr == "" {
		return tg_session_set_proxy(sessionID, nil)
	}

	// Parse: "http://ip1:8080\nhttp://ip2:8080\nsocks5://ip3:1080"
	var proxies []string
	for _, line := range strings.Split(listStr, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			proxies = append(proxies, line)
		}
	}
	if len(proxies) == 0 {
		return errNetwork
	}

	rotate := int(rotateEveryN)
	if rotate <= 0 {
		rotate = 1
	}

	s.mu.Lock()
	s.proxyList = proxies
	s.proxyIdx = 0
	s.proxyRotate = rotate
	s.proxy = proxies[0]
	s.mu.Unlock()

	return errOrOK(s.rebuild())
}

func errOrOK(err error) C.int {
	if err != nil {
		return errNetwork
	}
	return errOK
}

// ─── Anti-Detection ───────────────────────────────────────

//export tg_session_set_rotate
func tg_session_set_rotate(sessionID *C.char, rotateGroup C.int, everyN C.int, tlsRefreshEvery C.int) C.int {
	id := C.GoString(sessionID)
	apiSessionsMu.RLock()
	s, ok := apiSessions[id]
	apiSessionsMu.RUnlock()
	if !ok {
		return errSession
	}
	s.mu.Lock()
	s.rotateGroup = int(rotateGroup)
	s.rotateEvery = int(everyN)
	s.tlsRefresh = int(tlsRefreshEvery)
	if s.tlsRefresh <= 0 {
		s.tlsRefresh = 50 // default
	}
	s.mu.Unlock()
	return errOK
}

// ─── Profile Management ───────────────────────────────────

//export tg_session_set_profile
func tg_session_set_profile(sessionID *C.char, profileID C.int) C.int {
	id := C.GoString(sessionID)
	apiSessionsMu.RLock()
	s, ok := apiSessions[id]
	apiSessionsMu.RUnlock()
	if !ok {
		return errSession
	}
	s.mu.Lock()
	s.profileID = int(profileID)
	err := s.rebuild()
	s.mu.Unlock()
	if err != nil {
		return errProfile
	}
	return errOK
}

//export tg_session_get_profile
func tg_session_get_profile(sessionID *C.char) C.int {
	id := C.GoString(sessionID)
	apiSessionsMu.RLock()
	s, ok := apiSessions[id]
	apiSessionsMu.RUnlock()
	if !ok {
		return -1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return C.int(s.profileID)
}

// ─── Cookie Management ────────────────────────────────────

//export tg_session_get_cookies
func tg_session_get_cookies(sessionID *C.char, requestURL *C.char) *C.char {
	id := C.GoString(sessionID)
	apiSessionsMu.RLock()
	s, ok := apiSessions[id]
	apiSessionsMu.RUnlock()
	if !ok {
		return nil
	}

	u, err := url.Parse(C.GoString(requestURL))
	if err != nil {
		return nil
	}

	s.mu.Lock()
	cookies := s.client.GetCookies(u)
	s.mu.Unlock()

	// Format: "name1=value1; name2=value2"
	var parts []string
	for _, c := range cookies {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return cString(strings.Join(parts, "; "))
}

//export tg_session_set_cookies
func tg_session_set_cookies(sessionID *C.char, requestURL *C.char, cookies *C.char) C.int {
	id := C.GoString(sessionID)
	apiSessionsMu.RLock()
	s, ok := apiSessions[id]
	apiSessionsMu.RUnlock()
	if !ok {
		return errSession
	}

	u, err := url.Parse(C.GoString(requestURL))
	if err != nil {
		return errNetwork
	}

	cookieStr := C.GoString(cookies)
	var httpCookies []*http.Cookie
	for _, pair := range strings.Split(cookieStr, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		c := &http.Cookie{Name: strings.TrimSpace(parts[0])}
		if len(parts) == 2 {
			c.Value = strings.TrimSpace(parts[1])
		}
		httpCookies = append(httpCookies, c)
	}

	s.mu.Lock()
	s.client.SetCookies(u, httpCookies)
	s.mu.Unlock()
	return errOK
}

//export tg_session_clear_cookies
func tg_session_clear_cookies(sessionID *C.char) C.int {
	id := C.GoString(sessionID)
	apiSessionsMu.RLock()
	s, ok := apiSessions[id]
	apiSessionsMu.RUnlock()
	if !ok {
		return errSession
	}
	s.mu.Lock()
	jar := tls_client.NewCookieJar()
	s.client.SetCookieJar(jar)
	s.mu.Unlock()
	return errOK
}

//export tg_session_set_cookie_store
func tg_session_set_cookie_store(sessionID *C.char, enable C.int) C.int {
	id := C.GoString(sessionID)
	apiSessionsMu.RLock()
	s, ok := apiSessions[id]
	apiSessionsMu.RUnlock()
	if !ok {
		return errSession
	}
	s.mu.Lock()
	if enable == 0 {
		s.client.SetCookieJar(nil)
	} else {
		jar := tls_client.NewCookieJar()
		s.client.SetCookieJar(jar)
	}
	s.mu.Unlock()
	return errOK
}

// ─── Requests ─────────────────────────────────────────────

//export tg_get
func tg_get(sessionID *C.char, requestURL *C.char) *C.TgResponse {
	return apiRequest(sessionID, "GET", requestURL, nil, nil)
}

//export tg_post
func tg_post(sessionID *C.char, requestURL *C.char, body *C.char) *C.TgResponse {
	var b *string
	if body != nil {
		s := C.GoString(body)
		b = &s
	}
	return apiRequest(sessionID, "POST", requestURL, b, nil)
}

//export tg_post_bin
func tg_post_bin(sessionID *C.char, requestURL *C.char, data unsafe.Pointer, dataLen C.int) *C.TgResponse {
	var bodyStr string
	if data != nil && dataLen > 0 {
		bodyStr = string(C.GoBytes(data, dataLen))
	}
	return apiRequest(sessionID, "POST", requestURL, &bodyStr, nil)
}

//export tg_request
func tg_request(sessionID *C.char, method *C.char, requestURL *C.char, headers *C.char, body *C.char) *C.TgResponse {
	var b *string
	if body != nil {
		s := C.GoString(body)
		b = &s
	}
	var h *string
	if headers != nil {
		s := C.GoString(headers)
		h = &s
	}
	return apiRequest(sessionID, C.GoString(method), requestURL, b, h)
}

// ─── Convenience ──────────────────────────────────────────

//export tg_get_body
func tg_get_body(sessionID *C.char, requestURL *C.char) *C.char {
	r := tg_get(sessionID, requestURL)
	if r == nil || r.errorCode != errOK {
		if r != nil {
			tg_response_free(r)
		}
		return nil
	}
	body := C.CString(C.GoString(r.body))
	tg_response_free(r)
	return body
}

//export tg_get_status
func tg_get_status(sessionID *C.char, requestURL *C.char) C.int {
	r := tg_get(sessionID, requestURL)
	if r == nil {
		return 0
	}
	s := r.status
	tg_response_free(r)
	return s
}

// ─── Response Access ──────────────────────────────────────

//export tg_response_status
func tg_response_status(r *C.TgResponse) C.int {
	if r == nil { return 0 }
	return r.status
}

//export tg_response_body
func tg_response_body(r *C.TgResponse) *C.char {
	if r == nil { return nil }
	return r.body
}

//export tg_response_body_len
func tg_response_body_len(r *C.TgResponse) C.int {
	if r == nil { return 0 }
	return r.bodyLen
}

//export tg_response_headers
func tg_response_headers(r *C.TgResponse) *C.char {
	if r == nil { return nil }
	return r.headers
}

//export tg_response_header
func tg_response_header(r *C.TgResponse, name *C.char) *C.char {
	if r == nil || r.headers == nil { return nil }
	key := C.GoString(name)
	for _, line := range strings.Split(C.GoString(r.headers), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), key) {
			return cString(strings.TrimSpace(parts[1]))
		}
	}
	return nil
}

//export tg_response_error
func tg_response_error(r *C.TgResponse) *C.char {
	if r == nil { return nil }
	return r.error
}

//export tg_response_error_code
func tg_response_error_code(r *C.TgResponse) C.int {
	if r == nil { return -1 }
	return r.errorCode
}

//export tg_response_free
func tg_response_free(r *C.TgResponse) {
	if r == nil { return }
	if r.body != nil    { C.free(unsafe.Pointer(r.body)) }
	if r.headers != nil { C.free(unsafe.Pointer(r.headers)) }
	if r.error != nil   { C.free(unsafe.Pointer(r.error)) }
	C.free(unsafe.Pointer(r))
}

// ─── Internal ─────────────────────────────────────────────

func apiRequest(sessionID *C.char, method string, requestURL *C.char, body, headers *string) *C.TgResponse {
	id := C.GoString(sessionID)

	apiSessionsMu.RLock()
	s, ok := apiSessions[id]
	apiSessionsMu.RUnlock()
	if !ok {
		return cErrorResponse(errSession, "session not found")
	}

	// ─── Anti-detection: rotation + TLS refresh ───────────
	s.mu.Lock()
	s.reqCount++

	// Profile rotation (Chaos mode: random profile every request + force TLS refresh)
	if s.rotateGroup == 6 {
		s.profileID = int(profiles.ChaosProfile())
		s.rebuild()
	} else if s.rotateGroup > 0 && s.rotateEvery > 0 && s.reqCount%s.rotateEvery == 0 {
		nextPID, err := profiles.NextRotateProfile(profiles.RotateGroup(s.rotateGroup), s.reqCount)
		if err == nil {
			s.profileID = int(nextPID)
			s.rebuild()
		}
	}

	// TLS context refresh (new ClientHello, new session ticket)
	// Chaos: always refresh; normal: periodic
	if s.rotateGroup == 6 || (s.tlsRefresh > 0 && s.reqCount%s.tlsRefresh == 0) {
		s.rebuild()
	}

	// Proxy rotation
	if len(s.proxyList) > 0 && s.proxyRotate > 0 && s.reqCount%s.proxyRotate == 0 {
		s.proxyIdx = (s.proxyIdx + 1) % len(s.proxyList)
		s.proxy = s.proxyList[s.proxyIdx]
		s.rebuild()
	}
	s.mu.Unlock()

	urlStr := C.GoString(requestURL)

	s.mu.Lock()
	client := s.client
	s.mu.Unlock()

	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, urlStr, strings.NewReader(*body))
	} else {
		req, err = http.NewRequest(method, urlStr, nil)
	}
	if err != nil {
		return cErrorResponse(errHTTP, fmt.Sprintf("build request: %v", err))
	}

	if reqURL, parseErr := url.Parse(urlStr); parseErr == nil {
		req.URL = reqURL
	}

	if headers != nil {
		for _, line := range strings.Split(*headers, "\n") {
			line = strings.TrimSpace(line)
			if line == "" { continue }
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		code := errNetwork
		errStr := err.Error()
		if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") {
			code = errTimeout
		}
		return cErrorResponse(C.int(code), fmt.Sprintf("request: %v", err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return cErrorResponse(errHTTP, fmt.Sprintf("read body: %v", err))
	}

	// Format response headers
	var headerLines []string
	keys := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range resp.Header[k] {
			headerLines = append(headerLines, k+": "+v)
		}
	}

	var headerStr *C.char
	if len(headerLines) > 0 {
		headerStr = cString(strings.Join(headerLines, "\n"))
	}

	return cResponse(resp.StatusCode, respBody, headerStr)
}

func cResponse(status int, body []byte, headerStr *C.char) *C.TgResponse {
	r := (*C.TgResponse)(C.malloc(C.size_t(unsafe.Sizeof(C.TgResponse{}))))
	r.status = C.int(status)
	r.errorCode = errOK
	r.bodyLen = C.int(len(body))
	r.body = cStringFromBytes(body)
	r.headers = headerStr
	r.error = nil
	return r
}

func cErrorResponse(code C.int, msg string) *C.TgResponse {
	r := (*C.TgResponse)(C.malloc(C.size_t(unsafe.Sizeof(C.TgResponse{}))))
	r.status = 0
	r.errorCode = code
	r.bodyLen = 0
	r.body = nil
	r.headers = nil
	r.error = cString(msg)
	return r
}

func cString(s string) *C.char { return C.CString(s) }

