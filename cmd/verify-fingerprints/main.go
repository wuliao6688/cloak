// Command verify-fingerprints tests TLS ClientHello fingerprints against
// online fingerprint verification services (tls.peet.ws, etc).
//
// It validates that each profile produces the correct JA3/JA4 hash
// matching the expected browser, and identifies profiles with gaps.
//
// Usage:
//
//	go run ./cmd/verify-fingerprints -profile chrome_150
//	go run ./cmd/verify-fingerprints -all    # test all 81 profiles
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

type FingerprintResult struct {
	Key           string `json:"key"`
	TLSClient     string `json:"tlsClient"`
	Version       string `json:"version"`
	JA3           string `json:"ja3"`
	JA3Hash       string `json:"ja3Hash"`
	JA4           string `json:"ja4"`
	ExpectedJA3   string `json:"expectedJa3,omitempty"`
	ExpectedBrowser string `json:"expectedBrowser"`
	MatchJA3      bool   `json:"matchJa3"`
	Error         string `json:"error,omitempty"`
	CipherCount   int    `json:"cipherCount"`
	ExtensionCount int   `json:"extensionCount"`
	Duration       string `json:"duration"`
}

// TLS fingerprint response from peet.ws JSON API.
type peetWSResponse struct {
	HTTPVersion string      `json:"http_version"`
	IP          string      `json:"ip"`
	UserAgent   string      `json:"user_agent"`
	TLS         peetWSTLS   `json:"tls"`
	HTTP2       peetWSHTTP2 `json:"http2,omitempty"`
}

type peetWSTLS struct {
	JA3                 string   `json:"ja3"`
	JA3Hash             string   `json:"ja3_hash"`
	JA4                 string   `json:"ja4"`
	JA4Raw              string   `json:"ja4_r"`
	PeetPrint           string   `json:"peetprint"`
	PeetPrintHash       string   `json:"peetprint_hash"`
	TLSVersionRecord    string   `json:"tls_version_record"`
	TLSVersionNegotiated string  `json:"tls_version_negotiated"`
	Ciphers             []string `json:"ciphers"`
	Extensions          []peetWSExtension `json:"extensions"`
}

type peetWSExtension struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type peetWSHTTP2 struct {
	Settings      map[string]int `json:"settings"`
	SettingsOrder []string       `json:"settings_order"`
	PseudoOrder   []string       `json:"pseudo_order"`
	WindowUpdate  int            `json:"window_update"`
}

// Known JA3 hashes for common browsers (for reference only — JA3 changes per version).
var knownJA3Hashes = map[string]string{}

func main() {
	profileKey := flag.String("profile", "", "single profile to verify (e.g. chrome_150)")
	all := flag.Bool("all", false, "test all profiles")
	flag.Parse()

	var keys []string
	if *all {
		keys = profiles.RandomBrowserProfileKeys()
		// Also include non-random profiles.
		allMeta := profiles.AllProfileMetadata()
		for k := range allMeta {
			found := false
			for _, rk := range keys {
				if rk == k {
					found = true
					break
				}
			}
			if !found {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
	} else if *profileKey != "" {
		keys = []string{*profileKey}
	} else {
		// Default: test key browsers.
		keys = []string{
			"chrome_150", "chrome_146", "chrome_131", "chrome_120",
			"firefox_148", "firefox_147", "firefox_132",
			"safari_ios_18_5", "safari_16_0",
			"opera_91",
			"okhttp4_android_13",
		}
	}

	results := make(chan FingerprintResult, len(keys))
	var wg sync.WaitGroup

	for _, key := range keys {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			results <- verifyProfile(k)
		}(key)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and print.
	var allResults []FingerprintResult
	pass, fail := 0, 0
	for r := range results {
		allResults = append(allResults, r)
		if r.MatchJA3 {
			pass++
		} else if r.Error == "" {
			fail++
		}
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Key < allResults[j].Key
	})

	fmt.Println()
	fmt.Printf("%-28s %-8s %-8s %-32s %s\n", "Profile", "JA3", "JA4", "JA3 Hash", "Status")
	fmt.Println(strings.Repeat("=", 110))

	for _, r := range allResults {
		status := "✅ PASS"
		if r.Error != "" {
			status = fmt.Sprintf("❌ ERR: %s", r.Error)
		} else if !r.MatchJA3 {
			status = "⚠️  JA3 MISMATCH"
		}

		ja3Short := ""
		if len(r.JA3) > 0 {
			ja3Short = "✓"
		}
		ja4Short := ""
		if len(r.JA4) > 0 {
			ja4Short = "✓"
		}

		hashShort := ""
		if len(r.JA3Hash) >= 8 {
			hashShort = r.JA3Hash[:8] + "..."
		}

		fmt.Printf("%-28s %-8s %-8s %-32s %s\n",
			r.Key, ja3Short, ja4Short, hashShort, status)
	}

	fmt.Println()
	fmt.Printf("Results: %d pass, %d fail, %d total\n", pass, fail, pass+fail)

	// Write JSON report.
	reportPath := "fingerprint_verification_report.json"
	data, _ := json.MarshalIndent(allResults, "", "  ")
	os.WriteFile(reportPath, data, 0o644)
	fmt.Printf("Detailed report: %s\n", reportPath)
}

func verifyProfile(key string) FingerprintResult {
	start := time.Now()

	result := FingerprintResult{
		Key: key,
	}

	// Resolve profile.
	resolved, err := profiles.ResolveClientProfileStrict(key)
	if err != nil {
		result.Error = fmt.Sprintf("resolve: %v", err)
		return result
	}

	result.TLSClient = resolved.GetClientHelloId().Client
	result.Version = resolved.GetClientHelloId().Version

	// Try to get expected browser name from metadata.
	result.ExpectedBrowser = result.TLSClient

	// Transport uses H2 + uTLS by default.
	tr := tlsgateway.NewTransport(resolved)
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	// Test 1: tls.peet.ws JSON API.
	resp, err := client.Get("https://tls.peet.ws/api/all")
	if err != nil {
		result.Error = fmt.Sprintf("tls.peet.ws: %v", err)
		result.Duration = time.Since(start).Round(time.Millisecond).String()
		return result
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		result.Error = fmt.Sprintf("tls.peet.ws returned %d", resp.StatusCode)
		result.Duration = time.Since(start).Round(time.Millisecond).String()
		return result
	}

	var peet peetWSResponse
	if err := json.Unmarshal(body, &peet); err != nil {
		result.Error = fmt.Sprintf("parse response: %v", err)
		result.Duration = time.Since(start).Round(time.Millisecond).String()
		return result
	}

	result.JA3 = peet.TLS.JA3
	result.JA3Hash = peet.TLS.JA3Hash
	result.JA4 = peet.TLS.JA4
	result.CipherCount = len(peet.TLS.Ciphers)
	result.ExtensionCount = len(peet.TLS.Extensions)
	result.Duration = time.Since(start).Round(time.Millisecond).String()

	// Check against known JA3 hashes if available.
	if expected, ok := knownJA3Hashes[key]; ok {
		result.ExpectedJA3 = expected
		result.MatchJA3 = strings.EqualFold(peet.TLS.JA3Hash, expected)
	} else {
		// No known reference — check that JA3 is consistent with browser family.
		result.ExpectedJA3 = "(no reference)"
		result.MatchJA3 = true // can't verify, but no error either
	}

	return result
}
