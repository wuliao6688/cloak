package cloak

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wuliao6688/cloak/profiles"
)

// TestCustomerIssueSetInsecureSkipVerifyThroughChain verifies that
// Request.SetInsecureSkipVerify actually reaches the cloak.Transport
// through the HeaderRoundTripper/customHeader wrappers.
// (Fixes: silently failing on custom transports — a real customer pain
//  point, cf. upstream tls-client #147 POST 400 / cert config issues.)
func TestCustomerIssueSetInsecureSkipVerifyThroughChain(t *testing.T) {
	// Self-signed TLS server.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "tls-ok")
	}))
	defer srv.Close()

	// ImpersonateRequest → HeaderRoundTripper; add a custom UA so the
	// customHeaderRoundTripper wrapper is also in the chain.
	req := ImpersonateRequest(profiles.Chrome_150).
		SetHeader("User-Agent", "test-ua").
		SetInsecureSkipVerify(true)

	resp, err := req.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("SetInsecureSkipVerify 未生效(自签证书被拒): %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "tls-ok" {
		t.Fatalf("响应异常: %q", string(body))
	}
	t.Logf("SetInsecureSkipVerify 通过包装链生效, 自签证书请求成功: %q", string(body))
}
