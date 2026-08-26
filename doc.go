// Package cloak provides TLS/HTTP fingerprinting for Go HTTP clients.
//
// # Architecture
//
//	cloak/
//	├── transport_h2.go      ← Transport (一体式: TLS + HTTP头, 零fork)
//	├── transport_fprint.go  ← FingerprintTransport (fork http2, 完整H2定制)
//	├── transport_race.go    ← RaceTransport (H2 vs H1.1 racing)
//	├── fingerprint.go       ← H2指纹/8浏览器常量/Header排序/Multipart
//	├── header.go            ← HeaderRoundTripper (deprecated, Transport已内置)
//	├── middleware.go         ← TransportMiddleware
//	├── request.go           ← Request 流式 builder + hooks
//	├── response.go          ← Response + ResultState + TraceInfo
//	├── retry.go             ← 条件重试 + 指数退避
//	├── dump.go              ← DumpOptions
//	├── proxy.go             ← HTTP/HTTPS forward proxy
//	└── *.go                 ← 测试
//
// # Zero-fork design
//
// The default build uses only:
//   - utls (Tor Project) — TLS ClientHello customization
//   - golang.org/x/net/http2 (Go official) — HTTP/2 protocol
//   - Go standard library — everything else
//
// No external forks. Standard net/http. QUIC engine vendored in third_party/.
//
// # One-piece Transport (req-style)
//
// Transport handles both TLS fingerprinting and HTTP header injection.
// No need to manually wrap with HeaderRoundTripper:
//
//	tr := cloak.NewTransport(profile)
//	client := &http.Client{Transport: tr}
//	client.Get("https://www.akamai.com/")  // → 200
//
// # Fluent API
//
//	req := cloak.ImpersonateRequest(profile)
//	req.SetSuccessResult(&v).SetBearerAuthToken("tok").SetRetry(3, ...).Get(url)
package cloak
