# 快速开始

5 分钟上手 tls-client。核心入口就三个：`Impersonate`（标准）、`ImpersonateH3`（QUIC）、
`ImpersonateRequest`（链式 builder）。

## 1. 安装

```bash
go get github.com/bogdanfinn/tls-client
```

需要 Go 1.26+（项目在 Go 1.26.5 上开发测试）。

## 2. 一行代码伪装浏览器

```go
package main

import (
	"fmt"

	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/bogdanfinn/tls-client/tlsgateway"
)

func main() {
	// 用 Chrome 150 的完整指纹（TLS + HTTP/2）
	client := tlsgateway.Impersonate(profiles.Chrome_150)

	resp, err := client.Get("https://www.akamai.com/")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	fmt.Println(resp.StatusCode) // 200
}
```

选画像：`profiles.Chrome_150` / `Firefox_147` / `Safari_IOS_18_0` /
`Okhttp4Android13` / `Brave_146`……共 77 个（见 [画像体系](profiles.md)）。

## 3. HTTP/3（QUIC）模式

```go
// H3 racing：优先 HTTP/3，服务器不支持 QUIC 时自动降级 HTTP/2
client := tlsgateway.ImpersonateH3(profiles.Chrome_150)

resp, _ := client.Get("https://www.cloudflare.com/cdn-cgi/trace")
// 响应体里 http=http/3 表示成功走了 QUIC
```

## 4. 链式 builder（推荐）

```go
client := tlsgateway.ImpersonateChain(profiles.Chrome_150).
	EnableH3().                    // 启用 H3 racing（可选）
	SetUserAgent("Mozilla/5.0 ..."). // 覆盖 UA
	SetHeader("X-Custom", "v1").   // 加自定义头
	SetTimeout(15 * time.Second).  // 超时
	SetDebug(os.Stderr).           // 打印请求/响应
	Build()
```

## 5. 请求级 API：反序列化 + 重试 + 调试

```go
type User struct {
	Name string `json:"name"`
}
var user User
var apiErr struct{ Message string `json:"message"` }

resp, err := tlsgateway.ImpersonateRequest(profiles.Chrome_150).
	SetSuccessResult(&user).                        // 2xx 自动 JSON → user
	SetErrorResult(&apiErr).                        // 非 2xx 自动 JSON → apiErr
	SetRetry(3, tlsgateway.RetryOnServerError,      // 重试 3 次
		1*time.Second, 10*time.Second).          // 退避 1s-10s
	SetDump(tlsgateway.DefaultDumpOptions()).       // 打印请求/响应
	Get("https://api.example.com/user")

fmt.Println(user.Name)          // 已自动填充
fmt.Println(resp.Trace.TotalTime) // DNS/TCP/TLS/FirstByte 计时
```

## 6. POST / 表单 / 文件

```go
// JSON body
resp, _ := tlsgateway.ImpersonateRequest(profiles.Chrome_150).
	SetBodyString(`{"name":"alice"}`).
	SetHeader("Content-Type", "application/json").
	Post("https://api.example.com/users")

// 有序表单（表单字段顺序也是浏览器指纹的一部分）
resp, _ = tlsgateway.ImpersonateRequest(profiles.Chrome_150).
	SetOrderedFormData("username", "alice", "password", "secret").
	Post("https://api.example.com/login")
```

## 7. 代理

```go
// 出站代理：HTTP 代理转发（通过 TransportOptions 设置）
proxy := func(req *http.Request) (*url.URL, error) {
	return url.Parse("http://user:pass@proxy.example.com:8080")
}
tr := tlsgateway.NewTransportWithOptions(profiles.Chrome_150, tlsgateway.TransportOptions{
	Proxy: proxy,
})
client := &http.Client{Transport: tr, Timeout: 30 * time.Second}

// 入站代理：把 tls-client 变成本地正向代理服务
p := tlsgateway.NewProxy(":8080", profiles.Chrome_150)
go p.ListenAndServe()
```

## 8. 自检指纹

```go
info := tlsgateway.ImpersonateChain(profiles.Chrome_150).
	Transport().SelfCheck("https://tls.peet.ws/api/all")
fmt.Println(info.JA4) // 例如 t13d1516h2_8daaf6152771_...
```

## 下一步

- [API 参考](api.md) — 全部公开 API
- [指纹体系](fingerprint.md) — 三层指纹原理
- [HTTP/3 指南](h3.md) — QUIC 详解
- [验证矩阵](verification.md) — 平台与客户场景验证
