package http3

import (
	"crypto/tls"

	utls "github.com/wuliao6688/utls"
)

// toStdTLSState converts a utls.ConnectionState to a crypto/tls.ConnectionState.
// The two structs have identical fields; utls's is used internally by the
// QUIC TLS stack, while net/http's Response.TLS expects crypto/tls's type.
func toStdTLSState(s utls.ConnectionState) tls.ConnectionState {
	return tls.ConnectionState{
		Version:                     s.Version,
		HandshakeComplete:           s.HandshakeComplete,
		DidResume:                   s.DidResume,
		CipherSuite:                 s.CipherSuite,
		NegotiatedProtocol:          s.NegotiatedProtocol,
		NegotiatedProtocolIsMutual:  s.NegotiatedProtocolIsMutual,
		ServerName:                  s.ServerName,
		PeerCertificates:            s.PeerCertificates,
		VerifiedChains:              s.VerifiedChains,
		SignedCertificateTimestamps: s.SignedCertificateTimestamps,
		OCSPResponse:                s.OCSPResponse,
		TLSUnique:                   s.TLSUnique,
	}
}
