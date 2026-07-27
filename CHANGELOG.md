# Changelog

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
