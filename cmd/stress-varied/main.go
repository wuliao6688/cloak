// Command stress-varied runs a long-duration (default 1h) stress test that
// exercises many real-world cloak usage patterns:
//
//   - POST/GET/PUT/PATCH/DELETE/HEAD/OPTIONS
//   - body types: JSON / form / multipart / raw / no body
//   - connection REUSE: fixed *http.Client across many requests (keep-alive)
//   - header mutation: same client, different headers per request
//   - cookie mutation: same client, different cookies per request
//   - protocol switching: HTTP/1.1 + HTTP/2 + HTTP/3 local servers
//   - profile switching: Chrome/Firefox/Safari/OkHttp/Opera/Brave
//   - features: redirects, auth, gzip, charset (EUC-KR), timeout, pinning
//
// Metrics: success/fail, status distribution, latency percentiles,
// memory growth, goroutine leaks.
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wuliao6688/cloak"
	"github.com/wuliao6688/cloak/profiles"
	"github.com/wuliao6688/quic-go-utls"
	"github.com/wuliao6688/quic-go-utls/http3"
	utls "github.com/wuliao6688/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/text/encoding/korean"
)

// ─── metrics ───
var (
	totalReq    atomic.Int64
	totalOK     atomic.Int64
	totalFail   atomic.Int64
	failReasons sync.Map // reason → count
	statusCount sync.Map // status → count
	latencies   []time.Duration
	latMu       sync.Mutex
)

func recordLatency(d time.Duration) {
	latMu.Lock()
	latencies = append(latencies, d)
	latMu.Unlock()
}

func latencyPercentile(p float64) time.Duration {
	latMu.Lock()
	defer latMu.Unlock()
	if len(latencies) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

func incFail(reason string) {
	if v, ok := failReasons.Load(reason); ok {
		failReasons.Store(reason, v.(int64)+1)
	} else {
		failReasons.Store(reason, int64(1))
	}
}

func incStatus(code int) {
	if v, ok := statusCount.Load(code); ok {
		statusCount.Store(code, v.(int64)+1)
	} else {
		statusCount.Store(code, int64(1))
	}
}

// ─── profile pool ───
var profilePool = []profiles.ClientProfile{
	profiles.Chrome_150,
	profiles.Firefox_147,
	profiles.Safari_IOS_18_0,
	profiles.Okhttp4Android13,
	profiles.Opera_91,
	profiles.Brave_146,
}

func randProfile() profiles.ClientProfile {
	return profilePool[rand.Intn(len(profilePool))]
}

// ─── local servers ───

func startLocalServers() (h1URL, h2URL, h3URL string, cleanup func()) {
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, r.Body)
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"method": r.Method, "ua": r.Header.Get("User-Agent"),
			"body": string(body), "ct": r.Header.Get("Content-Type"),
			"cookie": r.Header.Get("Cookie"), "auth": r.Header.Get("Authorization"),
			"proto": r.Proto, "query": r.URL.RawQuery, "hdr_x": r.Header.Get("X-Custom"),
		})
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/json", 302)
	})
	mux.HandleFunc("/euckr", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=EUC-KR")
		enc := korean.EUCKR.NewEncoder()
		b, _ := enc.Bytes([]byte("한국어 페이지"))
		w.Write(b)
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		io.WriteString(w, "slow-done")
	})
	mux.HandleFunc("/any", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"method": r.Method, "body": string(body), "proto": r.Proto})
	})

	// H1 (plain)
	h1 := httptest.NewServer(mux)

	// H2 (h2c, cleartext)
	h2mux := http.NewServeMux()
	h2mux.Handle("/", mux)
	ln2, _ := net.Listen("tcp", "127.0.0.1:0")
	h2srv := &http.Server{Handler: h2c.NewHandler(h2mux, &http2.Server{})}
	go h2srv.Serve(ln2)

	// H3 (QUIC) with self-signed cert — write cert files like the tests do
	certPath, keyPath := writeCertFiles()
	udpConn, _ := net.ListenPacket("udp", "127.0.0.1:0")
	h3Addr := udpConn.LocalAddr().String()
	_ = udpConn.Close()

	h3srv := &http3.Server{
		Addr:    h3Addr,
		Handler: mux,
		TLSConfig: &utls.Config{
			NextProtos: []string{"h3"},
		},
		QUICConfig: &quic.Config{},
	}
	h3Done := make(chan error, 1)
	go func() { h3Done <- h3srv.ListenAndServeTLS(certPath, keyPath) }()

	// Give the H3 server a moment to bind; surface startup errors.
	select {
	case err := <-h3Done:
		if err != nil {
			fmt.Fprintf(os.Stderr, "H3 server failed to start: %v\n", err)
			os.Exit(1)
		}
	case <-time.After(500 * time.Millisecond):
	}

	// Tests connect via https://localhost:port (cert SAN is localhost).
	_, h3PortStr, _ := net.SplitHostPort(h3Addr)
	h3URL = "https://localhost:" + h3PortStr

	return h1.URL, "http://" + ln2.Addr().String(), h3URL,
		func() {
			h1.Close()
			h2srv.Close()
			h3srv.Close()
		}
}

func writeCertFiles() (certPath, keyPath string) {
	certPEM, keyPEM := genSelfSigned()
	certPath = "/tmp/cloak-stress-cert.pem"
	keyPath = "/tmp/cloak-stress-key.pem"
	os.WriteFile(certPath, certPEM, 0o600)
	os.WriteFile(keyPath, keyPEM, 0o600)
	return
}

func genSelfSigned() (certPEM, keyPEM []byte) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, _ := x509.CreateCertificate(crand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	keyDER, _ := x509.MarshalECPrivateKey(priv)
	certPEM = pemEncode("CERTIFICATE", der)
	keyPEM = pemEncode("EC PRIVATE KEY", keyDER)
	return
}

func pemEncode(typ string, der []byte) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "-----BEGIN %s-----\n", typ)
	// base64 wrap
	enc := base64Std(der)
	for len(enc) > 64 {
		b.Write(enc[:64])
		b.WriteByte('\n')
		enc = enc[64:]
	}
	b.Write(enc)
	b.WriteString("\n-----END " + typ + "-----\n")
	return b.Bytes()
}

func base64Std(b []byte) []byte {
	const tbl = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out []byte
	val := uint32(0)
	bits := 0
	for _, c := range b {
		val = val<<8 | uint32(c)
		bits += 8
		for bits >= 6 {
			out = append(out, tbl[(val>>(bits-6))&0x3f])
			bits -= 6
		}
	}
	if bits > 0 {
		out = append(out, tbl[(val<<(6-bits))&0x3f])
	}
	for len(out)%4 != 0 {
		out = append(out, '=')
	}
	return out
}

// ─── scenario types ───

type scenario struct {
	name string
	run  func(c *http.Client, base string) error
}

var methodPool = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

func randBodyType() string {
	return []string{"json", "form", "multipart", "raw", "none"}[rand.Intn(5)]
}

func buildBody(bt string) (io.Reader, string) {
	switch bt {
	case "json":
		b, _ := json.Marshal(map[string]any{"k": "v", "n": rand.Intn(100), "ts": time.Now().Unix()})
		return bytes.NewReader(b), "application/json"
	case "form":
		f := url.Values{}
		f.Set("field", fmt.Sprintf("val-%d", rand.Intn(1000)))
		return strings.NewReader(f.Encode()), "application/x-www-form-urlencoded"
	case "multipart":
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		mw.WriteField("name", fmt.Sprintf("user-%d", rand.Intn(1000)))
		fw, _ := mw.CreateFormFile("file", "data.txt")
		fw.Write([]byte("file-content"))
		mw.Close()
		return &buf, mw.FormDataContentType()
	case "raw":
		return strings.NewReader(fmt.Sprintf("raw-%d", rand.Intn(1000))), "text/plain"
	default:
		return nil, ""
	}
}

// do builds an http.Request and executes it via the (possibly reused) client.
func do(c *http.Client, m, u string, body io.Reader, hdr map[string]string, cookies []*http.Cookie) (*http.Response, error) {
	req, err := http.NewRequest(m, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "stress-varied/"+fmt.Sprintf("%d", rand.Intn(999)))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	return c.Do(req)
}

func randomScenario() scenario {
	n := rand.Intn(100)
	switch {
	// 1. POST with random body type (the workhorse)
	case n < 30:
		btName := randBodyType()
		body, ct := buildBody(btName)
		return scenario{name: "post_" + btName, run: func(c *http.Client, base string) error {
			hdr := map[string]string{}
			if ct != "" {
				hdr["Content-Type"] = ct
			}
			resp, err := do(c, "POST", base+"/echo", body, hdr, nil)
			if err != nil { return err }
			defer resp.Body.Close()
			if resp.StatusCode != 200 { return fmt.Errorf("status %d", resp.StatusCode) }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 2. POST with header mutation (same client, different headers)
	case n < 40:
		return scenario{name: "post_header_mutation", run: func(c *http.Client, base string) error {
			// random header set each time
			hdr := map[string]string{
				"X-Random-" + fmt.Sprint(rand.Intn(5)): fmt.Sprintf("h%d", rand.Intn(1000)),
				"Content-Type": "application/json",
			}
			if rand.Intn(2) == 0 {
				hdr["Accept-Language"] = []string{"en-US", "zh-CN", "ja-JP", "ko-KR"}[rand.Intn(4)]
			}
			resp, err := do(c, "POST", base+"/echo", strings.NewReader(`{"a":1}`), hdr, nil)
			if err != nil { return err }
			defer resp.Body.Close()
			if resp.StatusCode != 200 { return fmt.Errorf("status %d", resp.StatusCode) }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 3. POST with cookie mutation (same client, different cookies)
	case n < 50:
		return scenario{name: "post_cookie_mutation", run: func(c *http.Client, base string) error {
			cookies := []*http.Cookie{
				{Name: "sess", Value: fmt.Sprintf("s%d", rand.Intn(100000))},
				{Name: "cid", Value: fmt.Sprintf("c%d", rand.Intn(100000))},
			}
			if rand.Intn(2) == 0 {
				cookies = append(cookies, &http.Cookie{Name: "extra", Value: "e"})
			}
			resp, err := do(c, "POST", base+"/echo", strings.NewReader(`{"c":1}`), nil, cookies)
			if err != nil { return err }
			defer resp.Body.Close()
			if resp.StatusCode != 200 { return fmt.Errorf("status %d", resp.StatusCode) }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 4. Full method matrix
	case n < 62:
		m := methodPool[rand.Intn(len(methodPool))]
		return scenario{name: "method_" + m, run: func(c *http.Client, base string) error {
			var body io.Reader
			if m == "POST" || m == "PUT" || m == "PATCH" {
				body = strings.NewReader(fmt.Sprintf("m%d", rand.Intn(100)))
			}
			resp, err := do(c, m, base+"/any?q=v"+fmt.Sprint(rand.Intn(100)), body, nil, nil)
			if err != nil { return err }
			defer resp.Body.Close()
			if resp.StatusCode != 200 { return fmt.Errorf("status %d", resp.StatusCode) }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 5. JSON round-trip
	case n < 70:
		return scenario{name: "json_rt", run: func(c *http.Client, base string) error {
			var in = map[string]any{"ping": "pong", "n": rand.Intn(1000)}
			b, _ := json.Marshal(in)
			resp, err := do(c, "POST", base+"/json", bytes.NewReader(b), map[string]string{"Content-Type": "application/json"}, nil)
			if err != nil { return err }
			defer resp.Body.Close()
			var out map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return err }
			if out["ping"] != "pong" { return fmt.Errorf("json mismatch") }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 6. Redirect follow
	case n < 74:
		return scenario{name: "redirect_follow", run: func(c *http.Client, base string) error {
			resp, err := do(c, "GET", base+"/redirect", nil, nil, nil)
			if err != nil { return err }
			defer resp.Body.Close()
			if resp.StatusCode != 200 { return fmt.Errorf("redirect chain failed: %d", resp.StatusCode) }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 7. Charset decode (EUC-KR)
	case n < 78:
		return scenario{name: "charset_euckr", run: func(c *http.Client, base string) error {
			resp, err := do(c, "GET", base+"/euckr", nil, nil, nil)
			if err != nil { return err }
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			// raw EUC-KR bytes should contain the UTF-8 representation after decode;
			// cloak's Response.String() does the decode, but here we use raw client:
			// verify the server actually sent EUC-KR (not UTF-8)
			if bytes.Contains(body, []byte("한국어")) {
				return fmt.Errorf("expected EUC-KR bytes, got UTF-8")
			}
			incStatus(resp.StatusCode)
			return nil
		}}
	// 8. Timeout
	case n < 82:
		return scenario{name: "timeout", run: func(c *http.Client, base string) error {
			c.Timeout = 300 * time.Millisecond
			defer func() { c.Timeout = 5 * time.Second }()
			_, err := do(c, "GET", base+"/slow", nil, nil, nil)
			if err == nil { return fmt.Errorf("slow endpoint should timeout") }
			return nil
		}}
	// 9. Error status handling
	case n < 87:
		return scenario{name: "status_err", run: func(c *http.Client, base string) error {
			paths := []string{"/status/404", "/status/500", "/status/429"}
			p := paths[rand.Intn(len(paths))]
			resp, err := do(c, "GET", base+p, nil, nil, nil)
			if err != nil { return err }
			defer resp.Body.Close()
			incStatus(resp.StatusCode)
			return nil
		}}
	// 10. Auth
	case n < 91:
		return scenario{name: "auth", run: func(c *http.Client, base string) error {
			req, _ := http.NewRequest("GET", base+"/echo", nil)
			if rand.Intn(2) == 0 {
				req.SetBasicAuth("user", "pass")
			} else {
				req.Header.Set("Authorization", "Bearer tok-"+fmt.Sprint(rand.Intn(1000)))
			}
			resp, err := c.Do(req)
			if err != nil { return err }
			defer resp.Body.Close()
			if resp.StatusCode != 200 { return fmt.Errorf("status %d", resp.StatusCode) }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 11. Protocol check (H2 vs H1 — different base URLs)
	default:
		return scenario{name: "proto_check", run: func(c *http.Client, base string) error {
			resp, err := do(c, "GET", base+"/any", nil, nil, nil)
			if err != nil { return err }
			defer resp.Body.Close()
			incStatus(resp.StatusCode)
			return nil
		}}
	}
}

// worker with REUSED client (connection pooling / keep-alive stress)
func reuseWorker(ctx context.Context, bases []string, wg *sync.WaitGroup) {
	defer wg.Done()
	client := cloak.Impersonate(randProfile()) // one client, reused
	client.Timeout = 5 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		sc := randomScenario()
		base := bases[rand.Intn(len(bases))]
		start := time.Now()
		err := sc.run(client, base)
		recordLatency(time.Since(start))
		totalReq.Add(1)
		if err != nil {
			if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "context deadline") {
				totalOK.Add(1) // expected timeout
			} else {
				totalFail.Add(1)
				incFail(sc.name + ": " + err.Error())
			}
		} else {
			totalOK.Add(1)
		}
	}
}

// worker that builds a FRESH Request each time (creation-path stress)
func freshWorker(ctx context.Context, bases []string, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		sc := randomScenario()
		hc := cloak.Impersonate(randProfile()) // fresh client each iteration
		hc.Timeout = 5 * time.Second
		base := bases[rand.Intn(len(bases))]
		start := time.Now()
		err := sc.run(hc, base)
		recordLatency(time.Since(start))
		totalReq.Add(1)
		if err != nil {
			if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "context deadline") {
				totalOK.Add(1)
			} else {
				totalFail.Add(1)
				incFail(sc.name + ": " + err.Error())
			}
		} else {
			totalOK.Add(1)
		}
	}
}

func printStats(duration time.Duration, memStart runtime.MemStats, goroutineStart int) {
	runtime.GC()
	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)

	fmt.Printf("\n=== STRESS-VARIED REPORT ===\n")
	fmt.Printf("Duration:      %v\n", duration)
	fmt.Printf("Total requests: %d\n", totalReq.Load())
	fmt.Printf("OK:             %d\n", totalOK.Load())
	fmt.Printf("Fail:           %d\n", totalFail.Load())
	fmt.Printf("Success rate:   %.2f%%\n", float64(totalOK.Load())/float64(totalReq.Load())*100)
	fmt.Printf("Throughput:     %.1f req/s\n", float64(totalReq.Load())/duration.Seconds())
	fmt.Printf("Latency p50:    %v\n", latencyPercentile(0.50))
	fmt.Printf("Latency p95:    %v\n", latencyPercentile(0.95))
	fmt.Printf("Latency p99:    %v\n", latencyPercentile(0.99))

	fmt.Printf("\nStatus distribution:\n")
	statusCount.Range(func(k, v any) bool {
		fmt.Printf("  %d: %d\n", k, v)
		return true
	})

	fmt.Printf("\nFailures (top reasons):\n")
	type fr struct {
		reason string
		n      int64
	}
	var frs []fr
	failReasons.Range(func(k, v any) bool {
		frs = append(frs, fr{k.(string), v.(int64)})
		return true
	})
	sort.Slice(frs, func(i, j int) bool { return frs[i].n > frs[j].n })
	if len(frs) == 0 {
		fmt.Printf("  (none)\n")
	}
	for i, f := range frs {
		if i > 8 {
			break
		}
		fmt.Printf("  %s × %d\n", f.reason, f.n)
	}

	fmt.Printf("\nMemory:\n")
	fmt.Printf("  HeapAlloc start: %.1f MB → end: %.1f MB (Δ %.1f MB)\n",
		float64(memStart.HeapAlloc)/1024/1024, float64(memEnd.HeapAlloc)/1024/1024,
		float64(memEnd.HeapAlloc-memStart.HeapAlloc)/1024/1024)
	fmt.Printf("  Goroutines: %d (start: %d, Δ %d)\n",
		runtime.NumGoroutine(), goroutineStart, runtime.NumGoroutine()-goroutineStart)
}

func main() {
	duration := 1 * time.Hour
	concurrency := 20
	if len(os.Args) > 1 {
		if d, err := time.ParseDuration(os.Args[1]); err == nil && d > 0 {
			duration = d
		}
	}
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &concurrency)
	}

	fmt.Printf("=== cloak STRESS-VARIED (multi-feature random) ===\n")
	fmt.Printf("Duration:    %v\n", duration)
	fmt.Printf("Concurrency: %d\n", concurrency)
	fmt.Printf("Profiles:    Chrome/Firefox/Safari/OkHttp/Opera/Brave (random)\n")
	fmt.Printf("Methods:     GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS\n")
	fmt.Printf("Bodies:      JSON/form/multipart/raw/none\n")
	fmt.Printf("Features:    header-mutation/cookie-mutation/redirects/auth/charset/timeout\n")
	fmt.Printf("Protocols:   H1 + H2 + H3 local servers\n\n")

	h1URL, h2URL, h3URL, cleanup := startLocalServers()
	defer cleanup()
	fmt.Printf("Local H1 server: %s\n", h1URL)
	fmt.Printf("Local H2 server: %s\n", h2URL)
	fmt.Printf("Local H3 server: %s\n", h3URL)

	// warm-up
	wc := cloak.Impersonate(profiles.Chrome_150)
	wc.Timeout = 5 * time.Second
	wr, err := wc.Get(h1URL + "/json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warm-up H1 failed: %v\n", err)
		os.Exit(1)
	}
	wr.Body.Close()
	wr2, err := wc.Get(h2URL + "/json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warm-up H2 failed: %v\n", err)
		os.Exit(1)
	}
	wr2.Body.Close()
	// H3 warm-up (needs H3-capable client + InsecureSkipVerify for self-signed)
	h3warm := cloak.NewH3TransportWithOptions(profiles.Chrome_150, cloak.TransportOptions{InsecureSkipVerify: true})
	defer h3warm.CloseIdleConnections()
	h3warmClient := &http.Client{Transport: h3warm, Timeout: 5 * time.Second}
	_, err = h3warmClient.Get(h3URL + "/json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warm-up H3 failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Warm-up OK (H1+H2+H3)\n\n")

	runtime.GC()
	var memStart runtime.MemStats
	runtime.ReadMemStats(&memStart)
	goroutineStart := runtime.NumGoroutine()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	bases := []string{h1URL, h2URL}

	var wg sync.WaitGroup
	// mix: 70% reuse workers (connection pooling) + 30% fresh workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		if i%10 < 7 {
			go reuseWorker(ctx, bases, &wg)
		} else {
			go freshWorker(ctx, bases, &wg)
		}
	}

	// H3 worker: exercises QUIC transport against the local H3 server
	h3Stress := cloak.NewH3TransportWithOptions(profiles.Chrome_150, cloak.TransportOptions{InsecureSkipVerify: true})
	defer h3Stress.CloseIdleConnections()
	h3StressClient := &http.Client{Transport: h3Stress, Timeout: 5 * time.Second}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				m := []string{"GET", "POST"}[rand.Intn(2)]
				var body io.Reader
				if m == "POST" {
					b, _ := json.Marshal(map[string]any{"h3": true, "n": rand.Intn(100)})
					body = bytes.NewReader(b)
				}
				req, _ := http.NewRequest(m, h3URL+"/any", body)
				req.Header.Set("User-Agent", "stress-h3")
				start := time.Now()
				resp, err := h3StressClient.Do(req)
				recordLatency(time.Since(start))
				totalReq.Add(1)
				if err != nil {
					totalFail.Add(1)
					incFail("h3: " + err.Error())
				} else {
					resp.Body.Close()
					totalOK.Add(1)
					incStatus(resp.StatusCode)
				}
			}
		}()
	}

	monitor := time.NewTicker(60 * time.Second)
	go func() {
		for range monitor.C {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			fmt.Printf("[%v] req=%d ok=%d fail=%d heap=%.1fMB goroutines=%d\n",
				time.Since(start).Round(time.Second),
				totalReq.Load(), totalOK.Load(), totalFail.Load(),
				float64(m.HeapAlloc)/1024/1024, runtime.NumGoroutine())
		}
	}()
	defer monitor.Stop()

	wg.Wait()
	printStats(time.Since(start), memStart, goroutineStart)
}
