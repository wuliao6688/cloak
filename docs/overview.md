# 项目概览

## 是什么

tls-client 是一个 Go 编写的 HTTP 客户端库，核心能力是**模拟真实浏览器的网络指纹**。
当你的程序用 `net/http` 发请求时，服务端可以通过 TLS 握手、HTTP/2 SETTINGS、
HTTP/3 QUIC 参数等特征识别出"这不是浏览器"。tls-client 让这些特征与真实浏览器
**逐字节一致**。

## 解决什么问题

| 场景 | 没有 tls-client | 有 tls-client |
|---|---|---|
| 抓取 Akamai/Cloudflare 保护的站点 | 403 / 1020 / JS 挑战 | 正常 200 |
| 批量注册/登录自动化 | 请求被风控识别 | 与浏览器无异 |
| 移动 App 数据对接 | ClientHello 暴露 Go/curl | 完全对齐 OkHttp/Safari |
| 高并发采集 | TLS 指纹不一致触发限流 | 指纹稳定 |

## 三层指纹

```
┌─────────────────────────────────────────────────────┐
│  HTTP/3 (QUIC)        ← UQUICClient + H3 SETTINGS   │
│    TLS over QUIC      ← 浏览器 ClientHello 注入      │
├─────────────────────────────────────────────────────┤
│  HTTP/2                ← SETTINGS/伪头/优先级/GREASE │
│    TLS 1.3             ← uTLS 浏览器指纹             │
├─────────────────────────────────────────────────────┤
│  HTTP/1.1              ← 头排序/UA/规范             │
│    自动协商 H2/H3      ← Chrome 式协议赛跑          │
└─────────────────────────────────────────────────────┘
```

## 设计哲学

1. **零 fork 依赖**：默认构建只用 `uTLS` + `golang.org/x/net` + 标准库。
   （H3 的 QUIC 引擎 fork 在 `third_party/`，核心层不依赖 fhttp。）

2. **一体式 Transport**：TLS + HTTP 头注入 + 协议协商都在一个 `Transport` 里，
   不需要外部 HeaderRoundTripper 包装。

3. **req 风格 API**：链式 Request builder、自动反序列化、条件重试、中间件、
   ResultState 全部借鉴 req 的成熟设计。

4. **验证优先**：每个能力都有测试（`go test -race`）+ 外部平台验证
   （`cmd/verify-fingerprints`，14 平台）。

## 能力矩阵

### 核心

- **TLS 指纹**：77 画像（Chrome 103-150 / Firefox 102-148 / Safari 15-26 /
  Brave / Opera / OkHttp / 移动端定制）
- **HTTP/2 指纹**：SETTINGS、伪头顺序、优先级帧、ConnectionFlow、StreamID、GREASE
- **HTTP/3 指纹**：QUIC TLS ClientHello 注入（UQUICClient）+ H3 SETTINGS / GREASE /
  Priority / 伪头顺序
- **协议赛跑**：H3 vs H2 并行、域名级缓存、自动降级（Chrome Happy Eyeballs）

### 便捷

- 浏览器头自动注入（UA / Accept / Sec-CH-UA / Accept-Language）
- Header 排序（Chrome / Firefox / Safari 规范顺序）
- 自动 JSON 反序列化（`SetSuccessResult` / `SetErrorResult`）
- 条件重试 + 指数退避（`SetRetry`）
- 请求/响应中间件（`OnRequest` / `OnResponse` / `MiddlewareChain`）
- 响应落盘（`SetOutputFile`）
- 调试模式（`DevMode` / `SetDebug` / `SetDump`）
- 正向代理（HTTP / HTTPS CONNECT 隧道）
- TraceInfo（DNS/TCP/TLS/FirstByte 七点计时）

### 稳定

- 并发安全（100 并发 × 50 轮 0 失败、零 goroutine 泄漏）
- 超时正确性（500ms 超时精确触发）
- gzip/重定向/代理正确性（客户场景回归测试）

## 与生态对比

| | tls-client | 上游 bogdanfinn | curl_cffi | imroc/req |
|---|---|---|---|---|
| 语言 | Go | Go | Python | Go |
| H3 QUIC TLS 指纹 | ✅ | ❌ | ❌ | ❌ |
| 移动画像 | ✅ 18 个 | ❌ | ❌ | ❌ |
| 零 fork | ✅ | ❌ | ❌ | ✅ |
| 代理 | ✅ 内置 | ❌ | ✅ | ✅ |
