# API 速览

## Client 创建

```go
// 简化 API（自动 TLS + HTTP 指纹）
client := tlsgateway.Impersonate(profile)            // *http.Client
req   := tlsgateway.ImpersonateRequest(profile)       // *Request
builder := tlsgateway.ImpersonateChain(profile)       // *ChainBuilder

// 链式 API
client := tlsgateway.ImpersonateChain(profile).
    SetUserAgent("custom/1.0").
    SetDebug(os.Stderr).
    SetTimeout(30 * time.Second).
    WithOrderedHeaders().
    Build()
```

## Transport

```go
tr := tlsgateway.NewTransport(profile)               // 默认 (x/net/http2)
tr := tlsgateway.NewFingerprintTransport(profile)     // 完整 H2 指纹

tr.SetProfile(newProfile)
tr.SetDebug(os.Stderr)
wrapped := tr.Wrap(
    tlsgateway.DebugMiddleware(logFunc),
    tlsgateway.UserAgentMiddleware("custom/1.0"),
)
```

## Request

```go
req := tlsgateway.ImpersonateRequest(profile)
req.SetHeader("Authorization", "Bearer xxx")
req.SetSuccessResult(&result)          // 2xx 自动 JSON unmarshal
req.SetErrorResult(&errResp)           // 非2xx 自动 unmarshal
req.SetDump(tlsgateway.DefaultDumpOptions())
req.SetRetry(3, condition, 1*time.Second, 10*time.Second)

resp, err := req.Get("https://api.example.com")
```

## Response

```go
resp.StatusCode
resp.BodyBytes()                    // 缓存响应体
resp.UnmarshalJson(&v)
resp.UnmarshalXml(&v)
resp.IsSuccess()                    // 2xx?
resp.SuccessResult()                // 自动填充的成功结果
resp.ErrorResult()                  // 自动填充的错误结果
resp.Trace.TotalTime                // 请求总耗时
resp.Trace.DNSLookupTime
resp.Trace.TCPConnectTime
resp.Trace.TLSHandshakeTime
resp.Trace.FirstResponseTime
resp.Trace.IsConnReused
```

## 指纹

```go
info, _ := tlsgateway.SelfCheck(profile)    // JA3+JA4
tlsgateway.DumpFingerprint(profile)
fp := tlsgateway.BrowserFingerprint(name)
// fp.Settings, fp.InitialStreamID, fp.ConnectionFlow, ...
```

## Debug & Dump

```go
tr.SetDebug(os.Stderr)
opts := tlsgateway.FullDumpOptions()
req.SetDump(opts)
```

## Retry

```go
tlsgateway.RetryOnServerError
tlsgateway.RetryOnAnyError
```

## Proxy

```go
proxy := tlsgateway.NewProxy(profile)
proxy.ListenAndServe(":8080")
```
