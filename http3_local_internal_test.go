package tls_client

import (
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"net"
	stdhttp "net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/quic-go-utls/http3"
	"github.com/bogdanfinn/tls-client/profiles"
	tls "github.com/bogdanfinn/utls"
)

func TestHTTP3TransportAppliesProfileFingerprintConfiguration(t *testing.T) {
	tests := []struct {
		name                    string
		profile                 profiles.ClientProfile
		expectGeneratedGrease   bool
		expectMaxResponseHeader int
		expectFingerprint       string
		expectFingerprintHash   string
	}{
		{
			name:                    "chrome_144",
			profile:                 profiles.Chrome_144,
			expectGeneratedGrease:   true,
			expectMaxResponseHeader: CHROME_MAX_FIELD_SECTION_SIZE,
			expectFingerprint:       "1:65536;6:262144;7:100;51:1;GREASE|GREASE|984832|m,a,s,p",
			expectFingerprintHash:   "ba909fc3dc419ea5c5b26c6323ac1879",
		},
		{
			name:                    "firefox_147",
			profile:                 profiles.Firefox_147,
			expectGeneratedGrease:   false,
			expectMaxResponseHeader: -1,
			expectFingerprint:       "1:65536;7:20;727725890:0;16765559:1;51:1;8:1|GREASE|m,s,a,p",
			expectFingerprintHash:   "d50d4e585c22bb92b6c86b592aa2d586",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectedSettings := test.profile.GetHttp3Settings()
			expectedOrder := test.profile.GetHttp3SettingsOrder()
			expectedPseudoHeaders := test.profile.GetHttp3PseudoHeaderOrder()

			transport, err := buildHTTP3Transport(&http3Config{
				insecureSkipVerify:     true,
				http3Settings:          expectedSettings,
				http3SettingsOrder:     expectedOrder,
				http3PriorityParam:     test.profile.GetHttp3PriorityParam(),
				http3PseudoHeaderOrder: expectedPseudoHeaders,
				http3SendGreaseFrames:  test.profile.GetHttp3SendGreaseFrames(),
			})
			if err != nil {
				t.Fatal(err)
			}

			retiring, ok := transport.(*retiringHTTP3Transport)
			if !ok {
				t.Fatalf("unexpected HTTP/3 wrapper type %T", transport)
			}
			underlying, ok := retiring.transport.(*http3.Transport)
			if !ok {
				t.Fatalf("unexpected HTTP/3 transport type %T", retiring.transport)
			}

			for id, expectedValue := range expectedSettings {
				if actualValue, exists := underlying.AdditionalSettings[id]; !exists || actualValue != expectedValue {
					t.Fatalf("setting %d mismatch: expected=%d actual=%d exists=%v", id, expectedValue, actualValue, exists)
				}
			}
			if !reflect.DeepEqual(underlying.PseudoHeaderOrder, expectedPseudoHeaders) {
				t.Fatalf("pseudo-header order mismatch: expected=%v actual=%v", expectedPseudoHeaders, underlying.PseudoHeaderOrder)
			}
			if underlying.PriorityParam != test.profile.GetHttp3PriorityParam() {
				t.Fatalf("priority mismatch: expected=%d actual=%d", test.profile.GetHttp3PriorityParam(), underlying.PriorityParam)
			}
			if underlying.SendGreaseFrames != test.profile.GetHttp3SendGreaseFrames() {
				t.Fatalf("GREASE frame behavior mismatch: expected=%v actual=%v", test.profile.GetHttp3SendGreaseFrames(), underlying.SendGreaseFrames)
			}
			if underlying.MaxResponseHeaderBytes != test.expectMaxResponseHeader {
				t.Fatalf("max response header bytes mismatch: expected=%d actual=%d", test.expectMaxResponseHeader, underlying.MaxResponseHeaderBytes)
			}
			if !underlying.EnableDatagrams {
				t.Fatal("HTTP/3 datagrams must be enabled for browser-compatible profiles")
			}

			if test.expectGeneratedGrease {
				assertGeneratedHTTP3GreaseSetting(t, underlying, expectedSettings, expectedOrder)
			} else {
				if !reflect.DeepEqual(underlying.AdditionalSettingsOrder, expectedOrder) {
					t.Fatalf("settings order mismatch: expected=%v actual=%v", expectedOrder, underlying.AdditionalSettingsOrder)
				}
				if len(underlying.AdditionalSettings) != len(expectedSettings) {
					t.Fatalf("unexpected generated HTTP/3 setting: expected=%v actual=%v", expectedSettings, underlying.AdditionalSettings)
				}
			}

			actualFingerprint := logicalHTTP3Fingerprint(t, underlying, expectedSettings)
			if actualFingerprint != test.expectFingerprint {
				t.Fatalf("HTTP/3 fingerprint mismatch:\nexpected: %s\nactual:   %s", test.expectFingerprint, actualFingerprint)
			}
			actualHash := fmt.Sprintf("%x", md5.Sum([]byte(actualFingerprint)))
			if actualHash != test.expectFingerprintHash {
				t.Fatalf("HTTP/3 fingerprint hash mismatch: expected=%s actual=%s", test.expectFingerprintHash, actualHash)
			}
		})
	}
}

func logicalHTTP3Fingerprint(t *testing.T, transport *http3.Transport, profileSettings map[uint64]uint64) string {
	t.Helper()

	settings := make(map[uint64]uint64, len(transport.AdditionalSettings)+2)
	for id, value := range transport.AdditionalSettings {
		settings[id] = value
	}
	if transport.MaxResponseHeaderBytes >= 0 {
		settings[0x6] = uint64(transport.MaxResponseHeaderBytes)
	}
	if transport.EnableDatagrams {
		settings[0x33] = 1
	}

	orderedSettings := make([]string, 0, len(settings))
	for _, id := range transport.AdditionalSettingsOrder {
		value, exists := settings[id]
		if !exists {
			t.Fatalf("HTTP/3 settings order references absent setting %d", id)
		}
		if _, isProfileSetting := profileSettings[id]; !isProfileSetting && id >= 0x21 && (id-0x21)%0x1f == 0 {
			orderedSettings = append(orderedSettings, "GREASE")
		} else {
			orderedSettings = append(orderedSettings, fmt.Sprintf("%d:%d", id, value))
		}
		delete(settings, id)
	}
	if len(settings) != 0 {
		t.Fatalf("HTTP/3 settings order omits settings %v", settings)
	}

	fingerprintParts := []string{strings.Join(orderedSettings, ";")}
	if transport.SendGreaseFrames {
		fingerprintParts = append(fingerprintParts, "GREASE")
	}
	if transport.PriorityParam != 0 {
		fingerprintParts = append(fingerprintParts, strconv.FormatUint(uint64(transport.PriorityParam), 10))
	}

	pseudoHeaders := make([]string, 0, len(transport.PseudoHeaderOrder))
	for _, pseudoHeader := range transport.PseudoHeaderOrder {
		switch pseudoHeader {
		case ":method":
			pseudoHeaders = append(pseudoHeaders, "m")
		case ":authority":
			pseudoHeaders = append(pseudoHeaders, "a")
		case ":scheme":
			pseudoHeaders = append(pseudoHeaders, "s")
		case ":path":
			pseudoHeaders = append(pseudoHeaders, "p")
		default:
			t.Fatalf("unknown HTTP/3 pseudo-header %q", pseudoHeader)
		}
	}
	fingerprintParts = append(fingerprintParts, strings.Join(pseudoHeaders, ","))
	return strings.Join(fingerprintParts, "|")
}

func assertGeneratedHTTP3GreaseSetting(t *testing.T, transport *http3.Transport, profileSettings map[uint64]uint64, profileOrder []uint64) {
	t.Helper()

	if len(transport.AdditionalSettings) != len(profileSettings)+1 {
		t.Fatalf("expected exactly one generated GREASE setting: profile=%v actual=%v", profileSettings, transport.AdditionalSettings)
	}
	if len(transport.AdditionalSettingsOrder) != len(profileOrder)+1 {
		t.Fatalf("expected GREASE setting at end of order: profile=%v actual=%v", profileOrder, transport.AdditionalSettingsOrder)
	}
	if !reflect.DeepEqual(transport.AdditionalSettingsOrder[:len(profileOrder)], profileOrder) {
		t.Fatalf("profile settings order changed: expected=%v actual=%v", profileOrder, transport.AdditionalSettingsOrder)
	}

	greaseID := transport.AdditionalSettingsOrder[len(profileOrder)]
	greaseValue, exists := transport.AdditionalSettings[greaseID]
	if !exists {
		t.Fatalf("generated GREASE setting %d is absent from settings", greaseID)
	}
	if greaseID < 0x21 || (greaseID-0x21)%0x1f != 0 {
		t.Fatalf("generated setting ID %d is not an HTTP/3 GREASE value", greaseID)
	}
	if greaseValue == 0 {
		t.Fatal("generated GREASE setting value must be non-zero")
	}
}

func TestProtocolRacingHTTP3HandlesConcurrentRequestsAgainstLocalServer(t *testing.T) {
	certificateServer := httptest.NewTLSServer(stdhttp.HandlerFunc(func(stdhttp.ResponseWriter, *stdhttp.Request) {}))
	standardCertificate := certificateServer.TLS.Certificates[0]
	certificateServer.Close()

	packetConn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	var handled atomic.Int64
	server := &http3.Server{
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{{
			Certificate: standardCertificate.Certificate,
			PrivateKey:  standardCertificate.PrivateKey,
			Leaf:        standardCertificate.Leaf,
		}}},
		Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if request.ProtoMajor != 3 {
				http.Error(response, "request did not use HTTP/3", http.StatusHTTPVersionNotSupported)
				return
			}
			handled.Add(1)
			_, _ = io.WriteString(response, "ok")
		}),
	}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(packetConn)
	}()
	t.Cleanup(func() {
		_ = server.Close()
		_ = packetConn.Close()
		select {
		case serveErr := <-serveDone:
			if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && !errors.Is(serveErr, net.ErrClosed) {
				t.Errorf("local HTTP/3 server failed: %v", serveErr)
			}
		case <-time.After(5 * time.Second):
			t.Error("local HTTP/3 server did not stop")
		}
	})

	client, err := NewHttpClient(nil,
		WithClientProfile(profiles.Chrome_144),
		WithInsecureSkipVerify(),
		WithProtocolRacing(),
		WithTimeoutSeconds(10),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)

	const workers = 64
	const requestsPerWorker = 25
	start := make(chan struct{})
	errorsByWorker := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		waitGroup.Add(1)
		go func(worker int) {
			defer waitGroup.Done()
			<-start
			for requestIndex := 0; requestIndex < requestsPerWorker; requestIndex++ {
				request, requestErr := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/%d/%d", packetConn.LocalAddr(), worker, requestIndex), nil)
				if requestErr != nil {
					errorsByWorker <- requestErr
					return
				}
				response, requestErr := client.Do(request)
				if requestErr != nil {
					errorsByWorker <- fmt.Errorf("worker %d request %d: %w", worker, requestIndex, requestErr)
					return
				}
				body, readErr := io.ReadAll(response.Body)
				closeErr := response.Body.Close()
				if readErr != nil {
					errorsByWorker <- fmt.Errorf("worker %d request %d read: %w", worker, requestIndex, readErr)
					return
				}
				if closeErr != nil {
					errorsByWorker <- fmt.Errorf("worker %d request %d close: %w", worker, requestIndex, closeErr)
					return
				}
				if response.ProtoMajor != 3 || string(body) != "ok" {
					errorsByWorker <- fmt.Errorf("worker %d request %d received proto=%q body=%q", worker, requestIndex, response.Proto, body)
					return
				}
			}
		}(worker)
	}

	close(start)
	waitGroup.Wait()
	close(errorsByWorker)
	for workerErr := range errorsByWorker {
		t.Error(workerErr)
	}

	expectedRequests := int64(workers * requestsPerWorker)
	if actual := handled.Load(); actual != expectedRequests {
		t.Fatalf("local HTTP/3 server handled %d requests, expected %d", actual, expectedRequests)
	}
}
