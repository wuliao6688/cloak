# tls-client（增强维护版）

`tls-client` 是一个基于 Go 的可定制 HTTP 客户端。它不仅能修改 `User-Agent`，还可以控制 TLS ClientHello、HTTP/2 设置与帧顺序、HTTP/3 参数、请求头顺序等协议细节，用于尽可能复现真实浏览器或移动客户端的网络指纹。

本仓库基于上游项目 [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client)，继续使用 Go、[`fhttp`](https://github.com/bogdanfinn/fhttp) 和定制版 [`uTLS`](https://github.com/bogdanfinn/utls) 技术栈。在保持上游 API 与画像兼容性的基础上，本地增强重点放在：

- 画像解析、元数据与能力边界；
- 并发安全和动态配置隔离；
- HTTP/2 与 HTTP/3 协议竞速的请求安全；
- CFFI session 生命周期和并行模型；
- 可持续跟进上游的测试与合并规范。

本仓库借鉴了 [kurl-client](https://gitee.com/hqs666/kurl-client) 在画像治理、session 隔离和并发设计方面的思路，但没有移植其 Rust/BoringSSL 实现。

## 主要能力

| 分类 | 能力 |
| --- | --- |
| 协议 | HTTP/1.1、HTTP/2、HTTP/3，支持 ALPN 协商与按 host 缓存 Transport |
| TLS 指纹 | Chrome、Firefox、Safari、Brave、Opera、OkHttp 以及部分定制客户端画像 |
| HTTP 指纹 | HTTP/2 SETTINGS、优先级帧、伪头顺序、连接窗口；HTTP/3 SETTINGS、GREASE 与优先级参数 |
| 请求控制 | 普通 Header、默认 Header 合并、Header 顺序、Cookie Jar、重定向策略、超时 |
| 网络 | HTTP/HTTPS、SOCKS4、SOCKS5 代理，自定义 Dialer，本地地址与 IPv4/IPv6 限制 |
| 安全与观测 | 证书 Pinning、TLS Key Log、带宽统计、调试日志、请求前/响应后 Hook |
| 长连接 | WebSocket 使用与 HTTP 客户端一致的 TLS 指纹和 Dialer |
| FFI | C shared library，以及仓库内的 Python、Node.js、TypeScript、C# 示例 |
| 稳定性增强 | 防御性配置复制、动态客户端指针切换、按目标单飞初始化、请求体和响应体所有权治理 |

> TLS/HTTP 指纹只是网络协议层特征，不等于完整的浏览器环境，也不能单独解决所有反自动化检测。

## 环境要求

- Go **1.26.5**，以根目录 [`.tool-versions`](./.tool-versions) 和 [`go.mod`](./go.mod) 为准；
- HTTP/3 需要目标服务和网络环境支持 UDP/QUIC；
- 构建 `c-shared`、部分平台的 `-race` 或跨平台 CFFI 时，需要可用的 C 编译器；
- 画像在线集成测试会访问外部指纹服务，默认测试流程不会运行它们。

## 获取与依赖方式

### 开发当前仓库

```bash
git clone <你的仓库地址> tls-client
cd tls-client
go mod download
```

### 在另一个本地 Go 项目中使用当前工作树

模块路径仍保持为 `github.com/bogdanfinn/tls-client`。在本地增强版尚未发布到你自己的远程仓库前，可以在调用方 `go.mod` 中使用：

```go
require github.com/bogdanfinn/tls-client v1.15.1

replace github.com/bogdanfinn/tls-client => ../tls-client
```

直接执行以下命令获取的是上游已发布版本，不一定包含本仓库尚未发布的增强：

```bash
go get github.com/bogdanfinn/tls-client
```

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

	// 请求自身的字段优先于 WithDefaultHeaders 中的同名字段。
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
	log.Printf("status=%d body=%s", resp.StatusCode, body)
}
```

## 客户端画像

### 固定画像

画像注册表位于 [`profiles/profiles.go`](./profiles/profiles.go)。建议通过解析 API 获取画像，而不是在业务层直接遍历或修改 `MappedTLSClients`。

```go
profile := profiles.ResolveClientProfile("chrome_146")
```

### 严格解析与兼容解析

| API | 未知标识符行为 |
| --- | --- |
| `ResolveClientProfile` | 为兼容旧调用方，回退到 `DefaultClientProfile` |
| `ResolveClientProfileWithKey` | 回退默认画像，并返回空 key |
| `ResolveClientProfileStrict` | 返回 `ErrUnknownClientProfile` |
| `ResolveClientProfileWithKeyStrict` | 返回规范 key、画像或明确错误 |

严格解析会忽略标识符首尾空白，并进行大小写不敏感匹配。CFFI 默认使用严格解析，避免拼写错误静默变成默认画像。

### `random` 与 `chaos`

```go
key, profile := profiles.ResolveClientProfileWithKey("random")
_ = key
_ = profile
```

`random` 和 `chaos` 当前都表示从可信候选集合中随机选择一个已注册画像。候选集合会排除：

- Zalando、Nike、Mesh、MMS 等业务定制画像；
- `_PSK`、`_PSK_PQ` 等显式会话恢复画像；
- 元数据中声明了 `KnownGaps` 的画像。

随机选择的是已有真实画像，而不是随机拼接 TLS 参数，避免生成现实中不存在的组合指纹。

### 画像元数据

```go
metadata, ok := profiles.GetProfileMetadata("chrome_150")
if ok {
	log.Println(metadata.TLSBase)
	log.Println(metadata.KnownGaps)
	log.Println(metadata.VerifiedAgainst)
}
```

相关 API：

- `GetProfileMetadata`：查询单个画像；
- `AllProfileMetadata`：返回元数据注册表的防御性副本；
- `ProfilesWithKnownGaps`：列出仍有已知差异的画像；
- `RandomBrowserProfileKeys`：列出随机选择候选项。

`ClientProfile` 的 map、slice、优先级指针以及 `ClientHelloID` 的 seed/weights 指针 Getter 均返回副本，调用方修改返回值不会污染全局画像。

## 常用客户端选项

| 选项 | 说明 |
| --- | --- |
| `WithClientProfile` | 指定 TLS、HTTP/2、HTTP/3 画像 |
| `WithTimeoutSeconds` / `WithTimeoutMilliseconds` | 设置客户端超时，两者不能同时用于 CFFI 请求 |
| `WithCookieJar` | 注入 Cookie Jar |
| `WithProxyUrl` | 设置 HTTP/HTTPS/SOCKS4/SOCKS5 代理 |
| `WithProxyDialerFactory` | 自定义代理 Dialer 工厂 |
| `WithDialContext` | 完全接管 TCP Dial，使用者自行负责代理逻辑 |
| `WithDefaultHeaders` | 按字段合并默认 Header，请求字段优先，字段名大小写不敏感 |
| `WithConnectHeaders` | 设置 HTTP CONNECT 请求头 |
| `WithNotFollowRedirects` / `WithCustomRedirectFunc` | 控制重定向 |
| `WithForceHttp1` | 强制 HTTP/1.1 |
| `WithDisableHttp3` | 禁用 HTTP/3 |
| `WithProtocolRacing` | 启用 HTTP/3 与延迟 HTTP/2 竞速 |
| `WithRandomTLSExtensionOrder` | 随机 TLS 扩展顺序，使用前应确认目标画像确实具有该行为 |
| `WithCertificatePinning` | 启用证书 Pinning |
| `WithTransportOptions` | 配置连接池、压缩、缓冲区、Root CA、客户端证书和 Key Log |
| `WithBandwidthTracker` | 启用带宽统计 |
| `WithPreHook` / `WithPostHook` | 注册请求前和响应后 Hook |
| `WithCatchPanics` | 将请求处理 panic 转换为显式 error |
| `WithDebugBodyLimit` | 限制调试日志保留的请求/响应 Body 字节数；默认 64 KiB，设为 0 可关闭 Body 预览 |

Header、证书 Pin 列表和 `TransportOptions` 在客户端构建时会进行防御性复制，避免调用方后续修改输入对象造成数据竞争或隐式配置漂移。

## HTTP/3 Protocol Racing

启用方式：

```go
client, err := tls_client.NewHttpClient(nil,
	tls_client.WithClientProfile(profiles.Chrome_146),
	tls_client.WithProtocolRacing(),
)
```

约束与行为：

- 不能与 `WithForceHttp1` 或 `WithDisableHttp3` 同时使用，否则客户端构建失败；
- 不能与 HTTP/HTTPS/SOCKS 代理、自定义 TCP DialContext、本地地址、IPv4/IPv6 限制、证书 Pinning 或带宽统计同时使用；当前实现会拒绝这些组合，避免 QUIC 连接绕过配置；
- HTTP/3 立即尝试，HTTP/2 在可取消的 300ms 延迟后尝试；
- 只有 `GET`、`HEAD`、`OPTIONS` 会参与竞速；
- 有请求体时必须提供 `GetBody`，确保 HTTP/2 与 HTTP/3 使用独立副本；
- POST 等非安全请求不会进入双协议竞速，只发送一次；没有协议缓存时走 HTTP/2，已有缓存时可以复用单一已知协议；
- 获胜协议按目标地址缓存，HTTP/3 缓存的是实际获胜 Transport；
- 输家响应体和临时 HTTP/3 Transport 会被关闭；
- 获胜请求的 context 会保持到响应体 EOF 或 `Close`，不会因竞速结束而提前取消。

## 并发与动态配置

同一个 `HttpClient` 可以被多个 goroutine 使用。动态配置采用“构建新状态并切换指针”的方式，不复制已经投入使用的 `http.Client` 内部同步状态。

- `SetProxy`：构建新 Transport、同步更新 Dialer，然后在锁外关闭旧 Transport 的空闲连接；
- `SetProxy` 的代理/Dialer/Transport 构建不再持有客户端状态读写锁，正在执行的请求可以继续使用旧快照；
- `SetFollowRedirect`：为后续请求切换新的客户端状态，不修改正在使用的客户端快照；
- `SetCookieJar`：切换新 Jar，已开始的请求继续使用自己的快照；
- Transport 初始化按目标地址单飞，TLS 握手期间不会持有全局 map 锁；
- Transport 缓存使用 LRU 上限，默认每个 Client 最多 256 个 host/protocol 条目；可通过 `TransportOptions.MaxCachedTransports` 调整，`-1` 保留无限缓存；被驱逐的 HTTP/3 Transport 会等活动响应体 EOF/Close 后再关闭；
- HTTP/2 重连握手继承当前等待该 host 的请求 context；只有所有相关请求都取消时，共享握手才会被取消；
- Header 和画像配置对外返回或接收时尽量使用防御性副本。

调试模式只记录有限 Body 预览，不会为了日志将完整请求或响应读入内存；不可重放的请求体会跳过预览。

### CFFI session 并发模型

- 不同 `sessionId` 可以并行构建和发送请求；
- 同一 `sessionId` 使用 flight 租约串行执行完整请求生命周期；
- 租约覆盖代理/重定向修改、请求发送、Cookie 更新和响应体读取，避免两个请求互相覆盖动态代理；
- `RemoveSession` 与 `ClearSessionCache` 会等待相关 session 操作完成，再在长临界区之外关闭连接；
- session 创建失败不会写入空客户端；
- session 缓存默认最多保留 1024 个条目并按 LRU 回收；正在执行或等待 flight 的 session 不会被驱逐；
- idle TTL 默认关闭，可通过 Go API `tls_client_cffi_src.ConfigureSessionCache` 或环境变量 `TLS_CLIENT_SESSION_CACHE_TTL=30m` 启用；`TLS_CLIENT_SESSION_CACHE_MAX_ENTRIES=-1` 可恢复无限容量。

## WebSocket

WebSocket 复用 HTTP 客户端的 TLS Dialer。当前建议为 WebSocket 客户端启用 HTTP/1.1：

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

完整连接方式请参考 [`websocket.go`](./websocket.go) 和 [`tests/websocket_test.go`](./tests/websocket_test.go)。

## CFFI / 多语言调用

CFFI 入口位于 [`cffi_dist`](./cffi_dist)，请求构建和 session 管理位于 [`cffi_src`](./cffi_src)。示例目录：

- [`cffi_dist/example_python`](./cffi_dist/example_python)
- [`cffi_dist/example_node`](./cffi_dist/example_node)
- [`cffi_dist/example_typescript`](./cffi_dist/example_typescript)
- [`cffi_dist/example_csharp`](./cffi_dist/example_csharp)

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

CFFI 请求可设置 `maxResponseBodyBytes` 限制内存响应大小；设置
`streamOutputPath` 时同样会限制写入文件的响应字节数。`streamOutputBlockSize`
默认使用 32 KiB，显式设置时必须为正数且不超过 16 MiB。字节响应会直接流式
编码到最终 Base64 data URL，避免为大型响应额外构造未使用的原始字符串副本。

`transportOptions.maxCachedTransports` 控制单个 CFFI Client 的 Transport LRU 上限：`0` 使用默认 256，`-1` 表示不限制。CFFI session 缓存可在进程启动前配置：

```text
TLS_CLIENT_SESSION_CACHE_MAX_ENTRIES=1024
TLS_CLIENT_SESSION_CACHE_TTL=30m
```

也可以在运行时调用导出的 C ABI：

```text
configureSessionCache({"maxEntries":1024,"idleTTL":"30m"})
```

`maxEntries` 为 `0` 时恢复默认 1024，`-1` 表示无限；`idleTTL` 使用 Go duration 语法，空字符串或 `"0s"` 关闭 TTL。容量或 TTL 清理只回收没有活动 flight 的 session；默认关闭 TTL 且未超过容量时，请求释放路径不会扫描全局 session 缓存。

`cffi_dist/go.mod` 使用：

```go
replace github.com/bogdanfinn/tls-client => ../
```

因此仓库内构建的动态库会链接当前工作树，而不是已发布的旧上游版本。不要在同步上游时无意删除该 `replace`。

本机具备 Go 和 C 编译器后，可以在对应平台直接构建：

```bash
cd cffi_dist
go mod download
go build -buildmode=c-shared -o dist/tls-client.so .
```

Windows 使用 Go 1.26.5 时，链接阶段的临时 DLL 基础名不要包含 `-`；Go 生成的 `.def` 会把该名称写入未加引号的 `LIBRARY` 指令，MinGW 会将连字符解析为语法错误。先用安全名称构建，再按分发约定重命名 DLL 和头文件：

```powershell
go build -buildmode=c-shared -o dist/tls_client.dll .
Move-Item dist/tls_client.dll dist/tls-client.dll
Move-Item dist/tls_client.h dist/tls-client.h
```

macOS 输出文件可改为 `dist/tls-client.dylib`。跨平台构建参考 [`cffi_dist/build.sh`](./cffi_dist/build.sh) 和两个 Dockerfile；脚本中的 Windows 目标已内置“安全临时名 → 兼容分发名”处理。

## 测试与验证

### 不访问公网的基础验证

```bash
gofmt -w <本次修改的 Go 文件>
go test . ./profiles ./bandwidth ./cffi_src
go test -run '^$' ./...
go vet ./...
go test -race . ./profiles ./bandwidth ./cffi_src
```

嵌套 CFFI module 必须单独编译：

```bash
cd cffi_dist
go test -run '^$' ./...
```

仓库中的部分 `tests` 会访问公网，不能在离线验证时直接运行完整的 `go test ./...`。CI 使用精确正则执行本地 `httptest` 集成用例，并离线校验已记录 JA3/Akamai hash 与注册画像实际生成的 JA3；配置见 [`.github/workflows/ci.yml`](./.github/workflows/ci.yml)。

### 在线画像验证

[`tests/ja3_integration_test.go`](./tests/ja3_integration_test.go) 使用 `integration` build tag：

```bash
go test -tags=integration ./tests -run '^TestJA3Integration_'
```

该测试需要访问外部指纹服务，只应在网络可用且明确需要验证画像时运行。

## 已知边界

- `DefaultClientProfile` 当前为 `Chrome_150`；其 TLS 基础来自 `Chrome_146`，ML-DSA 签名算法仍等待上游 uTLS 支持，可通过 `ProfilesWithKnownGaps` 查询；
- 显式 PSK 画像用于会话恢复场景，不应作为随机 session 的首次握手画像；
- HTTP/3 是否可用取决于目标服务、UDP 网络、防火墙和 QUIC 实现；
- 自定义 JA3/HTTP2/HTTP3 参数可以生成现实中不存在的组合，调用方需要自行验证；
- `InsecureSkipVerify` 会降低证书验证安全性，并且不能与证书 Pinning 同时启用；
- TLS/HTTP 指纹无法模拟 JavaScript、DOM、字体、Canvas、行为轨迹等浏览器环境。

## 目录结构

```text
tls-client/
├─ client.go                 HTTP 客户端、动态状态、Hook、Cookie/代理接口
├─ client_options.go         客户端配置项和防御性复制
├─ roundtripper.go           TLS 握手、Transport 缓存、HTTP/1.1/2/3 路由
├─ racer.go                  HTTP/3 与 HTTP/2 协议竞速
├─ websocket.go              WebSocket 封装
├─ profiles/                 浏览器画像、解析器、元数据
├─ bandwidth/                带宽统计
├─ cffi_src/                 CFFI 请求、响应和 session 管理
├─ cffi_dist/                c-shared 入口和多语言示例（独立 Go module）
├─ tests/                    本地与在线集成测试
└─ AGENTS.md                 上游同步和本地增强保留规则
```

## 跟进上游更新

本仓库保留上游模块路径，并预计继续合并 `bogdanfinn/tls-client` 的更新。由于本地增强集中在客户端状态、Transport、协议竞速、画像和 CFFI 等高冲突区域，后续同步不能简单使用整文件 `ours/theirs` 覆盖。

详细合并规则、冲突热点、本地不变量和验证矩阵见 [`AGENTS.md`](./AGENTS.md)。以后让自动化工具或 AI 协助同步上游前，应先读取该文件。

## 上游与致谢

- 上游项目：[bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client)
- HTTP 实现：[bogdanfinn/fhttp](https://github.com/bogdanfinn/fhttp)
- TLS 实现：[bogdanfinn/utls](https://github.com/bogdanfinn/utls)、[refraction-networking/utls](https://github.com/refraction-networking/utls)
- 设计参考：[kurl-client](https://gitee.com/hqs666/kurl-client)
- 上游详细文档：[Open Source Oasis](https://bogdanfinn.gitbook.io/open-source-oasis/)

许可证见 [`LICENSE`](./LICENSE)。
