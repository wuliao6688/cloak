// Package main demonstrates HTTP/3 protocol racing usage.
//
//	go run racing_example.go
package main

import (
	"fmt"
	"io"
	"log"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

func main() {
	// --- Protocol Racing: HTTP/3 vs HTTP/2 ---
	// HTTP/3 is attempted immediately; HTTP/2 is delayed by 300ms.
	// Whichever responds first wins; the loser is cancelled.

	client, err := tls_client.NewHttpClient(
		tls_client.NewNoopLogger(),
		tls_client.WithClientProfile(profiles.Chrome_146),
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithCookieJar(tls_client.NewCookieJar()),
		tls_client.WithProtocolRacing(),
	)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}
	defer client.CloseIdleConnections()

	start := time.Now()

	req, err := http.NewRequest(http.MethodGet, "https://www.google.com", nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set(http.HeaderOrderKey, "accept,accept-language,user-agent")
	req.Header.Set("accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	elapsed := time.Since(start)
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Protocol: %s\n", resp.Proto)
	fmt.Printf("Status:   %d\n", resp.StatusCode)
	fmt.Printf("Latency:  %v\n", elapsed)
	fmt.Printf("Body:     %d bytes\n", len(body))

	// Second request to same host reuses the winning protocol
	start2 := time.Now()
	resp2, err := client.Do(req.Clone(req.Context()))
	if err != nil {
		log.Printf("second request failed: %v", err)
		return
	}
	defer resp2.Body.Close()
	fmt.Printf("\nSecond request (cached protocol): %v\n", time.Since(start2))

	// Racing constraints to be aware of:
	// - Only GET, HEAD, OPTIONS participate in racing
	// - Cannot combine with proxy, custom dialer, cert pinning, bandwidth tracking
}
