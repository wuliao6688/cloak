package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/bogdanfinn/tls-client/tlsgateway"
)

var (
	completed  atomic.Int64
	failed     atomic.Int64
	akamaiPass atomic.Int64
)

func main() {
	duration := 10 * time.Minute
	concurrency := 20

	if len(os.Args) > 1 {
		d, _ := time.ParseDuration(os.Args[1])
		if d > 0 { duration = d }
	}

	fmt.Printf("=== TLS Client High-Concurrency Stress Test ===\n")
	fmt.Printf("Duration:    %v\n", duration)
	fmt.Printf("Concurrency: %d\n", concurrency)
	fmt.Printf("Profile:     Chrome-150\n")
	fmt.Printf("Targets:     Akamai + Cloudflare + tls.peet.ws\n\n")

	urls := []string{
		"https://www.akamai.com/",
		"https://www.cloudflare.com/",
		"https://tls.peet.ws/api/all",
	}

	// Initial memory
	runtime.GC()
	var memStart runtime.MemStats
	runtime.ReadMemStats(&memStart)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// Launch workers
	for i := 0; i < concurrency; i++ {
		go worker(ctx, urls)
	}

	// Monitor
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	printStats := func() {
		elapsed := time.Since(start)
		c := completed.Load()
		f := failed.Load()
		a := akamaiPass.Load()
		total := c + f

		runtime.GC()
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		goroutines := runtime.NumGoroutine()
		fmt.Printf("[%5.0fs]  reqs=%d  ok=%d  fail=%d  akamai200=%d  mem=%.1fMB(heap)  goroutines=%d\n",
			elapsed.Seconds(), total, c, f, a,
			float64(mem.HeapInuse)/1024/1024, goroutines,
		)
	}

	go func() {
		for range ticker.C {
			printStats()
		}
	}()

	// Wait for completion
	<-ctx.Done()
	cancel()

	// Final stats
	runtime.GC()
	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)

	elapsed := time.Since(start)
	c := completed.Load()
	f := failed.Load()
	a := akamaiPass.Load()

	fmt.Println()
	fmt.Println("=== FINAL REPORT ===")
	fmt.Printf("Duration:     %v\n", elapsed.Round(time.Second))
	fmt.Printf("Concurrency:  %d\n", concurrency)
	fmt.Printf("Total Reqs:   %d\n", c+f)
	fmt.Printf("Success:      %d (%.1f%%)\n", c, float64(c)/float64(c+f)*100)
	fmt.Printf("Failed:       %d\n", f)
	fmt.Printf("Akamai 200:   %d\n", a)
	fmt.Printf("Req/s:        %.1f\n", float64(c+f)/elapsed.Seconds())
	fmt.Printf("Goroutines:   %d\n", runtime.NumGoroutine())
	fmt.Printf("HeapStart:    %.1f MB\n", float64(memStart.HeapInuse)/1024/1024)
	fmt.Printf("HeapEnd:      %.1f MB\n", float64(memEnd.HeapInuse)/1024/1024)
	fmt.Printf("HeapDelta:    %+.1f MB\n", float64(memEnd.HeapInuse-memStart.HeapInuse)/1024/1024)

	if memEnd.HeapInuse > memStart.HeapInuse*2 {
		fmt.Println("\n⚠️  HEAP DOUBLED — possible memory leak!")
	} else {
		fmt.Println("\n✅ No significant memory growth")
	}

	goroutines := runtime.NumGoroutine()
	if goroutines > concurrency*3 {
		fmt.Printf("⚠️  Goroutine leak: %d (> %d)\n", goroutines, concurrency*3)
	} else {
		fmt.Printf("✅ Goroutine count stable: %d\n", goroutines)
	}
}

func worker(ctx context.Context, urls []string) {
	client := tlsgateway.Impersonate(profiles.Chrome_150)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			url := urls[0]
			resp, err := client.Get(url)
			if err != nil {
				failed.Add(1)
				continue
			}
			resp.Body.Close()
			completed.Add(1)
			if resp.StatusCode == 200 {
				akamaiPass.Add(1)
			}
		}
	}
}
