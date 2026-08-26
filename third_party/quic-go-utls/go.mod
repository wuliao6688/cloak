module github.com/wuliao6688/quic-go-utls

go 1.24.1

require (
	github.com/quic-go/qpack v0.6.0
	github.com/wuliao6688/utls v1.7.7-barnius
	golang.org/x/crypto v0.46.0
	golang.org/x/net v0.48.0
	golang.org/x/sys v0.39.0
)

require (
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/text v0.32.0 // indirect
)

replace github.com/wuliao6688/utls => ../utls
