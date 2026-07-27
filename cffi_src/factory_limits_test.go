package tls_client_cffi_src

import (
	"encoding/base64"
	"io"
	"strings"
	"testing"

	http "github.com/bogdanfinn/fhttp"
	"github.com/stretchr/testify/require"
)

func TestBuildRequest_InvalidURLWithHostOverrideReturnsError(t *testing.T) {
	host := "example.com"
	request, requestErr := BuildRequest(RequestInput{
		RequestMethod:       http.MethodGet,
		RequestUrl:          "://invalid-url",
		RequestHostOverride: &host,
	})
	require.Nil(t, request)
	require.NotNil(t, requestErr)
}

func TestReadResponseBodyEnforcesLimit(t *testing.T) {
	_, err := readResponseBody(strings.NewReader("12345"), 4)
	require.ErrorContains(t, err, "exceeds configured limit")

	body, err := readResponseBody(strings.NewReader("1234"), 4)
	require.NoError(t, err)
	require.Equal(t, []byte("1234"), body)

	body, err = readResponseBody(strings.NewReader("safe"), int64(1<<63-1))
	require.NoError(t, err)
	require.Equal(t, []byte("safe"), body)
}

func TestBuildResponseEnforcesLimit(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("12345")),
	}

	_, responseErr := BuildResponse("", false, resp, nil, RequestInput{MaxResponseBodyBytes: 4})
	require.NotNil(t, responseErr)
	require.ErrorContains(t, responseErr, "exceeds configured limit")
}

func TestEncodeByteResponse(t *testing.T) {
	body := []byte("binary\x00payload")
	expected := "data:" + http.DetectContentType(body) + ";base64," + base64.StdEncoding.EncodeToString(body)
	require.Equal(t, expected, encodeByteResponse(body))
}

func BenchmarkEncodeByteResponse1MiB(b *testing.B) {
	body := []byte(strings.Repeat("0123456789abcdef", 64*1024))
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for range b.N {
		_ = encodeByteResponse(body)
	}
}
