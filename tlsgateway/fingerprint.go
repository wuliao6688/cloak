package tlsgateway

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/bogdanfinn/tls-client/internal/header"
)

// ─── H2 SETTINGS types ──────────────────────────────────────────────────

type H2SettingID uint16

const (
	H2SettingHeaderTableSize      H2SettingID = 0x1
	H2SettingEnablePush           H2SettingID = 0x2
	H2SettingMaxConcurrentStreams H2SettingID = 0x3
	H2SettingInitialWindowSize    H2SettingID = 0x4
	H2SettingMaxFrameSize         H2SettingID = 0x5
	H2SettingMaxHeaderListSize    H2SettingID = 0x6
)

type H2Setting struct {
	ID  H2SettingID
	Val uint32
}

// PriorityParam is an HTTP/2 PRIORITY parameter.
type PriorityParam struct {
	StreamDep uint32
	Exclusive bool
	Weight    uint8
}

// PriorityFrame is a single H2 PRIORITY frame sent after initial SETTINGS.
type PriorityFrame struct {
	StreamID      uint32
	PriorityParam PriorityParam
}

// H2Fingerprint holds the complete H2 fingerprint configuration
// for a specific browser brand and version.
type H2Fingerprint struct {
	// H2 SETTINGS frame values and order.
	Settings []H2Setting
	// Initial stream ID (Chrome=3, Firefox=1).
	InitialStreamID uint32
	// Connection-level flow control window.
	ConnectionFlow uint32
	// HEADERS frame priority.
	HeaderPriority PriorityParam
	// PRIORITY frames sent after SETTINGS (Firefox sends 6).
	PriorityFrames []PriorityFrame
	// Pseudo-header wire order.
	PseudoHeaderOrder []string
	// Regular header wire order.
	HeaderOrder []string
	// Default request headers (UA, Accept, Sec-CH-UA, etc.).
	Headers map[string]string
}

// ─── Browser fingerprints (from req) ────────────────────────────────────

var (
	// ─── Chrome 120 ──────────────────────────────────────────────────

	ChromeSettings = []H2Setting{
		{ID: H2SettingHeaderTableSize, Val: 65536},
		{ID: H2SettingEnablePush, Val: 0},
		{ID: H2SettingMaxConcurrentStreams, Val: 1000},
		{ID: H2SettingInitialWindowSize, Val: 6291456},
		{ID: H2SettingMaxHeaderListSize, Val: 262144},
	}

	ChromeConnectionFlow = uint32(15663105)

	ChromeHeaderPriority = PriorityParam{
		StreamDep: 0,
		Exclusive: true,
		Weight:    255,
	}

	ChromePseudoHeaderOrder = []string{
		":method", ":authority", ":scheme", ":path",
	}

	ChromeHeaderOrder = []string{
		"host", "pragma", "cache-control", "sec-ch-ua",
		"sec-ch-ua-mobile", "sec-ch-ua-platform",
		"upgrade-insecure-requests", "user-agent", "accept",
		"sec-fetch-site", "sec-fetch-mode", "sec-fetch-user",
		"sec-fetch-dest", "referer", "accept-encoding",
		"accept-language", "cookie",
	}

	ChromeHeaders = map[string]string{
		"pragma":                    "no-cache",
		"cache-control":             "no-cache",
		"sec-ch-ua":                 `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`,
		"sec-ch-ua-mobile":          "?0",
		"sec-ch-ua-platform":        `"macOS"`,
		"upgrade-insecure-requests": "1",
		"user-agent":                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"accept":                    "text/html,application/xhtml+xml,application/xml,application/json;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"sec-fetch-site":            "none",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-user":            "?1",
		"sec-fetch-dest":            "document",
		"accept-language":           "zh-CN,zh;q=0.9",
	}

	// ─── Firefox 120 ─────────────────────────────────────────────────

	FirefoxSettings = []H2Setting{
		{ID: H2SettingHeaderTableSize, Val: 65536},
		{ID: H2SettingInitialWindowSize, Val: 131072},
		{ID: H2SettingMaxFrameSize, Val: 16384},
	}

	FirefoxConnectionFlow = uint32(12517377)

	FirefoxPriorityFrames = []PriorityFrame{
		{StreamID: 3, PriorityParam: PriorityParam{StreamDep: 0, Exclusive: false, Weight: 200}},
		{StreamID: 5, PriorityParam: PriorityParam{StreamDep: 0, Exclusive: false, Weight: 100}},
		{StreamID: 7, PriorityParam: PriorityParam{StreamDep: 0, Exclusive: false, Weight: 0}},
		{StreamID: 9, PriorityParam: PriorityParam{StreamDep: 7, Exclusive: false, Weight: 0}},
		{StreamID: 11, PriorityParam: PriorityParam{StreamDep: 3, Exclusive: false, Weight: 0}},
		{StreamID: 13, PriorityParam: PriorityParam{StreamDep: 0, Exclusive: false, Weight: 240}},
	}

	FirefoxHeaderPriority = PriorityParam{
		StreamDep: 13,
		Exclusive: false,
		Weight:    41,
	}

	FirefoxPseudoHeaderOrder = []string{
		":method", ":path", ":authority", ":scheme",
	}

	FirefoxHeaderOrder = []string{
		"host", "user-agent", "accept", "accept-language",
		"accept-encoding", "referer", "cookie", "pragma",
		"cache-control", "sec-fetch-dest", "sec-fetch-mode",
		"sec-fetch-site", "sec-fetch-user", "upgrade-insecure-requests",
	}

	FirefoxHeaders = map[string]string{
		"user-agent":                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:120.0) Gecko/20100101 Firefox/120.0",
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		"accept-language":           "zh-CN,zh;q=0.8,zh-TW;q=0.7,zh-HK;q=0.5,en-US;q=0.3,en;q=0.2",
		"upgrade-insecure-requests": "1",
		"sec-fetch-dest":            "document",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-site":            "same-origin",
		"sec-fetch-user":            "?1",
	}

	// ─── Safari 17 ───────────────────────────────────────────────────

	SafariSettings = []H2Setting{
		{ID: H2SettingHeaderTableSize, Val: 65536},
		{ID: H2SettingEnablePush, Val: 0},
		{ID: H2SettingInitialWindowSize, Val: 2097152},
		{ID: H2SettingMaxHeaderListSize, Val: 262144},
	}

	SafariConnectionFlow = uint32(10485760)

	SafariHeaderPriority = PriorityParam{
		StreamDep: 0,
		Exclusive: true,
		Weight:    254,
	}

	SafariPseudoHeaderOrder = []string{
		":method", ":scheme", ":path", ":authority",
	}

	SafariHeaderOrder = []string{
		"host", "pragma", "cache-control",
		"upgrade-insecure-requests", "user-agent", "accept",
		"sec-fetch-site", "sec-fetch-mode", "sec-fetch-user",
		"sec-fetch-dest", "referer", "accept-encoding",
		"accept-language", "cookie",
	}

	SafariHeaders = map[string]string{
		"user-agent":                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"accept-language":           "zh-CN,zh-Hans;q=0.9",
		"upgrade-insecure-requests": "1",
		"sec-fetch-site":            "none",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-dest":            "document",
	}

	// ─── Edge (Chromium-based, shares Chrome H2) ─────────────────────

	EdgeHeaders = map[string]string{
		"user-agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
		"accept":                    ChromeHeaders["accept"],
		"accept-language":           "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6",
		"sec-ch-ua":                 `"Not_A Brand";v="8", "Chromium";v="120", "Microsoft Edge";v="120"`,
		"sec-ch-ua-mobile":          "?0",
		"sec-ch-ua-platform":        `"Windows"`,
		"upgrade-insecure-requests": "1",
		"sec-fetch-site":            "none",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-user":            "?1",
		"sec-fetch-dest":            "document",
	}

	// ─── QQ 浏览器 (Chromium-based) ──────────────────────────────────

	QQHeaders = map[string]string{
		"user-agent":                "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.5845.97 Safari/537.36 QQBrowser/13.0.0",
		"accept":                    ChromeHeaders["accept"],
		"accept-language":           "zh-CN,zh;q=0.9",
		"upgrade-insecure-requests": "1",
		"sec-fetch-site":            "none",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-dest":            "document",
	}

	// ─── 360 浏览器 (Chromium-based) ─────────────────────────────────

	B360Headers = map[string]string{
		"user-agent":                "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.5845.97 Safari/537.36 360EE",
		"accept":                    ChromeHeaders["accept"],
		"accept-language":           "zh-CN,zh;q=0.9",
		"upgrade-insecure-requests": "1",
		"sec-fetch-site":            "none",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-dest":            "document",
	}

	// ─── Android (OkHttp-based, matches req's SetTLSFingerprintAndroid) ─────

	AndroidSettings = ChromeSettings // Shares Chromium H2

	AndroidHeaders = map[string]string{
		"user-agent":      "Dalvik/2.1.0 (Linux; U; Android 13; Pixel 7 Build/TQ1A.221205.011)",
		"accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		"accept-language": "zh-CN,zh;q=0.9",
	}

	// ─── iOS (Safari WebKit-based) ────────────────────────────────────

	IOSHeaders = map[string]string{
		"user-agent":      "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
		"accept":          SafariHeaders["accept"],
		"accept-language": "zh-CN,zh-Hans;q=0.9",
	}
)

// BrowserFingerprint returns the full H2 fingerprint for a browser family.
// Recognize Chrome/Firefox/Safari by profile name prefix.
func BrowserFingerprint(name string) *H2Fingerprint {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "chrome") || strings.Contains(lower, "brave"):
		return &H2Fingerprint{
			Settings:          ChromeSettings,
			InitialStreamID:   3,
			ConnectionFlow:    ChromeConnectionFlow,
			HeaderPriority:    ChromeHeaderPriority,
			PseudoHeaderOrder: ChromePseudoHeaderOrder,
			HeaderOrder:       ChromeHeaderOrder,
			Headers:           ChromeHeaders,
		}
	case strings.Contains(lower, "firefox"):
		return &H2Fingerprint{
			Settings:          FirefoxSettings,
			InitialStreamID:   1,
			ConnectionFlow:    FirefoxConnectionFlow,
			HeaderPriority:    FirefoxHeaderPriority,
			PriorityFrames:    FirefoxPriorityFrames,
			PseudoHeaderOrder: FirefoxPseudoHeaderOrder,
			HeaderOrder:       FirefoxHeaderOrder,
			Headers:           FirefoxHeaders,
		}
	case strings.Contains(lower, "safari"):
		return &H2Fingerprint{
			Settings:          SafariSettings,
			InitialStreamID:   1,
			ConnectionFlow:    SafariConnectionFlow,
			HeaderPriority:    SafariHeaderPriority,
			PseudoHeaderOrder: SafariPseudoHeaderOrder,
			HeaderOrder:       SafariHeaderOrder,
			Headers:           SafariHeaders,
		}
	case strings.Contains(lower, "opera"):
		// Opera uses Chrome's H2 fingerprint.
		fp := BrowserFingerprint("chrome")
		fp.Headers = map[string]string{
			"user-agent":                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 OPR/91.0.0.0",
			"accept":                    ChromeHeaders["accept"],
			"accept-language":           ChromeHeaders["accept-language"],
			"sec-ch-ua":                 `"Opera";v="91", "Not)A;Brand";v="99"`,
			"sec-ch-ua-mobile":          "?0",
			"sec-ch-ua-platform":        `"macOS"`,
			"upgrade-insecure-requests": "1",
			"sec-fetch-site":            "none",
			"sec-fetch-mode":            "navigate",
			"sec-fetch-user":            "?1",
			"sec-fetch-dest":            "document",
		}
		return fp
	case strings.Contains(lower, "qq"):
		return &H2Fingerprint{
			Settings:          ChromeSettings,
			InitialStreamID:   3,
			ConnectionFlow:    ChromeConnectionFlow,
			HeaderPriority:    ChromeHeaderPriority,
			PseudoHeaderOrder: ChromePseudoHeaderOrder,
			HeaderOrder:       ChromeHeaderOrder,
			Headers:           QQHeaders,
		}
	case strings.Contains(lower, "360"):
		return &H2Fingerprint{
			Settings:          ChromeSettings,
			InitialStreamID:   3,
			ConnectionFlow:    ChromeConnectionFlow,
			HeaderPriority:    ChromeHeaderPriority,
			PseudoHeaderOrder: ChromePseudoHeaderOrder,
			HeaderOrder:       ChromeHeaderOrder,
			Headers:           B360Headers,
		}
	case strings.Contains(lower, "ios"):
		return &H2Fingerprint{
			Settings:          SafariSettings,
			InitialStreamID:   1,
			ConnectionFlow:    SafariConnectionFlow,
			HeaderPriority:    SafariHeaderPriority,
			PseudoHeaderOrder: SafariPseudoHeaderOrder,
			HeaderOrder:       SafariHeaderOrder,
			Headers:           IOSHeaders,
		}
	case strings.Contains(lower, "android") || strings.Contains(lower, "okhttp"):
		return &H2Fingerprint{
			Settings:          AndroidSettings,
			InitialStreamID:   3,
			ConnectionFlow:    ChromeConnectionFlow,
			HeaderPriority:    ChromeHeaderPriority,
			PseudoHeaderOrder: ChromePseudoHeaderOrder,
			HeaderOrder:       ChromeHeaderOrder,
			Headers:           AndroidHeaders,
		}
	case strings.Contains(lower, "edge"):
		return &H2Fingerprint{
			Settings:          ChromeSettings,
			InitialStreamID:   3,
			ConnectionFlow:    ChromeConnectionFlow,
			HeaderPriority:    ChromeHeaderPriority,
			PseudoHeaderOrder: ChromePseudoHeaderOrder,
			HeaderOrder:       ChromeHeaderOrder,
			Headers:           EdgeHeaders,
		}
	default:
		return &H2Fingerprint{
			Settings:          ChromeSettings,
			InitialStreamID:   3,
			ConnectionFlow:    ChromeConnectionFlow,
			HeaderPriority:    ChromeHeaderPriority,
			PseudoHeaderOrder: ChromePseudoHeaderOrder,
			HeaderOrder:       ChromeHeaderOrder,
			Headers:           ChromeHeaders,
		}
	}
}

// RandomFingerprint returns a randomly selected browser fingerprint.
// Equivalent to req's SetTLSFingerprintRandomized.
func RandomFingerprint() *H2Fingerprint {
	browsers := []func() *H2Fingerprint{
		func() *H2Fingerprint { return BrowserFingerprint("chrome") },
		func() *H2Fingerprint { return BrowserFingerprint("firefox") },
		func() *H2Fingerprint { return BrowserFingerprint("safari") },
		func() *H2Fingerprint { return BrowserFingerprint("edge") },
	}
	var b [1]byte
	rand.Read(b[:])
	return browsers[int(b[0])%len(browsers)]()
}

// ─── H2 Fingerprint constants (alias for backward compat) ──────────────

var (
	ChromeH2Settings         = ChromeSettings
	ChromeH2ConnectionFlow   = ChromeConnectionFlow
	ChromeH2HeaderPriority   = ChromeHeaderPriority
	FirefoxH2Settings        = FirefoxSettings
	FirefoxH2ConnectionFlow  = FirefoxConnectionFlow
	FirefoxH2PriorityFrames  = FirefoxPriorityFrames
	FirefoxH2HeaderPriority  = FirefoxHeaderPriority
	SafariH2Settings         = SafariSettings
	SafariH2ConnectionFlow   = SafariConnectionFlow
	SafariH2HeaderPriority   = SafariHeaderPriority
)

// ─── Header ordering ────────────────────────────────────────────────────

var orderMap sync.Map

type OrderedHeadersRoundTripper struct {
	transport         http.RoundTripper
	headerOrder       []string
	pseudoHeaderOrder []string
	orderMap          map[string]int
}

// Unwrap exposes the inner transport for option setters.
func (o *OrderedHeadersRoundTripper) Unwrap() http.RoundTripper {
	return o.transport
}

func NewOrderedHeadersRoundTripper(transport http.RoundTripper, headerOrder []string) *OrderedHeadersRoundTripper {
	return NewOrderedHeadersRoundTripperFull(transport, headerOrder, nil)
}

func NewOrderedHeadersRoundTripperFull(
	transport http.RoundTripper,
	headerOrder, pseudoHeaderOrder []string,
) *OrderedHeadersRoundTripper {
	om := make(map[string]int, len(headerOrder))
	for i, h := range headerOrder {
		om[strings.ToLower(h)] = i
	}
	return &OrderedHeadersRoundTripper{
		transport:         transport,
		headerOrder:       headerOrder,
		pseudoHeaderOrder: pseudoHeaderOrder,
		orderMap:          om,
	}
}

func (o *OrderedHeadersRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	oldHeaders := req.Header.Clone()
	for k := range req.Header {
		req.Header.Del(k)
	}

	added := make(map[string]bool)
	for _, h := range o.headerOrder {
		vals, ok := oldHeaders[h]
		if !ok {
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

	for k, vals := range oldHeaders {
		if !added[k] {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}
	}

	if len(o.headerOrder) > 0 {
		req.Header.Set(header.HeaderOrderKey, strings.Join(o.headerOrder, ","))
	}
	if len(o.pseudoHeaderOrder) > 0 {
		req.Header.Set(header.PseudoHeaderOrderKey, strings.Join(o.pseudoHeaderOrder, ","))
	}

	return o.transport.RoundTrip(req)
}

// ─── Multipart boundary ─────────────────────────────────────────────────

const webkitFormBoundaryAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789AB"

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
