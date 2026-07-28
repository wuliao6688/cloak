// Package tlsgateway provides TLS ClientHello fingerprinting for go HTTP clients.
//
// # Layered architecture
//
//	tlsgateway/
//	├── transport_h2.go      ← Transport (default, zero fork: uTLS + x/net/http2)
//	├── transport_race.go    ← RaceTransport (zero fork: H2 vs H1.1 racing)
//	├── transport_h3.go      ← H3Transport (-tags h3: QUIC/HTTP3 via quic-go-utls)
//	├── transport_fhttp.go   ← FhttpTransport (-tags fhttp: Akamai-level H2 control)
//	└── proxy.go             ← HTTP/S forward proxy for any language
//
// # Quick start
//
//	// Default (zero fork)
//	tr := tlsgateway.NewTransport(profiles.Chrome_150)
//	client := &http.Client{Transport: tr}
//	resp, _ := client.Get("https://example.com")
//
//	// With racing (zero fork)
//	tr := tlsgateway.NewRaceTransport(profiles.Chrome_150, tlsgateway.DefaultRaceOptions())
//
//	// With HTTP/3 (go build -tags h3)
//	tr := tlsgateway.NewH3Transport(profiles.Chrome_150, tlsgateway.H3Options{PreferH3: true})
//
//	// With fhttp (go build -tags fhttp)
//	tr := tlsgateway.NewFhttpTransport(profiles.Chrome_150, tlsgateway.FhttpOptions{})
//
// # Proxy mode (any language)
//
//	go run ./cmd/tlsgateway-proxy -profile chrome_150
//	HTTPS_PROXY=http://localhost:8080 curl https://example.com
//
// # Dependencies
//
//	Default:   utls (Tor team) + x/net/http2 (Go team) + stdlib  ← ZERO FORK
//	+h3:       + quic-go-utls (bogdanfinn fork, QUIC mandates this)
//	+fhttp:    + fhttp (bogdanfinn fork, for Akamai-grade H2 control)
package tlsgateway
