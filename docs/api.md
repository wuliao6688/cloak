# API 参考

所有公开 API 速查。包路径：`github.com/wuliao6688/cloak`（核心）和
`github.com/wuliao6688/cloak/profiles`（画像）。

## 1. 入口函数

| 函数 | 说明 |
|---|---|
| `Impersonate(profile) *http.Client` | 标准客户端：TLS + HTTP/2 指纹 + 浏览器头 |
| `ImpersonateH3(profile) *http.Client` | H3 racing 客户端：HTTP/3 优先，自动降级 |
| `ImpersonateChain(profile) *ChainBuilder` | 链式配置入口 |
| `ImpersonateRequest(profile) *Request` | 请求级 builder（无需先建 client） |
| `DevMode(profile) *http.Client` | 调试模式（完整 dump 到 stderr） |
| `NewTransport(profile) *Transport` | 底层 Transport |
| `NewTransportWithOptions(profile, TransportOptions) *Transport` | 带选项 |
| `NewH3Transport(profile) *H3Transport` | 纯 H3 RoundTripper |
| `NewH3RaceTransport(profile) *H3RaceTransport` | H3 racing RoundTripper |
| `NewRaceTransport(profile) *RaceTransport` | H2 vs H1 racing（无 H3） |
| `NewFingerprintTransport(profile) *FingerprintTransport` | 底层（fork http2） |
| `NewProxy(addr, profile) *Proxy` | 本地正向代理 |
| `NewMiddlewareChain(...) *MiddlewareChain` | 中间件链 |
| `NewHeaderRoundTripper(transport, profile)` | 浏览器头包装器 |

## 2. TransportOptions

```go
type TransportOptions struct {
	RandomExtensionOrder bool                        // 随机 TLS 扩展顺序
	ServerNameOverwrite  string                      // SNI 覆盖
	Proxy                func(*http.Request) (*url.URL, error) // HTTP 代理
	InsecureSkipVerify   bool                        // 跳过证书验证
	PinningHosts         map[string][]string         // 证书 pinning(OkHttp 风格)
}
```

### 证书 Pinning（防 MITM）

`PinningHosts` 为指定域名启用证书固定——只信任预置的证书指纹，中间人攻击
（伪造证书）会被拒绝。支持精确域名和通配符：

```go
// 指纹 = SHA-256(证书 DER) 的 base64
//   sum := sha256.Sum256(cert.Raw)
//   base64.StdEncoding.EncodeToString(sum[:])

tr := cloak.NewTransportWithOptions(profiles.Chrome_150, cloak.TransportOptions{
	PinningHosts: map[string][]string{
		"api.example.com":  {"AbCdEfGhIjKlMnOpQrStUvWxYz1234567890ab=="}, // 精确
		"*.example.com":    {"XyZ..."},  // 通配符: 匹配所有子域
	},
})
```

- pin 不匹配 → 握手失败（`cloak: pinning: certificate mismatch`）
- 未配置 pin 的域名 → 标准证书验证不受影响

## 3. ChainBuilder（ImpersonateChain 返回值）

| 方法 | 说明 |
|---|---|
| `SetUserAgent(ua)` | 覆盖 UA |
| `SetHeader(k, v)` | 加自定义头 |
| `SetTimeout(d)` | 超时 |
| `SetDebug(w)` | 调试输出 |
| `EnableH3()` | **启用 HTTP/3 racing** |
| `WithOrderedHeaders()` | H1 头排序 |
| `SetH2Fingerprint(fp)` / `WithH2Fingerprint(fp)` | 自定义 H2 指纹 |
| `AsChrome() / AsFirefox() / AsSafari() / AsEdge() / AsQQ() / As360() / AsAndroid() / AsIOS()` | 预设浏览器指纹 |
| `AsRandom()` | 随机指纹 |
| `Build() *http.Client` | 构建 |
| `Transport() *Transport` | 取底层 Transport |

## 4. Transport

| 方法 | 说明 |
|---|---|
| `RoundTrip(req)` | 实现 http.RoundTripper，自动协商 H2/H1 |
| `SetProfile(profile)` | 动态切换画像 |
| `SetRandomExtensionOrder(bool)` | 随机扩展顺序 |
| `SetInsecureSkipVerify(bool)` | 跳过证书验证 |
| `SetDebug(w)` | 调试输出 |
| `SelfCheck(url) FingerprintInfo` | 自检 TLS 指纹（JA3/JA4） |
| `DialTLS(ctx, network, addr)` | 原始 uTLS 连接（CONNECT 用） |
| `Wrap(middleware...) *Transport` | 应用中间件 |
| `CloseIdleConnections()` | 关闭空闲连接 |

## 5. Request（请求级 builder）

### 执行

| 方法 | 说明 |
|---|---|
| `Get(url) (*Response, error)` | GET |
| `Post(url) (*Response, error)` | POST |

### 配置

| 方法 | 说明 |
|---|---|
| `SetHeader(k, v)` / `SetHeaders(map)` | 请求头 |
| `SetHeaderNonCanonical(k, v)` | 精确大小写头（指纹相关） |
| `SetCommonHeaders(map)` | 通用头 |
| `SetBody(io.Reader)` / `SetBodyString(s)` / `SetBodyBytes(b)` | 请求体 |
| `SetOrderedFormData(kvs...)` | 有序表单（字段顺序=指纹） |
| `SetQueryParam(k, v)` / `SetQueryParams(map)` | 查询参数 |
| `SetCommonQueryParams(map)` | 通用查询参数 |
| `SetPathParam(k, v)` / `SetPathParams(map)` | REST 路径参数 |
| `SetBaseURL(base)` | 基础 URL |
| `SetCookies(cookies...)` | Cookie |
| `SetBasicAuth(u, p)` / `SetBearerAuthToken(t)` | 认证 |
| `SetInsecureSkipVerify(bool)` | 跳过证书验证（穿透包装链） |

### 结果处理

| 方法 | 说明 |
|---|---|
| `SetSuccessResult(&v)` | 2xx 自动 JSON → v |
| `SetErrorResult(&v)` | 非 2xx 自动 JSON → v |
| `SetRetry(count, cond, min, max)` | 条件重试 + 指数退避 |
| `SetDump(opts)` | 请求/响应 dump |
| `SetOutputFile(path)` | 响应落盘 |
| `SetOutput(io.Writer)` | 响应写流 |
| `OnRequest(fn)` | 请求前钩子 |
| `OnResponse(fn)` | 响应后钩子 |

### 重试条件

| 常量 | 说明 |
|---|---|
| `RetryOnAnyError` | 任何错误都重试 |
| `RetryOnServerError` | 5xx 重试 |
| `GetRetryIntervalFunc` | 自定义退避 |

## 6. Response

| 方法 | 说明 |
|---|---|
| `String() / ToString() / Bytes() / BodyBytes()` | 响应体 |
| `SuccessResult() / ErrorResult()` | 反序列化结果 |
| `IsSuccess() / IsError()` | 状态判断 |
| `ResultState()` | 结果状态 |
| `UnmarshalJson(&v) / UnmarshalXml(&v) / UnmarshalErr(&v)` | 手动反序列化 |
| `Trace` | TraceInfo（DNS/TCP/TLS/FirstByte 七点计时） |

## 7. HTTP/3

| 类型/方法 | 说明 |
|---|---|
| `H3Transport` | 纯 H3 RoundTripper（`RoundTrip`/`SetProfile`/`SetDebug`） |
| `H3RaceTransport` | H3 vs H2 racing（`RoundTrip`/`SetProfile`/`CloseIdleConnections`） |
| `NewH3TransportWithOptions(profile, opts)` | 带选项构造 |
| `NewH3RaceTransportWithOptions(profile, opts, raceOpts)` | 带选项 + race 选项 |
| `RaceOptions` | `Timeout` / `H2Delay` 配置 |
| `DefaultRaceOptions()` | 默认 race 选项 |

## 8. 中间件

| 类型 | 说明 |
|---|---|
| `Middleware` | `func(http.RoundTripper) http.RoundTripper` |
| `MiddlewareChain` | `.Use(m1, m2...)` / `.Build()` |
| `DebugMiddleware` | 调试中间件 |
| `UserAgentMiddleware(ua)` | UA 中间件 |

## 9. Proxy（正向代理）

| 方法 | 说明 |
|---|---|
| `NewProxy(addr, profile) *Proxy` | 创建 |
| `ListenAndServe() error` | 启动 |
| `Shutdown(ctx)` | 优雅关闭 |
| `SetProfile(profile)` | 动态切换画像 |
| `SetInsecureSkipVerify(bool)` | 跳过证书验证 |

## 10. profiles 包

| API | 说明 |
|---|---|
| `profiles.Chrome_150`（等 77 个画像常量） | 浏览器画像 |
| `profiles.AllClientProfiles() map[string]ClientProfile` | 全部画像 |
| `profiles.ResolveClientProfileStrict(key) (ClientProfile, error)` | 按 key 解析 |
| `profiles.NewClientProfile(...)` | 自定义画像 |
| `profiles.DefaultClientProfile` | 默认画像 |
| `profiles.ErrUnknownClientProfile` | 未知画像错误 |
| `profiles.MappedTLSClients` | 兼容映射 |
| `ClientProfile` 的 `GetXxx()` 系列 | 画像字段读取 |

## 11. 指纹工具

| API | 说明 |
|---|---|
| `BrowserFingerprint(name) *H2Fingerprint` | 浏览器 H2 指纹 |
| `RandomFingerprint()` | 随机指纹 |
| `DumpFingerprint(...)` | 导出指纹 |
| `ChromeMultipartBoundary()` / `FirefoxMultipartBoundary()` | multipart 边界 |
| `H2Fingerprint` / `H2Setting` / `H2SettingID` / `PriorityFrame` / `PriorityParam` | H2 指纹类型 |
