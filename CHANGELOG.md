# Changelog

## v1.2.0 — 商用级 DLL API

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
