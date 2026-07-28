package tls_client

import (
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/fhttp/httptest"
	"github.com/bogdanfinn/tls-client/profiles"
)

// =============================================================================
// Sustained High-Concurrency Stress Test (10 minutes)
// Detects slow leaks: goroutine creep, memory creep, connection leaks
// =============================================================================

func TestStressSustained10Min(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 10-minute sustained stress test in short mode")
	}

	const (
		duration       = 10 * time.Minute
		concurrency    = 50
		reportInterval = 30 * time.Second
	)

	// Start local HTTPS test server
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		body := make([]byte, 2048)
		for i := range body {
			body[i] = byte(i % 256)
		}
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	var (
		totalRequests  atomic.Int64
		totalFailures  atomic.Int64
		totalBytes     atomic.Int64
		totalLatencyUs atomic.Int64
		peakGoroutines atomic.Int64
		peakHeapAlloc  atomic.Int64
		peakHeapObj    atomic.Int64
	)

	type memSample struct {
		ts         time.Time
		alloc      uint64
		heapObj    uint64
		goroutines int
	}
	var memSamples []memSample
	var sampleMu sync.Mutex

	// Monitor goroutine: periodic sampling
	stopCh := make(chan struct{})
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(reportInterval)
		defer ticker.Stop()

		startTime := time.Now()
		var startMem runtime.MemStats
		runtime.ReadMemStats(&startMem)

		t.Logf("[初始] Alloc=%s Sys=%s HeapObj=%d Goroutines=%d",
			formatBytes(startMem.Alloc), formatBytes(startMem.Sys),
			startMem.HeapObjects, runtime.NumGoroutine())

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				var mem runtime.MemStats
				runtime.ReadMemStats(&mem)
				n := runtime.NumGoroutine()

				if int64(n) > peakGoroutines.Load() {
					peakGoroutines.Store(int64(n))
				}
				if int64(mem.Alloc) > peakHeapAlloc.Load() {
					peakHeapAlloc.Store(int64(mem.Alloc))
				}
				if int64(mem.HeapObjects) > peakHeapObj.Load() {
					peakHeapObj.Store(int64(mem.HeapObjects))
				}

				sampleMu.Lock()
				memSamples = append(memSamples, memSample{
					ts:         time.Now(),
					alloc:      mem.Alloc,
					heapObj:    mem.HeapObjects,
					goroutines: n,
				})
				sampleMu.Unlock()

				reqs := totalRequests.Load()
				avgLat := time.Duration(0)
				if reqs > 0 {
					avgLat = time.Duration(totalLatencyUs.Load()/reqs) * time.Microsecond
				}
				elapsed := time.Since(startTime).Round(time.Second)

				t.Logf("[%v] Req=%d Fail=%d Lat=%v G=%d PeakG=%d Alloc=%s HeapObj=%d",
					elapsed, reqs, totalFailures.Load(), avgLat,
					n, peakGoroutines.Load(),
					formatBytes(mem.Alloc), mem.HeapObjects)
			}
		}
	}()

	// Create a pool of clients for reuse
	clientPool := make([]HttpClient, concurrency)
	for i := 0; i < concurrency; i++ {
		client, err := NewHttpClient(NewNoopLogger(),
			WithClientProfile(profiles.Chrome_150),
			WithTimeoutSeconds(30),
			WithForceHttp1(),
			WithInsecureSkipVerify(),
			WithCookieJar(NewCookieJar()),
		)
		if err != nil {
			t.Fatalf("failed to create client %d: %v", i, err)
		}
		clientPool[i] = client
	}

	// Cleanup all clients
	defer func() {
		for _, c := range clientPool {
			if c != nil {
				c.CloseIdleConnections()
			}
		}
	}()

	// Worker goroutines
	deadline := time.After(duration)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			client := clientPool[idx]
			targetURL := ts.URL
			for {
				select {
				case <-deadline:
					return
				default:
				}
				start := time.Now()
				req, err := http.NewRequest(http.MethodGet, targetURL, nil)
				if err != nil {
					totalFailures.Add(1)
					continue
				}
				resp, err := client.Do(req)
				totalLatencyUs.Add(int64(time.Since(start)))
				totalRequests.Add(1)
				if err != nil {
					totalFailures.Add(1)
					continue
				}
				n, _ := io.Copy(io.Discard, resp.Body)
				totalBytes.Add(n)
				resp.Body.Close()

				// Also exercise profile resolution occasionally
				if totalRequests.Load()%1000 == 0 {
					profiles.ResolveClientProfileWithKey("random")
				}
			}
		}(i)
	}
	wg.Wait()
	close(stopCh)
	<-monitorDone

	// ========== Final Diagnostics ==========

	// Force multiple GCs and wait for finalizers
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	time.Sleep(300 * time.Millisecond)

	var finalMem runtime.MemStats
	runtime.ReadMemStats(&finalMem)
	finalGoroutines := runtime.NumGoroutine()

	// Report final summary
	reqs := totalRequests.Load()
	fails := totalFailures.Load()
	totalB := totalBytes.Load()
	avgLat := time.Duration(0)
	if reqs > 0 {
		avgLat = time.Duration(totalLatencyUs.Load()/reqs) * time.Microsecond
	}
	throughput := float64(reqs) / duration.Seconds()

	t.Logf("")
	t.Logf("========== 持续压力测试结果 ==========")
	t.Logf("时长:         %v", duration)
	t.Logf("并发:         %d", concurrency)
	t.Logf("总请求:       %d", reqs)
	t.Logf("总失败:       %d (%.4f%%)", fails, float64(fails)/float64(reqs)*100)
	t.Logf("总流量:       %s", formatBytes(uint64(totalB)))
	t.Logf("请求速率:     %.0f req/s", throughput)
	t.Logf("平均延迟:     %v", avgLat)
	t.Logf("峰值 Goroutines: %d", peakGoroutines.Load())
	t.Logf("最终 Goroutines: %d", finalGoroutines)
	t.Logf("峰值 HeapAlloc:  %s", formatBytes(uint64(peakHeapAlloc.Load())))
	t.Logf("最终 HeapAlloc:  %s", formatBytes(finalMem.Alloc))
	t.Logf("最终 HeapObjects: %d", finalMem.HeapObjects)
	t.Logf("最终 HeapInuse:   %s", formatBytes(finalMem.HeapInuse))
	t.Logf("总 GC 次数:       %d", finalMem.NumGC)
	t.Logf("总 GC 暂停:       %v", time.Duration(finalMem.PauseTotalNs))
	t.Logf("=======================================")

	// Failure rate check
	if reqs > 0 && float64(fails)/float64(reqs) > 0.01 {
		t.Errorf("⚠️  失败率过高: %.2f%% (> 1%%)", float64(fails)/float64(reqs)*100)
	}

	// Goroutine leak check
	if finalGoroutines > 200 {
		t.Errorf("⚠️  疑似 Goroutine 泄漏: 最终 %d (> 200)", finalGoroutines)
	}

	// Memory leak check: compare first and last samples
	if len(memSamples) >= 2 {
		first := memSamples[0]
		last := memSamples[len(memSamples)-1]
		heapGrowth := int64(last.heapObj) - int64(first.heapObj)

		// Re-check after GC
		gcHeapObj := int64(finalMem.HeapObjects)
		t.Logf("内存趋势: 首HeapObj=%d 末HeapObj=%d GC后HeapObj=%d 增长=%d",
			first.heapObj, last.heapObj, gcHeapObj, gcHeapObj-int64(first.heapObj))

		if gcHeapObj > 0 && gcHeapObj-int64(first.heapObj) > 50000 {
			t.Errorf("⚠️  疑似内存泄漏: GC后 HeapObjects 增长 %d (> 50000)",
				gcHeapObj-int64(first.heapObj))
		}
		_ = heapGrowth
	}

	// Heap fragmentation check
	if finalMem.HeapInuse > 0 && finalMem.HeapIdle > 5*finalMem.HeapInuse {
		t.Logf("⚠️  Heap 碎片化: Inuse=%s Idle=%s (Idle/Inuse=%.1fx)",
			formatBytes(finalMem.HeapInuse), formatBytes(finalMem.HeapIdle),
			float64(finalMem.HeapIdle)/float64(finalMem.HeapInuse))
	}
}

// formatBytes converts uint64 bytes to a human-readable string.
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}


// =============================================================================
