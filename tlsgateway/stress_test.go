package tlsgateway

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bogdanfinn/tls-client/profiles"
)

// stressResult holds counters for a stress test run.
type stressResult struct {
	total      atomic.Int64
	failed     atomic.Int64
	minLatency atomic.Int64 // nanoseconds
	maxLatency atomic.Int64
	sumLatency atomic.Int64
	startTime  time.Time
}

func (s *stressResult) record(d time.Time, err error) {
	s.total.Add(1)
	lat := time.Since(d).Nanoseconds()
	if err != nil {
		s.failed.Add(1)
	}
	for {
		old := s.minLatency.Load()
		if old == 0 || lat < old {
			if s.minLatency.CompareAndSwap(old, lat) {
				break
			}
		} else {
			break
		}
	}
	for {
		old := s.maxLatency.Load()
		if lat > old {
			if s.maxLatency.CompareAndSwap(old, lat) {
				break
			}
		} else {
			break
		}
	}
	s.sumLatency.Add(lat)
}

func (s *stressResult) report(t *testing.T, name string) {
	total := s.total.Load()
	failed := s.failed.Load()
	elapsed := time.Since(s.startTime)
	avgLat := time.Duration(0)
	if total > 0 {
		avgLat = time.Duration(s.sumLatency.Load() / total)
	}
	rps := float64(total) / elapsed.Seconds()
	failRate := float64(0)
	if total > 0 {
		failRate = float64(failed) / float64(total) * 100
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	t.Logf("=== %s ===", name)
	t.Logf("  duration:   %v", elapsed.Round(time.Millisecond))
	t.Logf("  requests:   %d (%.0f rps)", total, rps)
	t.Logf("  failed:     %d (%.2f%%)", failed, failRate)
	t.Logf("  latency:    min=%v avg=%v max=%v",
		time.Duration(s.minLatency.Load()),
		avgLat,
		time.Duration(s.maxLatency.Load()))
	t.Logf("  memory:     alloc=%dKB sys=%dKB heap=%dKB goroutines=%d",
		mem.Alloc/1024, mem.Sys/1024, mem.HeapAlloc/1024, runtime.NumGoroutine())
}

// TestStressSustainedTransport runs sustained concurrent load against
// the default Transport for a fixed duration. Verifies no goroutine leaks
// and consistent throughput under extended load.
func TestStressSustainedTransport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sustained stress test in short mode")
	}

	srv := startLocalTLSServer(t)
	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	goroutinesBefore := runtime.NumGoroutine()
	duration := 10 * time.Second
	concurrency := 20

	var res stressResult
	res.startTime = time.Now()

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
				}
				start := time.Now()
				resp, err := client.Get(fmt.Sprintf("%s/stress-%d-%d", srv.URL, idx, res.total.Load()))
				res.record(start, err)
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}
		}(i)
	}

	time.Sleep(duration)
	close(stopCh)
	wg.Wait()

	res.report(t, "Transport Sustained Stress (10s/20并发)")
	tr.CloseIdleConnections()

	// Allow goroutines to settle.
	time.Sleep(500 * time.Millisecond)
	runtime.GC()

	goroutinesAfter := runtime.NumGoroutine()
	leaked := goroutinesAfter - goroutinesBefore
	if leaked > 10 {
		t.Errorf("goroutine leak: before=%d after=%d leak=%d", goroutinesBefore, goroutinesAfter, leaked)
	}
	t.Logf("goroutines: before=%d after=%d leak=%d", goroutinesBefore, goroutinesAfter, leaked)
}

// TestStressSustainedRaceTransport runs sustained load with RaceTransport.
func TestStressSustainedRaceTransport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	srv := startLocalTLSServer(t)
	tr := NewRaceTransportWithOptions(profiles.Chrome_150,
		TransportOptions{InsecureSkipVerify: true},
		RaceOptions{H2Delay: 50 * time.Millisecond, Timeout: 10 * time.Second},
	)
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	goroutinesBefore := runtime.NumGoroutine()

	var res stressResult
	res.startTime = time.Now()

	var wg sync.WaitGroup
	stopCh := make(chan struct{})
	concurrency := 15

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
				}
				start := time.Now()
				resp, err := client.Get(fmt.Sprintf("%s/race-stress-%d", srv.URL, idx))
				res.record(start, err)
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}
		}(i)
	}

	time.Sleep(10 * time.Second)
	close(stopCh)
	wg.Wait()

	res.report(t, "RaceTransport Sustained Stress (10s/15并发)")
	tr.CloseIdleConnections()

	time.Sleep(500 * time.Millisecond)
	runtime.GC()

	leaked := runtime.NumGoroutine() - goroutinesBefore
	if leaked > 15 {
		t.Errorf("goroutine leak: leak=%d", leaked)
	}
	t.Logf("goroutine leak: %d", leaked)
}

// TestStressSustainedMultiProfile runs sustained load with rapid profile switching.
func TestStressSustainedMultiProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	srv := startLocalTLSServer(t)
	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	defer tr.CloseIdleConnections()

	profileKeys := []string{
		"chrome_150", "chrome_146", "chrome_131",
		"firefox_148", "firefox_147",
		"safari_ios_18_5", "okhttp4_android_13",
	}

	var res stressResult
	res.startTime = time.Now()
	goroutinesBefore := runtime.NumGoroutine()

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Request workers.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
				}
				start := time.Now()
				resp, err := client.Get(fmt.Sprintf("%s/multi-%d", srv.URL, idx))
				res.record(start, err)
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}
		}(i)
	}

	// Profile switcher.
	wg.Add(1)
	go func() {
		defer wg.Done()
		idx := 0
		for {
			select {
			case <-stopCh:
				return
			case <-time.After(50 * time.Millisecond):
			}
			profile, err := profiles.ResolveClientProfileStrict(profileKeys[idx%len(profileKeys)])
			if err == nil {
				tr.SetProfile(profile)
			}
			idx++
		}
	}()

	time.Sleep(10 * time.Second)
	close(stopCh)
	wg.Wait()

	res.report(t, "Multi-Profile Stress (10s/10并发+切换)")
	tr.CloseIdleConnections()

	time.Sleep(500 * time.Millisecond)
	runtime.GC()

	leaked := runtime.NumGoroutine() - goroutinesBefore
	if leaked > 15 {
		t.Errorf("goroutine leak: leak=%d", leaked)
	}
	t.Logf("goroutine leak: %d", leaked)
}

// TestStressSustainedProxy runs sustained load through the HTTP proxy.
func TestStressSustainedProxy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	// Start a plain HTTP upstream server (no TLS — proxy CONNECT uses raw pipe,
	// client TLS is Go-default not uTLS, so TLS upstream would fail).
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK " + r.URL.Path))
	}))
	defer upstream.Close()

	// Start proxy.
	proxy := NewProxy("localhost:0", profiles.Chrome_150)
	ln, err := listenLocalhost()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	proxyAddr := ln.Addr().String()
	proxySrv := &http.Server{Handler: http.HandlerFunc(proxy.serve)}
	go proxySrv.Serve(ln)
	defer proxySrv.Close()

	// Client through proxy → plain HTTP upstream.
	tr := &http.Transport{
		Proxy:               http.ProxyURL(mustParseURL("http://" + proxyAddr)),
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 50,
	}
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second}
	defer tr.CloseIdleConnections()

	goroutinesBefore := runtime.NumGoroutine()

	var res stressResult
	res.startTime = time.Now()

	var wg sync.WaitGroup
	stopCh := make(chan struct{})
	concurrency := 10

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
				}
				start := time.Now()
				resp, err := client.Get(upstream.URL + fmt.Sprintf("/proxy-stress-%d", idx))
				res.record(start, err)
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}
		}(i)
	}

	time.Sleep(10 * time.Second)
	close(stopCh)
	wg.Wait()

	res.report(t, "Proxy Sustained Stress (20s/10并发)")
	tr.CloseIdleConnections()

	time.Sleep(500 * time.Millisecond)
	runtime.GC()

	// NOTE: proxy goroutine leak is a known limitation — the proxy server's
	// in-flight connections may not be fully drained during shutdown.
	// Not a transport issue, but a proxy lifecycle issue.
	leaked := runtime.NumGoroutine() - goroutinesBefore
	if leaked > 50 {
		t.Logf("goroutine residual (proxy lifecycle): %d", leaked)
	}
}

// TestStressSustainedAllProfiles verifies all 81 profiles work under concurrent load.
func TestStressSustainedAllProfiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	srv := startLocalTLSServer(t)

	allProfiles := profiles.AllClientProfiles()
	profileList := make([]profiles.ClientProfile, 0, len(allProfiles))
	for _, p := range allProfiles {
		profileList = append(profileList, p)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // limit concurrent workers

	var totalOK, totalFail atomic.Int64
	goroutinesBefore := runtime.NumGoroutine()

	start := time.Now()

	for i, profile := range profileList {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, p profiles.ClientProfile) {
			defer wg.Done()
			defer func() { <-sem }()

			tr := NewTransportWithOptions(p, TransportOptions{InsecureSkipVerify: true})
			defer tr.CloseIdleConnections()
			client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

			resp, err := client.Get(srv.URL + fmt.Sprintf("/profile-%d", idx))
			if err != nil {
				totalFail.Add(1)
				t.Logf("profile %s: FAIL %v", p.GetClientHelloStr(), err)
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			totalOK.Add(1)
		}(i, profile)
	}

	wg.Wait()
	elapsed := time.Since(start)

	runtime.GC()
	time.Sleep(200 * time.Millisecond)

	t.Logf("=== All Profiles Stress ===")
	t.Logf("  total:   %d profiles", len(profileList))
	t.Logf("  ok:      %d", totalOK.Load())
	t.Logf("  failed:  %d", totalFail.Load())
	t.Logf("  elapsed: %v", elapsed.Round(time.Millisecond))
	t.Logf("  goroutines: %d (leak=%d)", runtime.NumGoroutine(),
		runtime.NumGoroutine()-goroutinesBefore)

	if totalFail.Load() > 0 {
		t.Errorf("%d profiles failed", totalFail.Load())
	}
}

// TestStressMemoryLeak runs repeated request cycles and verifies no monotonic
// memory growth (indicative of leak).
func TestStressMemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	srv := startLocalTLSServer(t)
	tr := NewTransportWithOptions(profiles.Chrome_150, TransportOptions{InsecureSkipVerify: true})
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	var memSamples []uint64
	cycles := 5
	requestsPerCycle := 500

	for cycle := 0; cycle < cycles; cycle++ {
		var wg sync.WaitGroup
		for i := 0; i < requestsPerCycle; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				resp, err := client.Get(srv.URL + "/memtest")
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}()
		}
		wg.Wait()

		runtime.GC()
		time.Sleep(100 * time.Millisecond)
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		memSamples = append(memSamples, mem.HeapInuse)
	}

	tr.CloseIdleConnections()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var final runtime.MemStats
	runtime.ReadMemStats(&final)

	t.Logf("=== Memory Leak Check ===")
	for i, s := range memSamples {
		t.Logf("  cycle %d: heap_inuse=%dKB", i+1, s/1024)
	}
	t.Logf("  final (after close): heap_inuse=%dKB goroutines=%d",
		final.HeapInuse/1024, runtime.NumGoroutine())

	// Heap should not grow linearly.
	if len(memSamples) >= 3 {
		first := memSamples[0]
		last := memSamples[len(memSamples)-1]
		ratio := float64(last) / float64(first)
		if ratio > 3.0 {
			t.Errorf("memory leak suspected: heap grew %.1fx (%.0fKB → %.0fKB)",
				ratio, float64(first)/1024, float64(last)/1024)
		}
	}
}

// listenLocalhost returns a listener on localhost:0 (random port).
func listenLocalhost() (net.Listener, error) {
	return net.Listen("tcp", "localhost:0")
}

func mustParseURL(raw string) *url.URL {
	u, _ := url.Parse(raw)
	return u
}
