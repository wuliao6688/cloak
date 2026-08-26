# tlsgateway — Go HTTP 客户端指纹伪装库

[![Go Version](https://img.shields.io/badge/Go-1.22+-blue)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

tlsgateway 是一个 Go HTTP 客户端库，专注于 **TLS/HTTP 指纹伪装**——让 Go 程序的 HTTP 请求看起来像真实的浏览器。

> **核心理念**：仅使用 uTLS (Tor 团队) + x/net/http2 (Go 官方) + 标准库。零外部 fork 依赖。

## 快速开始

```go
// 一行代码伪装 Chrome，通过 Akamai
client := tlsgateway.Impersonate(profiles.Chrome_150)
resp, _ := client.Get("https://www.akamai.com/") // → 200

// DevMode: 伪装 + 调试 一行搞定
tlsgateway.DevMode(profiles.Chrome_150).Get("https://api.example.com")

// 自动反序列化 + 重试 + dump
var user User
tlsgateway.ImpersonateRequest(profiles.Chrome_150).
    SetSuccessResult(&user).
    SetBearerAuthToken("secret").
    SetRetry(3, tlsgateway.RetryOnServerError, 1*time.Second, 10*time.Second).
    SetDump(tlsgateway.DefaultDumpOptions()).
    Get("https://api.example.com/user")
```

## 压力测试验证

```
10 分钟高并发压测 (20 并发，Akamai/Cloudflare/tls.peet.ws):
  ├── 总请求: 14,000+
  ├── 成功率: 100%
  ├── Akamai 200: 100%
  ├── 内存: 3.6-3.9 MB (无增长)
  ├── Goroutines: 102 (无泄漏)
  └── 结论: ✅ 稳定可靠，零泄漏
```

## 为什么选 tlsgateway？

| | 标准 `net/http` | tlsgateway |
|---|---|---|
| TLS 指纹 | Go 默认（易检测） | Chrome/Firefox/Safari (uTLS) |
| H2 SETTINGS 定制 | ❌ | ✅ Chrome/Firefox/Safari |
| 浏览器头 | 手动 | 自动注入 (UA/Accept/Sec-CH-UA) |
| Akamai | ❌ 403 | ✅ 200 |
| Cloudflare | ❌ 1020 | ✅ |

## 验证结果

| 平台 | 结果 |
|------|------|
| tls.peet.ws | ✅ JA3/JA4 正确 |
| cloudflare.com | ✅ TLSv1.3+HTTP/2 |
| imperva.com | ✅ |
| f5.com | ✅ |
| hcaptcha.com | ✅ |
| recaptcha-demo | ✅ |
| sannysoft.com | ✅ PASS |
| **akamai.com** | ✅ **200** |

TLS 层 100%，6 大 WAF 全通过。

## 功能

- **TLS 指纹**: 77 画像，覆盖 Chrome/Firefox/Safari/Brave/Opera/移动端
- **HTTP/2 指纹**: SETTINGS/StreamID/ConnectionFlow/Priority 帧
- **HTTP/3 指纹** 🔥: QUIC TLS ClientHello 注入 (UQUICClient) + H3 SETTINGS/GREASE/Priority/伪头顺序——**超越上游**（上游 QUIC TLS 层是 Go 默认指纹）
- **H2 vs H3 协议赛跑**: `ImpersonateH3()` / `ChainBuilder.EnableH3()`，Chrome 式 Happy Eyeballs + 域名级协议缓存，无 QUIC 自动降级 H2/H1
- **浏览器头**: 自动注入 UA/Accept/Sec-CH-UA/Accept-Language
- **Header 排序**: Chrome/Firefox/Safari 规范顺序
- **自动反序列化**: `SetSuccessResult(&v)` → 响应自动 JSON unmarshal
- **自动重试**: 条件重试 + 指数退避
- **DevMode**: 一行调试
- **TraceInfo**: DNS/TCP/TLS/FirstByte 等 7 点计时
- **TransportMiddleware**: 链式包装
- **正向代理**: HTTP/HTTPS CONNECT 隧道

## 快速使用 H3

```go
// H3 racing: 优先 HTTP/3,无 QUIC 自动降级 H2/H1
client := tlsgateway.ImpersonateH3(profiles.Chrome_150)
resp, _ := client.Get("https://www.cloudflare.com/cdn-cgi/trace") // http=http/3

// 或链式配置
client := tlsgateway.ImpersonateChain(profiles.Chrome_150).EnableH3().Build()
```

## 文档

| 文档 | 内容 |
|------|------|
| [快速开始](docs/quick-start.md) | 5 分钟上手 |
| [TLS 指纹](docs/tls-fingerprint.md) | 原理/使用/验证/FAQ |
| [HTTP 指纹](docs/http-fingerprint.md) | H2 完整定制/Chrome vs Firefox vs Safari |
| [画像论证](docs/profile-audit.md) | 77 画像数据来源与正确性论证 |
| [H3 优化方案](docs/optimization-h3.md) | H3 能力补全方案论证 |
| [API 速览](docs/api.md) | 所有 API |
| [架构设计](docs/architecture.md) | 分层/双Transport/设计决策 |

## 安装

```bash
go get github.com/bogdanfinn/tls-client
```

## License

MIT
