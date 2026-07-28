# HTTP 指纹

## 什么是 HTTP 指纹？

不同 HTTP 客户端在以下维度上存在差异，这些差异构成 **HTTP 指纹**：

### HTTP/1.1 维度

- User-Agent 的值
- Header 及其排列顺序
- 请求头默认值（Accept、Accept-Language、Sec-Fetch-* 等）

### HTTP/2 维度

- **H2 SETTINGS 帧**：值列表及其排列顺序
- **WINDOW_UPDATE 帧**：初始流控窗口值
- **Priority 帧**：列表及其排列顺序（Firefox 发送 6 个）
- **伪头顺序**：:method, :authority, :scheme, :path 的排列
- **请求头顺序**：header 帧中 flag 与 priority 选项的值
- **Stream ID**：起始流 ID（Chrome=3, Firefox=1）

## 如何伪装 HTTP 指纹？

tlsgateway 覆盖了上述所有维度。使用 `Impersonate()` 时自动应用全部伪装。

### 各浏览器指纹对比

| 维度 | Chrome | Firefox | Safari |
|------|--------|---------|--------|
| H2 SETTINGS | 1,2,3,4,6 | 1,4,5 | 1,2,4,6 |
| Stream ID | 3 | 1 | 1 |
| ConnectionFlow | 15663105 | 12517377 | 10485760 |
| Priority 帧 | 无 | 6 个 | 无 |
| 伪头顺序 | :method,:authority,:scheme,:path | :method,:path,:authority,:scheme | :method,:scheme,:path,:authority |

### 自定义 H2 SETTINGS

```go
// 使用 FingerprintTransport 获得完整 H2 控制
tr := tlsgateway.NewFingerprintTransport(profiles.Chrome_150)
// 内部自动应用 Chrome 的 H2 SETTINGS：
// {1:65536, 2:0, 3:1000, 4:6291456, 6:262144}
```

### 获取浏览器指纹全部信息

```go
fp := tlsgateway.BrowserFingerprint("Chrome-150")
fmt.Printf("Settings: %v\n", fp.Settings)           // 5 个 SETTINGS
fmt.Printf("StreamID: %d\n", fp.InitialStreamID)     // 3
fmt.Printf("Flow: %d\n", fp.ConnectionFlow)          // 15663105
fmt.Printf("PseudoOrder: %v\n", fp.PseudoHeaderOrder)
fmt.Printf("Headers: %d defaults\n", len(fp.Headers)) // 13 个默认头
```

### Header 排序

```go
client := tlsgateway.ImpersonateChain(profiles.Chrome_150).
    WithOrderedHeaders().   // Chrome 规范顺序
    Build()

// 发出的请求头按 Chrome 顺序排列：
// host, pragma, cache-control, sec-ch-ua, ..., cookie
```

Header 排序是通过 `__header_order__` 和 `__pseudo_header_order__` 特殊 header 实现的（借鉴 req 设计）。这些 key 在发送时自动剥离。

### Multipart 边界

```go
// Chrome/WebKit 风格边界
boundary := tlsgateway.ChromeMultipartBoundary()
// 输出: ----WebKitFormBoundary + 16位随机字符

// Firefox 风格边界
boundary := tlsgateway.FirefoxMultipartBoundary()
// 输出: --------------------------- + 3组8位随机数
```

## 如何验证 HTTP 指纹？

使用在线工具检查当前请求的完整 HTTP 指纹：

```go
type Result struct {
    TLS struct {
        JA3 string `json:"ja3"`
        JA4 string `json:"ja4"`
    } `json:"tls"`
    HTTPVersion string `json:"http_version"`
    UserAgent   string `json:"user_agent"`
}

var r Result
tlsgateway.ImpersonateRequest(profiles.Chrome_150).
    SetSuccessResult(&r).
    Get("https://tls.peet.ws/api/all")

fmt.Printf("HTTP/%s, UA=%s, JA3=%s\n",
    r.HTTPVersion, r.UserAgent, r.TLS.JA3)
```

## 已知限制

| 限制 | 说明 |
|------|------|
| H2 帧级排序需 fork | 完整 H2 伪头/头排序需使用 FingerprintTransport |
| DataDome | JS 引擎必需，纯 HTTP 无法通过 |
| HTTP/3 | 暂不支持（可加 quic-go） |
