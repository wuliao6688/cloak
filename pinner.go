package tls_client

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	http "github.com/bogdanfinn/fhttp"
	tls "github.com/bogdanfinn/utls"
)

var DefaultBadPinHandler = func(req *http.Request) {
	fmt.Println("this is the default bad pin handler")
}

var ErrBadPinDetected = errors.New("bad ssl pin detected")

// pinHeader holds a set of pinned SPKI SHA256 hashes for a host.
// Replaces the deprecated hpkp.Header from github.com/tam7t/hpkp.
type pinHeader struct {
	Permanent         bool
	Sha256Pins        []string
	IncludeSubDomains bool
}

// Matches returns true if the given pin matches any of the pinned hashes.
func (p *pinHeader) Matches(pin string) bool {
	for _, h := range p.Sha256Pins {
		if h == pin {
			return true
		}
	}
	return false
}

// spkiFingerprint computes the base64-encoded SHA256 hash of the
// certificate's Subject Public Key Info (SPKI) — the standard pin format.
func spkiFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(hash[:])
}

type certificatePinner struct {
	certificatePins     map[string][]string
	pinnedHosts         map[string]*pinHeader
	wildcardPinnedHosts map[string]*pinHeader
}

type CertificatePinner interface {
	Pin(conn *tls.UConn, host string) error
}

func NewCertificatePinner(certificatePins map[string][]string) (CertificatePinner, error) {
	certificatePins = cloneCertificatePins(certificatePins)
	pinner := &certificatePinner{
		certificatePins:     certificatePins,
		pinnedHosts:         make(map[string]*pinHeader, len(certificatePins)),
		wildcardPinnedHosts: make(map[string]*pinHeader),
	}

	err := pinner.init()
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate certificate pinner: %w", err)
	}

	return pinner, nil
}

func (cp *certificatePinner) init() error {
	if len(cp.certificatePins) == 0 {
		return nil
	}

	for host, pinsByHost := range cp.certificatePins {
		host = normalizePinHost(host)
		if host == "" {
			return errors.New("certificate pin host must not be empty")
		}

		includeSubdomains := strings.HasPrefix(host, "*.")
		if includeSubdomains {
			host = strings.TrimPrefix(host, "*.")
			if host == "" {
				return errors.New("wildcard certificate pin host must include a base domain")
			}
		}

		pinnedHost := &pinHeader{
			Permanent:         true,
			Sha256Pins:        append([]string(nil), pinsByHost...),
			IncludeSubDomains: includeSubdomains,
		}
		if includeSubdomains {
			cp.wildcardPinnedHosts[host] = pinnedHost
		} else {
			cp.pinnedHosts[host] = pinnedHost
		}
	}

	return nil
}

func (cp *certificatePinner) Pin(conn *tls.UConn, host string) error {
	if len(cp.certificatePins) == 0 {
		return nil
	}

	pinnedHost := cp.lookupPinnedHost(host)

	if pinnedHost == nil {
		// host is not pinned, we treat it as valid
		return nil
	}

	peerCertificates := conn.ConnectionState().PeerCertificates
	actualPins := make([]string, 0, len(peerCertificates))

	for _, peerCert := range peerCertificates {
		peerPin := spkiFingerprint(peerCert)
		if pinnedHost.Matches(peerPin) {
			return nil
		}
		actualPins = append(actualPins, peerPin)
	}

	return fmt.Errorf("%w, found pins: %v", ErrBadPinDetected, actualPins)
}

func (cp *certificatePinner) lookupPinnedHost(host string) *pinHeader {
	host = normalizePinHost(host)
	if host == "" {
		return nil
	}

	if pinnedHost := cp.pinnedHosts[host]; pinnedHost != nil {
		return pinnedHost
	}
	if pinnedHost := cp.wildcardPinnedHosts[host]; pinnedHost != nil {
		return pinnedHost
	}

	for {
		dot := strings.IndexByte(host, '.')
		if dot < 0 || dot == len(host)-1 {
			return nil
		}
		host = host[dot+1:]
		if pinnedHost := cp.pinnedHosts[host]; pinnedHost != nil && pinnedHost.IncludeSubDomains {
			return pinnedHost
		}
		if pinnedHost := cp.wildcardPinnedHosts[host]; pinnedHost != nil {
			return pinnedHost
		}
	}
}

func normalizePinHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}
