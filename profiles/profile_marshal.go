package profiles

import (
	"fmt"
	"net"

	tls "github.com/wuliao6688/utls"
)

// marshalProfileClientHello builds a full TLS ClientHello from the profile
// and returns the raw handshake message bytes (without the record layer header).
func marshalProfileClientHello(profile ClientProfile) ([]byte, error) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	config := &tls.Config{ServerName: "example.com", OmitEmptyPsk: true}
	uConn := tls.UClient(clientConn, config, profile.GetClientHelloId(), false, false, false)
	if err := uConn.BuildHandshakeStateWithoutSession(); err != nil {
		return nil, err
	}
	if err := uConn.MarshalClientHello(); err != nil {
		return nil, err
	}
	return append([]byte(nil), uConn.HandshakeState.Hello.Raw...), nil
}

// parseMarshaledClientHello wraps raw ClientHello handshake bytes in a TLS
// record layer header and parses them with the uTLS Fingerprinter.
func parseMarshaledClientHello(raw []byte) (*tls.ClientHelloSpec, error) {
	if len(raw) > 0xffff {
		return nil, fmt.Errorf("ClientHello is too large: %d bytes", len(raw))
	}

	record := make([]byte, 5, len(raw)+5)
	record[0] = 22 // TLS handshake record
	record[1] = 0x03
	record[2] = 0x01
	record[3] = byte(len(raw) >> 8)
	record[4] = byte(len(raw))
	record = append(record, raw...)

	fingerprinter := tls.Fingerprinter{AllowBluntMimicry: true}
	return fingerprinter.FingerprintClientHello(record)
}
