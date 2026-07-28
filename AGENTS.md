## 项目架构

本项目有两层：

| 层 | 位置 | 作用 |
|----|------|------|
| **tlsgateway**（主推荐） | `tlsgateway/`、`cmd/tlsgateway-proxy/` | 零 fork Transport + 代理 + 浏览器头注入 |
| **profiles**（共享） | `profiles/` | 81 预置画像、指纹验证 |

> **不依赖 bogdanfinn 上游库**：默认构建仅用 uTLS (Tor团队) + x/net/http2 (Go官方) + 标准库。无 fhttp、无 quic-go-utls。

### tlsgateway 架构

| 文件 | 职责 |
|------|------|
| `transport_h2.go` | Transport: H2 + H1.1 自动降级 + Debug |
| `transport_race.go` | RaceTransport: H2 vs H1.1 Protocol Racing |
| `impersonate.go` | Impersonate/ImpersonateChain/SelfCheck/DumpFingerprint |
| `header.go` | HeaderRoundTripper: 按画像注入浏览器 UA/Accept |
| `middleware.go` | TransportMiddleware: 链式包装 Debug/UA/Tracing |
| `proxy.go` | HTTP/HTTPS 正向代理 (CONNECT 隧道) |

### 快速使用

```go
// 一行代码伪装浏览器
client := tlsgateway.Impersonate(profiles.Chrome_150)
resp, _ := client.Get("https://www.akamai.com/")

// 链式 API
client := tlsgateway.ImpersonateChain(profiles.Firefox_148).
    SetDebug(os.Stderr).
    SetTimeout(10 * time.Second).
    Build()

// 指纹自检
info, _ := tlsgateway.SelfCheck(profiles.Chrome_150)
fmt.Printf("JA3: %s  JA4: %s\n", info.JA3Hash, info.JA4)

// TransportMiddleware
tr := tlsgateway.NewTransport(p)
wrapped := tr.Wrap(
    tlsgateway.DebugMiddleware(logf),
    tlsgateway.UserAgentMiddleware("custom/1.0"),
)
```

## profiles — 共享画像层

81 个预置画像覆盖 Chrome/Firefox/Safari/Brave/Opera/OkHttp/移动客户端。

### 画像更新来源

上游 `bogdanfinn/tls-client` 发布新浏览器版本画像时，提取其 TLS ClientHello 参数更新到本项目。方式：

1. 提取上游新画像的 TLS ClientHello 参数 → 更新 `profiles/internal_browser_profiles.go`
2. `go run ./cmd/export-profiles -o profiles.json`
3. `go test -count=1 -run 'TestAllMappedProfilesProduceValidClientHellos' ./profiles`
4. 更新 `profiles/metadata.go`（TLSBase、KnownGaps、VerifiedAgainst）

### 画像行为不变量

- `ResolveClientProfileStrict` 对未知画像返回 error，不退化为默认
- `random` / `chaos` 从真实注册画像选择，排除业务定制、PSK 变体、KnownGaps 非空
- All Getter 返回防御性副本
- JSON 画像支持热加载

### profiles 测试

```bash
go test -race -count=1 ./profiles
go test -race -count=1 -run 'TestAllMappedProfiles' ./profiles
```

## tlsgateway — 轻量层（主推荐）

### 设计原则

- **零 fork 依赖**
- 标准兼容：实现 `http.RoundTripper`
- 自动降级：H2 → H1.1（uTLS withForceHttp1）
- HeaderRoundTripper 解决 Akamai HTTP 层检测

### 测试

```bash
go test -race -count=1 ./tlsgateway                    # 全部 14 项
go test -race -count=1 -run TestStress -v ./tlsgateway # 压力测试 6 项
```

## 编码规范

- error 不以标点结尾，不以大写开头 (Go 惯例)
- map/slice/pointer Getter 返回防御性副本
- 画像注册表读写使用独立锁

## 指纹验证 — 全平台人机检测 (强制质量门禁)

**任何画像新增/修改/版本升级后，必须运行全平台验证。**

### 验证命令

```bash
go run ./cmd/verify-fingerprints
go run ./cmd/verify-fingerprints -all
go run ./cmd/verify-fingerprints -all -json > fingerprints_$(date +%Y%m%d).json
```

### 验证平台 (12 个)

| # | 平台 | 类型 | 检测维度 |
|---|------|------|---------|
| 1 | tls.peet.ws | TLS API | JA3、JA4 |
| 2 | browserleaks.com | TLS API | JA3、JA3N |
| 3 | browserscan.net | TLS API | 指纹页面加载 |
| 4 | cloudflare.com | WAF/CDN | TLS + HTTP/2 |
| 5 | imperva.com | WAF/CDN | 企业级 WAF |
| 6 | f5.com | WAF/CDN | Shape Security |
| 7 | akamai.com | WAF/CDN | H2 + HTTP 头检测 |
| 8 | datadome.co | WAF/CDN | JS 行为分析 |
| 9 | hcaptcha.com | WAF/CDN | 人机验证 |
| 10 | recaptcha-demo | WAF/CDN | Google reCAPTCHA |
| 11 | sannysoft.com | WAF/CDN | 综合检测 |
| 12 | httpbin.org | HTTP | 连通性 |

### 质量门禁

| 平台 | 最低通过率 | 不通过时的处理 |
|------|-----------|---------------|
| tls.peet.ws | **100%** | JA3/JA4 必须唯一 |
| browserleaks.com | **100%** | JA3/JA3N 交叉一致 |
| browserscan.net | **100%** | 页面加载正常 |
| cloudflare.com | **100%** | 零拦截 |
| imperva.com | **100%** | 零拦截 |
| f5.com | **100%** | 零拦截 |
| hcaptcha.com | **100%** | 页面可加载 |
| recaptcha-demo | **100%** | 页面可加载 |
| sannysoft.com | **100%** | PASS |
| akamai.com | ≥0% | TLS通过→403。HeaderRoundTripper 可处理 |
| datadome.co | 0% | JS 引擎必需，非 TLS 限制 |
| httpbin.org | ≥50% | 外部限流 |

**判定规则**：
- **通过**：tls.peet.ws + browserleaks + browserscan + cloudflare + imperva + f5 + hcaptcha + recaptcha + sannysoft 全部 100%
- **阻塞**：任一 100% 平台 JA3/JA4 为空或与其他平台不一致

### 当前验证基线 (v1.7.x)

```
TLS APIs:        tls.peet.ws ✅  browserleaks ✅  browserscan ✅  3/3
WAF/CDN:         cloudflare ✅  imperva ✅  f5 ✅  hcaptcha ✅  
                 recaptcha ✅  sannysoft ✅                  6/8
                 akamai ⚠️ (HeaderRoundTripper)  datadome ⚠️ (JS)
TLS 层通过率: 9/9 核心平台 (100%)
```

### 人机验证不变量

- 每次新增画像 → 全平台验证
- 每次升级 uTLS → 全平台回归
- 每次修改 profiles/ → 全平台回归
- 发版前 → 全量 81 画像 + 全平台
- 验证结果附在 CHANGELOG
