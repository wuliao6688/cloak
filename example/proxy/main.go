// Package main demonstrates common tls-client patterns.
//
// Usage:
//
//	go run proxy_example.go
//	go run racing_example.go
//	go run cookie_example.go
package main

import (
	"fmt"
	"io"
	"log"
	"net/url"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

func main() {
	// --- Proxy example: SOCKS5 proxy with Chrome 146 fingerprint ---
	jar := tls_client.NewCookieJar()

	client, err := tls_client.NewHttpClient(
		tls_client.NewNoopLogger(),
		tls_client.WithClientProfile(profiles.Chrome_146),
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithCookieJar(jar),
		tls_client.WithDefaultHeaders(http.Header{
			"accept":          []string{"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
			"accept-language": []string{"zh-CN,zh;q=0.9,en;q=0.8"},
			"user-agent":      []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"},
		}),
		tls_client.WithProxyUrl("socks5://127.0.0.1:1080"),
	)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}
	defer client.CloseIdleConnections()

	// Dynamic proxy switch
	if err := client.SetProxy("http://user:pass@proxy.example.com:8080"); err != nil {
		log.Printf("dynamic proxy switch failed (expected if no proxy running): %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://httpbin.org/ip", nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set(http.HeaderOrderKey, "accept,accept-language,user-agent")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("request failed (expected if no proxy): %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %d\nBody: %s\n", resp.StatusCode, string(body))

	// Cookie jar inspection
	u, _ := url.Parse("https://httpbin.org")
	cookies := client.GetCookies(u)
	fmt.Printf("Cookies stored: %d\n", len(cookies))
}


