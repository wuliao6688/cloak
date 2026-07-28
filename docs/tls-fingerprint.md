# TLS 指纹

## 什么是 TLS 指纹？

TLS 握手时，客户端发送 **ClientHello** 消息，包含：

- 支持的加密套件列表及其排列顺序
- 支持的 TLS 版本
- 压缩方式
- TLS Extensions 列表及其排列顺序
- 其他特征（GREASE、SNI、ALPN 等）

这些特征组合起来就是 **TLS 指纹**。不同的 TLS 实现（Go 标准库、Chrome、Firefox）发出的 ClientHello 各不相同，服务端可以通过识别 TLS 指纹来区分访问者类型。

### JA3 与 JA4

| 指纹 | 计算方式 |
|------|---------|
| **JA3** | TLSVersion,Ciphers,Extensions,EllipticCurves,EllipticCurvePointFormats → MD5 |
| **JA4** | t + TLSVersion + SNI + ALPN + CiphersHash + ExtensionsHash |

Go 标准库的 JA3/JA4 与 Chrome 完全不同，因此很容易被反爬系统识别。

## 如何伪装 TLS 指纹？

tlsgateway 基于 Tor 团队维护的 [uTLS](https://github.com/refraction-networking/utls) 库，可以精确模拟 Chrome/Firefox/Safari 等浏览器的 ClientHello。

### 一行代码伪装

```go
// 模拟 Chrome 150 的 TLS 指纹
client := tlsgateway.Impersonate(profiles.Chrome_150)
resp, _ := client.Get("https://www.akamai.com/") // 200 OK
```

### 指定具体浏览器版本

```go
// Chrome 146 (Akamai 最低要求)
tlsgateway.Impersonate(profiles.Chrome_146)

// Firefox 148
tlsgateway.Impersonate(profiles.Firefox_148)

// Safari iOS 18.5
tlsgateway.Impersonate(profiles.Safari_iOS_18_5)
```

### 动态切换画像

```go
tr := tlsgateway.NewTransport(profiles.Chrome_150)
// ... 后续切换到 Firefox
tr.SetProfile(profiles.Firefox_148)
```

### 选择 FignerprintTransport（H2 完整定制）

```go
// FingerprintTransport 使用 internal/http2 fork
// 支持 H2 SETTINGS/StreamID/ConnectionFlow 完整定制
tr := tlsgateway.NewFingerprintTransport(profiles.Chrome_150)
client := &http.Client{Transport: tr}
```

## 如何验证 TLS 指纹？

### SelfCheck — 内建自检

```go
info, _ := tlsgateway.SelfCheck(profiles.Chrome_150)
fmt.Printf("JA3: %s\n", info.JA3Hash)
fmt.Printf("JA4: %s\n", info.JA4)
```

### DumpFingerprint — 完整打印

```go
tlsgateway.DumpFingerprint(profiles.Chrome_150)
// 输出:
// ── Firefox/148 ─────────────────────────
//   JA3:  bf73f7e6b8e6e3b3a3a72e0e9e29e2b2
//   JA4:  t13d1713h2_bc7e8b6e6b...
//   Protocols: h2,http/1.1
```

### 在线验证平台

| 平台 | URL | 检测维度 |
|------|-----|---------|
| tls.peet.ws | https://tls.peet.ws/api/all | JA3, JA4 |
| browserleaks.com | https://browserleaks.com/tls | JA3, JA3N |
| browserscan.net | https://www.browserscan.net/ | 综合指纹 |

使用验证工具（在项目根目录）：

```bash
go run ./cmd/verify-fingerprints -profiles "chrome_150,firefox_148"
```

## 平台通过率

| 平台 | 结果 |
|------|------|
| tls.peet.ws | ✅ JA3/JA4 正确 |
| browserleaks.com | ✅ JA3/JA3N 一致 |
| cloudflare.com | ✅ TLSv1.3+HTTP/2 |
| imperva.com | ✅ |
| f5.com | ✅ |
| **akamai.com** | ✅ **200** |
| hcaptcha.com | ✅ |
| recaptcha-demo | ✅ |
| sannysoft.com | ✅ PASS |

TLS 层：**9/9 核心平台 100% 通过**

## 常见问题

### Q: Akamai 返回 403 怎么办？

Akamai 除了检查 TLS 指纹外，还检查 HTTP 头（User-Agent、Sec-CH-UA 等）。使用 `Impersonate()` 会自动注入浏览器头，通常能通过。如果使用原始 Transport，需要手动加 `HeaderRoundTripper`：

```go
tr := tlsgateway.NewTransport(p)
htr := tlsgateway.NewHeaderRoundTripper(tr, p)
client := &http.Client{Transport: htr}
```

### Q: Chrome 131 为什么被 Akamai 拒绝？

Akamai 只接受较新版本的 Chrome（≥146）。这是 Akamai 的策略，不是 tlsgateway 的 bug。请使用 `profiles.Chrome_150` 或 `profiles.Chrome_146`。

### Q: DataDome 能过吗？

DataDome 需要完整的 JavaScript 引擎来执行挑战脚本，纯 HTTP 库无法通过。这是已知限制。
