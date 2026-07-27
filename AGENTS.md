# AGENTS.md

## 项目架构

本项目有两层：

| 层 | 位置 | 作用 |
|----|------|------|
| **tlsgateway**（主推荐） | `tlsgateway/`、`cmd/tlsgateway-proxy/` | 轻量 Transport + 本地代理。依赖仅为 uTLS + 标准库。 |
| **完整 tls-client**（Fork 版本） | 根目录、`cffi_src/`、`cffi_dist/` | 完整协议控制（H3、Racing、CFFI）。依赖 fhttp fork。 |
| **profiles**（共享） | `profiles/` | 两层共用：画像定义、解析、JSON 加载、热加载、指纹验证。 |

## profiles — 共享画像层

这是两层共用的核心资产。81 个预置画像覆盖 Chrome/Firefox/Safari/Brave/Opera/OkHttp/移动客户端。

### 画像更新来源

上游 `bogdanfinn/tls-client` 发布新浏览器版本画像时，提取其 TLS ClientHello 参数（密码套件、扩展、H2/H3 settings）更新到本项目。方式：

1. **提取上游新画像的 TLS/HTTP2/HTTP3 参数** → 更新 `profiles/internal_browser_profiles.go`
2. **导出 JSON** → `go run ./cmd/export-profiles -o profiles.json`
3. **运行验证** → `go test -count=1 -run 'TestAllMappedProfilesProduceValidClientHellos' ./profiles`
4. **更新 metadata** → `profiles/metadata.go`（TLSBase、KnownGaps、VerifiedAgainst）

### 画像行为不变量

- `ResolveClientProfileStrict` 对未知画像返回 `ErrUnknownClientProfile`（不退化为默认）
- `random` / `chaos` 从真实注册画像选择，排除业务定制（zalando/nike/cloudscraper/mms/mesh/confirmed）、`_PSK` 变体、`KnownGaps` 非空画像
- 所有 Getter 返回防御性副本，调用方不能污染全局注册表
- JSON 画像支持热加载（`profiles.Watcher`）
- JA3 指纹在序列化/反序列化后等价（GREASE 随机化不影响语义验证）

### profiles 测试

```bash
go test -race -count=1 ./profiles                                    # 全部
go test -race -count=1 -run 'TestAllMappedProfiles' ./profiles      # 81画像验证
go test -race -count=1 -run 'TestProfileJSON' ./profiles            # JSON往返
go test -race -count=1 -run 'TestWatcher' ./profiles                # 热加载
```

## tlsgateway — 轻量层（主推荐）

### 设计原则

- **不 fork**：使用 `net/http.Transport` + uTLS `DialTLSContext` hook
- **标准兼容**：实现 `http.RoundTripper`，任何 `http.Client` 可用
- **最简依赖**：仅 `utls` + `golang.org/x/net/http2` + 标准库

### 代码位置

| 文件 | 行数 | 职责 |
|------|------|------|
| `tlsgateway/transport.go` | 217 | `http.RoundTripper` 实现（H2 + H1 fallback） |
| `tlsgateway/proxy.go` | 301 | HTTP/HTTPS 正向代理（CONNECT 隧道） |
| `cmd/tlsgateway-proxy/main.go` | 79 | CLI 入口 |
| `cmd/verify-fingerprints/main.go` | 257 | 在线指纹验证工具 |

### tlsgateway 测试

```bash
go test -race -count=1 -run 'TestTransport' ./tlsgateway            # Transport (8 tests)
go test -race -count=1 -run 'TestProxy' ./tlsgateway               # 代理 (5 tests)
```

### profiles 测试

```bash
go test -race -count=1 ./profiles                                   # 全部 29 tests
```

## 完整 tls-client（Fork 版本）

### 定位

仅在需要以下功能时维护：HTTP/3、定制 H2 SETTINGS/优先级帧、Protocol Racing、C shared library (CFFI)。

### 关键文件

| 文件 | 职责 |
|------|------|
| `client.go` | HTTP 客户端与动态状态 |
| `roundtripper.go` | TLS 握手、Transport 缓存、H2 握手 |
| `racer.go` | HTTP/3 与 HTTP/2 Protocol Racing |
| `cffi_src/factory.go` | CFFI session 管理 |

### Fork 版本行为不变量

- 动态代理切换使用状态快照，不能复制活跃 `http.Client`
- Transport 按目标单飞初始化，不同目标并行
- Racing 仅 GET/HEAD/OPTIONS 参与；POST/PUT 等不得重复发送
- CFFI session 租约覆盖完整 request-response 生命周期；release 幂等
- `cffi_dist/go.mod` 必须保留 `replace ... => ../`

## 高并发压力测试

```bash
go test -race -count=1 -run '^TestStress' -timeout 120s .
```

13 项测试覆盖：客户端并发创建（200 goroutines × 10000 clients）、Transport 缓存压力、画像解析、Goroutine 泄露检测、KeyedLock 竞态。uTLS `ClientHelloID.ToSpec()` 在 ≥100 goroutines 并发下存在已知竞态（库层面限制），调用方必要时自行加锁。

## 全量验证

```bash
go vet ./... && go build ./...
go test -race -count=1 . ./profiles ./bandwidth ./tlsgateway
go test -race -count=1 -run '^TestStress' -timeout 120s .           # 高并发
```

## 编码规范

- error 字符串不以标点结尾，不以大写开头（Go 惯例）
- map/slice/pointer 字段的 Getter 返回防御性副本
- 不在锁内执行网络 I/O 或关闭连接
- 画像注册表读写使用独立锁，避免锁升级/递归
