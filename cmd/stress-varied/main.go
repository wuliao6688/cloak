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
	// Status endpoints (for retry/error scenarios).
	mux.HandleFunc("/status/404", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/status/500", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	mux.HandleFunc("/status/429", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) })

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

// reqPool tracks all Request builder instances created by scenarios so
// the worker can CloseIdleConnections on them (prevents connection/goroutine
// leaks in long runs — each ImpersonateRequest creates a fresh client).
var reqPool = struct {
	sync.Mutex
	reqs []*cloak.Request
}{}

// newReq creates a Request builder and registers it for cleanup.
func newReq() *cloak.Request {
	r := cloak.ImpersonateRequest(randProfile())
	reqPool.Lock()
	reqPool.reqs = append(reqPool.reqs, r)
	reqPool.Unlock()
	return r
}

// closeAllReqs releases all pooled Request clients (called periodically
// by workers). This is the equivalent of client.CloseIdleConnections for
// the Request builder API.
func closeAllReqs() {
	reqPool.Lock()
	defer reqPool.Unlock()
	for _, r := range reqPool.reqs {
		r.CloseIdleConnections()
	}
	reqPool.reqs = reqPool.reqs[:0]
}

func randomScenario() scenario {
	n := rand.Intn(100)
	switch {
	// 1. Request builder chain: SetHeader+SetQueryParam+SetCookies+Get+String()
	case n < 12:
		return scenario{name: "req_chain_get", run: func(c *http.Client, base string) error {
			req := newReq().
				SetHeader("X-Trace", fmt.Sprintf("t%d", rand.Intn(1000))).
				SetHeaderNonCanonical("x-lower", "v").
				SetQueryParam("q", fmt.Sprintf("q%d", rand.Intn(100))).
				SetCookies(&http.Cookie{Name: "sess", Value: fmt.Sprintf("s%d", rand.Intn(1000))}).
				SetBaseURL(base)
			resp, err := req.Get("/echo")
			if err != nil { return err }
			s := resp.String()
			if !strings.Contains(s, "sess=") { return fmt.Errorf("cookie not echoed: %s", s[:min(80, len(s))]) }
			if resp.IsError() { return fmt.Errorf("IsError on 200") }
			if !resp.IsSuccess() { return fmt.Errorf("IsSuccess false") }
			if resp.ResultState() != cloak.ResultSuccess { return fmt.Errorf("ResultState=%d", resp.ResultState()) }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 2. SetBody(JSON bytes) + Post + UnmarshalJson
	case n < 24:
		return scenario{name: "req_bodyjson_post", run: func(c *http.Client, base string) error {
			var in = map[string]any{"ping": "pong", "n": rand.Intn(1000)}
			b, _ := json.Marshal(in)
			resp, err := newReq().
				SetBaseURL(base).
				SetBody(bytes.NewReader(b)).
				SetHeader("Content-Type", "application/json").
				Post("/json")
			if err != nil { return err }
			var out map[string]any
			if err := resp.UnmarshalJson(&out); err != nil { return err }
			if out["ping"] != "pong" { return fmt.Errorf("json round-trip mismatch") }
			if len(resp.BodyBytes()) == 0 { return fmt.Errorf("BodyBytes empty") }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 3. SetBodyString + content-type + Post + Bytes
	case n < 34:
		return scenario{name: "req_bodystring_post", run: func(c *http.Client, base string) error {
			resp, err := newReq().
				SetBaseURL(base).
				SetBodyString(fmt.Sprintf("raw-%d", rand.Intn(1000))).
				SetHeader("Content-Type", "text/plain").
				Post("/echo")
			if err != nil { return err }
			if len(resp.Bytes()) == 0 { return fmt.Errorf("Bytes empty") }
			if _, err := resp.ToString(); err != nil { return err }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 4. SetOrderedFormData (multipart-style) + Post
	case n < 42:
		return scenario{name: "req_orderedform_post", run: func(c *http.Client, base string) error {
			resp, err := newReq().
				SetBaseURL(base).
				SetOrderedFormData("name", fmt.Sprintf("u%d", rand.Intn(1000)), "k", "v").
				Post("/echo")
			if err != nil { return err }
			if resp.IsError() { return fmt.Errorf("form post failed") }
			_ = resp.String() // drain body — else setRequestCancel goroutine leaks
			incStatus(resp.StatusCode)
			return nil
		}}
	// 5. Auth variants
	case n < 50:
		return scenario{name: "req_auth", run: func(c *http.Client, base string) error {
			var req *cloak.Request
			if rand.Intn(2) == 0 {
				req = newReq().SetBasicAuth("user", "pass")
			} else {
				req = newReq().SetBearerAuthToken("tok-" + fmt.Sprint(rand.Intn(1000)))
			}
			resp, err := req.SetBaseURL(base).Get("/echo")
			if err != nil { return err }
			s := resp.String()
			if !strings.Contains(s, "Basic") && !strings.Contains(s, "Bearer") {
				return fmt.Errorf("auth header not echoed")
			}
			incStatus(resp.StatusCode)
			return nil
		}}
	// 6. OnRequest hook
	case n < 57:
		return scenario{name: "req_onrequest_hook", run: func(c *http.Client, base string) error {
			var hookCalled atomic.Int32
			resp, err := newReq().
				SetBaseURL(base).
				OnRequest(func(req *http.Request) error {
					hookCalled.Add(1)
					req.Header.Set("X-Hook", "yes")
					return nil
				}).
				Get("/echo")
			if err != nil { return err }
			if hookCalled.Load() == 0 { return fmt.Errorf("OnRequest hook not called") }
			_ = resp.String() // drain body — else setRequestCancel goroutine leaks
			incStatus(resp.StatusCode)
			return nil
		}}
	// 8. Retry
	case n < 65:
		return scenario{name: "req_retry", run: func(c *http.Client, base string) error {
			var attempts atomic.Int32
			resp, err := newReq().
				SetBaseURL(base).
				OnRequest(func(req *http.Request) error {
					attempts.Add(1)
					return nil
				}).
				SetRetry(2, func(resp *cloak.Response, err error) bool {
					return err == nil && resp != nil && resp.StatusCode == 429
				}, 10*time.Millisecond, 50*time.Millisecond).
				Get("/status/429")
			if err != nil { return err }
			_ = resp.String() // drain final 429 body — else setRequestCancel goroutine leaks
			if attempts.Load() < 2 { return fmt.Errorf("retry not triggered, attempts=%d", attempts.Load()) }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 9. Charset decode via Response.String()
	case n < 72:
		return scenario{name: "req_charset", run: func(c *http.Client, base string) error {
			resp, err := newReq().SetBaseURL(base).Get("/euckr")
			if err != nil { return err }
			s := resp.String()
			if !strings.Contains(s, "한국어") { return fmt.Errorf("EUC-KR decode failed: %q", s) }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 10. UnmarshalXml
	case n < 76:
		return scenario{name: "req_unmarshal_xml", run: func(c *http.Client, base string) error {
			resp, err := newReq().
				SetBaseURL(base).
				SetHeader("Accept", "application/xml").
				Get("/echo")
			if err != nil { return err }
			// JSON body into xml → error expected is fine; the call itself must not panic
			var v any
			_ = resp.UnmarshalXml(&v)
			incStatus(resp.StatusCode)
			return nil
		}}
	// 11. Low-level full method matrix (bypass Request builder)
	case n < 88:
		m := methodPool[rand.Intn(len(methodPool))]
		return scenario{name: "method_" + m, run: func(c *http.Client, base string) error {
			var body io.Reader
			if m == "POST" || m == "PUT" || m == "PATCH" {
				body = strings.NewReader(fmt.Sprintf("m%d", rand.Intn(100)))
			}
			req, err := http.NewRequest(m, base+"/any?q=v"+fmt.Sprint(rand.Intn(100)), body)
			if err != nil { return err }
			req.Header.Set("User-Agent", "stress-varied")
			resp, err := c.Do(req)
			if err != nil { return err }
			defer func() {
				io.Copy(io.Discard, resp.Body) // drain for connection reuse
				resp.Body.Close()
			}()
			if resp.StatusCode != 200 { return fmt.Errorf("status %d", resp.StatusCode) }
			incStatus(resp.StatusCode)
			return nil
		}}
	// 12. Redirect via Request builder
	default:
		return scenario{name: "req_redirect", run: func(c *http.Client, base string) error {
			resp, err := newReq().SetBaseURL(base).Get("/redirect")
			if err != nil { return err }
			if resp.StatusCode != 200 { return fmt.Errorf("redirect chain failed: %d", resp.StatusCode) }
			_ = resp.String() // drain body — else setRequestCancel goroutine leaks
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
	localReqs := 0
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
		// Release pooled Request clients created by this worker's
		// Request-builder scenarios (leak prevention).
		localReqs++
		if os.Getenv("NO_CLEAN") != "1" {
			closeAllReqs() // aggressive: every iteration, for leak isolation
		}
		if localReqs%100 == 0 {
			_ = localReqs
		}
	}
}

// worker that builds a FRESH Request each time (creation-path stress)
func freshWorker(ctx context.Context, bases []string, wg *sync.WaitGroup) {
	defer wg.Done()
	localReqs := 0
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
		// Fresh client per iteration: must close idle connections to avoid
		// keep-alive goroutine leaks (the real leak this stress found).
		if tr, ok := hc.Transport.(interface{ CloseIdleConnections() }); ok {
			tr.CloseIdleConnections()
		}
		// Release pooled Request clients too.
		localReqs++
		closeAllReqs() // aggressive: every iteration, for leak isolation
		if localReqs%100 == 0 {
			_ = localReqs
		}
	}
}

// dumpStacks enables goroutine signature dumping at exit (leak diagnosis).
var dumpStacks bool

func printStats(duration time.Duration, memStart runtime.MemStats, goroutineStart int) {
	runtime.GC()
	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)

	// If -dump flag set, print goroutine stack signatures before stats.
	if dumpStacks {
		buf := make([]byte, 64<<20) // 64MB buffer for full goroutine dump
		n := runtime.Stack(buf, true)
		lines := strings.Split(string(buf[:n]), "\n")
		sigCount := map[string]int{}
		for i := 0; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "goroutine ") {
				// First function frame = what the goroutine is blocked on
				first := ""
				for j := 1; j <= 6 && i+j < len(lines); j++ {
					line := strings.TrimSpace(lines[i+j])
					if strings.HasPrefix(line, "created by") {
						break
					}
					if line != "" {
						first = line
						break
					}
				}
				// normalize: strip args in parens
				if idx := strings.Index(first, "("); idx > 0 {
					first = first[:idx]
				}
				sigCount[first]++
			}
		}
		fmt.Printf("\n=== GOROUTINE SIGNATURES (top 15) ===\n")
		type sg struct {
			sig string
			n   int
		}
		var sgs []sg
		for s, n := range sigCount {
			sgs = append(sgs, sg{s, n})
		}
		sort.Slice(sgs, func(i, j int) bool { return sgs[i].n > sgs[j].n })
		totalG := 0
		for _, s := range sgs {
			totalG += s.n
		}
		fmt.Printf("  total goroutines: %d\n", totalG)
		for i, s := range sgs {
			if i >= 15 {
				break
			}
			fmt.Printf("  ×%d  %s\n", s.n, s.sig)
		}
	}

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
	if len(os.Args) > 3 && os.Args[3] == "-dump" {
		dumpStacks = true
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
	// Mix workers based on env flags for leak isolation:
	//   NO_H3=1   disable H3 workers
	//   NO_FRESH=1 disable fresh workers (only reuse)
	noH3 := os.Getenv("NO_H3") == "1"
	noFresh := os.Getenv("NO_FRESH") == "1"
	fmt.Printf("Workers: H3=%v Fresh=%v\n", !noH3, !noFresh)

	// mix: 70% reuse workers (connection pooling) + 30% fresh workers
	for i := 0; i < concurrency; i++ {
		if noFresh && i%10 >= 7 {
			continue
		}
		wg.Add(1)
		if i%10 < 7 {
			go reuseWorker(ctx, bases, &wg)
		} else {
			go freshWorker(ctx, bases, &wg)
		}
	}

	// H3 worker: exercises QUIC transport against the local H3 server
	if !noH3 {
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
						// MUST drain the body before Close — otherwise the
						// connection can't be reused and setRequestCancel
						// goroutines leak (classic net/http pitfall).
						io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
						totalOK.Add(1)
						incStatus(resp.StatusCode)
					}
				}
			}()
		}
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
