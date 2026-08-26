package cloak

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wuliao6688/cloak/profiles"
)

// ─── 客户问题验证：C3 gzip 响应乱码 ───
// 上游 #32: Accept-Encoding 设置后 body 是乱码。
// 验证: Transport 在浏览器 UA/头 + gzip 服务器下能正确解压。
func TestCustomerIssueGzipResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "" && !strings.Contains(r.Header.Get("Accept-Encoding"), "identity") {
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			gz.Write([]byte("gzip-ok-内容"))
			gz.Close()
			return
		}
		io.WriteString(w, "plain-ok")
	}))
	defer srv.Close()

	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	// 浏览器画像自带 Accept-Encoding: gzip, deflate, br
	resp, err := client.Get(srv.URL + "/gzip")
	if err != nil { t.Fatalf("GET: %v", err) }
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "gzip-ok") {
		t.Fatalf("gzip 响应乱码: got %q", string(body))
	}
	t.Logf("gzip 响应正常: %q", string(body))
}

// ─── 客户问题验证：C2 重定向 ───
// 上游 #14: WithNotFollowRedirects 反而跟随；httpbin cookies 无限循环。
// 验证: 标准 net/http 重定向行为正确(自动跟随 + 可禁用)。
func TestCustomerIssueRedirects(t *testing.T) {
	// 重定向链: /a → /b → /final
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/b", 302) })
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/final", 302) })
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "redirected-ok") })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	defer tr.CloseIdleConnections()

	// 自动跟随(默认)
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Get(srv.URL + "/a")
	if err != nil { t.Fatalf("GET /a: %v", err) }
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "redirected-ok" {
		t.Fatalf("重定向未跟随: got %q", string(body))
	}
	t.Logf("自动跟随重定向正常: %q", string(body))

	// 禁用跟随
	client2 := &http.Client{Transport: tr, Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp2, err := client2.Get(srv.URL + "/a")
	if err != nil { t.Fatalf("GET /a (no-follow): %v", err) }
	resp2.Body.Close()
	if resp2.StatusCode != 302 {
		t.Fatalf("禁用跟随应返回 302, got %d", resp2.StatusCode)
	}
	t.Logf("禁用跟随正常: 302")
}

// ─── 客户问题验证：A3 无超时挂起 ───
// 上游 #33/#578: 请求永久挂起,超时/异常不触发。
// 验证: 慢服务器 + client 超时能正确返回,不挂死。
func TestCustomerIssueTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // 慢服务器
		io.WriteString(w, "late")
	}))
	defer srv.Close()

	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	defer tr.CloseIdleConnections()

	client := &http.Client{Transport: tr, Timeout: 500 * time.Millisecond}
	start := time.Now()
	_, err := client.Get(srv.URL + "/slow")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("慢服务器应超时,但请求成功了")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("超时未生效: 耗时 %v", elapsed)
	}
	t.Logf("超时正确触发: %v (%v)", err, elapsed)
}

// ─── 客户问题验证：A1/A2 并发稳定性 ───
// 上游 #53: 100 并发 nil deref；#71: 运行数小时后崩溃。
// 验证: 100 并发 × 50 轮 = 5000 请求,无 panic、无 goroutine 泄漏。
func TestCustomerIssueConcurrencyStability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	defer tr.CloseIdleConnections()

	const workers = 100
	const rounds = 50

	before := runtime.NumGoroutine()
	var wg sync.WaitGroup
	errCh := make(chan error, workers*rounds)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
			for i := 0; i < rounds; i++ {
				resp, err := client.Get(srv.URL + "/conc")
				if err != nil { errCh <- err; continue }
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	close(errCh)

	errCount := 0
	for range errCh { errCount++ }
	if errCount > 0 {
		t.Fatalf("并发请求失败 %d 个", errCount)
	}

	// goroutine 泄漏检查(短暂等待后应回落)
	time.Sleep(300 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+10 {
		t.Fatalf("goroutine 泄漏: before=%d after=%d", before, after)
	}
	t.Logf("100 并发 × 50 轮 = %d 请求全部成功, goroutines %d→%d", workers*rounds, before, after)
}

// ─── 客户问题验证：C1 证书配置静默失效(已修复) ───
// 上游 #147 POST 400 常因证书/配置失效; Request.SetInsecureSkipVerify
// 曾对自定义 Transport 静默无效(类型断言 *http.Transport 不匹配包装链)。
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
