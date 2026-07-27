# PROJECT_STATE.md

> 最后更新: 2026-07-27

## 项目定位

三层架构，从轻到重：

| 层 | 定位 | 代码量 |
|----|------|--------|
| **tlsgateway** | 轻量 TLS Transport + 本地代理。仅需 uTLS + x/net/http2。 | ~600 行 |
| **完整 tls-client (Fork)** | fhttp fork 全协议控制：H2 SETTINGS定制、H3、Protocol Racing。 | ~5000 行 |
| **profiles** | 两层共用：81 画像定义、JSON 热加载、指纹验证。 | ~800 行（新增）|

## 当前状态

| 维度 | 状态 |
|------|------|
| 编译 | ✅ `go build ./...` `go vet ./...` 全绿 |
| tlsgateway 测试 | ✅ Transport 8 + Proxy 5，全部 race-clean |
| tlsgateway H2 | ✅ x/net/http2 + H1 fallback |
| profiles 测试 | ✅ 82 画像验证 + JSON 往返 + 热加载 |
| 指纹验证 | ✅ tls.peet.ws 在线验证，11/11 核心画像 JA3/JA4 正确 |
| Fork 版本测试 | ✅ race tests 全通过 |
| 高并发压力 | 🔄 测试中（10分钟，50 并发） |

## 本次会话完成的工作

### H2 集成
- `tlsgateway/transport.go` — 217 行，`x/net/http2` + H1 自动 fallback
- 修复: 不再用 `forceHTTP1`，uTLS ALPN 协商后由 H2 transport 处理
- 修复: 检测 H2 协议错误后自动降级 HTTP/1.1+TLS

### CFFI 决策
- 删除 `cmd/tlsgateway-cffi/`（新建的轻量 C ABI）
- **保留 `cffi_src/` 原版（1744 行）** — session 模型 + 连接池 + 完整生命周期

### 指纹验证
- `cmd/verify-fingerprints/main.go` — 在线指纹验证工具
- 11 个核心画像通过 tls.peet.ws 验证，JA3/JA4 全部正确
- 指纹家族分组合理（Chrome/FF/Safari/Opera/OkHttp 正确聚类）

### 并发修复
- `profiles/watcher.go` — Stop 后 ReloadNow 不 panic
- `profiles/registry_mutex.go` — 全局 RWMutex 保护画像注册表
- 死锁修复: `isRandomBrowserProfileKey` 在读锁内暂停时直接读 metadata map

### 代码质量
- 死代码删除: `newProtocolRacer`, `handleClientError`
- 清理: `cmd/tlsgateway-cffi/`, `fingerprint_verification_report.json`
- `go mod tidy` 清理无用依赖

## 已知限制

| 限制 | 说明 |
|------|------|
| uTLS ToSpec() 竞态 | ≥100 goroutines 并发（上游库层面） |
| tlsgateway H2 SETTINGS | 不可定制（Go 默认），需 Fork 版本 |
| tlsgateway H3/Racing | 不支持，需 Fork 版本 |
| 原 CFFI session 池 | 无状态化替代（不再做轻量 C ABI） |
