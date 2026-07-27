// tlsgateway_api.go — 商用级 CFFI API（零 JSON，类型安全）
//
// 设计原则：
//   1. 零 JSON — 调用方不需要构造/解析 JSON
//   2. Session 模型 — 复用连接/Cookie，Thread-safe
//   3. 简单 — 6 个核心函数覆盖 90% 场景
//   4. 内存安全 — 显式 free，无泄漏
//
// 用法（C）:
//   char* s = tg_session_create(TLS_PROFILE_CHROME_150, 30, NULL);
//   TgResponse* r = tg_get(s, "https://httpbin.org/ip");
//   printf("status=%d body=%.*s\n", tg_response_status(r), tg_response_body_len(r), tg_response_body(r));
//   tg_response_free(r); tg_session_free(s);
//
// 用法（易语言）:
//   session = tg_session_create(1, 30, "")  ' Chrome 150, 30s timeout
//   resp = tg_get(session, "https://httpbin.org/ip")
//   状态码 = tg_response_status(resp)
//   返回文本 = tg_response_body(resp)
//   tg_response_free(resp)
//   tg_session_free(session)

package main

/*
#include <stdlib.h>

typedef struct {
	int    status;
	char*  body;
	int    bodyLen;
	char*  error;
} TgResponse;
*/
import "C"
import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"unsafe"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/google/uuid"
)

// ─── Session ──────────────────────────────────────────────

type apiSession struct {
	id        string
	profileID int
	client    tls_client.HttpClient
	timeout   int
	proxy     string
	mu        sync.Mutex
}

var (
	apiSessions   = make(map[string]*apiSession)
	apiSessionsMu sync.RWMutex
)

// ─── Core API ─────────────────────────────────────────────

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
		return cString(fmt.Sprintf("ERR: unknown profile ID %d", pid))
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
		return cString(fmt.Sprintf("ERR: %v", err))
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

// ─── Request API ──────────────────────────────────────────

//export tg_get
func tg_get(sessionID *C.char, requestURL *C.char) *C.TgResponse {
	return apiRequest(sessionID, "GET", C.GoString(requestURL), nil, nil)
}

//export tg_post
func tg_post(sessionID *C.char, requestURL *C.char, body *C.char) *C.TgResponse {
	var b *string
	if body != nil {
		s := C.GoString(body)
		b = &s
	}
	return apiRequest(sessionID, "POST", C.GoString(requestURL), b, nil)
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
	return apiRequest(sessionID, C.GoString(method), C.GoString(requestURL), b, h)
}

// ─── Response Access ──────────────────────────────────────

//export tg_response_status
func tg_response_status(r *C.TgResponse) C.int {
	if r == nil {
		return 0
	}
	return r.status
}

//export tg_response_body
func tg_response_body(r *C.TgResponse) *C.char {
	if r == nil {
		return nil
	}
	return r.body
}

//export tg_response_body_len
func tg_response_body_len(r *C.TgResponse) C.int {
	if r == nil {
		return 0
	}
	return r.bodyLen
}

//export tg_response_error
func tg_response_error(r *C.TgResponse) *C.char {
	if r == nil {
		return nil
	}
	return r.error
}

//export tg_response_free
func tg_response_free(r *C.TgResponse) {
	if r == nil {
		return
	}
	if r.body != nil {
		C.free(unsafe.Pointer(r.body))
	}
	if r.error != nil {
		C.free(unsafe.Pointer(r.error))
	}
	C.free(unsafe.Pointer(r))
}

// ─── Internal ─────────────────────────────────────────────

func apiRequest(sessionID *C.char, method, urlStr string, body, headers *string) *C.TgResponse {
	id := C.GoString(sessionID)

	apiSessionsMu.RLock()
	s, ok := apiSessions[id]
	apiSessionsMu.RUnlock()
	if !ok {
		return cErrorResponse(fmt.Sprintf("session not found: %s", id))
	}

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
		return cErrorResponse(fmt.Sprintf("build request: %v", err))
	}

	// Parse URL for cookie handling
	if reqURL, parseErr := url.Parse(urlStr); parseErr == nil {
		req.URL = reqURL
	}

	// Apply per-request headers
	if headers != nil {
		for _, line := range strings.Split(*headers, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return cErrorResponse(fmt.Sprintf("request: %v", err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return cErrorResponse(fmt.Sprintf("read body: %v", err))
	}

	return cResponse(resp.StatusCode, respBody)
}

func cResponse(status int, body []byte) *C.TgResponse {
	r := (*C.TgResponse)(C.malloc(C.size_t(unsafe.Sizeof(C.TgResponse{}))))
	r.status = C.int(status)
	r.bodyLen = C.int(len(body))
	r.body = cStringFromBytes(body)
	r.error = nil
	return r
}

func cErrorResponse(msg string) *C.TgResponse {
	r := (*C.TgResponse)(C.malloc(C.size_t(unsafe.Sizeof(C.TgResponse{}))))
	r.status = 0
	r.bodyLen = 0
	r.body = nil
	r.error = cString(msg)
	return r
}

func cString(s string) *C.char {
	return C.CString(s)
}

func cStringFromBytes(value []byte) *C.char {
	if len(value) == 0 {
		return C.CString("")
	}
	buffer := C.malloc(C.size_t(len(value) + 1))
	if buffer == nil {
		return nil
	}
	bytes := unsafe.Slice((*byte)(buffer), len(value)+1)
	copy(bytes, value)
	bytes[len(value)] = 0
	return (*C.char)(buffer)
}
