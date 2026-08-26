package cloak

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wuliao6688/cloak/profiles"
)

// TestCustomDialContext verifies DialContext replaces the socket dialer
// (custom DNS / SOCKS / routing scenarios — customer issue D3).
func TestCustomDialContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "dial-ok")
	}))
	defer srv.Close()

	var dialCalls atomic.Int32
	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{
		// The custom dialer resolves through a fixed target: pretend the
		// request host is unreachable, but our dialer redirects to srv.
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialCalls.Add(1)
			// Replace the target with the real server (simulates custom
			// DNS / proxy routing).
			d := &net.Dialer{}
			return d.DialContext(ctx, network, srv.Listener.Addr().String())
		},
	})
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	// Host won't resolve normally; only the custom dialer can reach it.
	resp, err := client.Get("http://custom-dns.invalid/")
	if err != nil {
		t.Fatalf("custom dialer 应生效: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "dial-ok" {
		t.Fatalf("响应异常: %q", string(body))
	}
	if dialCalls.Load() == 0 {
		t.Fatal("自定义 DialContext 未被调用")
	}
	t.Logf("自定义 DialContext 生效, 调用 %d 次: %q", dialCalls.Load(), string(body))
}

// TestLocalAddrBinding verifies LocalAddr binds the source address
// (multi-NIC / source-IP selection — customer issue D7).
func TestLocalAddrBinding(t *testing.T) {
	// LocalAddr must be a reachable local address; use the loopback IPv4.
	local := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "local-ok")
	}))
	defer srv.Close()

	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{
		LocalAddr: local,
	})
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("LocalAddr 请求失败: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "local-ok" {
		t.Fatalf("响应异常: %q", string(body))
	}
	t.Logf("LocalAddr 绑定生效(127.0.0.1): %q", string(body))
}

// TestLocalAddrBindingFails verifies an unreachable LocalAddr fails the
// dial (proves the binding is actually applied).
func TestLocalAddrBindingFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	// 192.0.2.1 is TEST-NET-1 (reserved, never routed) — binding fails.
	unreachable := &net.TCPAddr{IP: net.IPv4(192, 0, 2, 1)}

	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{
		LocalAddr: unreachable,
	})
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	_, err := client.Get(srv.URL)
	if err == nil {
		t.Fatal("不可达 LocalAddr 应导致连接失败")
	}
	if !strings.Contains(err.Error(), "cannot assign requested address") &&
		!strings.Contains(err.Error(), "bind") {
		t.Logf("失败信息: %v", err)
	}
	t.Logf("不可达 LocalAddr 正确失败: %v", err)
}
