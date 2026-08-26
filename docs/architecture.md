# 架构设计

## 目录结构

```
tls-client/
├── tlsgateway/              ← 核心库（全部公开 API）
│   ├── impersonate.go       ← 入口：Impersonate/ChainBuilder/DevMode
│   ├── request.go           ← 请求级 builder（req 风格）
│   ├── response.go          ← Response + ResultState + TraceInfo
│   ├── transport_h2.go      ← Transport（一体式：TLS + H2/H1 + 浏览器头）
│   ├── transport_h3.go      ← H3Transport（纯 QUIC RoundTripper）
│   ├── transport_h3race.go  ← H3RaceTransport（H3 vs H2 赛跑）
│   ├── transport_race.go    ← RaceTransport（H2 vs H1 赛跑，无 H3）
│   ├── transport_fprint.go  ← FingerprintTransport（fork http2 底层）
│   ├── fingerprint.go       ← H2 指纹 / 浏览器常量 / 头排序
│   ├── header.go            ← HeaderRoundTripper（浏览器头注入）
│   ├── middleware.go        ← 中间件链
│   ├── retry.go             ← 条件重试 + 指数退避
│   ├── dump.go              ← 请求/响应 dump
│   ├── proxy.go             ← 本地正向代理
│   └── *_test.go            ← 测试
├── profiles/                ← 77 画像
│   ├── internal_browser_profiles.go   ← Chrome/Safari/Firefox/Opera/Brave
│   ├── contributed_browser_profiles.go ← 更多浏览器版本
│   ├── internal_custom_profiles.go    ← OkHttp/移动端
│   ├── contributed_custom_profiles.go ← Nike/Zalando/Mesh 等
│   ├── profiles.go          ← 注册表 + NewClientProfile
│   ├── resolver.go          ← key 解析
│   ├── metadata.go          ← 画像元数据
│   └── h2settings.go        ← H2 SETTINGS 定义
├── internal/
│   ├── http2/               ← x/net/http2 fork（SETTINGS/StreamID/Priority 定制）
│   ├── httpcommon/          ← 共享 HTTP 公共代码
│   └── header/              ← 头排序辅助
├── third_party/
│   └── quic-go-utls/        ← quic-go fork（UQUICClient 指纹注入 + H3）
├── cmd/
│   ├── verify-fingerprints/ ← 14 平台指纹验证工具
│   ├── stress/              ← 压力测试工具
│   ├── tlsgateway-proxy/    ← 代理服务（画像热加载）
│   └── export-profiles/     ← 画像导出工具
├── docs/                    ← 文档
├── README.md
└── go.mod
```

## 分层架构

```
┌────────────────────────────────────────────────┐
│ 公开 API 层                                      │
│  Impersonate / ImpersonateH3 / ImpersonateChain │
│  ImpersonateRequest / DevMode / NewProxy        │
├────────────────────────────────────────────────┤
│ 请求层 (req 风格)                                │
│  Request builder → Response / ResultState       │
│  重试 / 中间件 / dump / 反序列化 / 输出落盘       │
├────────────────────────────────────────────────┤
│ 传输层 (RoundTripper)                           │
│  Transport (H2+H1) │ H3Transport │ H3RaceTransport│
│  RaceTransport    │ FingerprintTransport        │
├────────────────────────────────────────────────┤
│ 指纹层                                           │
│  uTLS (TLS) │ internal/http2 (H2) │ quic-go-utls (H3)│
├────────────────────────────────────────────────┤
│ 数据层                                           │
│  profiles: 77 画像 (TLS Spec + H2 + H3 字段)    │
└────────────────────────────────────────────────┘
```

## 核心设计决策

### 1. 一体式 Transport（TLS + HTTP 头 + 协议协商）

`Transport` 一个对象完成所有事：
- uTLS 握手（按画像构造 ClientHello）
- HTTP/2 vs HTTP/1.1 自动协商（按域名缓存）
- 浏览器头自动注入（UA / Accept / Sec-CH-UA / Accept-Language）

**为什么**：早期架构用 `HeaderRoundTripper` 包装，但用户经常忘记包装导致
头不一致。一体式保证"选了 Chrome 画像就一定发 Chrome 的头"。

```go
type Transport struct {
	profile       profiles.ClientProfile // 当前画像
	h2            *http2.Transport       // fork 的 H2
	h1            *http.Transport        // H1.1（ALPN 强制 http/1.1）
	h1p           *http.Transport        // 纯明文 HTTP
	browserHeaders map[string]string     // 画像浏览器头
	protocolCache  sync.Map              // host → h2/h1 缓存
}
```

### 2. 零 fork 依赖（默认构建）

默认构建只用 `uTLS` + `golang.org/x/net` + 标准库。internal/http2 是
x/net 的 fork（定制 SETTINGS/StreamID/Priority），但 API 兼容。

H3 是例外：QUIC 是全新协议栈，必须引入 quic-go。放在 `third_party/`
（go.mod replace 指向本地 fork），核心 QUIC 层不依赖 fhttp。

### 3. H3 指纹注入（UQUICClient）

上游 bogdanfinn 的 H3 用标准 `QUICClient`——QUIC TLS 层是 Go 默认指纹。
本项目 fork quic-go-utls 后改用 `UQUICClient + HelloCustom + ApplyPreset`，
把浏览器 ClientHello 注入 QUIC TLS 握手（详见 [HTTP/3 指南](h3.md)）。

### 4. 协议赛跑（Chrome Happy Eyeballs）

```
首次请求 → H3 + H2 并行 → 先成功者胜 → 按域名缓存
后续请求 → 直接用缓存协议
H3 失败 → 缓存 h2 → 之后全走 H2
非幂等方法 → 绝不并发（只走缓存协议）
```

### 5. 画像注册表

```go
// profiles/profiles.go
var canonicalTLSClients = map[string]ClientProfile{  // 不可变注册表
	"chrome_150":  Chrome_150,
	"firefox_147": Firefox_147,
	// ... 77 个
}
```

- 注册表**不可变**（防止外部修改影响全局解析）
- `AllClientProfiles()` 返回防御性副本
- Getter 返回 `maps.Clone` / `slices.Clone` 副本（防并发写）

## 关键流程

### 请求生命周期

```
Request.Get(url)
  → buildURL()（baseURL + pathParams + queryParams）
  → 注入请求头（SetHeader 优先 > 通用头 > 浏览器默认）
  → 中间件链（OnRequest）
  → Transport.RoundTrip
      → 协议缓存命中？→ 直接走 h2/h1
      → 未命中 → 尝试 h2 → 失败降级 h1 → 缓存
      → TLS 握手（uTLS 按画像）→ H2 SETTINGS 注入
  → 响应处理（gzip 解压 / 反序列化 / dump / 输出）
  → OnResponse 中间件
  → Response（含 TraceInfo 七点计时）
```

### H3 racing 流程

```
H3RaceTransport.RoundTrip
  → https? + 幂等方法?（否则走 base）
  → 协议缓存命中 → 走 h3 / h2
  → 未命中 → race():
      H3 goroutine（req.Clone）
      H2 goroutine（req.Clone, H2Delay 后启动）
      谁先成功 → 缓存协议 → 返回
      都失败 → 返回 H2 错误
```

## 依赖

| 依赖 | 用途 | 是否 fork |
|---|---|---|
| `github.com/bogdanfinn/utls` | TLS 指纹 | 否（Tor 团队） |
| `golang.org/x/net` | HTTP/2（源） | 否 |
| `internal/http2` | H2 指纹定制 | ✅ fork（API 兼容） |
| `third_party/quic-go-utls` | QUIC + H3 | ✅ fork（UQUICClient 注入） |
| `github.com/quic-go/qpack` | H3 QPACK | 否 |

## 与上游的架构差异

| | 本项目 | 上游 bogdanfinn |
|---|---|---|
| H3 QUIC TLS 指纹 | ✅ UQUICClient 注入 | ❌ Go 默认 |
| HTTP 库 | 标准 net/http | fhttp（net/http fork） |
| 浏览器头 | Transport 内置 | HeaderRoundTripper 包装 |
| 代理 | ✅ 内置 | ❌ |
| API 风格 | req 风格链式 | 配置对象 |
