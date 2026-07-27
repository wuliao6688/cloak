# tls-client → tlsgateway

轻量 TLS 指纹 HTTP 工具包。提供三种使用方式，从最轻到最全。

[![Go](https://img.shields.io/badge/Go-1.26.5-00ADD8?logo=go)](https://go.dev/)
[![CI](https://github.com/wuliao6688/tls-client/actions/workflows/ci.yml/badge.svg)](https://github.com/wuliao6688/tls-client/actions/workflows/ci.yml)

> TLS/HTTP 指纹只覆盖网络协议层，不等同于完整浏览器环境。

## 使用三种方式

### 🥇 方式一：tlsgateway Transport（最轻，~217行，零 fhttp 依赖）

Go 原生 `http.RoundTripper`，兼容 `net/http.Client`、`gin`、`echo`、`chi` 等所有标准生态。

```go
import (
    "net/http"
    "github.com/bogdanfinn/tls-client/profiles"
    "github.com/bogdanfinn/tls-client/tlsgateway"
)

tr := tlsgateway.NewTransport(profiles.Chrome_150)
client := &http.Client{Transport: tr}
resp, _ := client.Get("https://example.com")
```

| 能力 | tlsgateway |
|------|-----------|
| TLS ClientHello 指纹 | ✅ uTLS |
| 标准库兼容 | ✅ `http.RoundTripper` |
| HTTP/2 | ✅ 标准库 H2 |
| 代理 | ✅ `http.Transport.Proxy` |
| 画像热加载 | ✅ `profiles.Watcher` |
| 代码量 | **186 行** |

### 🥈 tlsgateway-proxy（本地代理）— 312 行

本地 HTTP/HTTPS 代理。**任何语言都能用**——只设 `HTTPS_PROXY`。

```bash
go run ./cmd/tlsgateway-proxy -addr :8080 -profile chrome_150

# Python
HTTPS_PROXY=http://localhost:8080 python3 -c "import requests; requests.get('https://example.com')"

# Node.js
HTTPS_PROXY=http://localhost:8080 node -e "require('https').get('https://example.com')"

# curl
HTTPS_PROXY=http://localhost:8080 curl https://example.com

# 热切换画像（无需重启）
curl -X POST "http://localhost:8080/reload?profile=firefox_148"
```

### 🥉 完整 tls-client（Fork 版本）— ~8000 行

**仅在需要以下功能时使用**：HTTP/3、定制 H2 SETTINGS/优先级帧、Protocol Racing、C shared library（CFFI 多语言集成）。

```go
import (
    http "github.com/bogdanfinn/fhttp"
    tls_client "github.com/bogdanfinn/tls-client"
)

client, _ := tls_client.NewHttpClient(tls_client.NewNoopLogger(),
    tls_client.WithClientProfile(profiles.Chrome_150),
)
resp, _ := client.Get("https://example.com")
```

## 对比一览

| 维度 | tlsgateway | tlsgateway-proxy | 完整 tls-client |
|------|:---:|:---:|:---:|
| 代码量 | 186 行 | +382 行 | ~8000 行 |
| 标准库兼容 | ✅ | N/A | ❌ fhttp fork |
| TLS 指纹 | ✅ | ✅ | ✅ |
| H2 SETTINGS/优先级 | ❌ | ❌ | ✅ |
| HTTP/3 | ❌ | ❌ | ✅ |
| Protocol Racing | ❌ | ❌ | ✅ |
| 跨语言 | ❌ (Go only) | ✅ HTTP_PROXY | ✅ C shared library |
| 画像热加载 | ✅ | ✅ | ✅ |

## 客户端画像

81 个预置画像：Chrome 103→150、Firefox 102→148、Safari、Brave、Opera、OkHttp 及移动客户端。

```go
// 严格解析（推荐）
profile, err := profiles.ResolveClientProfileStrict("chrome_150")

// 随机画像（排除业务定制/实验性画像）
key, profile := profiles.ResolveClientProfileWithKey("random")
```

画像可从 JSON 加载，支持热重载——Chrome 151 发布时更新 JSON 即可，无需重编译：

```bash
go run ./cmd/export-profiles -o profiles.json   # 导出 81 个画像
# 编辑 profiles.json 添加 chrome_151 ...
```

```go
watcher, _ := profiles.NewWatcher("profiles.json", 30*time.Second)
go watcher.Start(ctx)
for range watcher.Updates() {
    log.Println("profiles reloaded")
}
```

## 项目结构

```
├─ tlsgateway/           轻量 Transport + 本地代理（主推荐）
│  ├─ transport.go       186行，http.RoundTripper
│  ├─ proxy.go           312行，HTTP/HTTPS 正向代理
│  └─ *_test.go          14个 race-clean 测试
├─ cmd/
│  ├─ tlsgateway-proxy/  代理 CLI 入口
│  └─ export-profiles/   画像 JSON 导出工具
├─ profiles/             画像定义、解析器、JSON 加载、热加载
├─ client.go             完整 tls-client（Fork 版本）
├─ roundtripper.go       TLS 握手与 Transport 缓存
├─ racer.go              HTTP/3 Protocol Racing
├─ cffi_src/             C shared library 支持
├─ stress_test.go        高并发压力测试
├─ tests/                在线集成测试（integration tag）
└─ profiles.json         导出的 81 个画像 JSON
```

## 来源

本项目源自 [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client)，持续吸收其画像更新。Go module 路径保留为 `github.com/bogdanfinn/tls-client` 以维持兼容性。

tlsgateway Transport 和代理是本项目的独立创新——基于 uTLS + 标准 `net/http.Transport`，无 fork 依赖。

许可证见 [`LICENSE`](./LICENSE)。
