package tls_client

import (
	"net/url"
	"testing"

	http "github.com/bogdanfinn/fhttp"
	"github.com/stretchr/testify/require"
)

func TestCookieJar_PreservesCookiesWithSameNameOnDifferentPaths(t *testing.T) {
	jar := NewCookieJar()
	rootURL, err := url.Parse("https://example.com/")
	require.NoError(t, err)
	apiURL, err := url.Parse("https://example.com/api")
	require.NoError(t, err)

	jar.SetCookies(rootURL, []*http.Cookie{{Name: "session", Value: "root", Path: "/"}})
	jar.SetCookies(apiURL, []*http.Cookie{{Name: "session", Value: "api", Path: "/api"}})

	require.Len(t, jar.Cookies(apiURL), 2)
	require.Len(t, jar.Cookies(rootURL), 1)
}

func TestCookieJar_GetAllCookiesReturnsDeepCopy(t *testing.T) {
	jar := NewCookieJar()
	u, err := url.Parse("https://example.com:8443/")
	require.NoError(t, err)
	jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "original", Path: "/"}})

	all := jar.GetAllCookies()
	require.Contains(t, all, "example.com")
	all["example.com"][0].Value = "mutated"
	delete(all, "example.com")

	again := jar.GetAllCookies()
	require.Equal(t, "original", again["example.com"][0].Value)
}

func TestCookieJar_DerivesDefaultPathBeforeSnapshotMerge(t *testing.T) {
	jar := NewCookieJar()
	apiURL, err := url.Parse("https://example.com/api/resource")
	require.NoError(t, err)
	rootURL, err := url.Parse("https://example.com/")
	require.NoError(t, err)

	jar.SetCookies(apiURL, []*http.Cookie{{Name: "session", Value: "api"}})
	jar.SetCookies(rootURL, []*http.Cookie{{Name: "session", Value: "root"}})

	require.Len(t, jar.Cookies(apiURL), 2)
	require.Len(t, jar.GetAllCookies()["example.com"], 2)
}
