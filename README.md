# tls-client

Go 语言的浏览器指纹 HTTP 客户端 —— **TLS 1.3 + HTTP/2 + HTTP/3 三层指纹全覆盖**，
让服务端无法区分你的请求和真实浏览器。

```
go get github.com/bogdanfinn/tls-client
```

## 为什么用它

| 能力 | tls-client | 上游 bogdanfinn | curl_cffi | imroc/req |
|---|---|---|---|---|
| TLS 指纹（JA3/JA4） | ✅ 77 画像 | ✅ | ✅ | ⚠️ 有限 |
| HTTP/2 指纹（SETTINGS/伪头/优先级） | ✅ 完整定制 | ✅ | ✅ | ❌ |
| **HTTP/3 指纹（QUIC TLS + H3 SETTINGS）** | ✅ **全链路** | ⚠️ QUIC TLS 层是 Go 默认 | ⚠️ 仅部分 | ❌ |
| **H2 vs H3 协议赛跑** | ✅ Chrome 式 | ✅ | ✅ | ❌ |
| 零 fork 依赖（标准 net/http） | ✅ | ❌ 依赖 fhttp | ❌ C 扩展 | ✅ |
| 移动端画像 | ✅ OkHttp/Nike/Zalando | ❌ | ❌ | ❌ |
| 内置正向代理 | ✅ | ❌ | ❌ | ❌ |

**关键差异**：上游和 curl_cffi 的 H3 只做了 HTTP/3 应用层指纹（SETTINGS 帧），
QUIC TLS 握手层仍是 Go/curl 默认指纹——**tls-client 通过 UQUICClient 把浏览器
ClientHello 注入 QUIC TLS 握手**，是唯一三层指纹全对齐的实现。

## 快速上手

```go
package main

import (
	"fmt"

	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/bogdanfinn/tls-client/tlsgateway"
)

func main() {
	// 普通模式：TLS + HTTP/2 指纹
	client := tlsgateway.Impersonate(profiles.Chrome_150)
	resp, _ := client.Get("https://www.akamai.com/")
	fmt.Println(resp.StatusCode) // 200

	// H3 模式：HTTP/3 (QUIC) 优先，自动降级 H2
	client3 := tlsgateway.ImpersonateH3(profiles.Chrome_150)
	resp, _ = client3.Get("https://www.cloudflare.com/cdn-cgi/trace")
	// 响应头里会看到 http=http/3
}
```

## 文档

| 文档 | 内容 |
|---|---|
| [概览](docs/overview.md) | 项目是什么、能力矩阵、设计哲学 |
| [快速开始](docs/quick-start.md) | 5 分钟上手：请求/反序列化/重试/调试 |
| [API 参考](docs/api.md) | 全部公开 API 速查 |
| [指纹体系](docs/fingerprint.md) | TLS/HTTP2/HTTP3 三层指纹原理与实现 |
| [HTTP/3 指南](docs/h3.md) | QUIC 能力详解、H3 racing、与上游差异 |
| [画像体系](docs/profiles.md) | 77 个预置画像清单、数据来源论证 |
| [验证矩阵](docs/verification.md) | 14 平台验证 + 客户场景测试 + 修复记录 |
| [架构设计](docs/architecture.md) | 分层、设计决策、目录结构 |

## 验证基线

```
go test -race -count=1 ./...        # 全部测试（含并发/H3/画像有效性）
go run ./cmd/verify-fingerprints    # 14 平台指纹验证
go run ./cmd/stress 10m             # 10 分钟压力测试
```

- 12 个 TLS/WAF 平台 + 2 个 HTTP/3 平台
- Akamai / Cloudflare / Imperva / F5 / DataDome / HCaptcha / reCAPTCHA / Sannysoft
- 100 并发稳定性、goroutine 零泄漏、内存 3.6-4.0 MB 无增长

## License

MIT
