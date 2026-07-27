package tests

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/bogdanfinn/tls-client/profiles"
	tls "github.com/bogdanfinn/utls"
	"github.com/stretchr/testify/require"
)

func TestRecordedFingerprintHashesAreConsistent(t *testing.T) {
	for family, fingerprints := range clientFingerprints {
		for clientHello, expected := range fingerprints {
			family, clientHello, expected := family, clientHello, expected
			t.Run(family+"/"+clientHello, func(t *testing.T) {
				require.NotEmpty(t, expected[ja3String])
				require.Equal(t, fmt.Sprintf("%x", md5.Sum([]byte(expected[ja3String]))), expected[ja3Hash])
				require.NotEmpty(t, expected[akamaiFingerprint])
				require.Equal(t, fmt.Sprintf("%x", md5.Sum([]byte(expected[akamaiFingerprint]))), expected[akamaiFingerprintHash])
			})
		}
	}
}

func TestRecordedJA3MatchesMappedProfiles(t *testing.T) {
	for key, profile := range profiles.MappedTLSClients {
		key, profile := key, profile

		if strings.Contains(strings.ToUpper(key), "_PSK") || profile.GetClientHelloId().RandomExtensionOrder {
			continue
		}

		expected, ok := recordedFingerprint(profile.GetClientHelloStr())
		if !ok {
			continue
		}

		t.Run(key, func(t *testing.T) {
			raw, err := marshalMappedProfileClientHello(profile)
			require.NoError(t, err)

			actual, err := ja3FromClientHello(raw)
			require.NoError(t, err)
			require.Equal(t, expected[ja3String], actual)
			require.Equal(t, expected[ja3Hash], fmt.Sprintf("%x", md5.Sum([]byte(actual))))
		})
	}
}

func recordedFingerprint(clientHello string) (map[string]string, bool) {
	for _, fingerprints := range clientFingerprints {
		if expected, ok := fingerprints[clientHello]; ok {
			return expected, true
		}
	}
	return nil, false
}

func marshalMappedProfileClientHello(profile profiles.ClientProfile) ([]byte, error) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	uConn := tls.UClient(clientConn, &tls.Config{
		ServerName:   "example.com",
		OmitEmptyPsk: true,
	}, profile.GetClientHelloId(), false, false, false)
	if err := uConn.BuildHandshakeStateWithoutSession(); err != nil {
		return nil, err
	}
	if err := uConn.MarshalClientHello(); err != nil {
		return nil, err
	}
	return append([]byte(nil), uConn.HandshakeState.Hello.Raw...), nil
}

func ja3FromClientHello(raw []byte) (string, error) {
	if len(raw) < 4 || raw[0] != 1 {
		return "", fmt.Errorf("invalid ClientHello handshake header")
	}
	if handshakeLen := int(raw[1])<<16 | int(raw[2])<<8 | int(raw[3]); handshakeLen != len(raw)-4 {
		return "", fmt.Errorf("ClientHello length mismatch: header=%d actual=%d", handshakeLen, len(raw)-4)
	}

	pos := 4
	if pos+2+32 > len(raw) {
		return "", fmt.Errorf("ClientHello is truncated before random")
	}
	legacyVersion := binary.BigEndian.Uint16(raw[pos : pos+2])
	pos += 2 + 32

	if _, err := readVector8(raw, &pos, "session ID"); err != nil {
		return "", err
	}
	cipherBytes, err := readVector16(raw, &pos, "cipher suites")
	if err != nil {
		return "", err
	}
	if len(cipherBytes)%2 != 0 {
		return "", fmt.Errorf("cipher suite vector has odd length")
	}

	cipherSuites := make([]uint16, 0, len(cipherBytes)/2)
	for offset := 0; offset < len(cipherBytes); offset += 2 {
		value := binary.BigEndian.Uint16(cipherBytes[offset : offset+2])
		if !isGREASE(value) {
			cipherSuites = append(cipherSuites, value)
		}
	}

	if _, err := readVector8(raw, &pos, "compression methods"); err != nil {
		return "", err
	}
	extensionBytes, err := readVector16(raw, &pos, "extensions")
	if err != nil {
		return "", err
	}
	if pos != len(raw) {
		return "", fmt.Errorf("unexpected trailing ClientHello bytes: %d", len(raw)-pos)
	}

	var extensionIDs, supportedGroups, pointFormats []uint16
	for extensionPos := 0; extensionPos < len(extensionBytes); {
		if extensionPos+4 > len(extensionBytes) {
			return "", fmt.Errorf("TLS extension header is truncated")
		}
		id := binary.BigEndian.Uint16(extensionBytes[extensionPos : extensionPos+2])
		length := int(binary.BigEndian.Uint16(extensionBytes[extensionPos+2 : extensionPos+4]))
		extensionPos += 4
		if extensionPos+length > len(extensionBytes) {
			return "", fmt.Errorf("TLS extension %d is truncated", id)
		}
		data := extensionBytes[extensionPos : extensionPos+length]
		extensionPos += length

		if !isGREASE(id) {
			extensionIDs = append(extensionIDs, id)
		}
		switch id {
		case 10:
			groups, err := parseUint16Vector(data, "supported groups")
			if err != nil {
				return "", err
			}
			for _, group := range groups {
				if !isGREASE(group) {
					supportedGroups = append(supportedGroups, group)
				}
			}
		case 11:
			pointBytes, err := parseUint8Vector(data, "EC point formats")
			if err != nil {
				return "", err
			}
			for _, point := range pointBytes {
				pointFormats = append(pointFormats, uint16(point))
			}
		}
	}

	return strings.Join([]string{
		strconv.FormatUint(uint64(legacyVersion), 10),
		joinUint16(cipherSuites),
		joinUint16(extensionIDs),
		joinUint16(supportedGroups),
		joinUint16(pointFormats),
	}, ","), nil
}

func readVector8(data []byte, pos *int, name string) ([]byte, error) {
	if *pos >= len(data) {
		return nil, fmt.Errorf("ClientHello is truncated before %s length", name)
	}
	length := int(data[*pos])
	(*pos)++
	if *pos+length > len(data) {
		return nil, fmt.Errorf("ClientHello %s is truncated", name)
	}
	value := data[*pos : *pos+length]
	*pos += length
	return value, nil
}

func readVector16(data []byte, pos *int, name string) ([]byte, error) {
	if *pos+2 > len(data) {
		return nil, fmt.Errorf("ClientHello is truncated before %s length", name)
	}
	length := int(binary.BigEndian.Uint16(data[*pos : *pos+2]))
	*pos += 2
	if *pos+length > len(data) {
		return nil, fmt.Errorf("ClientHello %s is truncated", name)
	}
	value := data[*pos : *pos+length]
	*pos += length
	return value, nil
}

func parseUint8Vector(data []byte, name string) ([]byte, error) {
	pos := 0
	value, err := readVector8(data, &pos, name)
	if err != nil {
		return nil, err
	}
	if pos != len(data) {
		return nil, fmt.Errorf("unexpected trailing %s bytes", name)
	}
	return value, nil
}

func parseUint16Vector(data []byte, name string) ([]uint16, error) {
	pos := 0
	value, err := readVector16(data, &pos, name)
	if err != nil {
		return nil, err
	}
	if pos != len(data) || len(value)%2 != 0 {
		return nil, fmt.Errorf("malformed %s vector", name)
	}

	result := make([]uint16, 0, len(value)/2)
	for offset := 0; offset < len(value); offset += 2 {
		result = append(result, binary.BigEndian.Uint16(value[offset:offset+2]))
	}
	return result, nil
}

func isGREASE(value uint16) bool {
	return value&0x0f0f == 0x0a0a
}

func joinUint16(values []uint16) string {
	stringsValues := make([]string, len(values))
	for index, value := range values {
		stringsValues[index] = strconv.FormatUint(uint64(value), 10)
	}
	return strings.Join(stringsValues, "-")
}
