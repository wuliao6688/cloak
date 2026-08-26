module github.com/wuliao6688/cloak

go 1.26.5

require (
	github.com/stretchr/testify v1.11.1
	github.com/wuliao6688/quic-go-utls v0.0.0-00010101000000-000000000000
	github.com/wuliao6688/utls v1.7.7-barnius
	golang.org/x/net v0.48.0
)

replace github.com/wuliao6688/utls => ./third_party/utls

replace github.com/wuliao6688/quic-go-utls => ./third_party/quic-go-utls

require (
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
