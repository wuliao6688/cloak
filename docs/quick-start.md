# 快速开始

5 分钟上手 tlsgateway，让你的 Go 程序请求看起来像浏览器。

## 1. 安装

```bash
go get github.com/bogdanfinn/tls-client
```

## 2. 一行代码伪装 Chrome

```go
package main

import (
    "fmt"
    "github.com/bogdanfinn/tls-client/profiles"
    "github.com/bogdanfinn/tls-client/tlsgateway"
)

func main() {
    client := tlsgateway.Impersonate(profiles.Chrome_150)
    resp, _ := client.Get("https://www.akamai.com/")
    fmt.Println(resp.StatusCode) // 200
}
```

## 3. 自动反序列化 + 重试 + 调试

```go
type User struct {
    Name string `json:"name"`
}

var user User
var apiErr struct{ Message string `json:"message"` }

resp, err := tlsgateway.ImpersonateRequest(profiles.Chrome_150).
    SetSuccessResult(&user).                         // 自动 JSON unmarshal
    SetErrorResult(&apiErr).                         // 错误时自动 unmarshal
    SetRetry(3, tlsgateway.RetryOnServerError,       // 重试 3 次
        1*time.Second, 10*time.Second).              // 退避 1s-10s
    SetDump(tlsgateway.DefaultDumpOptions()).        // 打印请求响应头
    Get("https://api.example.com/user")

// user.Name 已自动填充
fmt.Printf("User: %s, Trace: %v\n", user.Name, resp.Trace.TotalTime)
```

## 4. 链式 API 自定义

```go
client := tlsgateway.ImpersonateChain(profiles.Firefox_148).
    SetUserAgent("custom-bot/1.0").
    SetTimeout(30 * time.Second).
    SetDebug(os.Stderr).
    WithOrderedHeaders().   // Chrome 规范 header 顺序
    Build()
```

## 5. 选择画像

tlsgateway 内置 81 个预置画像：

```go
// Chrome
profiles.Chrome_150        // Chrome 150 (最新)
profiles.Chrome_146        // Chrome 146
profiles.Chrome_131        // Chrome 131

// Firefox
profiles.Firefox_148       // Firefox 148

// Safari
profiles.Safari_iOS_18_5   // Safari iOS 18.5

// 其他
profiles.Brave_146         // Brave
profiles.OkHttp4Android13  // OkHttp
```

## 6. 验证指纹

```go
// 检查当前 TLS 指纹
info, _ := tlsgateway.SelfCheck(profiles.Chrome_150)
fmt.Printf("JA3: %s\nJA4: %s\n", info.JA3Hash, info.JA4)

// 打印完整指纹
tlsgateway.DumpFingerprint(profiles.Firefox_148)
```

## 7. TransportMiddleware

```go
tr := tlsgateway.NewTransport(profiles.Chrome_150)

// 链式包装中间件
wrapped := tr.Wrap(
    tlsgateway.DebugMiddleware(func(format string, args ...any) {
        log.Printf("[tlsgateway] "+format, args...)
    }),
    tlsgateway.UserAgentMiddleware("my-crawler/1.0"),
)

client := &http.Client{Transport: wrapped}
```

## 下一步

- [TLS 指纹](tls-fingerprint.md) — 深入了解 TLS 指纹原理
- [HTTP 指纹](http-fingerprint.md) — H2 SETTINGS/Header 排序完整定制
- [API 速览](api.md) — 所有 API 一览
