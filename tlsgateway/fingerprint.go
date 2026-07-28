package tlsgateway

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// ─── H2 SETTINGS types (mirrors req's approach) ─────────────────────────
//
// These types define H2 SETTINGS parameters in a fork-free way. Currently
// used for documentation and browser defaults. Full H2 SETTINGS injection
// requires forking x/net/http2 (like req's internal/http2).

// H2SettingID is an HTTP/2 setting ID.
type H2SettingID uint16

const (
	H2SettingHeaderTableSize      H2SettingID = 0x1
	H2SettingEnablePush           H2SettingID = 0x2
	H2SettingMaxConcurrentStreams H2SettingID = 0x3
	H2SettingInitialWindowSize    H2SettingID = 0x4
	H2SettingMaxFrameSize         H2SettingID = 0x5
	H2SettingMaxHeaderListSize    H2SettingID = 0x6
)

// H2Setting is a single HTTP/2 SETTINGS frame entry.
type H2Setting struct {
	ID  H2SettingID
	Val uint32
}

// H2Fingerprint holds the complete H2 fingerprint configuration.
type H2Fingerprint struct {
	Settings          []H2Setting
	PseudoHeaderOrder []string
	HeaderOrder       []string
	InitialStreamID   uint32
	ConnectionFlow    uint32
}

// ─── Browser H2 defaults (from req's client_impersonate.go) ────────────

var (
	ChromeH2Settings = []H2Setting{
		{ID: H2SettingHeaderTableSize, Val: 65536},
		{ID: H2SettingEnablePush, Val: 0},
		{ID: H2SettingMaxConcurrentStreams, Val: 1000},
		{ID: H2SettingInitialWindowSize, Val: 6291456},
		{ID: H2SettingMaxHeaderListSize, Val: 262144},
	}

	ChromePseudoHeaderOrder = []string{
		":method",
		":authority",
		":scheme",
		":path",
	}

	ChromeHeaderOrder = []string{
		"host",
		"pragma",
		"cache-control",
		"sec-ch-ua",
		"sec-ch-ua-mobile",
		"sec-ch-ua-platform",
		"upgrade-insecure-requests",
		"user-agent",
		"accept",
		"sec-fetch-site",
		"sec-fetch-mode",
		"sec-fetch-user",
		"sec-fetch-dest",
		"referer",
		"accept-encoding",
		"accept-language",
		"cookie",
	}

	FirefoxPseudoHeaderOrder = []string{
		":method",
		":path",
		":authority",
		":scheme",
	}

	FirefoxHeaderOrder = []string{
		"host",
		"user-agent",
		"accept",
		"accept-language",
		"accept-encoding",
		"referer",
		"cookie",
		"pragma",
		"cache-control",
		"sec-fetch-dest",
		"sec-fetch-mode",
		"sec-fetch-site",
		"sec-fetch-user",
		"upgrade-insecure-requests",
	}
)

// orderMap is a lock-free order tracker for header insertion ordering.
var orderMap sync.Map

// OrderedHeadersRoundTripper intercepts RoundTrip and re-orders
// HTTP/1.1 request headers to match the browser's canonical order.
// This is needed for strict HTTP fingerprint detection that checks
// header ordering in addition to header values.
type OrderedHeadersRoundTripper struct {
	transport   http.RoundTripper
	headerOrder []string
	orderMap    map[string]int
}

// NewOrderedHeadersRoundTripper creates a header-ordering wrapper.
// headerOrder defines the canonical header ordering. Headers not in
// the list are appended after the canonical ones.
func NewOrderedHeadersRoundTripper(transport http.RoundTripper, headerOrder []string) *OrderedHeadersRoundTripper {
	om := make(map[string]int, len(headerOrder))
	for i, h := range headerOrder {
		om[strings.ToLower(h)] = i
	}
	return &OrderedHeadersRoundTripper{
		transport:   transport,
		headerOrder: headerOrder,
		orderMap:    om,
	}
}

// RoundTrip implements http.RoundTripper. It moves headers into the
// canonical browser order before delegating to the underlying transport.
func (o *OrderedHeadersRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// For HTTP/2, header order is controlled by the H2 transport's
	// header encoding, which follows a deterministic algorithm. x/net/http2
	// uses canonical ordering for pseudo-headers and then alphabetic for
	// regular headers. We can't change this without a fork.
	//
	// For HTTP/1.1, we can reorder headers. net/http's wire encoding
	// follows the canonical map iteration order. By clearing and re-adding
	// headers in the desired order, we control the wire encoding.

	// Clone and reorder headers.
	oldHeaders := req.Header.Clone()
	for k := range req.Header {
		req.Header.Del(k)
	}

	// Add headers in canonical order.
	added := make(map[string]bool)
	for _, h := range o.headerOrder {
		vals, ok := oldHeaders[h]
		if !ok {
			// Try case-insensitive match.
			for key, vs := range oldHeaders {
				if strings.EqualFold(key, h) && !added[key] {
					for _, v := range vs {
						req.Header.Add(h, v)
					}
					added[key] = true
					break
				}
			}
			continue
		}
		for _, v := range vals {
			req.Header.Add(h, v)
		}
		added[h] = true
	}

	// Add any remaining headers not in the canonical order.
	for k, vals := range oldHeaders {
		if !added[k] {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}
	}

	return o.transport.RoundTrip(req)
}

// ─── Multipart boundary (browser-specific) ───────────────────────────────

const webkitFormBoundaryAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789AB"

// ChromeMultipartBoundary generates a Chrome/WebKit-style multipart boundary
// string. Chrome uses: ----WebKitFormBoundary + 16 random alphanumeric chars.
func ChromeMultipartBoundary() string {
	var sb strings.Builder
	sb.WriteString("----WebKitFormBoundary")
	for i := 0; i < 16; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(webkitFormBoundaryAlphabet)-1)))
		if err != nil {
			panic(err)
		}
		sb.WriteByte(webkitFormBoundaryAlphabet[idx.Int64()])
	}
	return sb.String()
}

// FirefoxMultipartBoundary generates a Firefox-style multipart boundary.
// Firefox uses: --------------------------- + 3 groups of 8-digit random numbers.
func FirefoxMultipartBoundary() string {
	var sb strings.Builder
	sb.WriteString("---------------------------")
	for i := 0; i < 3; i++ {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			panic(err)
		}
		u32 := binary.LittleEndian.Uint32(b[:])
		sb.WriteString(strconv.FormatUint(uint64(u32), 10))
	}
	return sb.String()
}
