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
	racingDelay := 25 * time.Millisecond
	racingTimeout := 2 * time.Second
	rootCAs := x509.NewCertPool()
	transportOptions := &TransportOptions{
		MaxIdleConns:             10,
		IdleConnTimeout:          &idleTimeout,
		ProtocolRacingHTTP2Delay: &racingDelay,
		ProtocolRacingTimeout:    &racingTimeout,
		RootCAs:                  rootCAs,
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
	racingDelay = time.Second
	racingTimeout = time.Minute

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
	if *config.transportOptions.ProtocolRacingHTTP2Delay != 25*time.Millisecond || *config.transportOptions.ProtocolRacingTimeout != 2*time.Second {
		t.Fatal("protocol racing timing pointers were not copied")
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

func TestValidateConfigRejectsNegativeTLSClientSessionCacheSize(t *testing.T) {
	err := validateConfig(&httpClientConfig{
		transportOptions: &TransportOptions{TLSClientSessionCacheSize: -1},
	})
	if err == nil {
		t.Fatal("expected a negative TLS client session cache size to be rejected")
	}
}

func TestTLSClientSessionCacheSizeUsesDefaultAndOverride(t *testing.T) {
	if got := tlsClientSessionCacheSize(nil); got != DefaultTLSClientSessionCacheSize {
		t.Fatalf("unexpected default TLS session cache size: %d", got)
	}
	if got := tlsClientSessionCacheSize(&TransportOptions{TLSClientSessionCacheSize: 128}); got != 128 {
		t.Fatalf("unexpected configured TLS session cache size: %d", got)
	}
}

func TestValidateConfigRejectsInvalidProtocolRacingTimings(t *testing.T) {
	negative := -time.Millisecond
	if err := validateConfig(&httpClientConfig{transportOptions: &TransportOptions{ProtocolRacingHTTP2Delay: &negative}}); err == nil {
		t.Fatal("expected a negative protocol racing HTTP/2 delay to be rejected")
	}
	zero := time.Duration(0)
	if err := validateConfig(&httpClientConfig{transportOptions: &TransportOptions{ProtocolRacingTimeout: &zero}}); err == nil {
		t.Fatal("expected a zero protocol racing timeout to be rejected")
	}
}
