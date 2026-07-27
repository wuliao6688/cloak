// Command tlsgateway-proxy runs a local HTTP forward proxy that applies
// TLS ClientHello fingerprints to outbound connections.
//
// Usage:
//
//	go run ./cmd/tlsgateway-proxy -addr :8080 -profile chrome_150
//
//	# Use from any language or tool:
//	HTTPS_PROXY=http://localhost:8080 curl https://example.com
//	HTTPS_PROXY=http://localhost:8080 python3 -c "import requests; print(requests.get('https://httpbin.org/ip').text)"
//
// Profile hot-reload:
//
//	curl -X POST "http://localhost:8080/reload?path=profiles.json&profile=chrome_151"
//
// Health check:
//
//	curl http://localhost:8080/health
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/bogdanfinn/tls-client/tlsgateway"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	profileKey := flag.String("profile", "chrome_150", "TLS client profile")
	jsonPath := flag.String("profiles", "", "optional JSON profiles file to load at startup")
	flag.Parse()

	// Load profiles from JSON if specified.
	if *jsonPath != "" {
		log.Printf("loading profiles from %s", *jsonPath)
		file, err := profiles.LoadProfilesFromJSONFile(*jsonPath)
		if err != nil {
			log.Fatalf("load profiles: %v", err)
		}
		if err := profiles.MergeJSONProfilesIntoRegistry(file); err != nil {
			log.Fatalf("merge profiles: %v", err)
		}
		log.Printf("loaded %d profiles", len(file.Profiles))
	}

	// Resolve the TLS profile.
	resolved, err := profiles.ResolveClientProfileStrict(*profileKey)
	if err != nil {
		log.Fatalf("resolve profile %q: %v", *profileKey, err)
	}

	log.Printf("tlsgateway proxy starting with profile: %s", resolved.GetClientHelloStr())

	p := tlsgateway.NewProxy(*addr, resolved)

	// Graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		log.Println("shutting down...")
		shutdownCtx, sc := context.WithTimeout(context.Background(), 5*1000000000) // 5s
		defer sc()
		if err := p.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	if err := p.ListenAndServe(); err != nil {
		log.Printf("server stopped: %v", err)
	}
}
