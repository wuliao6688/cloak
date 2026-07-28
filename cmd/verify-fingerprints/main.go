// Package main — TLS fingerprint verification across 7+ detection platforms.
//
// Platforms tested:
//
//	TLS Fingerprint APIs:
//	  tls.peet.ws        — JA3, JA4, cipher suites, extensions
//	  browserleaks.com   — JA3, JA3N, Akamai fingerprint
//
//	WAF / CDN Sites:
//	  cloudflare.com     — world's largest CDN
//	  imperva.com        — enterprise WAF
//	  f5.com             — Shape Security (F5)
//	  akamai.com         — strictest H2 fingerprinting
//	  datadome.co        — behavioral + TLS detection
//
//	HTTP Layer:
//	  httpbin.org        — header inspection
//
//	DNS / IP:
//	  ipify.org          — public IP check
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

	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/bogdanfinn/tls-client/tlsgateway"
)

// ─── Result types ───

type FPCheck struct {
	Profile  string `json:"profile"`
	Platform string `json:"platform"`
	Category string `json:"category"` // "tls_api", "waf_cdn", "http"
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

// ─── Check functions ───

type CheckFunc func(tr *tlsgateway.Transport, profile string, timeout time.Duration) FPCheck

func checkPeerWS(tr *tlsgateway.Transport, profile string, timeout time.Duration) FPCheck {
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

func checkBrowserLeaks(tr *tlsgateway.Transport, profile string, timeout time.Duration) FPCheck {
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
		JA3Hash string `json:"ja3_hash"`
		JA3NHash string `json:"ja3n_hash"`
		Akamai  string `json:"akamai_hash"`
	}
	json.Unmarshal(body, &data)
	if data.JA3Hash != "" {
		fp.Pass = true
		fp.Detail = fmt.Sprintf("JA3=%s JA3N=%s Akamai=%s",
			data.JA3Hash, data.JA3NHash, data.Akamai)
	} else {
		fp.Error = "no JA3 in response"
	}
	return fp
}

func checkCloudflareTrace(tr *tlsgateway.Transport, profile string, timeout time.Duration) FPCheck {
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
	if resp.StatusCode == 403 || resp.StatusCode == 503 || strings.Contains(s, "challenge") {
		fp.Error = fmt.Sprintf("Cloudflare challenge (status %d)", resp.StatusCode)
		return fp
	}
	// Extract TLS version and HTTP version
	tlsVer := extractLine(s, "tls=")
	httpVer := extractLine(s, "http=")
	fp.Pass = true
	fp.Detail = fmt.Sprintf("TLS=%s HTTP=%s", tlsVer, httpVer)
	return fp
}

func checkImperva(tr *tlsgateway.Transport, profile string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: "imperva.com", Category: "waf_cdn"}
	client := &http.Client{Transport: tr, Timeout: timeout}
	start := time.Now()
	resp, err := client.Get("https://www.imperva.com/")
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		fp.Error = err.Error()
		return fp
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		fp.Pass = true
	} else {
		fp.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return fp
}

func checkF5(tr *tlsgateway.Transport, profile string, timeout time.Duration) FPCheck {
	fp := FPCheck{Profile: profile, Platform: "f5.com", Category: "waf_cdn"}
	client := &http.Client{Transport: tr, Timeout: timeout}
	start := time.Now()
	resp, err := client.Get("https://www.f5.com/")
	fp.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		fp.Error = err.Error()
		return fp
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		fp.Pass = true
	} else {
		fp.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return fp
}

func checkAkamai(tr *tlsgateway.Transport, profile string, timeout time.Duration) FPCheck {
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
		fp.Error = "ACCESS_DENIED: TLS passed, HTTP headers need browser UA/Accept/Accept-Language"
		return fp
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		fp.Pass = true
	} else {
		fp.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return fp
}

func checkDataDome(tr *tlsgateway.Transport, profile string, timeout time.Duration) FPCheck {
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
	s := string(body)
	if strings.Contains(s, "datadome") && resp.StatusCode == 403 {
		fp.Error = "JS_REQUIRED: DataDome needs JavaScript execution (not a TLS limitation)"
		return fp
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		fp.Pass = true
	} else {
		fp.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return fp
}

func checkHTTPBin(tr *tlsgateway.Transport, profile string, timeout time.Duration) FPCheck {
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

// ─── Main ───

func keyProfiles() []string {
	return []string{
		"chrome_150", "chrome_131", "chrome_109",
		"firefox_148", "firefox_133",
		"safari_ios_18_5", "safari_18_1",
		"brave_146",
		"opera_91",
		"okhttp4_android_13",
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
		{"cloudflare", checkCloudflareTrace},
		{"imperva.com", checkImperva},
		{"f5.com", checkF5},
		{"akamai.com", checkAkamai},
		{"datadome.co", checkDataDome},
		{"httpbin.org", checkHTTPBin},
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
		fmt.Println("║        TLS Fingerprint Verification — 8 Platforms           ║")
		fmt.Println("╠══════════════════════════════════════════════════════════════╣")
		fmt.Printf("║  profiles: %-3d   services: %-2d   timeout: %-6v      ║\n",
			len(keys), len(checks), *timeoutFlag)
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
				tr := tlsgateway.NewTransport(p)
				defer tr.CloseIdleConnections()
				r := fn(tr, p.GetClientHelloStr(), *timeoutFlag)
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}(key, profile, c.Name, c.Fn)
		}
	}
	wg.Wait()

	if *jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(results)
		return
	}

	// ─── Grouped report ───

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
		if len(detail) > 60 {
			detail = detail[:60] + "..."
		}
		fmt.Printf("%-28s %-18s %-4s %s  %s\n",
			trunc(r.Profile, 28), r.Platform, r.Status(), detail, r.Duration)
	}

	// Summary
	total := len(results)
	passed := 0
	for _, r := range results {
		if r.Pass {
			passed++
		}
	}

	// Per-category summary
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

	fmt.Printf("\n╔═══════════════════════════════════════════╗\n")
	fmt.Printf("║  SUMMARY: %d/%d checks passed              ║\n", passed, total)
	for cat, s := range catStats {
		icon := "✅"
		ratio := float64(s.pass) / float64(s.total)
		if ratio < 0.5 {
			icon = "❌"
		} else if ratio < 1.0 {
			icon = "⚠️"
		}
		fmt.Printf("║  %s %-10s: %d/%d                       ║\n", icon, catNames[cat], s.pass, s.total)
	}
	fmt.Printf("╚═══════════════════════════════════════════╝\n")

	if passed < total {
		os.Exit(1)
	}
}
