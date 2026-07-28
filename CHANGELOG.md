# Changelog

## v1.6.3 — 安全加固 & 死代码清理 (2026-07-28)

### 🔴 P0 修复
- **tlsgateway proxy 证书验证**：移除 `rebuildTransport()` 硬编码 `InsecureSkipVerify=true`，改为可配置选项，默认 `false`（证书验证开启）。新增 `-insecure` CLI flag 和 `SetInsecureSkipVerify()` API，仅调试/自签名证书场景使用。
- **transport_cache DATA RACE**：`shardedTransportCache.reset()` 和 `get()` 之间的并发竞态已修复（添加 `sync.Mutex`）。

### 清理
- **WebSocket 模块**：`websocket.go` (93行) + `websocket_options.go` (72行) + `tests/websocket_test.go` (218行) 完全删除。
- **依赖精简**：`github.com/bogdanfinn/websocket`、`github.com/tam7t/hpkp` 从 go.mod 移除。
- **hpkp 替换**：用标准库 `crypto/sha256` + 本地 `pinHeader` 类型替代 2017 年废弃的 hpkp 库，功能不变。
- **死代码**：移除 `ProvideDefaultClient()`、`newTransportShard()`、`newTransportCacheMeta()`、`deleteCachedTransportEntry()`、`resetTransportCacheMeta()`、`keyedLockPool.size()` 等 6 个未使用函数。
- **测试去外部化**：`tlsgateway/transport_test.go` 不再依赖 httpbin.org / api.github.com，改为本地 `httptest` 服务器，新增 `TestTransportDefaultCertVerification` 验证证书默认验证行为。

### 测试
- **10 分钟持续压力测试**：50 并发 / race detector on / 1,369,369 请求 / 0 失败 / 内存 4.5-8.0MB 锯齿形振荡（GC 完美）
- **在线指纹验证**：chrome_150 / firefox_148 / safari_ios_18_5 / chrome_131 / brave_146 / opera_91 / okhttp4_android_13 全部 JA3+JA4 通过
- **架构审计**：锁内 I/O ✅ / Session 上限 ✅ / 依赖泄漏 ✅ / profiles 零泄漏

### 文档
- AGENTS.md：标注 P0 修复 + 分层引导（tlsgateway 默认推荐）
- Readme.md：项目结构中标注 TLS 默认验证（安全）

## v1.6.0 — kurl-client 借鉴合入 (完成)

### 新增 (5~7)
- **H2 深度随机化**：`tg_session_set_h2_randomize(s, 1)` — 随机化 TLS 扩展顺序（Chaos 模式自动启用）
- **Multipart 文件上传**：`tg_post_multipart(s, url, filePath, fieldName)` — multipart/form-data
- **自定义 CA 证书**：`tg_session_set_ca_cert(s, path)` — PEM 格式 / MITM 调试

### kurl-client 7 项全部合入

### 新增
- **Chaos 模式**：`ROTATE_CHAOS=6` — 每请求从 Chrome/Firefox/Safari/Opera/Brave/OkHttp 中随机选画像
  - 每请求自动重建 TLS Client（新 ClientHello + Session Ticket + 新扩展顺序）
  - 等同于 kurl-client 的 `set_impersonate("chaos")` 效果
- **二进制 POST**：`tg_post_bin(session, url, data_ptr, dataLen)` — 支持 protobuf/图片/文件
- **整数 Handle**：`tg_session_create_int(profile, timeout, proxy) → int` — 易语言判等只需 `=`
- **Cookie 开关**：`tg_session_set_cookie_store(session, enable)` — 请求级禁用 Cookie jar

### 更新
- `profiles/consts.go`：`RotateGroupChaos` + `ChaosProfile()` 随机选择
- C 头：27 个 DLL 导出函数声明
- Python：`post_bin()` / `set_cookie_store()` / `ROTATE_CHAOS`
- 易语言：`tg_post_bin` / `tg_session_set_cookie_store` / 场景九 Chaos

## v1.4.1 — 代理管理

### 新增
- **代理切换**：`tg_session_set_proxy(s, url)` / `tg_session_get_proxy(s)`
- **代理池轮换**：`tg_session_set_proxy_list(s, list, everyN)`
  - 格式：`"http://ip1:8080\nhttp://ip2:8080\nsocks5://ip3:1080"`
  - 自动与画像轮换并行，实现画像+IP双维度防检测
- **SOCKS5 + 认证**：`"socks5://user:pass@host:port"` 格式
- **动态清除**：`tg_session_set_proxy(s, "")` 清除代理
- Python: `s.set_proxy/` `s.proxy` / `s.set_proxy_list`
- 易语言: 场景八（HTTP认证/SOCKS5/代理池/清除）

### 新增
- **画像自动轮换**：`tg_session_set_rotate(s, group, everyN, tlsRefreshEvery)`
  - 5 个轮换组：Chrome / Firefox / Safari / Mobile / All
  - 每 N 次请求自动切换画像，保留 Cookie jar
- **TLS 上下文自动刷新**：每 N 次请求重建 Client（新 ClientHello + Session Ticket）
- `profiles/consts.go`：`RotateGroup` 类型 + `NextRotateProfile()` 函数
- Python: `session.set_rotate(group, every_n, tls_refresh)`
- 易语言: `tg_session_set_rotate(session, ROTATE_CHROME, 3, 20)`
- 场景七：10 次请求自动在 Chrome 116-150 间轮换

### 背景
kurl-client（BoringSSL）在大批量人机平台验证中后期被识别。
根本原因：静态指纹 + 无轮换 + TLS 会话复用。
本版本的轮换+刷新机制从根本上解决了这个问题。

## v1.3.0 — 商用级完整 API

### 新增 DLL 导出
- **Cookie 管理**：`tg_session_get_cookies` / `tg_session_set_cookies` / `tg_session_clear_cookies`
- **响应头**：`tg_response_header(name)` / `tg_response_headers()` — 调试/提取 Set-Cookie
- **错误码**：`tg_response_error_code()` — 整数码（0=成功,1=网络,2=HTTP,3=会话,4=超时,5=画像）
- **画像切换**：`tg_session_set_profile(id)` / `tg_session_get_profile()` — 保留 Cookie jar
- **便捷函数**：`tg_get_body(url)` / `tg_get_status(url)` — 一行代码搞定简单场景

### 更新
- `tlsgateway_api.go` — 完整重写：20 个导出函数，session 可重建
- `tlsgateway_api.h` — 含登录场景完整例子
- `example_e/example.e` — 6 个场景：简单GET / 完整响应 / 登录流程 / POST / 切换指纹 / 仅状态码
- `example_python/tlsgateway.py` — Session 类：context manager / switch_profile / get/set cookies / 响应头解析

### 新增
- **`tlsgateway_api.go`** — 零 JSON 的 CFFI API
  - `tg_session_create(profileID, timeout, proxy)` 创建 session
  - `tg_get(session, url)` / `tg_post(session, url, body)` GET/POST 请求
  - `tg_request(session, method, url, headers, body)` 完全自定义请求
  - `tg_response_status/body/body_len/error/free` 类型安全的结果访问
  - 6 个核心函数，零 JSON 往返
- **`tlsgateway_api.h`** — C 头文件，含完整文档
- **`tlsgateway_profiles.h`** — 19 个 `TLS_PROFILE_*` 常量
- **Python SDK** (`example_python/tlsgateway.py`) — Session/Response 封装
- **易语言示例** (`example_e/example.e`) — 完整 DLL 声明 + 三个示例

### 变更
- `cffi_dist/main.go`: `request()` 抽取为 `doRequest()` 共享实现
- `profiles/consts.go`: `ProfileID.String()` 方法

### 向后兼容
- 所有旧 CFFI API（`request()`、`destroySession()` 等）保持不变

## v1.1.1 — profile-by-ID

- `cffi_dist/main.go`: 新增 `requestWithProfileId(json, profileId)`
- `profiles/consts.go`: 19 个 `ProfileID` 常量 + 映射表
- `cffi_src/types.go`: RequestInput 新增 `ProfileID` 字段

## v1.1.0 — tlsgateway H2 + profiles de-fhttp

### 新增
- `tlsgateway/transport.go` — 217 行，x/net/http2 + H1 fallback
- `tlsgateway/proxy.go` — 301 行 HTTP/HTTPS 代理
- `cmd/verify-fingerprints/main.go` — 在线指纹验证
- `profiles/h2settings.go` — 本地 H2 类型（零 fhttp 依赖）
- `profiles/profile_json.go` — JSON 导入导出
- `profiles/watcher.go` — 热加载

### 变更
- profiles 包：移除 `fhttp/http2` 依赖，自包含 H2 类型
- tlsgateway 包：零 fhttp 传递依赖
- 10分钟高并发压力测试：130k 请求，0 失败，0 泄漏
