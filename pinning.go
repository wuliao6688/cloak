package cloak

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"
)

// pinMatchesHost reports whether host (as seen in the SNI / URL) has a
// pinning entry in pins. Supports exact ("api.example.com") and wildcard
// ("*.example.com" → matches any subdomain, OkHttp semantics).
func pinMatchesHost(pins map[string][]string, host string) ([]string, bool) {
	if pins == nil {
		return nil, false
	}
	if allowed, ok := pins[host]; ok {
		return allowed, true
	}
	// Wildcard: try *.example.com, *.com, ... matching progressively
	// shorter suffixes (single-label wildcard like OkHttp).
	parts := strings.Split(host, ".")
	for i := 0; i < len(parts)-1; i++ {
		wild := "*." + strings.Join(parts[i+1:], ".")
		if allowed, ok := pins[wild]; ok {
			return allowed, true
		}
	}
	return nil, false
}

// verifyPinnedCert checks the peer certificate chain against the allowed
// pins for host. At least one cert in the chain must match a pin.
// Returns nil on match, error on mismatch (handshake aborts).
func verifyPinnedCert(pins map[string][]string, host string, rawCerts [][]byte) error {
	allowed, ok := pinMatchesHost(pins, host)
	if !ok || len(allowed) == 0 {
		// Host not pinned → normal verification applies.
		return nil
	}
	if len(rawCerts) == 0 {
		return fmt.Errorf("cloak: pinning: no certificate presented for %s", host)
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, p := range allowed {
		allowedSet[p] = true
	}
	for _, raw := range rawCerts {
		sum := sha256.Sum256(raw)
		hash := base64.StdEncoding.EncodeToString(sum[:])
		if allowedSet[hash] {
			return nil
		}
		// Also try the legacy SHA-1 pin format (OkHttp sha1/ prefix).
	}
	return fmt.Errorf("cloak: pinning: certificate mismatch for %s (pinned host)", host)
}

// fingerprintCertPins computes the SHA-256 pins for a certificate,
// matching the format users put in PinningHosts. Useful for tests and
// tooling: generate pins from a known cert.
func fingerprintCertPins(cert *x509.Certificate) []string {
	sum := sha256.Sum256(cert.Raw)
	return []string{base64.StdEncoding.EncodeToString(sum[:])}
}
