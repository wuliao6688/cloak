package tls_client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCertificatePinner_IsolatedPerClient(t *testing.T) {
	firstRaw, err := NewCertificatePinner(map[string][]string{
		"example.com": {"first-pin"},
	})
	require.NoError(t, err)

	secondRaw, err := NewCertificatePinner(map[string][]string{
		"example.com": {"second-pin"},
	})
	require.NoError(t, err)

	first := firstRaw.(*certificatePinner)
	second := secondRaw.(*certificatePinner)
	require.Equal(t, []string{"first-pin"}, first.lookupPinnedHost("example.com").Sha256Pins)
	require.Equal(t, []string{"second-pin"}, second.lookupPinnedHost("example.com").Sha256Pins)
}

func TestCertificatePinner_WildcardLookupIsCaseInsensitive(t *testing.T) {
	raw, err := NewCertificatePinner(map[string][]string{
		"*.Example.COM.": {"wildcard-pin"},
	})
	require.NoError(t, err)

	pinner := raw.(*certificatePinner)
	require.Equal(t, []string{"wildcard-pin"}, pinner.lookupPinnedHost("API.EXAMPLE.COM.").Sha256Pins)
	require.Equal(t, []string{"wildcard-pin"}, pinner.lookupPinnedHost("example.com").Sha256Pins)
	require.Nil(t, pinner.lookupPinnedHost("notexample.com"))
}
