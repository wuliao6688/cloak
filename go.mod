module github.com/bogdanfinn/tls-client

go 1.26.5

require (
	github.com/bogdanfinn/utls v1.7.7-barnius
	github.com/stretchr/testify v1.11.1
	golang.org/x/net v0.48.0
)

// Local fork of quic-go-utls with uTLS ClientHelloID injection into the
// QUIC TLS 1.3 handshake (HTTP/3 TLS fingerprinting). See third_party/.
require github.com/bogdanfinn/quic-go-utls v1.0.9-utls

replace github.com/bogdanfinn/quic-go-utls => ./third_party/quic-go-utls

require (
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
