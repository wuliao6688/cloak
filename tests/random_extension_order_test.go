package tests

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_RandomExtensionOrderChrome(t *testing.T) {
	options := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(profiles.Chrome_107),
		tls_client.WithRandomTLSExtensionOrder(),
	}

	client := newFingerprintTestClient(t, options...)

	req, err := http.NewRequest(http.MethodGet, peetApiEndpoint, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer resp.Body.Close()

	readBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	tlsApiResponse := TlsApiResponse{}
	if err := json.Unmarshal(readBytes, &tlsApiResponse); err != nil {
		t.Fatal(err)
	}

	// All Extensions have to occur in random order. Grease and Padding are staying in place
	extensions := strings.Split("5-0-35-16-18-10-23-65281-43-51-27-17513-45-13-11-21", "-")

	ja3String := tlsApiResponse.TLS.Ja3
	ja3StringParts := strings.Split(ja3String, ",")
	require.Len(t, ja3StringParts, 5, "invalid JA3 string: %q", ja3String)

	returnedExtensionParts := strings.Split(ja3StringParts[2], "-")
	assert.ElementsMatch(t, extensions, returnedExtensionParts)

	assert.Equal(t, "21", returnedExtensionParts[len(returnedExtensionParts)-1])
}

func TestClient_RandomExtensionOrderCustom(t *testing.T) {
	options := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(profiles.CloudflareCustom),
		tls_client.WithRandomTLSExtensionOrder(),
	}

	client := newFingerprintTestClient(t, options...)

	req, err := http.NewRequest(http.MethodGet, peetApiEndpoint, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer resp.Body.Close()

	readBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	tlsApiResponse := TlsApiResponse{}
	if err := json.Unmarshal(readBytes, &tlsApiResponse); err != nil {
		t.Fatal(err)
	}

	// All Extensions have to occur in random order. Grease and Padding are staying in place
	extensions := strings.Split("0-11-10-35-16-22-23-13", "-")

	ja3String := tlsApiResponse.TLS.Ja3
	ja3StringParts := strings.Split(ja3String, ",")
	require.Len(t, ja3StringParts, 5, "invalid JA3 string: %q", ja3String)

	returnedExtensionParts := strings.Split(ja3StringParts[2], "-")
	assert.ElementsMatch(t, extensions, returnedExtensionParts)
}
