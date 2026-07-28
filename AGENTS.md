## 项目架构

完整文档：[README.md](README.md) | [快速开始](docs/quick-start.md) | [TLS 指纹](docs/tls-fingerprint.md) | [HTTP 指纹](docs/http-fingerprint.md) | [API](docs/api.md) | [架构](docs/architecture.md)

### 项目结构

```
tls-client/
├── tlsgateway/          ← 主代码 (17 文件)
│   ├── impersonate.go   ← Impersonate/DevMode/SelfCheck/ChainBuilder
│   ├── request.go       ← Request 流式 builder + hooks
│   ├── response.go      ← Response + ResultState + TraceInfo
│   ├── fingerprint.go   ← H2指纹/8浏览器常量/Header排序/Multipart
│   ├── transport_h2.go  ← Transport (一体式: TLS + HTTP头)
│   ├── transport_fprint.go ← FingerprintTransport (fork http2)
│   ├── header.go        ← HeaderRoundTripper (deprecated, 保留向后兼容)
│   ├── middleware.go     ← TransportMiddleware
│   ├── retry.go         ← 条件重试 + 指数退避
│   ├── dump.go          ← DumpOptions
│   ├── proxy.go         ← 正向代理
│   └── *_test.go        ← 测试
├── internal/
│   ├── header/          ← SortKeyValues/HeaderOrderKey
│   ├── http2/           ← x/net/http2 fork (Settings/StreamID/Priority)
│   └── httpcommon/      ← httpcommon
├── profiles/            ← 81 预置画像 + 测试
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

### 设计原则

- **一体式架构**: Transport 内置 TLS + HTTP 头，无需外部 HeaderRoundTripper
- **req 靠拢**: API 设计、中间件、钩子、ResultState 全部借鉴 req
- **零 fork 依赖**: 默认构建仅 uTLS + x/net + stdlib
- **自动降级**: H2 → H1.1

### 压力测试基线

```
10 分钟 / 20 并发 / Akamai+Cloudflare+tls.peet.ws
  成功率: 99.8%+
  Akamai 200: 100%
  内存: 3.6-4.0 MB (无增长)
  Goroutines: 102 (无泄漏)
  结论: ✅ 稳定可靠
```

### 指纹验证基线

| 层 | 通过率 |
|----|--------|
| TLS APIs (tls.peet.ws/browserleaks/browserscan) | 100% |
| WAF/CDN (Cloudflare/Imperva/F5/HCaptcha/reCAPTCHA/Sannysoft) | 100% |
| Akamai (Transport 一体式) | ✅ 200 |
| DataDome | ❌ JS 引擎必需 |

### 编码规范

- error 不以标点结尾，不以大写开头
- map/slice/pointer Getter 返回防御性副本
- 测试用 -race 运行
- 画像新增 → 全平台验证 → 更新基线

### 质量门禁

```bash
go test -race -count=1 ./...          # 全部测试
go run ./cmd/verify-fingerprints      # 全平台指纹验证
go run ./cmd/stress 10m               # 10分钟压力测试
```
