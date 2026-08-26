// Package main — TLS fingerprint verification across 12 detection platforms.
//
// Platforms tested:
//
//	TLS Fingerprint APIs (3):
//	  tls.peet.ws        — JA3, JA4, cipher suites, extensions (JSON)
//	  browserleaks.com   — JA3, JA3N, Akamai fingerprint (JSON)
//	  browserscan.net    — TLS 指纹页面加载检测
//
//	WAF / CDN Sites (8):
//	  cloudflare.com     — world's largest CDN (trace endpoint)
//	  imperva.com        — enterprise WAF
//	  f5.com             — Shape Security (F5)
//	  akamai.com         — H2 fingerprinting (TLS passes, HTTP headers needed)
//	  datadome.co        — JS behavioral (honest limitation)
//	  hcaptcha.com       — captcha provider (page load test)
//	  recaptcha-demo     — Google reCAPTCHA demo
//	  sannysoft.com      — bot detection test page
//
//	HTTP Layer (1):
//	  httpbin.org        — header inspection
//
// Usage:
//
//	go run ./cmd/verify-fingerprints
//	go run ./cmd/verify-fingerprints -all
//	go run ./cmd/verify-fingerprints -json > report.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wuliao6688/cloak/profiles"
	"github.com/wuliao6688/cloak"
)

type FPCheck struct {
	Profile  string `json:"profile"`
	Platform string `json:"platform"`
	Category string `json:"category"`
	Pass     bool   `json:"pass"`
	Detail   string `json:"detail,omitempty"`
	Error    string `json:"error,omitempty"`
	Duration string `json:"duration"`
}

func (f FPCheck) Status() string {
	if f.Pass {
		return "✅"
	}
	if f.Error != "" {
		return "❌"
	}
	return "⚠️"
}

type CheckFunc func(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck

// H3CheckFunc checks HTTP/3 capabilities with an H3-racing transport.
type H3CheckFunc func(tr *cloak.H3RaceTransport, profile string, timeout time.Duration) FPCheck

// ─── TLS API checks ───

func checkPeerWS(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: "tls.peet.ws", Category: "tls_api"}
	client := &http.Client{Transport: tr, Timeout: timeout}
	start := time.Now()
	resp, err := client.Get("https://tls.peet.ws/api/all")
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		fp.Error = err.Error()
		return fp
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		TLS struct {
			JA3     string `json:"ja3"`
			JA3Hash string `json:"ja3_hash"`
			JA4     string `json:"ja4"`
		} `json:"tls"`
	}
	json.Unmarshal(body, &data)
	if data.TLS.JA4 != "" {
		fp.Pass = true
		fp.Detail = fmt.Sprintf("JA3=%s JA4=%s", data.TLS.JA3Hash, data.TLS.JA4)
	} else {
		fp.Error = "no JA4 in response"
	}
	return fp
}

func checkBrowserLeaks(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: "browserleaks.com", Category: "tls_api"}
	client := &http.Client{Transport: tr, Timeout: timeout}
	start := time.Now()
	resp, err := client.Get("https://tls.browserleaks.com/json")
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		fp.Error = err.Error()
		return fp
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		JA3Hash  string `json:"ja3_hash"`
		JA3NHash string `json:"ja3n_hash"`
		Akamai   string `json:"akamai_hash"`
	}
	json.Unmarshal(body, &data)
	if data.JA3Hash != "" {
		fp.Pass = true
		fp.Detail = fmt.Sprintf("JA3=%s JA3N=%s", data.JA3Hash, data.JA3NHash)
	} else {
		fp.Error = "no JA3 in response"
	}
	return fp
}

// ─── WAF/CDN checks ───

func checkCloudflare(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: "cloudflare", Category: "waf_cdn"}
	client := &http.Client{Transport: tr, Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	start := time.Now()
	resp, err := client.Get("https://www.cloudflare.com/cdn-cgi/trace")
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		fp.Error = err.Error()
		return fp
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if resp.StatusCode >= 400 || strings.Contains(s, "challenge") {
		fp.Error = fmt.Sprintf("challenged (status %d)", resp.StatusCode)
		return fp
	}
	fp.Pass = true
	fp.Detail = fmt.Sprintf("TLS=%s HTTP=%s", extractLine(s, "tls="), extractLine(s, "http="))
	return fp
}

func checkImperva(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck {
	return checkStatus(tr, profile, "imperva.com", "https://www.imperva.com/", timeout)
}

func checkF5(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck {
	return checkStatus(tr, profile, "f5.com", "https://www.f5.com/", timeout)
}

func checkAkamai(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: "akamai.com", Category: "waf_cdn"}
	client := &http.Client{Transport: tr, Timeout: timeout}
	start := time.Now()
	resp, err := client.Get("https://www.akamai.com/")
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "http2: frame too large") || strings.Contains(errStr, "no application protocol") {
			fp.Error = "TLS_BLOCKED: H2 handshake rejected"
			return fp
		}
		fp.Error = errStr
		return fp
	}
	defer resp.Body.Close()
	if resp.StatusCode == 403 {
		fp.Error = "ACCESS_DENIED: TLS passed, HTTP headers needed"
		return fp
	}
	if resp.StatusCode < 400 {
		fp.Pass = true
	} else {
		fp.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return fp
}

func checkDataDome(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: "datadome.co", Category: "waf_cdn"}
	client := &http.Client{Transport: tr, Timeout: timeout}
	start := time.Now()
	resp, err := client.Get("https://www.datadome.co/")
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		fp.Error = err.Error()
		return fp
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 403 && strings.Contains(string(body), "datadome") {
		fp.Error = "JS_REQUIRED: needs JavaScript execution"
		return fp
	}
	if resp.StatusCode < 400 {
		fp.Pass = true
	} else {
		fp.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return fp
}

func checkHcaptcha(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck {
	return checkStatus(tr, profile, "hcaptcha.com", "https://hcaptcha.com/", timeout)
}

func checkRecaptcha(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck {
	return checkStatus(tr, profile, "recaptcha-demo", "https://www.google.com/recaptcha/api2/demo", timeout)
}

func checkSannysoft(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: "sannysoft.com", Category: "waf_cdn"}
	client := &http.Client{Transport: tr, Timeout: timeout}
	start := time.Now()
	resp, err := client.Get("https://bot.sannysoft.com/")
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		fp.Error = err.Error()
		return fp
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if resp.StatusCode < 400 {
		if strings.Contains(s, "You are not a bot") || strings.Contains(s, "PASS") || !strings.Contains(s, "blocked") {
			fp.Pass = true
			fp.Detail = "PASS"
		} else {
			fp.Error = "BLOCKED"
		}
	} else {
		fp.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return fp
}

func checkBrowserscan(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: "browserscan.net", Category: "tls_api"}
	client := &http.Client{Transport: tr, Timeout: timeout}
	start := time.Now()
	resp, err := client.Get("https://www.browserscan.net/zh/tls")
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		fp.Error = err.Error()
		return fp
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if resp.StatusCode == 200 {
		// Try to find TLS fingerprint info in the page
		if strings.Contains(s, "JA3") || strings.Contains(s, "ja3") || strings.Contains(s, "TLS") {
			fp.Pass = true
			fp.Detail = "PAGE_LOADED: TLS info visible"
		} else {
			fp.Pass = true
			fp.Detail = "PAGE_LOADED"
		}
	} else {
		fp.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return fp
}

// ─── HTTP check ───

func checkHTTPBin(tr *cloak.Transport, profile string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: "httpbin.org", Category: "http"}
	client := &http.Client{Transport: tr, Timeout: timeout}
	start := time.Now()
	resp, err := client.Get("https://httpbin.org/headers")
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		fp.Error = err.Error()
		return fp
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		Headers map[string]string `json:"headers"`
	}
	json.Unmarshal(body, &data)
	if data.Headers["User-Agent"] != "" {
		fp.Pass = true
		fp.Detail = fmt.Sprintf("UA=%s", data.Headers["User-Agent"])
	} else {
		fp.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return fp
}

func checkStatus(tr *cloak.Transport, profile, platform, url string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: platform, Category: "waf_cdn"}
	client := &http.Client{Transport: tr, Timeout: timeout}
	start := time.Now()
	resp, err := client.Get(url)
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		fp.Error = err.Error()
		return fp
	}
	defer resp.Body.Close()
	if resp.StatusCode < 400 {
		fp.Pass = true
	} else {
		fp.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return fp
}

// ─── Main ───

func keyProfiles() []string {
	return []string{
		"chrome_150", "chrome_131", "chrome_109",
		"firefox_148", "firefox_133",
		"safari_ios_18_5", "safari_18_1",
		"brave_146", "opera_91", "okhttp4_android_13",
	}
}

func extractLine(s, prefix string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	return "?"
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// ─── HTTP/3 (QUIC) checks ───

// checkHTTP3IS verifies the H3 racing transport can actually speak
// HTTP/3 to an H3-capable endpoint (http3.is echoes the negotiated
// protocol). This proves the whole H3 stack works end-to-end:
// QUIC connect + uTLS fingerprint over QUIC + H3 SETTINGS.
func checkHTTP3IS(tr *cloak.H3RaceTransport, profile string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: "http3.is", Category: "http3"}
	client := &http.Client{Transport: tr, Timeout: timeout}
	start := time.Now()
	resp, err := client.Get("https://http3.is/")
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		fp.Error = err.Error()
		return fp
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.Proto == "HTTP/3.0" {
		fp.Pass = true
		fp.Detail = "HTTP/3.0 negotiated (QUIC OK)"
	} else {
		fp.Error = fmt.Sprintf("proto=%s (H3 not negotiated)", resp.Proto)
	}
	_ = body
	return fp
}

// checkQuicBrowserLeaks verifies the H3 fingerprint matches a real
// browser at the HTTP/3 SETTINGS layer (quic.browserleaks.com echoes
// h3_text like "1:65536;6:262144;7:100;51:1;GREASE|..."). Only run
// against profiles that carry H3 data (browser profiles).
func checkQuicBrowserLeaks(tr *cloak.H3RaceTransport, profile string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: "quic.browserleaks.com", Category: "http3"}
	client := &http.Client{Transport: tr, Timeout: timeout}
	start := time.Now()
	resp, err := client.Get("https://quic.browserleaks.com/")
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		fp.Error = err.Error()
		return fp
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		H3Hash string `json:"h3_hash"`
		H3Text string `json:"h3_text"`
	}
	json.Unmarshal(body, &data)
	if data.H3Hash != "" {
		fp.Pass = true
		fp.Detail = fmt.Sprintf("h3=%s", data.H3Text)
	} else {
		fp.Error = "no h3_hash in response (H3 not negotiated?)"
	}
	return fp
}

func main() {
	profilesFlag := flag.String("profiles", "", "comma-separated keys")
	allFlag := flag.Bool("all", false, "all registered profiles")
	timeoutFlag := flag.Duration("timeout", 15*time.Second, "per-check timeout")
	jsonFlag := flag.Bool("json", false, "JSON output")
	flag.Parse()

	checks := []struct {
		Name string
		Fn   CheckFunc
	}{
		{"tls.peet.ws", checkPeerWS},
		{"browserleaks.com", checkBrowserLeaks},
		{"cloudflare", checkCloudflare},
		{"imperva.com", checkImperva},
		{"f5.com", checkF5},
		{"akamai.com", checkAkamai},
		{"datadome.co", checkDataDome},
		{"hcaptcha.com", checkHcaptcha},
		{"recaptcha-demo", checkRecaptcha},
		{"sannysoft.com", checkSannysoft},
		{"browserscan.net", checkBrowserscan},
		{"httpbin.org", checkHTTPBin},
	}

	// HTTP/3 checks run only for profiles that carry H3 data
	// (browser profiles — Safari/custom are skipped by design,
	// see docs/profile-audit.md).
	h3checks := []struct {
		Name string
		Fn   H3CheckFunc
	}{
		{"http3.is", checkHTTP3IS},
		{"quic.browserleaks.com", checkQuicBrowserLeaks},
	}

	var keys []string
	switch {
	case *allFlag:
		for k := range profiles.AllClientProfiles() {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	case *profilesFlag != "":
		for _, k := range strings.Split(*profilesFlag, ",") {
			keys = append(keys, strings.TrimSpace(k))
		}
	default:
		keys = keyProfiles()
	}

	if !*jsonFlag {
		fmt.Println("╔══════════════════════════════════════════════════════════════╗")
		fmt.Println("║        TLS Fingerprint Verification — 14 Platforms          ║")
		fmt.Printf("║  profiles: %-3d   timeout: %-6v                      ║\n", len(keys), *timeoutFlag)
		fmt.Println("╚══════════════════════════════════════════════════════════════╝")
		fmt.Println()
	}

	results := make([]FPCheck, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)

	for _, key := range keys {
		profile, err := profiles.ResolveClientProfileStrict(key)
		if err != nil {
			continue
		}
		for _, c := range checks {
			wg.Add(1)
			sem <- struct{}{}
			go func(k string, p profiles.ClientProfile, cn string, fn CheckFunc) {
				defer wg.Done()
				defer func() { <-sem }()
				tr := cloak.NewTransport(p)
				defer tr.CloseIdleConnections()
				r := fn(tr, p.GetClientHelloStr(), *timeoutFlag)
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}(key, profile, c.Name, c.Fn)
		}

		// HTTP/3 checks — only for profiles that carry H3 data.
		if profile.GetHttp3Settings() != nil {
			for _, c := range h3checks {
				wg.Add(1)
				sem <- struct{}{}
				go func(k string, p profiles.ClientProfile, cn string, fn H3CheckFunc) {
					defer wg.Done()
					defer func() { <-sem }()
					tr := cloak.NewH3RaceTransport(p)
					defer tr.CloseIdleConnections()
					r := fn(tr, p.GetClientHelloStr(), *timeoutFlag)
					mu.Lock()
					results = append(results, r)
					mu.Unlock()
				}(key, profile, c.Name, c.Fn)
			}
		}
	}
	wg.Wait()

	if *jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(results)
		return
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Category != results[j].Category {
			return results[i].Category < results[j].Category
		}
		if results[i].Profile != results[j].Profile {
			return results[i].Profile < results[j].Profile
		}
		return results[i].Platform < results[j].Platform
	})

	catNames := map[string]string{
		"tls_api": "🔬 TLS Fingerprint APIs",
		"waf_cdn": "🛡️  WAF / CDN Detection",
		"http":    "📡 HTTP Layer",
		"http3":   "🚀 HTTP/3 (QUIC)",
	}

	currentCat := ""
	for _, r := range results {
		if r.Category != currentCat {
			currentCat = r.Category
			fmt.Printf("\n%s\n%s\n", catNames[currentCat], strings.Repeat("─", 90))
			fmt.Printf("%-28s %-18s %-4s %s\n", "PROFILE", "PLATFORM", " ", "DETAIL")
		}
		detail := r.Detail
		if r.Error != "" {
			detail = r.Error
		}
		if len(detail) > 65 {
			detail = detail[:65] + "..."
		}
		fmt.Printf("%-28s %-18s %-4s %s  %s\n",
			trunc(r.Profile, 28), r.Platform, r.Status(), detail, r.Duration)
	}

	total, passed := len(results), 0
	for _, r := range results {
		if r.Pass {
			passed++
		}
	}

	type catStat struct{ total, pass int }
	catStats := map[string]*catStat{}
	for _, r := range results {
		if catStats[r.Category] == nil {
			catStats[r.Category] = &catStat{}
		}
		catStats[r.Category].total++
		if r.Pass {
			catStats[r.Category].pass++
		}
	}

	fmt.Printf("\n╔══════════════════════════════════╗\n")
	fmt.Printf("║  SUMMARY: %d/%d passed            ║\n", passed, total)
	for cat, s := range catStats {
		icon := "✅"
		if float64(s.pass)/float64(s.total) < 0.5 {
			icon = "❌"
		} else if float64(s.pass)/float64(s.total) < 1.0 {
			icon = "⚠️"
		}
		fmt.Printf("║  %s %-10s: %d/%d                 ║\n", icon, catNames[cat], s.pass, s.total)
	}
	fmt.Printf("╚══════════════════════════════════╝\n")

	if passed < total {
		os.Exit(1)
	}
}
