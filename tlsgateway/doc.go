// Package tlsgateway provides TLS ClientHello fingerprinting for Go HTTP clients.
//
// # Architecture
//
//	tlsgateway/
//	├── transport_h2.go   ← Transport (core: uTLS + x/net/http2, zero fork)
//	├── transport_race.go  ← RaceTransport (H2 vs H1.1 racing, zero fork)
//	├── header.go          ← HeaderRoundTripper (browser UA/Accept headers)
//	├── proxy.go           ← HTTP/HTTPS forward proxy
//	├── doc.go             ← This file
//	├── transport_test.go
//	├── transport_race_test.go
//	└── stress_test.go
//
// # Zero-fork design
//
// The default build uses only:
//   - utls (Tor Project, 2498★) — TLS ClientHello customization
//   - golang.org/x/net/http2 (Go official) — HTTP/2 protocol
//   - Go standard library — everything else
//
// No bogdanfinn forks. No fhttp. No quic-go forks.
//
// # Dealing with Akamai
//
// The Transport handles TLS fingerprinting. For servers that also check
// HTTP headers (Akamai returns 403 "Access Denied" even with correct TLS),
// wrap the Transport with HeaderRoundTripper:
//
//	tr := tlsgateway.NewTransport(profile)
//	client := &http.Client{
//	    Transport: tlsgateway.NewHeaderRoundTripper(tr, profile),
//	}
package tlsgateway
