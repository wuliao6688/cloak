// Package main demonstrates cookie jar persistence and error handling.
//
//	go run cookie_example.go
package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

func main() {
	jar := tls_client.NewCookieJar()

	client, err := tls_client.NewHttpClient(
		tls_client.NewNoopLogger(),
		tls_client.WithClientProfile(profiles.Chrome_150),
		tls_client.WithTimeoutSeconds(15),
		tls_client.WithCookieJar(jar),
		tls_client.WithNotFollowRedirects(),
		tls_client.WithCatchPanics(),
	)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}
	defer client.CloseIdleConnections()

	// Request that sets cookies
	req, _ := http.NewRequest(http.MethodGet, "https://httpbin.org/cookies/set?session=abc123", nil)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("request failed: %v", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Verify cookies were stored
	u, _ := url.Parse("https://httpbin.org")
	cookies := client.GetCookies(u)
	fmt.Printf("Cookies after set: %d\n", len(cookies))
	for _, c := range cookies {
		fmt.Printf("  %s = %s\n", c.Name, c.Value)
	}

	// Request that reads cookies back
	req2, _ := http.NewRequest(http.MethodGet, "https://httpbin.org/cookies", nil)
	resp2, err := client.Do(req2)
	if err != nil {
		log.Printf("request failed: %v", err)
		return
	}
	defer resp2.Body.Close()
	body, _ := io.ReadAll(resp2.Body)
	fmt.Printf("Cookies echoed: %s\n", string(body))

	// Error handling with sentinel errors
	demoErrorHandling()
}

func demoErrorHandling() {
	// Simulating racing config error
	_, err := tls_client.NewHttpClient(
		tls_client.NewNoopLogger(),
		tls_client.WithClientProfile(profiles.Chrome_146),
		tls_client.WithProtocolRacing(),
		tls_client.WithProxyUrl("http://proxy:8080"),
	)

	if errors.Is(err, tls_client.ErrRacingNotSupported) {
		fmt.Println("\nCorrectly detected: racing + proxy = not supported")
	} else if err != nil {
		fmt.Printf("Unexpected error: %v\n", err)
	}
}
