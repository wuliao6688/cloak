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
	// ── 电商 ──
	{"https://www.nike.com/", "Akamai", "电商"},
	{"https://www.shein.com/", "Akamai", "跨境电商"},
	{"https://www.temu.com/", "Cloudflare", "跨境电商"},
	{"https://www.zara.com/", "Akamai", "电商"},
	{"https://www.hm.com/", "Akamai", "电商"},
	{"https://www.amazon.com/", "Akamai", "电商"},
	{"https://www.ebay.com/", "AkamaiBM", "电商"},
	{"https://www.walmart.com/", "Akamai", "电商"},
	{"https://www.bestbuy.com/", "Akamai", "电商"},
	{"https://www.target.com/", "Akamai", "电商"},
	// ── 社交 ──
	{"https://www.reddit.com/", "Cloudflare", "社区"},
	{"https://discord.com/login", "Cloudflare", "社交"},
	{"https://www.instagram.com/", "Meta", "社交"},
	{"https://x.com/", "Meta/CF", "社交"},
	{"https://www.facebook.com/", "Meta", "社交"},
	{"https://www.pinterest.com/", "Cloudflare", "社交"},
	{"https://www.linkedin.com/", "Cloudflare", "招聘社交"},
	// ── 内容/媒体 ──
	{"https://www.youtube.com/", "Google", "视频"},
	{"https://www.tiktok.com/", "Cloudflare", "短视频"},
	{"https://www.quora.com/", "Cloudflare", "问答"},
	{"https://www.medium.com/", "Cloudflare", "写作"},
	{"https://www.twitch.tv/", "Cloudflare", "直播"},
	{"https://www.dailymotion.com/", "DataDome", "视频"},
	{"https://www.nytimes.com/", "Cloudflare", "新闻"},
	{"https://www.bbc.com/", "Cloudflare", "新闻"},
	// ── 游戏 ──
	{"https://store.epicgames.com/", "Cloudflare", "游戏商店"},
	{"https://steamcommunity.com/", "Cloudflare", "游戏社区"},
	// ── 旅游 ──
	{"https://www.booking.com/", "Akamai", "旅游"},
	{"https://www.expedia.com/", "Cloudflare", "旅游"},
	{"https://www.airbnb.com/", "Cloudflare", "民宿"},
	{"https://www.tripadvisor.com/", "DataDome", "点评"},
	// ── 招聘 ──
	{"https://www.indeed.com/", "Cloudflare", "招聘"},
	{"https://www.glassdoor.com/", "Cloudflare", "招聘点评"},
	// ── 企业/数据 ──
	{"https://www.glassdoor.com/", "Cloudflare", "企业"},
	{"https://www.oracle.com/", "Akamai", "企业"},
	// ── 对照站 ──
	{"https://www.cloudflare.com/cdn-cgi/trace", "Cloudflare", "CF 对照"},
	{"https://www.akamai.com/", "Akamai", "Akamai 对照"},
	{"https://tls.peet.ws/api/all", "TLS测试", "指纹对照"},
	// ── 国内电商 ──
	{"https://www.taobao.com/", "国内", "淘宝"},
	{"https://www.jd.com/", "国内", "京东"},
	{"https://www.pinduoduo.com/", "国内", "拼多多"},
	{"https://www.1688.com/", "国内", "1688"},
	// ── 国内内容/社交 ──
	{"https://www.zhihu.com/", "国内", "知乎"},
	{"https://www.xiaohongshu.com/", "国内", "小红书"},
	{"https://www.douyin.com/", "国内", "抖音"},
	{"https://weibo.com/", "国内", "微博"},
	{"https://www.bilibili.com/", "国内", "B站"},
	{"https://www.baidu.com/", "国内", "百度"},
	{"https://www.toutiao.com/", "国内", "头条"},
	// ── 国内招聘/企业 ──
	{"https://www.zhipin.com/", "国内", "BOSS直聘"},
	{"https://www.tianyancha.com/", "国内", "天眼查"},
	{"https://www.qcc.com/", "国内", "企查查"},
	// ── 国内旅游 ──
	{"https://www.ctrip.com/", "国内", "携程"},
	{"https://hotels.ctrip.com/", "国内", "携程酒店"},
	{"https://www.qunar.com/", "国内", "去哪儿"},
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
