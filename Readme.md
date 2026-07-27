# tls-client

面向 Go 与多语言调用场景的 TLS 指纹 HTTP 客户端。

[![Go](https://img.shields.io/badge/Go-1.26.5-00ADD8?logo=go)](https://go.dev/)
[![CI](https://github.com/wuliao6688/tls-client/actions/workflows/ci.yml/badge.svg)](https://github.com/wuliao6688/tls-client/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-BSD--style-blue.svg)](./LICENSE)

`tls-client` 可以控制 TLS ClientHello、HTTP/2 设置与帧顺序、HTTP/3 参数、Header 顺序等协议细节，用于复现浏览器或移动客户端在网络协议层的行为。项目同时提供 Go API、WebSocket 支持和 C shared library，适合需要稳定连接复用、动态代理、画像治理及多语言集成的应用。

本仓库由 [wuliao6688](https://github.com/wuliao6688) 独立维护，拥有自己的功能基线、测试矩阵和维护规范。项目源自 [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client)，并持续吸收其协议与画像更新；当前仍保留原 Go module 路径，以维持源码和 API 兼容性。两者的发布内容不应视为完全相同。

> TLS/HTTP 指纹只覆盖网络协议层，不等同于完整浏览器环境，也不能单独绕过所有自动化检测。

## 核心能力

| 分类 | 能力 |
| --- | --- |
| 网络协议 | HTTP/1.1、HTTP/2、HTTP/3、ALPN、QUIC |
| 客户端画像 | Chrome、Firefox、Safari、Brave、Opera、OkHttp 及部分定制客户端 |
| 指纹控制 | TLS ClientHello、HTTP/2 SETTINGS、优先级帧、伪头顺序、HTTP/3 SETTINGS、GREASE |
| 请求能力 | Header 合并与排序、Cookie Jar、重定向、超时、压缩、请求/响应 Hook |
| 连接能力 | HTTP/HTTPS、SOCKS4、SOCKS5 代理，自定义 Dialer，本地地址和 IP 版本限制 |
| 安全与观测 | 证书 Pinning、TLS Key Log、带宽统计、受限 Body 调试日志 |
| 并发与缓存 | 请求状态快照、按目标单飞初始化、Transport LRU、TLS session 缓存 |
| 多语言集成 | C shared library，以及 Python、Node.js、TypeScript、C# 示例 |

## 环境要求

- Go 1.26.5，以 [`go.mod`](./go.mod) 和 [`.tool-versions`](./.tool-versions) 为准；
- HTTP/3 需要目标服务与本地网络支持 UDP/QUIC；
- 构建 C shared library、启用部分平台的 race detector 或执行跨平台构建时，需要 C 编译器。

## 安装

为了兼容现有生态，本仓库的 module 路径仍是 `github.com/bogdanfinn/tls-client`。在另一个本地项目中使用本仓库时，可通过 `replace` 指向检出的源码：

```go
require github.com/bogdanfinn/tls-client v0.0.0

replace github.com/bogdanfinn/tls-client => ../tls-client
```

直接执行 `go get github.com/bogdanfinn/tls-client` 获取的是原项目发布版本，不一定包含本仓库的功能和修复。若未来迁移到独立 module 路径，将作为明确的兼容性变更发布。

## 快速开始

```go
package main

import (
	"io"
	"log"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

func main() {
	profile, err := profiles.ResolveClientProfileStrict("chrome_146")
	if err != nil {
		log.Fatal(err)
	}

	client, err := tls_client.NewHttpClient(
		tls_client.NewNoopLogger(),
		tls_client.WithClientProfile(profile),
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithCookieJar(tls_client.NewCookieJar()),
		tls_client.WithDefaultHeaders(http.Header{
			"Accept":          []string{"*/*"},
			"Accept-Language": []string{"zh-CN,zh;q=0.9"},
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.CloseIdleConnections()

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		log.Fatal(err)
	}

	// 请求自身的同名字段优先于默认 Header。
	req.Header.Set("Accept", "text/html")
	req.Header[http.HeaderOrderKey] = []string{
		"accept",
		"accept-language",
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("status=%d bytes=%d", resp.StatusCode, len(body))
}
```

## 客户端画像

### 选择画像

推荐使用严格解析 API，让无效标识符尽早返回错误：

```go
profile, err := profiles.ResolveClientProfileStrict("chrome_146")
```

| API | 未知标识符的处理方式 |
| --- | --- |
| `ResolveClientProfile` | 回退到 `DefaultClientProfile`，兼容旧调用方 |
| `ResolveClientProfileWithKey` | 回退到默认画像并返回空 key |
| `ResolveClientProfileStrict` | 返回 `ErrUnknownClientProfile` |
| `ResolveClientProfileWithKeyStrict` | 返回规范 key、画像或明确错误 |

严格解析忽略首尾空白且大小写不敏感。CFFI 默认采用严格解析，避免拼写错误静默变成默认画像。

### 随机画像

`random` 和 `chaos` 会从已经注册并经过筛选的真实画像中随机选择，而不是随机拼接 TLS 参数：

```go
key, profile := profiles.ResolveClientProfileWithKey("random")
```

随机候选会排除：

- Zalando、Nike、Cloudscraper、MMS、Mesh、Confirmed 等业务定制画像；
- `_PSK`、`_PSK_PQ` 等显式会话恢复画像；
- 元数据中 `KnownGaps` 非空的画像。

可使用 `GetProfileMetadata`、`AllProfileMetadata`、`ProfilesWithKnownGaps` 和 `RandomBrowserProfileKeys` 查询画像信息。画像及元数据 Getter 返回防御性副本，调用方修改返回值不会污染全局注册表。

## 常用配置

| 选项 | 用途 |
| --- | --- |
| `WithClientProfile` | 设置 TLS、HTTP/2、HTTP/3 画像 |
| `WithTimeoutSeconds` / `WithTimeoutMilliseconds` | 设置客户端超时 |
| `WithCookieJar` | 注入 Cookie Jar |
| `WithProxyUrl` | 设置 HTTP、HTTPS、SOCKS4 或 SOCKS5 代理 |
| `WithDialContext` | 完全接管 TCP 连接建立过程 |
| `WithDefaultHeaders` | 按字段合并默认 Header，请求字段优先且名称大小写不敏感 |
| `WithConnectHeaders` | 设置 HTTP CONNECT Header |
| `WithNotFollowRedirects` / `WithCustomRedirectFunc` | 控制重定向策略 |
| `WithForceHttp1` | 强制使用 HTTP/1.1 |
| `WithDisableHttp3` | 禁用 HTTP/3 |
| `WithProtocolRacing` | 启用 HTTP/3 与延迟 HTTP/2 竞速 |
| `WithRandomTLSExtensionOrder` | 随机 TLS 扩展顺序 |
| `WithCertificatePinning` | 启用证书 Pinning |
| `WithTransportOptions` | 配置连接池、缓存、证书、Key Log 和 Protocol Racing 参数 |
| `WithBandwidthTracker` | 启用带宽统计 |
| `WithPreHook` / `WithPostHook` | 注册请求前和响应后 Hook |
| `WithCatchPanics` | 将请求处理 panic 转换为显式 error |
| `WithDebugBodyLimit` | 限制调试日志中的 Body 预览大小 |

Header、CONNECT Header、证书 Pin 列表与 `TransportOptions` 会在客户端构建时复制，调用方后续修改原始对象不会改变已创建客户端的配置。

## HTTP/3 Protocol Racing

Protocol Racing 会立即尝试 HTTP/3，并在默认 300 ms 的可取消延迟后尝试 HTTP/2；默认总等待时间为 10 秒。

```go
client, err := tls_client.NewHttpClient(nil,
	tls_client.WithClientProfile(profiles.Chrome_146),
	tls_client.WithProtocolRacing(),
)
```

使用约束：

- 只有 `GET`、`HEAD`、`OPTIONS` 会参与双协议竞速；
- 带 Body 的安全请求必须提供 `GetBody`，两个协议会使用互相独立的 Body 副本；
- `POST`、`PUT`、`PATCH`、`DELETE` 等请求不会因竞速被重复发送；
- 不能与 `WithForceHttp1`、`WithDisableHttp3` 同时启用；
- 当前不能与代理、自定义 TCP DialContext、本地地址、IPv4/IPv6 限制、证书 Pinning 或带宽统计组合使用；
- 可通过 `TransportOptions.ProtocolRacingHTTP2Delay` 和 `TransportOptions.ProtocolRacingTimeout` 调整时序；
- 获胜请求的 context 会保持到响应体 EOF 或 `Close`，输家响应体和临时 Transport 会被回收。

项目会缓存目标地址实际获胜的协议和 HTTP/3 Transport。缓存协议失效后，安全请求可以重新竞速。

## 并发与动态配置

同一个 `HttpClient` 可以由多个 goroutine 并发使用。`SetProxy`、`SetFollowRedirect` 和 `SetCookieJar` 通过切换新的客户端状态影响后续请求，已经开始的请求继续使用自己的状态快照；实现不会复制已经投入使用的 `http.Client`。

Transport 按目标单飞初始化，不同目标可以并行握手。缓存默认最多保留 256 个 host/protocol 条目，可通过 `TransportOptions.MaxCachedTransports` 调整，`-1` 表示不限制。支持恢复的画像默认保留 32 个 TLS session，可通过 `TransportOptions.TLSClientSessionCacheSize` 调整。

## WebSocket

WebSocket 会复用 HTTP 客户端的 TLS 画像和 Dialer。当前建议强制使用 HTTP/1.1：

```go
client, err := tls_client.NewHttpClient(nil,
	tls_client.WithClientProfile(profiles.Chrome_146),
	tls_client.WithForceHttp1(),
)
if err != nil {
	log.Fatal(err)
}

ws, err := tls_client.NewWebsocket(nil,
	tls_client.WithTlsClient(client),
	tls_client.WithUrl("wss://example.com/ws"),
	tls_client.WithHeaders(http.Header{
		http.HeaderOrderKey: []string{"host", "upgrade", "connection"},
	}),
)
```

更多用法见 [`websocket.go`](./websocket.go) 和 [`tests/websocket_test.go`](./tests/websocket_test.go)。

## CFFI 与多语言调用

[`cffi_dist`](./cffi_dist) 提供 C shared library 入口，并包含以下示例：

- [Python](./cffi_dist/example_python)
- [Node.js](./cffi_dist/example_node)
- [TypeScript](./cffi_dist/example_typescript)
- [C#](./cffi_dist/example_csharp)

请求示例：

```json
{
  "sessionId": "demo-session",
  "tlsClientIdentifier": "random",
  "requestMethod": "GET",
  "requestUrl": "https://example.com",
  "followRedirects": true,
  "withProtocolRacing": true,
  "headers": {
    "accept": "*/*"
  },
  "headerOrder": ["accept", "user-agent"]
}
```

### Session 模型

- 不同 `sessionId` 可以并行执行；
- 同一 `sessionId` 的完整请求生命周期串行执行，动态代理、Cookie 和重定向配置不会互相覆盖；
- `RemoveSession` 与 `ClearSessionCache` 会等待相关 session 操作结束后再回收连接；
- session 缓存默认最多保留 1024 个条目，正在执行或等待的 session 不会被驱逐；
- idle TTL 默认关闭，可通过 `TLS_CLIENT_SESSION_CACHE_TTL=30m` 启用；
- `TLS_CLIENT_SESSION_CACHE_MAX_ENTRIES=-1` 可关闭容量限制。

运行时也可以调用导出的 C ABI：

```text
configureSessionCache({"maxEntries":1024,"idleTTL":"30m"})
```

CFFI 返回的 JSON 缓冲区仍需通过 `freeMemory` 释放。大响应可通过 `maxResponseBodyBytes` 限制大小，或使用 `streamOutputPath` 流式写入文件。

### 构建动态库

`cffi_dist` 是独立 Go module，并通过下面的配置链接当前工作树：

```go
replace github.com/bogdanfinn/tls-client => ../
```

Linux 示例：

```bash
cd cffi_dist
go mod download
go build -buildmode=c-shared -o dist/tls-client.so .
```

Windows 使用 Go 1.26.x 时，链接阶段的 DLL 基础名不能包含 `-`。请先使用安全名称构建，再同时重命名 DLL 与头文件：

```powershell
go build -buildmode=c-shared -o dist/tls_client.dll .
Move-Item dist/tls_client.dll dist/tls-client.dll
Move-Item dist/tls_client.h dist/tls-client.h
```

跨平台构建见 [`cffi_dist/build.sh`](./cffi_dist/build.sh)、[`Dockerfile.alpine.compile`](./cffi_dist/Dockerfile.alpine.compile) 和 [`Dockerfile.ubuntu.compile`](./cffi_dist/Dockerfile.ubuntu.compile)。

## 已知限制

- 默认画像是 `Chrome_150`；其 TLS 基础来自 `Chrome_146`，ML-DSA 签名算法仍等待 uTLS 支持；
- 显式 PSK 画像用于会话恢复，不适合作为随机 session 的首次握手画像；
- HTTP/3 是否可用取决于目标服务、UDP 网络、防火墙和 QUIC 实现；
- 自定义 JA3、HTTP/2、HTTP/3 参数可能产生真实客户端中不存在的组合；
- `InsecureSkipVerify` 会降低证书校验安全性，且不能与证书 Pinning 同时使用；
- TLS/HTTP 指纹不能模拟 JavaScript、DOM、字体、Canvas 或用户行为轨迹。

## 开发与验证

不访问公网的基础验证：

```bash
go test . ./profiles ./bandwidth ./cffi_src
go test -run '^$' ./...
go vet ./...
go test -race . ./profiles ./bandwidth ./cffi_src
```

`cffi_dist` 需要单独编译：

```bash
cd cffi_dist
go test -run '^$' ./...
```

默认 CI 不运行依赖公网的完整 `tests`。在线画像验证需要显式使用 `integration` build tag：

```bash
go test -tags=integration ./tests -run '^TestJA3Integration_'
```

参与开发或同步来源项目更新前，请先阅读 [`AGENTS.md`](./AGENTS.md)。其中记录了本项目的行为不变量、冲突热点和完整验证要求。

## 项目结构

```text
tls-client/
├─ client.go             HTTP 客户端与动态状态
├─ client_options.go     客户端配置
├─ roundtripper.go       TLS 握手与 Transport 缓存
├─ racer.go              HTTP/3 与 HTTP/2 Protocol Racing
├─ websocket.go          WebSocket 支持
├─ profiles/             客户端画像、解析器与元数据
├─ bandwidth/            带宽统计
├─ cffi_src/             CFFI 请求、响应与 session 管理
├─ cffi_dist/            C shared library 与多语言示例
└─ tests/                本地及在线集成测试
```

## 来源与致谢

本项目建立在以下开源项目和设计工作的基础上：

- [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client)
- [bogdanfinn/fhttp](https://github.com/bogdanfinn/fhttp)
- [bogdanfinn/utls](https://github.com/bogdanfinn/utls)
- [refraction-networking/utls](https://github.com/refraction-networking/utls)
- [kurl-client](https://gitee.com/hqs666/kurl-client)

项目在保留来源版权与许可证要求的前提下独立维护。许可证全文见 [`LICENSE`](./LICENSE)。
