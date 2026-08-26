# 🔴 自动记录规则（最高优先级，任何 agent 开工前必读）

**遇到任何新问题（缺陷 / 环境陷阱 / 工具误操作 / 任何值得记住的坑），
第一步是把它写进 `docs/known-issues.md`（按标准格式），第二步才是修复。**

```
标准格式（docs/known-issues.md 中每个问题一条）：
### [日期] 标题
- **分类**: 泄漏 | 验证环境 | 工具误操作 | 兼容性 | 性能 | 其他
- **症状**: 发生了什么
- **根因**: 为什么
- **修复**: 怎么解决的（或"待修复"/"规避"）
- **防止复发**: 后续怎么做（规则）
- **状态**: ✅已修复 / ⚠️规避 / ❌待修复
```

**触发清单**（出现任一条就必须记录）：
- 排查超过 5 个工具调用的 bug / 诡异现象
- 修复了一个曾经踩过（或可能再踩）的坑
- 压测/验证工具暴露的问题（泄漏、超时、竞态）
- 环境相关的陷阱（代理、DNS、UDP、网络抖动）
- 批量替换/脚本操作造成的误伤
- 任何"以后可能会再遇到"的认知

**约束**：
1. 记录优先于修复——先记下来，再动手，防止"修完就忘"
2. 该规则**不依赖任何外部工具/记忆**，是仓库内的 agent 规则（AGENTS.md + docs/known-issues.md 一起被版本管理，任何 agent 读仓库都会遵守）
3. 新增问题后，若产生通用规则，同步更新下方"规则集"部分

---

## 项目架构

完整文档：[README.md](README.md) | [概览](docs/overview.md) | [快速开始](docs/quick-start.md) | [API](docs/api.md) | [指纹体系](docs/fingerprint.md) | [H3 指南](docs/h3.md) | [画像体系](docs/profiles.md) | [验证矩阵](docs/verification.md) | [架构](docs/architecture.md)

### 项目结构

```
cloak/
├── cloak/              ← 核心库（全部公开 API）
│   ├── impersonate.go       ← 入口：Impersonate/ImpersonateH3/ChainBuilder/DevMode
│   ├── request.go           ← Request 流式 builder + hooks
│   ├── response.go          ← Response + ResultState + TraceInfo
│   ├── transport_h2.go      ← Transport（一体式：TLS + H2/H1 + 浏览器头）
│   ├── transport_h3.go      ← H3Transport（纯 QUIC RoundTripper）
│   ├── transport_h3race.go  ← H3RaceTransport（H3 vs H2 赛跑）
│   ├── transport_race.go    ← RaceTransport（H2 vs H1 赛跑）
│   ├── transport_fprint.go  ← FingerprintTransport（fork http2）
│   ├── fingerprint.go       ← H2指纹/浏览器常量/Header排序/Multipart
│   ├── header.go            ← HeaderRoundTripper（浏览器头注入，兼容层）
│   ├── middleware.go        ← 中间件链
│   ├── retry.go             ← 条件重试 + 指数退避
│   ├── dump.go              ← 请求/响应 dump
│   ├── proxy.go             ← 正向代理
│   └── *_test.go            ← 测试（含 H3/客户场景/stress）
├── internal/
│   ├── http2/               ← x/net/http2 fork（SETTINGS/StreamID/Priority 定制）
│   ├── httpcommon/          ← 共享 HTTP 公共代码
│   └── header/              ← SortKeyValues/HeaderOrderKey
├── profiles/                ← 77 预置画像 + 测试
│   ├── internal_browser_profiles.go   ← Chrome/Safari/Firefox/Opera/Brave
│   ├── contributed_browser_profiles.go ← 更多浏览器版本
│   ├── internal_custom_profiles.go    ← OkHttp/移动端
│   ├── contributed_custom_profiles.go ← Nike/Zalando/Mesh 等
│   ├── profiles.go          ← 注册表 + NewClientProfile
│   ├── resolver.go          ← key 解析
│   └── metadata.go          ← 画像元数据
├── third_party/
│   └── quic-go-utls/        ← quic-go fork（UQUICClient 指纹注入 + H3）
├── cmd/
│   ├── verify-fingerprints/ ← 14 平台指纹验证工具
│   ├── stress/              ← 压力测试工具
│   ├── cloak-proxy/    ← 代理服务（画像热加载）
│   └── export-profiles/     ← 画像导出工具
├── docs/                    ← 文档（9 篇）
├── README.md
├── AGENTS.md
├── go.mod / go.sum
└── LICENSE
```

### 依赖

- `third_party/utls` — uTLS 本地 fork（TLS 指纹，Tor 团队 uTLS 衍生）
- `golang.org/x/net` — HTTP/2（源）
- `internal/http2` — x/net/http2 fork（H2 指纹定制，API 兼容）
- `third_party/quic-go-utls` — quic-go fork（UQUICClient 注入 QUIC TLS 指纹）
- `github.com/quic-go/qpack` — H3 QPACK
- 标准库

### 设计原则

- **一体式架构**: Transport 内置 TLS + HTTP 头 + H2/H1 协商
- **req 靠拢**: API 设计、中间件、钩子、ResultState 全部借鉴 req
- **零 fork 依赖**: 默认构建仅 uTLS + x/net + stdlib（H3 的 quic-go 在 third_party）
- **三层指纹**: TLS (uTLS) + H2 (internal/http2) + H3 (UQUICClient)
- **自动降级**: H3 → H2 → H1.1

### 压力测试基线

```
1 小时 / 20 并发 / 4 种 worker 模式(reuse/fresh/longLived/createDestroy)
+ H3×2 + 正向代理 / 6 画像随机 / 7 方法 / 5 body 类型
  总请求: 47,326,623 (4732 万)
  成功率: 100.00% (1 失败/4732万 — 压测工具自身竞态, 非库 bug)
  吞吐:   13,146 req/s
  延迟:   p50 770µs / p95 2.6ms / p99 11.2ms
  Goroutines: 全程 169-199 稳定 (零泄漏)
  结论:   ✅ 稳定可靠, 无连接/goroutine 泄漏
```

> 运行: `go run ./cmd/stress-varied 1h 20`(详见 docs/verification.md §1.1)

### 指纹验证基线

| 层 | 通过率 |
|----|--------|
| TLS APIs (tls.peet.ws/browserleaks/browserscan) | 100% |
| WAF/CDN (Cloudflare/Imperva/F5/HCaptcha/reCAPTCHA/Sannysoft) | 100% |
| Akamai (Transport 一体式) | ✅ 200 |
| HTTP/3 (http3.is/quic.browserleaks.com) | 需无 UDP 拦截环境 |
| DataDome | ❌ JS 引擎必需 |

> 运行: `go run ./cmd/verify-fingerprints -profiles chrome_150`
> 详见 docs/fingerprint-verification.md(含 curl vs cloak 对照证据)

### 编码规范

- error 不以标点结尾，不以大写开头
- map/slice/pointer Getter 返回防御性副本
- 测试用 -race 运行
- 画像新增 → 全平台验证 → 更新基线
- third_party 不做 gofmt 检查（上游代码）

### 质量门禁

```bash
go build ./...                        # 编译
go vet ./...                          # 静态检查
go test -race -count=1 ./...          # 全部测试
go run ./cmd/verify-fingerprints      # 全平台指纹验证
go run ./cmd/stress-varied 10m        # 多样化压力测试
```

### 关键陷阱

- `ClientProfile` 的 Getter 必须返回副本（防并发写）
- H3 的 `IsSet()` 在 utls 里是**反的**（空值返回 true），判断用 `!IsSet()` 表示"已设置"
- H3 racing 必须 `req.Clone()` 再分发给 goroutine（否则 data race）
- `Request.SetInsecureSkipVerify` 通过 `InsecureSkipVerrifier` 接口穿透包装链
- Safari/移动画像无 H3 数据是**有意的**（见 docs/profiles.md §H3 覆盖）

---

## ⚠️ 规则集（踩坑防复发）

> **遇到任何新问题 → 立即追加到 `docs/known-issues.md`（症状+根因+修复+防止复发），再修复。**
> 详细踩坑记录见 docs/known-issues.md，以下为规则要点。

### 连接/goroutine 泄漏规则

- 新增 API 若创建 client/transport，必须考虑连接生命周期（池化 or 文档化关闭义务）
- 任何读取响应的代码路径，body 必须被消费（drain）或显式关闭——**只 Close 不 ReadAll = 泄漏**
- 池化共享资源：refs=0 时**不删除条目**（删除会引发并发 create/destroy 循环）
- 压测工具（stress-varied）是泄漏检测器：**每 3 分钟看 goroutine 是否平台期**，暴涨=有泄漏

### 验证环境规则（本机透明代理）

- 验证工具必须先 **warm-up**（首个 TLS 连接被 MITM 检查 10-15s），再判定结果
- 失败要**分类归因**：环境（网络/DNS/UDP）vs 指纹 vs JS 挑战，不能一律归因
- 外网 UDP 被丢弃 → H3 平台需无代理环境；本地 H3 回环不受影响
- 判断指纹有效性看**服务端判定结果**（JA3/JA4 匹配 + WAF 放行），不是连通性
- curl 000/403 ≠ 网络不通——用 **curl vs cloak 同环境对照**判断

### 工具/脚本操作规则

- 新增 cmd/ 工具构建产物**绝不 commit**（先查 .gitignore，用 `git rm --cached` 移除误提交）
- 批量替换（sed/execute_code）后**必须**：grep 检查关键函数体 + go build + 冒烟测试
- 压测场景与服务器端点必须一一对应（缺 handler 会误判为库 bug）
- 压测工具自身场景也要 drain body（工具泄漏 ≠ 库泄漏，别误判）

### 提交/协作规则

- AGENTS.md 是**受保护文件**，agent 直接写会被拒绝——用户明确指示后才更新
- git commit 消息含 "restart/stop/gateway" 等词可能触发 gateway 拦截误报——拆成 commit 与 push 两步
- 项目身份：module = `github.com/wuliao6688/cloak`，核心包根目录 package cloak，依赖全部本地 fork（third_party/utls + third_party/quic-go-utls），**不引入任何外部 bogdanfinn 依赖**
