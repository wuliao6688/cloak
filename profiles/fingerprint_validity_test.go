package profiles

import (
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"

	tls "github.com/bogdanfinn/utls"
	"github.com/stretchr/testify/require"
)

func TestAllMappedProfilesProduceValidClientHellos(t *testing.T) {
	for _, key := range sortedMappedProfileKeys() {
		key := key
		profile := MappedTLSClients[key]

		t.Run(key, func(t *testing.T) {
			require.NotEmpty(t, profile.GetClientHelloStr(), "profile has no ClientHello identifier")

			raw, err := marshalProfileClientHello(profile)
			require.NoError(t, err)
			require.Greater(t, len(raw), 4, "marshaled ClientHello is empty")
			require.Equal(t, byte(1), raw[0], "unexpected TLS handshake message type")

			parsed, err := parseMarshaledClientHello(raw)
			require.NoError(t, err)
			require.NotEmpty(t, parsed.CipherSuites, "parsed ClientHello has no cipher suites")
			require.NotEmpty(t, parsed.Extensions, "parsed ClientHello has no TLS extensions")
			require.NoError(t, validateApplicationProtocols(parsed))
		})
	}
}

func TestMappedProfilesSupportConcurrentClientHelloGeneration(t *testing.T) {
	keys := sortedMappedProfileKeys()
	const workers = 8

	errors := make(chan error, workers)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, key := range keys {
				raw, err := marshalProfileClientHello(MappedTLSClients[key])
				if err != nil {
					errors <- fmt.Errorf("%s: %w", key, err)
					return
				}
				if len(raw) <= 4 || raw[0] != 1 {
					errors <- fmt.Errorf("%s: invalid marshaled ClientHello", key)
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

func BenchmarkMappedProfilesConcurrentClientHelloGeneration(b *testing.B) {
	keys := sortedMappedProfileKeys()
	if len(keys) == 0 {
		b.Fatal("no mapped profiles")
	}

	var next atomic.Uint64
	failures := make(chan error, 1)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			index := next.Add(1) - 1
			key := keys[index%uint64(len(keys))]
			raw, err := marshalProfileClientHello(MappedTLSClients[key])
			if err == nil && (len(raw) <= 4 || raw[0] != 1) {
				err = fmt.Errorf("invalid marshaled ClientHello")
			}
			if err != nil {
				select {
				case failures <- fmt.Errorf("%s: %w", key, err):
				default:
				}
				return
			}
		}
	})

	select {
	case err := <-failures:
		b.Fatal(err)
	default:
	}
}

func TestChrome150JA3ExtensionOrder(t *testing.T) {
	raw, err := marshalProfileClientHello(Chrome_150)
	require.NoError(t, err)

	actual, err := clientHelloJA3ExtensionIDs(raw)
	require.NoError(t, err)

	// Keep this in sync with the Chrome 150 fingerprint recorded by the online
	// integration test. JA3 ignores GREASE values but preserves extension order.
	expected := []uint16{17613, 43, 18, 65037, 51, 13, 10, 27, 23, 35, 0, 65281, 45, 11, 5, 16}
	require.Equal(t, expected, actual)
}

func validateApplicationProtocols(spec *tls.ClientHelloSpec) error {
	alpnProtocols := make(map[string]struct{})
	var alpsProtocols []string

	for _, extension := range spec.Extensions {
		switch typed := extension.(type) {
		case *tls.ALPNExtension:
			for _, protocol := range typed.AlpnProtocols {
				if protocol == "" {
					return fmt.Errorf("ALPN contains an empty protocol")
				}
				alpnProtocols[protocol] = struct{}{}
			}
		case *tls.ApplicationSettingsExtension:
			alpsProtocols = append(alpsProtocols, typed.SupportedProtocols...)
		case *tls.ApplicationSettingsExtensionNew:
			alpsProtocols = append(alpsProtocols, typed.SupportedProtocols...)
		}
	}

	for _, protocol := range alpsProtocols {
		if protocol == "" {
			return fmt.Errorf("ALPS contains an empty protocol")
		}
		if _, ok := alpnProtocols[protocol]; !ok {
			return fmt.Errorf("ALPS protocol %q is not advertised by ALPN", protocol)
		}
	}
	return nil
}

func clientHelloJA3ExtensionIDs(raw []byte) ([]uint16, error) {
	if len(raw) < 4 || raw[0] != 1 {
		return nil, fmt.Errorf("invalid ClientHello handshake header")
	}
	handshakeLen := int(raw[1])<<16 | int(raw[2])<<8 | int(raw[3])
	if handshakeLen != len(raw)-4 {
		return nil, fmt.Errorf("ClientHello length mismatch: header=%d actual=%d", handshakeLen, len(raw)-4)
	}

	pos := 4 + 2 + 32 // handshake header, legacy version, and random
	if pos >= len(raw) {
		return nil, fmt.Errorf("ClientHello is truncated before session ID")
	}
	pos++
	pos += int(raw[pos-1])

	readVector16 := func(name string) error {
		if pos+2 > len(raw) {
			return fmt.Errorf("ClientHello is truncated before %s length", name)
		}
		length := int(binary.BigEndian.Uint16(raw[pos : pos+2]))
		pos += 2
		if pos+length > len(raw) {
			return fmt.Errorf("ClientHello %s is truncated", name)
		}
		pos += length
		return nil
	}

	if err := readVector16("cipher suites"); err != nil {
		return nil, err
	}
	if pos >= len(raw) {
		return nil, fmt.Errorf("ClientHello is truncated before compression methods")
	}
	compressionLen := int(raw[pos])
	pos++
	if pos+compressionLen > len(raw) {
		return nil, fmt.Errorf("ClientHello compression methods are truncated")
	}
	pos += compressionLen

	if pos+2 > len(raw) {
		return nil, fmt.Errorf("ClientHello is truncated before extensions length")
	}
	extensionsLen := int(binary.BigEndian.Uint16(raw[pos : pos+2]))
	pos += 2
	extensionsEnd := pos + extensionsLen
	if extensionsEnd != len(raw) {
		return nil, fmt.Errorf("ClientHello extensions length mismatch: end=%d actual=%d", extensionsEnd, len(raw))
	}

	ids := make([]uint16, 0)
	for pos < extensionsEnd {
		if pos+4 > extensionsEnd {
			return nil, fmt.Errorf("TLS extension header is truncated")
		}
		id := binary.BigEndian.Uint16(raw[pos : pos+2])
		extensionLen := int(binary.BigEndian.Uint16(raw[pos+2 : pos+4]))
		pos += 4
		if pos+extensionLen > extensionsEnd {
			return nil, fmt.Errorf("TLS extension %d is truncated", id)
		}
		pos += extensionLen

		if id&0x0f0f != 0x0a0a { // RFC 8701 GREASE values are excluded from JA3.
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func sortedMappedProfileKeys() []string {
	keys := make([]string, 0, len(MappedTLSClients))
	for key := range MappedTLSClients {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
