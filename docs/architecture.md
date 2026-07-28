# 架构设计

## 依赖策略

**零外部 fork**：默认构建仅用 uTLS (Tor 团队) + x/net/http2 (Go 官方) + 标准库。

不依赖 bogdanfinn/fhttp、bogdanfinn/quic-go-utls 等上游库。

## 分层架构

```
┌─────────────────────────────────┐
│  tlsgateway/                    │  ← 用户层
│  ├── impersonate.go   API入口    │
│  ├── request.go       Request   │
│  ├── response.go      Response  │
│  ├── retry.go         重试       │
│  ├── dump.go          调试       │
│  ├── fingerprint.go   指纹数据   │
│  ├── header.go        头像注入   │
│  ├── middleware.go     中间件    │
│  ├── transport_h2.go  Transport │
│  ├── transport_fprint.go FPrint │
│  └── proxy.go         代理      │
├─────────────────────────────────┤
│  internal/                      │  ← 基础设施层
│  ├── http2/         x/net fork  │
│  ├── httpcommon/    httpcommon  │
│  └── header/        SortKeyVals │
├─────────────────────────────────┤
│  profiles/                      │  ← 画像层
│  └── 81 预置画像                 │
└─────────────────────────────────┘
```

## 双 Transport 设计

| | 默认 Transport | FingerprintTransport |
|---|---|---|
| HTTP/2 实现 | x/net/http2 (Go 官方) | internal/http2 (fork) |
| H2 SETTINGS | Go 默认 | Chrome/Firefox 浏览器值 |
| Stream ID | 1 | 3 (Chrome) / 1 (Firefox) |
| ConnectionFlow | Go 默认 | 浏览器值 |
| Priority 帧 | 无 | Firefox 6 个 |
| 依赖 | 零 fork | fork x/net/http2 |

选择建议：
- **默认 Transport** — 大多数场景，TLS + HTTP 头伪装已足够
- **FingerprintTransport** — 严格 H2 指纹检测的场景

## 关键设计决策

### 1. HeaderRoundTripper（解决 Akamai）

Akamai 在 TLS 层通过后，还检查 HTTP 头。单纯 uTLS 过 TLS 但返回 403。

**方案**：`HeaderRoundTripper` 按画像注入浏览器整套头部（UA/Accept/Sec-Ch-Ua 等）。
Akamai: 403 → 200。

### 2. __header_order__ trick（借鉴 req）

H2 伪头顺序控制通过特殊 HTTP header 传递：

```
req.Header.Set("__pseudo_header_order__", ":method,:authority,:scheme,:path")
```

H2 transport 编码时读取并排序后从 wire 剥离。不污染公开 API。

### 3. BrowserFingerprint（一站式指纹）

`BrowserFingerprint(name)` 根据 profile 名自动返回对应浏览器的全部指纹维度：
Settings、StreamID、ConnectionFlow、HeaderPriority、PriorityFrames、PseudoHeaderOrder、HeaderOrder、Headers。

### 4. H2→H1 降级

对不支持 H2 的服务器（或 Akamai 拒绝 H2 的情况），自动降级到 H1.1。

### 5. 指数退避重试

```go
// 第 1 次重试: 1s
// 第 2 次重试: 2s
// 第 3 次重试: 4s
// max: 10s
```

## 文件清单

| 文件 | 行数 | 职责 |
|------|------|------|
| impersonate.go | ~300 | Impersonate/ImpersonateChain/ImpersonateRequest/SelfCheck |
| transport_h2.go | ~300 | 默认 Transport (x/net/http2) |
| transport_fprint.go | ~230 | FingerprintTransport (fork http2) |
| fingerprint.go | ~380 | H2指纹类型/浏览器常量/Header排序/Multipart |
| request.go | ~170 | Request 流式 builder |
| response.go | ~110 | Response + TraceInfo + auto-unmarshal |
| header.go | ~100 | HeaderRoundTripper |
| middleware.go | ~120 | TransportMiddleware |
| retry.go | ~30 | 条件重试 + 退避 |
| dump.go | ~95 | DumpOptions 维度控制 |
| proxy.go | ~250 | HTTP/HTTPS 正向代理 |
| internal/http2/ | ~3400 | x/net/http2 fork |
| internal/httpcommon/ | ~1200 | httpcommon |
| internal/header/ | ~70 | SortKeyValues/HeaderOrderKey |
| profiles/ | ~2000 | 81 预置画像 |
