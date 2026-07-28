# tlsgateway — Go HTTP 客户端指纹伪装库

[![Go Version](https://img.shields.io/badge/Go-1.22+-blue)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

tlsgateway 是一个零外部 fork 依赖的 Go HTTP 客户端库，专注于**TLS/HTTP 指纹伪装**——让 Go 程序的 HTTP 请求看起来像真实的浏览器。

> **核心理念**：仅使用 uTLS (Tor 团队维护) + x/net/http2 (Go 官方) + 标准库。不依赖 bogdanfinn/fhttp、quic-go-utls 等上游库。

## 快速开始

```go
package main

import (
    "fmt"
    "github.com/bogdanfinn/tls-client/profiles"
    "github.com/bogdanfinn/tls-client/tlsgateway"
)

func main() {
    // 一行代码伪装 Chrome 浏览器
    resp, _ := tlsgateway.Impersonate(profiles.Chrome_150).
        Get("https://www.akamai.com/")
    fmt.Println(resp.StatusCode) // 200
}
```

## 为什么选择 tlsgateway？

### 对比标准 Go HTTP

| | 标准 `net/http` | tlsgateway |
|---|---|---|
| TLS 指纹 | Go 默认（易被检测） | Chrome/Firefox/Safari (uTLS) |
| H2 SETTINGS | Go 默认 | Chrome/Firefox/Safari 默认值 |
| 浏览器头 | 手动设置 | 自动注入 (UA/Accept/Sec-CH-UA) |
| Header 排序 | map 随机顺序 | 浏览器规范顺序 |
| Akamai | ❌ 403 | ✅ 200 |
| Cloudflare | ❌ 1020 | ✅ 200 |

### 对比 req

| | req | tlsgateway |
|---|---|---|
| uTLS | ✅ | ✅ |
| H2 SETTINGS 定制 | ✅ | ✅ |
| 零 fork | ❌ (fork x/net/http2) | ✅ (可选 fork) |
| Akamai | ✅ | ✅ |
| 文档中文 | ✅ | ✅ |

## 功能特性

- **TLS 指纹伪装**：81 个预置画像，覆盖 Chrome/Firefox/Safari/Brave/Opera
- **HTTP/2 指纹完整定制**：SETTINGS 帧、Stream ID、ConnectionFlow、Priority 帧
- **浏览器头自动注入**：User-Agent、Accept、Sec-CH-UA、Accept-Language 等
- **Header 排序**：Chrome/Firefox/Safari 规范顺序
- **自动反序列化**：SetSuccessResult/SetErrorResult，响应自动 JSON unmarshal
- **自动重试**：条件重试 + 指数退避
- **结构化调试**：DumpOptions 8 维控制
- **TraceInfo**：7 点计时 (DNS/TCP/TLS/FirstByte/Response/Total/ConnReused)
- **双 Transport**：默认 Transport (x/net/http2) + FingerprintTransport (fork http2)
- **TransportMiddleware**：链式包装 Debug/UA/Tracing
- **正向代理**：HTTP/HTTPS CONNECT 隧道

## 文档

| 文档 | 内容 |
|------|------|
| [快速开始](docs/quick-start.md) | 5 分钟上手 |
| [TLS 指纹](docs/tls-fingerprint.md) | TLS 指纹原理与使用 |
| [HTTP 指纹](docs/http-fingerprint.md) | HTTP/2 指纹完整定制 |
| [API 速览](docs/api.md) | 所有 API 一览 |
| [架构设计](docs/architecture.md) | 项目架构与设计决策 |

## 验证结果

| 平台 | 结果 |
|------|------|
| tls.peet.ws | ✅ JA3/JA4 |
| browserleaks.com | ✅ JA3/JA3N |
| cloudflare.com | ✅ TLSv1.3+HTTP/2 |
| imperva.com | ✅ |
| f5.com | ✅ |
| **akamai.com** | ✅ **200** |
| hcaptcha.com | ✅ |
| recaptcha-demo | ✅ |
| sannysoft.com | ✅ PASS |

TLS 层：**9/9 核心平台 100% 通过**

## 安装

```bash
go get github.com/bogdanfinn/tls-client
```

## License

MIT
