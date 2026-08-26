## 项目架构

完整文档：[README.md](README.md) | [概览](docs/overview.md) | [快速开始](docs/quick-start.md) | [API](docs/api.md) | [指纹体系](docs/fingerprint.md) | [H3 指南](docs/h3.md) | [画像体系](docs/profiles.md) | [验证矩阵](docs/verification.md) | [架构](docs/architecture.md)

### 项目结构

```
tls-client/
├── tlsgateway/              ← 核心库（全部公开 API）
│   ├── impersonate.go       ← 入口：Impersonate/ImpersonateH3/ChainBuilder/DevMode
│   ├── request.go           ← Request 流式 builder + hooks
│   ├── response.go          ← Response + ResultState + TraceInfo
│   ├── transport_h2.go      ← Transport（一体式：TLS + H2/H1 + 浏览器头）
│   ├── transport_h3.go      ← H3Transport（纯 QUIC RoundTripper）
│   ├── transport_h3race.go  ← H3RaceTransport（H3 vs H2 赛跑）
│   ├── transport_race.go    ← RaceTransport（H2 vs H1 赛跑）
│   ├── transport_fprint.go  ← FingerprintTransport（fork http2）
│   ├── fingerprint.go       ← H2指纹/浏览器常量/Header排序/Multipart
│   ├── header.go            ← HeaderRoundTripper（浏览器头注入，兼容层）
│   ├── middleware.go        ← 中间件链
│   ├── retry.go             ← 条件重试 + 指数退避
│   ├── dump.go              ← 请求/响应 dump
│   ├── proxy.go             ← 正向代理
│   └── *_test.go            ← 测试（含 H3/客户场景/stress）
├── internal/
│   ├── http2/               ← x/net/http2 fork（SETTINGS/StreamID/Priority 定制）
│   ├── httpcommon/          ← 共享 HTTP 公共代码
│   └── header/              ← SortKeyValues/HeaderOrderKey
├── profiles/                ← 77 预置画像 + 测试
│   ├── internal_browser_profiles.go   ← Chrome/Safari/Firefox/Opera/Brave
│   ├── contributed_browser_profiles.go ← 更多浏览器版本
│   ├── internal_custom_profiles.go    ← OkHttp/移动端
│   ├── contributed_custom_profiles.go ← Nike/Zalando/Mesh 等
│   ├── profiles.go          ← 注册表 + NewClientProfile
│   ├── resolver.go          ← key 解析
│   └── metadata.go          ← 画像元数据
├── third_party/
│   └── quic-go-utls/        ← quic-go fork（UQUICClient 指纹注入 + H3）
├── cmd/
│   ├── verify-fingerprints/ ← 14 平台指纹验证工具
│   ├── stress/              ← 压力测试工具
│   ├── tlsgateway-proxy/    ← 代理服务（画像热加载）
│   └── export-profiles/     ← 画像导出工具
├── docs/                    ← 文档（9 篇）
├── README.md
├── AGENTS.md
├── go.mod / go.sum
└── LICENSE
```

### 依赖

- `github.com/bogdanfinn/utls` — TLS 指纹（Tor 团队）
- `golang.org/x/net` — HTTP/2（源）
- `internal/http2` — x/net/http2 fork（H2 指纹定制，API 兼容）
- `third_party/quic-go-utls` — quic-go fork（UQUICClient 注入 QUIC TLS 指纹）
- `github.com/quic-go/qpack` — H3 QPACK
- 标准库

### 设计原则

- **一体式架构**: Transport 内置 TLS + HTTP 头 + H2/H1 协商
- **req 靠拢**: API 设计、中间件、钩子、ResultState 全部借鉴 req
- **零 fork 依赖**: 默认构建仅 uTLS + x/net + stdlib（H3 的 quic-go 在 third_party）
- **三层指纹**: TLS (uTLS) + H2 (internal/http2) + H3 (UQUICClient)
- **自动降级**: H3 → H2 → H1.1

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
| HTTP/3 (http3.is/quic.browserleaks.com) | 需无 UDP 拦截环境 |
| DataDome | ❌ JS 引擎必需 |

### 编码规范

- error 不以标点结尾，不以大写开头
- map/slice/pointer Getter 返回防御性副本
- 测试用 -race 运行
- 画像新增 → 全平台验证 → 更新基线
- third_party 不做 gofmt 检查（上游代码）

### 质量门禁

```bash
go build ./...                        # 编译
go vet ./...                          # 静态检查
go test -race -count=1 ./...          # 全部测试
go run ./cmd/verify-fingerprints      # 全平台指纹验证
go run ./cmd/stress 10m               # 10分钟压力测试
```

### 关键陷阱

- `ClientProfile` 的 Getter 必须返回副本（防并发写）
- H3 的 `IsSet()` 在 utls 里是**反的**（空值返回 true），判断用 `!IsSet()` 表示"已设置"
- H3 racing 必须 `req.Clone()` 再分发给 goroutine（否则 data race）
- `Request.SetInsecureSkipVerify` 通过 `InsecureSkipVerrifier` 接口穿透包装链
- Safari/移动画像无 H3 数据是**有意的**（见 docs/profiles.md §H3 覆盖）
