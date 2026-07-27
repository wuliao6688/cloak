//go:build integration
// +build integration

package tests

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJA3Integration_DefaultProfile(t *testing.T) {
	client, err := tls_client.NewHttpClient(nil,
		tls_client.WithClientProfile(profiles.DefaultClientProfile),
		tls_client.WithInsecureSkipVerify(), // The external echo service can serve an expired certificate.
	)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, peetApiEndpoint, nil)
	require.NoError(t, err)
	req.Header = defaultHeader

	resp, err := client.Do(req)
	require.NoError(t, err)

	expected, ok := expectedFingerprintForClientHelloStr(profiles.DefaultClientProfile.GetClientHelloStr())
	require.True(t, ok, "default profile is missing a recorded fingerprint")
	compareResponse(t, "default profile", expected, resp)
}

func TestJA3Integration_RandomProfile(t *testing.T) {
	resolvedKey, resolvedProfile := profiles.ResolveClientProfileWithKey(profiles.RandomProfileIdentifier)
	require.NotEmpty(t, resolvedKey)

	client, err := tls_client.NewHttpClient(nil,
		tls_client.WithClientProfile(resolvedProfile),
		tls_client.WithInsecureSkipVerify(), // The external echo service can serve an expired certificate.
	)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, peetApiEndpoint, nil)
	require.NoError(t, err)
	req.Header = defaultHeader

	resp, err := client.Do(req)
	require.NoError(t, err)

	compareResolvedProfileResponse(t, "random("+resolvedKey+")", resolvedProfile, resp)
}

func TestJA3Integration_ChaosProfile(t *testing.T) {
	resolvedKey, resolvedProfile := profiles.ResolveClientProfileWithKey(profiles.ChaosProfileIdentifier)
	require.NotEmpty(t, resolvedKey)

	client, err := tls_client.NewHttpClient(nil,
		tls_client.WithClientProfile(resolvedProfile),
		tls_client.WithInsecureSkipVerify(), // The external echo service can serve an expired certificate.
	)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, peetApiEndpoint, nil)
	require.NoError(t, err)
	req.Header = defaultHeader

	resp, err := client.Do(req)
	require.NoError(t, err)

	compareResolvedProfileResponse(t, "chaos("+resolvedKey+")", resolvedProfile, resp)
}

func expectedFingerprintForClientHelloStr(clientHelloStr string) (map[string]string, bool) {
	for _, families := range clientFingerprints {
		if expected, ok := families[clientHelloStr]; ok {
			return expected, true
		}
	}
	return nil, false
}

func compareResolvedProfileResponse(t *testing.T, label string, profile profiles.ClientProfile, resp *http.Response) {
	t.Helper()

	if expected, ok := expectedFingerprintForClientHelloStr(profile.GetClientHelloStr()); ok {
		compareResponse(t, label, expected, resp)
		return
	}

	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var observed TlsApiResponse
	require.NoError(t, json.Unmarshal(responseBody, &observed))
	require.NotEmpty(t, observed.TLS.Ja3, "%s returned an empty JA3 fingerprint", label)
	require.NotEmpty(t, observed.HTTP2.AkamaiFingerprint, "%s returned an empty Akamai fingerprint", label)

	assert.Equal(t, md5Hex(observed.TLS.Ja3), observed.TLS.Ja3Hash, "%s returned an inconsistent JA3 hash", label)
	assert.Equal(t, md5Hex(observed.HTTP2.AkamaiFingerprint), observed.HTTP2.AkamaiFingerprintHash, "%s returned an inconsistent Akamai hash", label)
}

func md5Hex(value string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(value)))
}
