package tls_client

import (
	"crypto/x509"
	"testing"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls "github.com/bogdanfinn/utls"
)

func TestMutableClientOptionsAreDefensivelyCopied(t *testing.T) {
	defaultHeaders := http.Header{"X-Test": {"original"}}
	connectHeaders := http.Header{"Proxy-Authorization": {"original"}}
	certificatePins := map[string][]string{"example.com": {"pin-one"}}
	idleTimeout := time.Second
	rootCAs := x509.NewCertPool()
	transportOptions := &TransportOptions{
		MaxIdleConns:    10,
		IdleConnTimeout: &idleTimeout,
		RootCAs:         rootCAs,
		Certificates: []tls.Certificate{{
			Certificate:                  [][]byte{{1, 2, 3}},
			OCSPStaple:                   []byte{4, 5, 6},
			SupportedSignatureAlgorithms: []tls.SignatureScheme{tls.ECDSAWithP256AndSHA256},
			SignedCertificateTimestamps:  [][]byte{{7, 8, 9}},
		}},
	}

	config := &httpClientConfig{}
	WithDefaultHeaders(defaultHeaders)(config)
	WithConnectHeaders(connectHeaders)(config)
	WithCertificatePinning(certificatePins, nil)(config)
	WithTransportOptions(transportOptions)(config)

	defaultHeaders.Set("X-Test", "modified")
	connectHeaders.Set("Proxy-Authorization", "modified")
	certificatePins["example.com"][0] = "modified"
	transportOptions.MaxIdleConns = 99
	transportOptions.Certificates[0].Certificate[0][0] = 99
	transportOptions.Certificates[0].OCSPStaple[0] = 99
	transportOptions.Certificates[0].SupportedSignatureAlgorithms[0] = tls.PKCS1WithSHA256
	transportOptions.Certificates[0].SignedCertificateTimestamps[0][0] = 99
	idleTimeout = 2 * time.Second

	if got := config.defaultHeaders.Get("X-Test"); got != "original" {
		t.Fatalf("default headers were not copied: %q", got)
	}
	if got := config.connectHeaders.Get("Proxy-Authorization"); got != "original" {
		t.Fatalf("connect headers were not copied: %q", got)
	}
	if got := config.certificatePins["example.com"][0]; got != "pin-one" {
		t.Fatalf("certificate pins were not copied: %q", got)
	}
	if config.transportOptions.MaxIdleConns != 10 {
		t.Fatalf("transport options were not copied: %d", config.transportOptions.MaxIdleConns)
	}
	if config.transportOptions.RootCAs == rootCAs {
		t.Fatal("root CA pool was not copied")
	}
	if *config.transportOptions.IdleConnTimeout != time.Second {
		t.Fatalf("idle timeout pointer was not copied: %v", *config.transportOptions.IdleConnTimeout)
	}
	if config.transportOptions.Certificates[0].Certificate[0][0] != 1 ||
		config.transportOptions.Certificates[0].OCSPStaple[0] != 4 ||
		config.transportOptions.Certificates[0].SupportedSignatureAlgorithms[0] != tls.ECDSAWithP256AndSHA256 ||
		config.transportOptions.Certificates[0].SignedCertificateTimestamps[0][0] != 7 {
		t.Fatal("TLS certificate byte slices were not deeply copied")
	}
}

func TestValidateConfigRejectsInvalidTransportCacheLimit(t *testing.T) {
	err := validateConfig(&httpClientConfig{
		transportOptions: &TransportOptions{MaxCachedTransports: -2},
	})
	if err == nil {
		t.Fatal("expected max cached transports below -1 to be rejected")
	}
}
