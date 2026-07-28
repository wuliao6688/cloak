## 项目架构

完整文档：[README.md](README.md) | [快速开始](docs/quick-start.md) | [TLS 指纹](docs/tls-fingerprint.md) | [HTTP 指纹](docs/http-fingerprint.md) | [API](docs/api.md) | [架构](docs/architecture.md)

### 项目结构

```
tls-client/
├── tlsgateway/          ← 主代码 (17 文件)
│   ├── impersonate.go   ← API 入口: Impersonate/DevMode/SelfCheck
│   ├── request.go       ← Request 流式 builder
│   ├── response.go      ← Response + TraceInfo + auto-unmarshal
│   ├── fingerprint.go   ← H2指纹/浏览器常量/Header排序/Multipart
│   ├── header.go        ← HeaderRoundTripper
│   ├── middleware.go     ← TransportMiddleware
│   ├── retry.go         ← 条件重试 + 指数退避
│   ├── dump.go          ← DumpOptions 维度控制
│   ├── transport_h2.go  ← 默认 Transport (H2+H1降级)
│   ├── transport_fprint.go ← FingerprintTransport (fork http2)
│   ├── transport_race.go← RaceTransport
│   ├── proxy.go         ← 正向代理
│   └── *_test.go        ← 测试
├── internal/
│   ├── header/          ← SortKeyValues/HeaderOrderKey
│   ├── http2/           ← x/net/http2 fork (Settings/StreamID)
│   └── httpcommon/      ← httpcommon (shared with http2)
├── profiles/            ← 81 预置画像 + 测试 + 验证
├── cmd/
│   ├── verify-fingerprints/ ← 12平台验证工具
│   ├── stress/              ← 压力测试工具
│   └── tlsgateway-proxy/    ← 代理服务
├── docs/                ← 文档
├── README.md
├── AGENTS.md
├── go.mod / go.sum
└── LICENSE
```

### 依赖

- `github.com/bogdanfinn/utls` (Tor 团队) — TLS 指纹
- `golang.org/x/net` (Go 官方) — HTTP/2
- 标准库

**不依赖**：bogdanfinn/fhttp、bogdanfinn/quic-go-utls、任何第三方 fork。

### 压力测试基线

```
10 分钟 / 20 并发 / Akamai+Cloudflare+tls.peet.ws
  成功率: 100%
  Akamai 200: 100%
  内存: 3.6-3.9 MB (无增长)
  Goroutines: 102 (无泄漏)
  结论: ✅ 稳定可靠
```

### 指纹验证基线

| 层 | 通过率 |
|----|--------|
| TLS APIs (tls.peet.ws/browserleaks/browserscan) | 100% (15/15) |
| WAF/CDN (Cloudflare/Imperva/F5/HCaptcha/reCAPTCHA/Sannysoft) | 100% (30/30) |
| Akamai (带 HeaderRoundTripper) | 100% |
| DataDome | ❌ JS 引擎必需 |

### 编码规范

- error 不以标点结尾，不以大写开头 (Go 惯例)
- map/slice/pointer Getter 返回防御性副本
- 测试用 `-race` 运行
- 画像新增 → 全平台验证 → 更新本文件基线

### 质量门禁

```bash
go test -race -count=1 ./...          # 全部测试
go run ./cmd/verify-fingerprints      # 全平台指纹验证
go run ./cmd/stress 10m               # 10分钟压力测试
```
