package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"
	"unsafe"

	http "github.com/bogdanfinn/fhttp"
	tls_client_cffi_src "github.com/bogdanfinn/tls-client/cffi_src"
	"github.com/google/uuid"
)

var (
	unsafePointers    = make(map[string]*C.char)
	unsafePointersLck = sync.Mutex{}
)

// storeJSONResponse copies marshaled JSON directly into C-owned memory. This
// avoids materializing a second payload-sized Go string before C.CString makes
// its own copy. The returned pointer keeps the existing freeMemory ABI.
func storeJSONResponse(responseID string, jsonResponse []byte) *C.char {
	response := cStringFromBytes(jsonResponse)
	if response == nil {
		return nil
	}

	unsafePointersLck.Lock()
	unsafePointers[responseID] = response
	unsafePointersLck.Unlock()
	return response
}

func cStringFromBytes(value []byte) *C.char {
	buffer := C.malloc(C.size_t(len(value) + 1))
	if buffer == nil {
		return nil
	}
	bytes := unsafe.Slice((*byte)(buffer), len(value)+1)
	copy(bytes, value)
	bytes[len(value)] = 0
	return (*C.char)(buffer)
}

//export freeMemory
func freeMemory(responseId *C.char) {
	responseIdString := C.GoString(responseId)

	unsafePointersLck.Lock()
	defer unsafePointersLck.Unlock()

	ptr, ok := unsafePointers[responseIdString]

	if !ok {
		return
	}

	C.free(unsafe.Pointer(ptr))

	delete(unsafePointers, responseIdString)
}

//export destroyAll
func destroyAll() *C.char {
	tls_client_cffi_src.ClearSessionCache()

	out := tls_client_cffi_src.DestroyOutput{
		Id:      uuid.New().String(),
		Success: true,
	}

	jsonResponse, marshallError := json.Marshal(out)

	if marshallError != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(marshallError)

		return handleErrorResponse("", false, clientErr)
	}

	return storeJSONResponse(out.Id, jsonResponse)
}

//export destroySession
func destroySession(destroySessionParams *C.char) *C.char {
	destroySessionParamsJson := C.GoString(destroySessionParams)

	destroySessionInput := tls_client_cffi_src.DestroySessionInput{}
	marshallError := json.Unmarshal([]byte(destroySessionParamsJson), &destroySessionInput)

	if marshallError != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(marshallError)

		return handleErrorResponse("", false, clientErr)
	}

	tls_client_cffi_src.RemoveSession(destroySessionInput.SessionId)

	out := tls_client_cffi_src.DestroyOutput{
		Id:      uuid.New().String(),
		Success: true,
	}

	jsonResponse, marshallError := json.Marshal(out)

	if marshallError != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(marshallError)

		return handleErrorResponse(destroySessionInput.SessionId, true, clientErr)
	}

	return storeJSONResponse(out.Id, jsonResponse)
}

//export configureSessionCache
func configureSessionCache(configurationParams *C.char) *C.char {
	configurationJSON := C.GoString(configurationParams)
	configuration := tls_client_cffi_src.ConfigureSessionCacheInput{}
	if err := json.Unmarshal([]byte(configurationJSON), &configuration); err != nil {
		return handleErrorResponse("", false, tls_client_cffi_src.NewTLSClientError(err))
	}

	idleTTL := time.Duration(0)
	if configuration.IdleTTL != "" {
		parsedTTL, err := time.ParseDuration(configuration.IdleTTL)
		if err != nil {
			return handleErrorResponse("", false, tls_client_cffi_src.NewTLSClientError(fmt.Errorf("invalid idleTTL: %w", err)))
		}
		idleTTL = parsedTTL
	}
	if err := tls_client_cffi_src.ConfigureSessionCache(configuration.MaxEntries, idleTTL); err != nil {
		return handleErrorResponse("", false, tls_client_cffi_src.NewTLSClientError(err))
	}

	maxEntries, configuredTTL := tls_client_cffi_src.SessionCacheConfiguration()
	out := tls_client_cffi_src.SessionCacheConfigurationOutput{
		Id:         uuid.New().String(),
		IdleTTL:    configuredTTL.String(),
		MaxEntries: maxEntries,
		Success:    true,
	}
	jsonResponse, err := json.Marshal(out)
	if err != nil {
		return handleErrorResponse("", false, tls_client_cffi_src.NewTLSClientError(err))
	}

	return storeJSONResponse(out.Id, jsonResponse)
}

//export getCookiesFromSession
func getCookiesFromSession(getCookiesParams *C.char) *C.char {
	getCookiesParamsJson := C.GoString(getCookiesParams)

	cookiesInput := tls_client_cffi_src.GetCookiesFromSessionInput{}
	marshallError := json.Unmarshal([]byte(getCookiesParamsJson), &cookiesInput)

	if marshallError != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(marshallError)

		return handleErrorResponse("", false, clientErr)
	}

	tlsClient, releaseSession, err := tls_client_cffi_src.GetClientForSessionOperation(cookiesInput.SessionId)
	if err != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(err)

		return handleErrorResponse(cookiesInput.SessionId, true, clientErr)
	}
	defer releaseSession()

	u, parsErr := url.Parse(cookiesInput.Url)
	if parsErr != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(parsErr)

		return handleErrorResponse(cookiesInput.SessionId, true, clientErr)
	}

	cookies := tlsClient.GetCookies(u)

	out := tls_client_cffi_src.CookiesFromSessionOutput{
		Id:      uuid.New().String(),
		Cookies: transformCookies(cookies),
	}

	jsonResponse, marshallError := json.Marshal(out)

	if marshallError != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(marshallError)

		return handleErrorResponse(cookiesInput.SessionId, true, clientErr)
	}

	return storeJSONResponse(out.Id, jsonResponse)
}

//export addCookiesToSession
func addCookiesToSession(addCookiesParams *C.char) *C.char {
	addCookiesParamsJson := C.GoString(addCookiesParams)

	cookiesInput := tls_client_cffi_src.AddCookiesToSessionInput{}
	marshallError := json.Unmarshal([]byte(addCookiesParamsJson), &cookiesInput)

	if marshallError != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(marshallError)

		return handleErrorResponse("", false, clientErr)
	}

	tlsClient, releaseSession, err := tls_client_cffi_src.GetClientForSessionOperation(cookiesInput.SessionId)
	if err != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(err)

		return handleErrorResponse(cookiesInput.SessionId, true, clientErr)
	}
	defer releaseSession()

	u, parsErr := url.Parse(cookiesInput.Url)
	if parsErr != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(parsErr)

		return handleErrorResponse(cookiesInput.SessionId, true, clientErr)
	}

	tlsClient.SetCookies(u, buildCookies(cookiesInput.Cookies))

	allCookies := tlsClient.GetCookies(u)

	out := tls_client_cffi_src.CookiesFromSessionOutput{
		Id:      uuid.New().String(),
		Cookies: transformCookies(allCookies),
	}

	jsonResponse, marshallError := json.Marshal(out)

	if marshallError != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(marshallError)

		return handleErrorResponse(cookiesInput.SessionId, true, clientErr)
	}

	return storeJSONResponse(out.Id, jsonResponse)
}

//export request
func request(requestParams *C.char) *C.char {
	requestParamsJson := C.GoString(requestParams)

	requestInput := tls_client_cffi_src.RequestInput{}
	marshallError := json.Unmarshal([]byte(requestParamsJson), &requestInput)

	if marshallError != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(marshallError)
		return handleErrorResponse("", false, clientErr)
	}

	return doRequest(requestInput)
}

func handleErrorResponse(sessionId string, withSession bool, err *tls_client_cffi_src.TLSClientError) *C.char {
	response := tls_client_cffi_src.Response{
		Id:      uuid.New().String(),
		Status:  0,
		Body:    err.Error(),
		Headers: nil,
		Cookies: nil,
	}

	if withSession {
		response.SessionId = sessionId
	}

	jsonResponse, marshallError := json.Marshal(response)

	if marshallError != nil {
		errStr := C.CString(marshallError.Error())

		return errStr
	}

	return storeJSONResponse(response.Id, jsonResponse)
}

func buildCookies(cookies []tls_client_cffi_src.Cookie) []*http.Cookie {
	var ret []*http.Cookie

	for _, cookie := range cookies {
		ret = append(ret, &http.Cookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Domain,
			Expires:  cookie.Expires.Time,
			MaxAge:   cookie.MaxAge,
			Secure:   cookie.Secure,
			HttpOnly: cookie.HttpOnly,
		})
	}

	return ret
}

func transformCookies(cookies []*http.Cookie) []tls_client_cffi_src.Cookie {
	var ret []tls_client_cffi_src.Cookie

	for _, cookie := range cookies {
		ret = append(ret, tls_client_cffi_src.Cookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Domain,
			MaxAge:   cookie.MaxAge,
			Secure:   cookie.Secure,
			HttpOnly: cookie.HttpOnly,
			Expires: tls_client_cffi_src.Timestamp{
				Time: cookie.Expires,
			},
		})
	}

	return ret
}

func main() {
}

//export requestWithProfileId
func requestWithProfileId(requestParams *C.char, profileId C.int) *C.char {
	requestParamsJson := C.GoString(requestParams)

	requestInput := tls_client_cffi_src.RequestInput{}
	marshallError := json.Unmarshal([]byte(requestParamsJson), &requestInput)
	if marshallError != nil {
		clientErr := tls_client_cffi_src.NewTLSClientError(marshallError)
		return handleErrorResponse("", false, clientErr)
	}

	// Override profile from integer ID (ignored if JSON already set a string identifier).
	if requestInput.TLSClientIdentifier == "" {
		requestInput.ProfileID = int(profileId)
	}

	return doRequest(requestInput)
}

// doRequest is the shared request handler used by request() and requestWithProfileId().
func doRequest(requestInput tls_client_cffi_src.RequestInput) *C.char {
	tlsClient, sessionId, withSession, releaseSession, err := tls_client_cffi_src.CreateClientForRequest(requestInput)
	if releaseSession != nil {
		defer releaseSession()
	}
	if err != nil {
		return handleErrorResponse(sessionId, withSession, err)
	}

	req, err := tls_client_cffi_src.BuildRequest(requestInput)
	if err != nil {
		return handleErrorResponse(sessionId, withSession, tls_client_cffi_src.NewTLSClientError(err))
	}

	cookies := buildCookies(requestInput.RequestCookies)
	if tlsClient.GetCookieJar() != nil && len(cookies) > 0 {
		tlsClient.SetCookies(req.URL, cookies)
	} else {
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
	}

	resp, reqErr := tlsClient.Do(req)
	if reqErr != nil {
		return handleErrorResponse(sessionId, withSession,
			tls_client_cffi_src.NewTLSClientError(fmt.Errorf("failed to do request: %w", reqErr)))
	}
	if resp == nil {
		return handleErrorResponse(sessionId, withSession,
			tls_client_cffi_src.NewTLSClientError(fmt.Errorf("response is nil")))
	}

	var targetCookies []*http.Cookie
	if resp.Request != nil && resp.Request.URL != nil {
		targetCookies = tlsClient.GetCookies(resp.Request.URL)
	}

	response, err := tls_client_cffi_src.BuildResponse(sessionId, withSession, resp, targetCookies, requestInput)
	if err != nil {
		return handleErrorResponse(sessionId, withSession, err)
	}

	jsonResponse, marshallError := json.Marshal(response)
	if marshallError != nil {
		return handleErrorResponse(sessionId, withSession,
			tls_client_cffi_src.NewTLSClientError(fmt.Errorf("failed to marshal response: %w", marshallError)))
	}

	return storeJSONResponse(response.Id, jsonResponse)
}
