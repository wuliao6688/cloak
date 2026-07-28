// Package main provides a standalone fingerprint verification tool.
// It tests TLS ClientHello fingerprints against online detection services
// and validates that profiles match real browser expectations.
//
// Usage:
//
//	go run ./cmd/verify-fingerprints [-all] [-profiles p1,p2] [-json] [-timeout 30s]
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

// Result holds a single check result.
type Result struct {
	Profile      string   `json:"profile"`
	Service      string   `json:"service"`
	Success      bool     `json:"success"`
	JA3          string   `json:"ja3,omitempty"`
	JA3Hash      string   `json:"ja3_hash,omitempty"`
	JA4          string   `json:"ja4,omitempty"`
	HTTPVersion  string   `json:"http_version,omitempty"`
	Ciphers      []string `json:"ciphers,omitempty"`
	Error        string   `json:"error,omitempty"`
	Duration     string   `json:"duration"`
}

// verifyPeerWS checks against tls.peet.ws/api/all.
func verifyPeerWS(tr *tlsgateway.Transport, profileName string, timeout time.Duration) Result {
	r := Result{Profile: profileName, Service: "tls.peet.ws"}
	client := &http.Client{Transport: tr, Timeout: timeout}

	start := time.Now()
	resp, err := client.Get("https://tls.peet.ws/api/all")
	r.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var data struct {
		HTTPVersion string `json:"http_version"`
		TLS         struct {
			JA3      string   `json:"ja3"`
			JA3Hash  string   `json:"ja3_hash"`
			JA4      string   `json:"ja4"`
			Ciphers  []string `json:"ciphers"`
		} `json:"tls"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		r.Error = fmt.Sprintf("parse: %v (%s)", err, truncate(string(body), 100))
		return r
	}
	if data.Error != "" {
		r.Error = data.Error
		return r
	}

	r.Success = true
	r.JA3 = data.TLS.JA3
	r.JA3Hash = data.TLS.JA3Hash
	r.JA4 = data.TLS.JA4
	r.HTTPVersion = data.HTTPVersion
	r.Ciphers = data.TLS.Ciphers
	return r
}

// verifyCloudflare checks if Cloudflare serves the request without a challenge.
// A 403/503 with challenge indicates fingerprint detection.
func verifyCloudflare(tr *tlsgateway.Transport, profileName string, timeout time.Duration) Result {
	r := Result{Profile: profileName, Service: "cloudflare-test"}
	client := &http.Client{Transport: tr, Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	start := time.Now()
	resp, err := client.Get("https://www.cloudflare.com/cdn-cgi/trace")
	r.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 || resp.StatusCode == 503 {
		r.Error = fmt.Sprintf("blocked/challenged (status %d)", resp.StatusCode)
		return r
	}

	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "cf-chl") || strings.Contains(string(body), "challenge") {
		r.Error = "Cloudflare challenge detected"
		return r
	}

	r.Success = true
	return r
}

// verifyHTTPBin checks basic HTTP behavior.
func verifyHTTPBin(tr *tlsgateway.Transport, profileName string, timeout time.Duration) Result {
	r := Result{Profile: profileName, Service: "httpbin.org"}
	client := &http.Client{Transport: tr, Timeout: timeout}

	start := time.Now()
	resp, err := client.Get("https://httpbin.org/ip")
	r.Duration = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		r.Error = fmt.Sprintf("status %d: %s", resp.StatusCode, truncate(string(body), 150))
		return r
	}
	r.Success = true
	return r
}

func keyProfiles() []string {
	return []string{
		"chrome_150", "chrome_146", "chrome_131", "chrome_124", "chrome_120",
		"firefox_148", "firefox_147", "firefox_133", "firefox_120",
		"safari_ios_18_5", "safari_ios_18_2", "safari_18_1",
		"brave_146", "brave_141",
		"opera_91", "opera_90",
		"okhttp4_android_13", "okhttp4_android_12",
		"chrome_109",
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func main() {
	profilesFlag := flag.String("profiles", "", "comma-separated profile keys")
	allFlag := flag.Bool("all", false, "test all registered profiles")
	timeoutFlag := flag.Duration("timeout", 30*time.Second, "per-check timeout")
	jsonFlag := flag.Bool("json", false, "output JSON")
	flag.Parse()

	timeout := *timeoutFlag

	var keys []string
	switch {
	case *allFlag:
		for k := range profiles.AllClientProfiles() {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	case *profilesFlag != "":
		keys = strings.Split(*profilesFlag, ",")
	default:
		keys = keyProfiles()
	}

	type checkFunc func(*tlsgateway.Transport, string, time.Duration) Result
	checks := []struct {
		name string
		fn   checkFunc
	}{
		{"tls.peet.ws", verifyPeerWS},
		{"cloudflare-test", verifyCloudflare},
		{"httpbin.org", verifyHTTPBin},
	}

	if !*jsonFlag {
		fmt.Println("=== TLS Fingerprint Verification ===")
		fmt.Printf("profiles: %d\n", len(keys))
		fmt.Printf("services: %s\n", strings.Join(func() []string {
			names := make([]string, len(checks))
			for i, c := range checks { names[i] = c.name }
			return names
		}(), ", "))
		fmt.Println()
	}

	results := make([]Result, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)

	for _, key := range keys {
		profile, err := profiles.ResolveClientProfileStrict(key)
		if err != nil {
			if !*jsonFlag {
				fmt.Printf("SKIP %s: %v\n", key, err)
			}
			continue
		}

		for _, check := range checks {
			wg.Add(1)
			sem <- struct{}{}
			go func(k string, p profiles.ClientProfile, ck string, fn checkFunc) {
				defer wg.Done()
				defer func() { <-sem }()

				tr := tlsgateway.NewTransport(p)
				defer tr.CloseIdleConnections()

				r := fn(tr, p.GetClientHelloStr(), timeout)

				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}(key, profile, check.name, check.fn)
		}
	}

	wg.Wait()

	if *jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(results)
		return
	}

	// Print results.
	fmt.Printf("%-30s %-18s %-6s %-32s %s\n", "PROFILE", "SERVICE", "STATUS", "JA3_HASH", "JA4")
	fmt.Println(strings.Repeat("-", 110))

	success := 0
	failed := 0
	var failures []string

	sort.Slice(results, func(i, j int) bool {
		if results[i].Profile != results[j].Profile {
			return results[i].Profile < results[j].Profile
		}
		return results[i].Service < results[j].Service
	})

	for _, r := range results {
		status := "✅"
		if !r.Success {
			status = "❌"
			failed++
			failures = append(failures, fmt.Sprintf("%s/%s: %s", r.Profile, r.Service, r.Error))
		} else {
			success++
		}

		ja3h := r.JA3Hash
		if ja3h == "" {
			ja3h = r.JA3
		}
		if len(ja3h) > 32 {
			ja3h = ja3h[:32]
		}

		ja4 := r.JA4
		if ja4 == "" {
			ja4 = r.HTTPVersion
		}
		if len(ja4) > 20 {
			ja4 = ja4[:20] + "..."
		}

		fmt.Printf("%-30s %-18s %s    %-32s %s  %s\n",
			truncate(r.Profile, 30), r.Service, status, ja3h, ja4, r.Duration)
	}

	fmt.Println()
	fmt.Printf("=== %d OK / %d FAIL / %d total ===\n", success, failed, len(results))
	if len(failures) > 0 {
		fmt.Println("\nFailures:")
		for _, f := range failures {
			fmt.Printf("  ❌ %s\n", f)
		}
		os.Exit(1)
	}
}
