package cloak

import (
	"net/http"
	"strings"

	"github.com/wuliao6688/cloak/profiles"
)

// HeaderRoundTripper wraps a Transport and injects profile-appropriate
// browser HTTP headers. This is needed for Akamai and similar CDNs that
// check HTTP headers (User-Agent, Accept, etc.) in addition to TLS
// ClientHello fingerprints.
//
// Does NOT depend on any fork — uses only standard library types.
type HeaderRoundTripper struct {
	transport http.RoundTripper
	headers   map[string]string
}

// NewHeaderRoundTripper creates a header-injecting wrapper that sets
// browser headers based on the profile.
func NewHeaderRoundTripper(transport http.RoundTripper, profile profiles.ClientProfile) *HeaderRoundTripper {
	headers := browserHeaders(profile)
	return &HeaderRoundTripper{
		transport: transport,
		headers:   headers,
	}
}

// RoundTrip implements http.RoundTripper. It adds browser headers
// before delegating to the wrapped transport.
func (h *HeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
	return h.transport.RoundTrip(req)
}

// Unwrap exposes the inner transport so option setters (e.g.
// SetInsecureSkipVerify) can reach the cloak.Transport underneath.
func (h *HeaderRoundTripper) Unwrap() http.RoundTripper {
	return h.transport
}

// CloseIdleConnections closes idle keep-alive connections in the inner
// transport (leak prevention for long-running workers).
func (h *HeaderRoundTripper) CloseIdleConnections() {
	if tr, ok := h.transport.(interface{ CloseIdleConnections() }); ok {
		tr.CloseIdleConnections()
	}
}

// browserHeaders returns browser-appropriate HTTP headers for the profile.
func browserHeaders(profile profiles.ClientProfile) map[string]string {
	// If the user already sets their own headers, don't override.
	// We only inject missing headers.
	name := profile.GetClientHelloStr()
	h := map[string]string{
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.9",
		"Cache-Control":   "max-age=0",
	}

	// Chrome family (Chrome, Brave, Edge — direct version)
	if strings.Contains(name, "Chrome") || strings.Contains(name, "chrome") ||
		strings.Contains(name, "Brave") || strings.Contains(name, "brave") ||
		strings.Contains(name, "Edge") || strings.Contains(name, "edge") {
		ver := extractVersion(name)
		h["User-Agent"] = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + ver + ".0.0.0 Safari/537.36"
		h["Sec-Ch-Ua"] = `"Chromium";v="` + ver + `", "Google Chrome";v="` + ver + `"`
		h["Sec-Ch-Ua-Platform"] = `"Windows"`
		h["Sec-Ch-Ua-Mobile"] = "?0"
		return h
	}

	// Opera: version is Opera's own numbering, Chrome engine = Opera + 18.
	// Opera 91 → Chrome 109, Opera 90 → Chrome 108, etc.
	if strings.Contains(name, "Opera") || strings.Contains(name, "opera") {
		operaVer := extractVersion(name)
		operaNum := atoi(operaVer)
		chromeVer := fmtVer(operaNum + 18)
		h["User-Agent"] = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeVer + ".0.0.0 Safari/537.36 OPR/" + operaVer + ".0.0.0"
		h["Sec-Ch-Ua"] = `"Chromium";v="` + chromeVer + `", "Not_A Brand";v="24", "Opera";v="` + operaVer + `"`
		h["Sec-Ch-Ua-Platform"] = `"Windows"`
		h["Sec-Ch-Ua-Mobile"] = "?0"
		return h
	}

	// Firefox family
	if strings.Contains(name, "Firefox") || strings.Contains(name, "firefox") {
		ver := extractVersion(name)
		h["User-Agent"] = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:" + ver + ".0) Gecko/20100101 Firefox/" + ver + ".0"
		return h
	}

	// Safari family
	if strings.Contains(name, "Safari") || strings.Contains(name, "safari") ||
		strings.Contains(name, "iOS") || strings.Contains(name, "ios") {
		h["User-Agent"] = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15"
		return h
	}

	// OkHttp / Android
	if strings.Contains(name, "okhttp") || strings.Contains(name, "OkHttp") ||
		strings.Contains(name, "Android") || strings.Contains(name, "android") {
		h["User-Agent"] = "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Mobile Safari/537.36"
		return h
	}

	// Fallback: generic browser UA
	h["User-Agent"] = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	return h
}

// extractVersion pulls the version number from a profile name like "Chrome-150" or "Firefox-148".
func extractVersion(name string) string {
	for i, c := range name {
		if c >= '0' && c <= '9' {
			return name[i:]
		}
	}
	return "150"
}

// atoi parses an integer from a string, returning 0 on error.
func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// fmtVer formats a version number as a string.
func fmtVer(v int) string {
	if v <= 0 {
		return "150"
	}
	result := ""
	for v > 0 {
		result = string(rune('0'+v%10)) + result
		v /= 10
	}
	return result
}
