# AGENTS.md

## 项目架构

本项目有三层：

| 层 | 位置 | 作用 |
|----|------|------|
| **tlsgateway**（主推荐） | `tlsgateway/`、`cmd/tlsgateway-proxy/` | 分层 Transport（默认零 fork + 按需激活 H3/fhttp）+ 本地代理 |
| **完整 tls-client**（Fork 版本） | 根目录、`cffi_src/`、`cffi_dist/` | 完整协议控制（H3、Racing、CFFI）。依赖 fhttp fork。 |
| **profiles**（共享） | `profiles/` | 三层共用：画像定义、解析、JSON 加载、热加载、指纹验证。 |

### tlsgateway 分层架构 (v1.7.0)

| 文件 | 构建 | fork 数 | 用途 |
|------|------|---------|------|
| `transport_h2.go` | 默认 | **0** | uTLS + x/net/http2 — H2 + H1.1 自动降级 |
| `transport_race.go` | 默认 | **0** | H2 vs H1.1 Protocol Racing (Happy Eyeballs) |
| `transport_h3.go` | `-tags h3` | 1 (quic-go-utls) | HTTP/3 (QUIC) — QUIC 协议内嵌 TLS，无法零 fork |
| `transport_fhttp.go` | `-tags fhttp` | 1 (fhttp) | Akamai 级 H2 SETTINGS/PRIORITY 定制 |

默认 `go build` = 零 fork（仅 uTLS Tor团队 + x/net/http2 Go官方 + 标准库）。

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
| `tlsgateway/proxy.go` | 301 | HTTP/HTTPS 正向代理（CONNECT 隧道）。**TLS 证书默认验证，`-insecure` flag 跳过。** |
| `cmd/tlsgateway-proxy/main.go` | 85 | CLI 入口（`-addr`、`-profile`、`-insecure`、`-profiles`）
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

> **设计分层**：绝大多数场景用 tlsgateway（217 行、零 fork 依赖）。Fork 版仅当需要 H3/Racing/CFFI 时使用。

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

## 指纹验证 — 全平台人机检测 (强制质量门禁)

**任何画像新增/修改/版本升级后，必须运行全平台验证并通过以下门槛。**

### 验证命令

```bash
# 关键画像快速验证（每次改画像必跑）
go run ./cmd/verify-fingerprints -profiles "chrome_150,firefox_148,safari_ios_18_5"

# 全量 81 画像验证（发版前必跑）
go run ./cmd/verify-fingerprints -all

# JSON 输出存档
go run ./cmd/verify-fingerprints -all -json > fingerprints_$(date +%Y%m%d).json
```

### 验证平台 (11 个)

| # | 平台 | 类型 | 检测维度 |
|---|------|------|---------|
| 1 | tls.peet.ws | TLS API | JA3、JA4、密码套件、扩展 |
| 2 | browserleaks.com | TLS API | JA3、JA3N、Akamai 指纹 |
| 3 | cloudflare.com | WAF/CDN | TLS + HTTP/2 指纹 |
| 4 | imperva.com | WAF/CDN | 企业级 WAF |
| 5 | f5.com | WAF/CDN | Shape Security 反自动化 |
| 6 | akamai.com | WAF/CDN | **H2 指纹 + HTTP 头检测** |
| 7 | datadome.co | WAF/CDN | JS 行为分析（需 JS 引擎） |
| 8 | hcaptcha.com | WAF/CDN | 人机验证页面加载 |
| 9 | recaptcha-demo | WAF/CDN | Google reCAPTCHA 加载 |
| 10 | sannysoft.com | WAF/CDN | 综合机器人检测 |
| 11 | httpbin.org | HTTP | 基础 HTTP 连通性 |

### 质量门禁 — 画像有效性判定

| 平台 | 最低通过率 | 不通过时的处理 |
|------|-----------|---------------|
| tls.peet.ws | **100%** | JA3/JA4 必须唯一有效 |
| browserleaks.com | **100%** | JA3/JA3N 必须交叉验证一致 |
| cloudflare.com | **100%** | 零拦截，HTTP/2 + TLSv1.3 |
| imperva.com | **100%** | 零拦截 |
| f5.com | **100%** | 零拦截 |
| akamai.com | ≥0%（非TLS层面） | TLS通过→403。需 HTTP header 伪装（UA/Accept等） |
| datadome.co | 0%（预期失败） | JS 引擎 + 行为模拟. 非 TLS 限制 |
| hcaptcha.com | **100%** | 人机验证页面可加载 |
| recaptcha-demo | **100%** | Google reCAPTCHA 页面可加载 |
| sannysoft.com | **100%** | 机器人检测 PASS |
| httpbin.org | ≥50% | 503 为外部限流，非指纹问题 |

**判定规则**：
- **通过**：tls.peet.ws + browserleaks + cloudflare + imperva + f5 全部 100%
- **阻塞**：任一个 100% 平台出现 JA3/JA4 为空或与其他平台不一致
- **TLS 层已知限制**：akamai 返回 403（HTTP 头层面，非 TLS），datadome 需要 JS

### 当前验证基线 (v1.7.x)

```
画像数: 10关键画像
────────────────────────────────────────────
tls.peet.ws:       10/10 ✅  JA3/JA4 全部唯一
browserleaks.com:  10/10 ✅  JA3+JA3N 交叉一致
cloudflare.com:    10/10 ✅  TLSv1.3+HTTP/2 零拦截
imperva.com:       10/10 ✅  零拦截
f5.com:            10/10 ✅  零拦截
akamai.com:         0/10 ⚠️  TLS 通过(403),需 HTTP header 伪装
datadome.co:        0/10 ⚠️  JS challenge (需 JS 引擎)
httpbin.org:        受外部限流
────────────────────────────────────────────
TLS 层通过率: 50/50 (100%) — 5核心平台零拦截
```

### 指纹唯一性约束

- 不同浏览器的 JA3 Hash 必须不同（Chrome ≠ Firefox ≠ Safari）
- 同浏览器相邻版本的 JA3 可相同（如 Chrome 109 = Opera 91 共享密码套件）
- JA4 指纹必须包含浏览器标识段（`t13d1516h2` = Chrome, `t13d1917h2` = Firefox）
- 密码套件列表必须与浏览器声明版本一致

### 验证失败时的处理流程

1. **JA3/JA4 为空或格式异常** → 检查 `isProtocolError` 覆盖范围，可能 H2 握手失败未被识别
2. **tls.peet.ws 通过但 browserleaks 失败** → 检查 JA3N 计算差异（SNI 影响）
3. **Cloudflare/Imperva 出现拦截** → 检查 uTLS ClientHelloID 是否正确映射
4. **Akamai 突然通过** → 可能是 H2 指纹缓存问题，需多次验证确认
5. **所有平台同时失败** → 检查网络连通性，验证工具自身功能

### 人机验证不变量

- 每次新增画像 → 必须全平台验证
- 每次升级 uTLS 依赖 → 必须回归全平台
- 每次修改 `profiles/` 代码 → 必须回归全平台
- 发版前 → 必须全量 81 画像验证 + 全平台
- **验证结果必须附在 CHANGELOG 中**（格式：`tls.peet.ws 10/10, cloudflare 10/10, akamai 0/10`）
