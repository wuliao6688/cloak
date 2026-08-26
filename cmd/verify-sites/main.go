// Command verify-sites tests cloak against real-world customer target
// sites protected by various WAFs (Cloudflare/Akamai/DataDome/Imperva...).
// Reports HTTP status + WAF fingerprint headers so we can argue which
// sites pass detection and which don't.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wuliao6688/cloak"
	"github.com/wuliao6688/cloak/profiles"
)

type siteResult struct {
	URL      string
	Status   int
	Proto    string
	CF       string // cf-ray
	Server   string
	WAF      string // detected WAF
	BodyHint string // challenge markers
	Err      string
}

// Customer-facing sites grouped by the WAF protecting them.
// (Verified via headers during the run; grouping is prior knowledge.)
var sites = []struct {
	URL  string
	WAF  string
	Note string
}{
	// Cloudflare
	{"https://www.nike.com/", "Cloudflare", "电商头部站"},
	{"https://www.reddit.com/", "Cloudflare", "社区头部站"},
	{"https://discord.com/login", "Cloudflare", "社交"},
	{"https://www.zara.com/", "Cloudflare", "电商"},
	{"https://steamcommunity.com/", "Cloudflare", "游戏社区"},
	{"https://www.cloudflare.com/cdn-cgi/trace", "Cloudflare", "CF 自测(对照)"},
	// Akamai
	{"https://www.akamai.com/", "Akamai", "Akamai 官网(对照)"},
	{"https://www.dell.com/", "Akamai", "电商"},
	{"https://www.adobe.com/", "Akamai", "软件"},
	{"https://www.oracle.com/", "Akamai", "企业"},
	{"https://www.ebay.com/", "Akamai", "电商"},
	// DataDome
	{"https://www.tripadvisor.com/", "DataDome", "点评"},
	{"https://www.dailymotion.com/", "DataDome", "视频"},
	// Imperva
	{"https://www.blizzard.com/", "Imperva", "游戏"},
	// PerimeterX / HUMAN
	{"https://www.urbanoutfitters.com/", "PerimeterX", "电商"},
	// 国内平台
	{"https://www.zhihu.com/", "国内", "知乎"},
	{"https://www.baidu.com/", "国内", "百度"},
	{"https://www.toutiao.com/", "国内", "头条"},
}

func main() {
	profileKey := flag.String("profile", "chrome_150", "profile key")
	timeout := flag.Duration("timeout", 15*time.Second, "per-site timeout")
	flag.Parse()

	profile, err := profiles.ResolveClientProfileStrict(*profileKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "profile:", err)
		os.Exit(1)
	}

	client := cloak.Impersonate(profile)
	client.Timeout = *timeout

	var wg sync.WaitGroup
	results := make([]siteResult, len(sites))
	sem := make(chan struct{}, 4)

	for i, s := range sites {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, s struct{ URL, WAF, Note string }) {
			defer wg.Done()
			defer func() { <-sem }()
			r := siteResult{URL: s.URL, WAF: s.WAF}
			start := time.Now()
			resp, err := client.Get(s.URL)
			if resp != nil {
				r.Proto = resp.Proto
			}
			if resp != nil {
				r.Status = resp.StatusCode
				r.CF = resp.Header.Get("cf-ray")
				r.Server = resp.Header.Get("Server")
				// WAF-specific headers
				for k := range resp.Header {
					lk := strings.ToLower(k)
					if strings.Contains(lk, "datadome") {
						r.WAF = "DataDome"
					}
					if strings.Contains(lk, "akamai") || strings.Contains(lk, "x-akamai") {
						r.WAF = "Akamai"
					}
					if strings.Contains(lk, "perimeterx") || strings.Contains(lk, "px-") {
						r.WAF = "PerimeterX"
					}
					if strings.Contains(lk, "imperva") || strings.Contains(lk, "x-iinfo") {
						r.WAF = "Imperva"
					}
				}
				// challenge markers in body
				if resp.Body != nil {
					b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
					body := string(b)
					resp.Body.Close()
					switch {
					case strings.Contains(body, "cf_chl") || strings.Contains(body, "challenge-platform"):
						r.BodyHint = "CF 挑战"
					case strings.Contains(body, "datadome"):
						r.BodyHint = "DD 挑战"
					case strings.Contains(body, "Just a moment") || strings.Contains(body, "cf-browser-verification"):
						r.BodyHint = "CF 验证页"
					case strings.Contains(body, "Access Denied") || strings.Contains(body, "Request blocked"):
						r.BodyHint = "拒绝页"
					case strings.Contains(body, "verify"):
						r.BodyHint = "验证页"
					default:
						r.BodyHint = ""
					}
				}
			}
			if err != nil {
				r.Err = err.Error()
			}
			r.URL = s.URL
			r.WAF = s.WAF + "(" + r.WAF + ")"
			_ = start
			results[i] = r
		}(i, s)
	}
	wg.Wait()

	fmt.Printf("%-45s %-6s %-6s %-22s %-12s %s\n", "SITE", "STATUS", "PROTO", "WAF(检测)", "SERVER", "HINT")
	fmt.Println(strings.Repeat("─", 130))
	for _, r := range results {
		status := fmt.Sprintf("%d", r.Status)
		mark := "❌"
		if r.Status == 200 || r.Status == 301 || r.Status == 302 {
			mark = "✅"
		}
		hint := r.BodyHint
		if r.Err != "" {
			hint = r.Err
			mark = "⚠️"
		}
		fmt.Printf("%-45s %s%-6s %-6s %-22s %-12s %s\n", r.URL, mark, status, r.Proto, r.WAF, r.Server, hint)
	}
}
