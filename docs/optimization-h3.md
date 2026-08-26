# H3/QUIC 能力补全 — 优化方案论证

> 调研日期：2026-08-26
> 目标：让项目能通过各类人机平台，HTTP/2 与 HTTP/3 全覆盖，指纹维度对齐真实浏览器。

---

## 1. 现状诊断（本仓库）

### 1.1 已具备

| 维度 | 状态 | 说明 |
|---|---|---|
| TLS 指纹 (uTLS) | ✅ 100% | tls.peet.ws / browserleaks / browserscan 全过 |
| HTTP/2 指纹 | ✅ | internal/http2 fork：SETTINGS/StreamID/ConnectionFlow/Priority 帧/伪头顺序/Header 排序 |
| WAF/CDN | ✅ | Cloudflare / Imperva / F5 / HCaptcha / reCAPTCHA / Sannysoft 全过 |
| Akamai | ✅ 200 | Transport 一体式（TLS+HTTP 头） |
| 画像 | ✅ 81 个 | Chrome/Firefox/Safari/Brave/Opera |
| H2→H1 降级 | ✅ | 自动降级 |

### 1.2 关键缺口：HTTP/3（QUIC）完全没有引擎

**铁证（代码级）**：

```bash
$ grep -rn "GetHttp3\|http3Settings\|http3PriorityParam" cloak/ internal/ cmd/
# 零命中 —— 画像里的 H3 字段没有任何引擎消费
```

- `profiles/profiles.go` 定义了完整的 H3 字段（`http3Settings` / `http3SettingsOrder` / `http3PriorityParam` / `http3PseudoHeaderOrder` / `http3SendGreaseFrames`）
- `profiles/internal_browser_profiles.go` 仅 **Chrome_144**、**Firefox_147/148** 等少量画像带 H3 数据（5 个）
- `cloak/transport_*.go` **完全不读取**这些字段 → **H3 数据是死代码**
- `go.mod` 无 quic-go / quic-go-utls 依赖
- `cmd/verify-fingerprints` 12 个平台**全部是 TCP+TLS+H2 检测，无任何 H3 验证**

### 1.3 为什么这是致命伤

1. **真实浏览器 2023+ 默认 H3**：Chrome/Firefox 对支持 QUIC 的站点优先走 HTTP/3，H3 流量占比已超 50%（Chrome 平台统计）。
2. **反爬方视角**：一个自称 "Chrome/150" 的客户端，UA/TLS/H2 全像浏览器，却**完全没有 QUIC 能力**——这本身就是强异常信号（Chrome 必然发 H3）。检测系统可以此给低分。
3. **纯 H3 站点无法访问**：部分 CDN/API 已 alt-svc 只暴露 h3 端点，无 H3 引擎直接失败。

---

## 2. 生态对标（类似项目横向对比）

| 项目 | 语言 | H2 | H3 | H3 指纹 | 协议选择策略 | 实现方式 |
|---|---|---|---|---|---|---|
| **本仓库** | Go | ✅ | ❌ | ❌ | H2→H1 降级 | uTLS + x/net/http2 fork |
| 同类 Go 库 | Go | ✅ | ✅ | ✅ | **Protocol Racing**（H2 vs H3 赛跑，Chrome 式 Happy Eyeballs） | fhttp + **quic-go-utls** |
| imroc/req | Go | ✅ | ✅ | ⚠️ 部分 | 自动检测 + 可强制 | quic-go + utls（H3 仅协议层） |
| curl_cffi / curl-impersonate | Python/C | ✅ | ✅ | ✅ | alt-svc + Happy Eyeballs | curl fork（BoringSSL） |
| 用户另一仓库 requests (wangluozhe) | Python | ✅ | ✅ | ✅ | — | cloak (Python 版) |

### 2.1 其他实现（基于 fhttp + quic-go-utls 的方案）的 H3 方案（已生产验证）

- 依赖 quic-go 的 fork + fhttp（net/http 的 fork）
- `racer.go`：`protocolRacer` 并行发起 H2 + H3 连接，**谁先响应用谁**，缓存每域名协议偏好（Chrome 的 Happy Eyeballs 逻辑）
- 测试 `tests/http3_fingerprint_test.go` 断言 browserleaks 的 H3 指纹：
  ```
  Chrome_144 期望: h3_text = "1:65536;6:262144;7:100;51:1;GREASE|GREASE|984832|m,a,s,p"
                  h3_hash = "ba909fc3dc419ea5c5b26c6323ac1879"
  ```
  即 **HTTP/3 SETTINGS（值+顺序+GREASE）| Priority Param | 伪头顺序** 三件套。
- 测试 `tests/http3_chrome_cloudflare_test.go`：Chrome 144 访问 Cloudflare `/cdn-cgi/trace` 返回 `http=http/3`。

**⚠️ 上游方案的隐藏缺口（本次调研实证）**：

quic-go-utls v1.0.9 的 QUIC **TLS 层没有注入浏览器指纹**：

```go
// quic-go-utls/internal/handshake/crypto_setup.go:96
cs.conn = tls.QUICClient(&tls.QUICConfig{ TLSConfig: tlsConf })   // ← 标准 utls.Client()
// 而非 tls.UQUICClient(config, clientHelloID)                     // ← 带指纹的入口
```

- `tls.QUICClient` 内部走 `utls.Client(nil, config)` → **Go 默认 ClientHello**（HelloGolang）
- `tls.UQUICClient(config, clientHelloID)` 才走 `utls.UClient()`（注入 Chrome/Firefox 指纹）
- 上游只验证了 **H3 应用层指纹**（browserleaks h3_text 是 SETTINGS 层指纹）和 Cloudflare 连通性，**从未验证 QUIC TLS 层 ClientHello**
- 结论：**上游的 QUIC TLS 指纹是不完整的**——检测方若抓 QUIC 握手（JA3h/JA4h），仍是 Go 默认指纹

这给了本项目**超越上游的机会**：QUIC TLS 层用 `UQUICClient` 注入指纹，真正做到 H3 全链路指纹一致。

### 2.2 imroc/req 的 H3 方案（简化版）

- `internal/http3/` 基于官方 quic-go + qpack
- **缺点**：H3 只做协议层，SETTINGS/Priority/GREASE 用 quic-go 默认值 → **H3 指纹不像浏览器**，仅能"能访问"，过不了严格 H3 指纹检测
- 对 H2 的指纹模拟较强（client_impersonate.go 全套）

### 2.3 curl_cffi / curl-impersonate（业界最强）

- H3 指纹完整（含 QUIC transport parameters、GREASE、UA 关联）
- 可自定义扩展顺序、grease 开关（与本项目画像字段一一对应）

---

## 3. 人机平台检测维度拆解

### 3.1 TLS 层（JA3/JA4）— 本仓库已 100%

### 3.2 HTTP/2 层 — 本仓库已 100%

### 3.3 HTTP/3 层（browserleaks h3_text 揭示的检测维度）

| 检测维度 | Chrome 144 真实值 | 说明 |
|---|---|---|
| SETTINGS 值 | 1:65536, 6:262144, 7:100, 51:1 | QPACK_MAX_TABLE_CAPACITY / MAX_FIELD_SECTION_SIZE / QPACK_BLOCKED_STREAMS / H3_DATAGRAM |
| SETTINGS 顺序 | 1 → 6 → 7 → 51 → GREASE | 顺序即指纹 |
| GREASE SETTINGS | ✅ 有 | 随机占位 |
| Priority Param | 984832 | 0x0F0700 = urgency:3 + incremental:0 + reprioritize:7 |
| 伪头顺序 | :method, :authority, :scheme, :path | m,a,s,p |
| GREASE 帧 | ✅ | 连接建立时发 GREASE 帧 |

### 3.4 QUIC 层（更深的检测维度，Akamai/Cloudflare 会看）

- QUIC Initial 包：版本号、DCID/SCID 长度（Chrome 固定 8/8）、random 值分布
- Transport Parameters：顺序、值（max_udp_payload_size、initial_max_data、ack_delay_exponent、disable_active_migration 等）、GREASE TP
- TLS-over-QUIC 的 ClientHello（uTLS 已覆盖大部分，但 ALPN 是 h3）
- UA ↔ H3 指纹联动（Chrome UA 必须配 Chrome H3 指纹）

### 3.5 浏览器 H3 指纹横向对比（本项目画像数据实证）

| 维度 | Chrome_144 | Firefox_147 | 说明 |
|---|---|---|---|
| H3 SETTINGS | 1:65536, 7:100 (+6:262144, 51:1) | 不同组合 | QPACK/MAX_FIELD_SECTION/H3_DATAGRAM |
| SETTINGS 顺序 | 1→6→7→51→GREASE | 不同 | 顺序即指纹 |
| GREASE SETTINGS | ✅ | ❌（发 GREASE 帧但非随机 SETTINGS） | 上游代码注释确认 |
| Priority Param | 984832 | 0（无） | 上游用 `>0` 判断 Chrome 系 |
| GREASE 帧 | ✅ | ✅ | 连接建立时发送 |
| 伪头顺序 | m,a,s,p | 不同 | — |

**结论**：H3 指纹**不是统一的**——Chrome/Firefox/Safari 各有一套，画像必须逐浏览器补全，不能用一个默认值糊弄。

---

## 4. 从用户角度反推需求

**场景**：用户（逆向/爬虫/自动化）需要绕过各种人机平台——Akamai、Cloudflare、DataDome、Imperva、F5、HCaptcha、reCAPTCHA、Sannysoft 等，且要"过各种人机平台、H2 H3 都需要"。

**反推**：
1. 平台方 2026 年的检测基线：**TLS + H2 + H3 三件套 + UA 一致性 + 行为链**。只做 TLS+H2 = 落后半个时代，检测方已经默认"现代浏览器必然 H3"。
2. 用户已有 Python 生态的 H3 能力（requests/wangluozhe、curl_cffi），Go 侧缺 H3 = 能力不对称。
3. 项目画像里已经预留了 H3 字段（说明上游方向明确），只是引擎没接上。
4. **反向论证**：如果只补 H2 不补 H3，检测方视角是——"TLS/H2 全像 Chrome，但 QUIC 能力为零"。真实 Chrome 必然尝试 H3（对支持 alt-svc:h3 的站点），**缺 H3 本身就是最强的非浏览器信号**。H3 不是"加分项"，是"及格线"。

---

## 5. 优化方案论证（基于代码级实证）

### 方案 A1：直接引入 quic-go-utls（对齐上游，最快）

**做法**：引入 quic-go 的 fork + fhttp，移植类似方案中的 buildHTTP3Transport + racer。

**优点**：上游同款，H3 应用层指纹（SETTINGS/GREASE/Priority/伪头）已验证通过 Cloudflare；画像字段与上游同源，迁移成本低。

**致命缺点（实证）**：
1. **fhttp 绑架**：quic-go-utls 的 http3 包 18 个文件依赖 fhttp（net/http fork），`RoundTrip` 签名是 `fhttp.Request` 而非 `net/http.Request` → **与项目"标准 net/http + 零 fork"原则直接冲突**，需要全项目替换为 fhttp 类型
2. **QUIC TLS 指纹缺失**：`crypto_setup.go:96` 用 `tls.QUICClient`（Go 默认 ClientHello），**QUIC 层 TLS 指纹是 Go 的**，过不了严格 H3 指纹检测
3. 依赖膨胀：引入 fhttp 全家桶

**结论：否决作为主方案**（除非愿意放弃"零 fork + 标准 net/http"原则）。

### 方案 A2：官方 quic-go + utls 自建 H3 引擎（推荐 ⭐）

**做法**：
- 依赖：官方 quic-go（纯 QUIC 核心**不依赖 fhttp**）+ uTLS（已是间接依赖）
- 新增 `cloak/transport_h3.go`：
  - 用 quic-go 的 `http3.Transport` 或直接 `quic.DialEarly` 建连
  - **TLS 层注入**：utls 提供 `tls.UQUICClient(config, clientHelloID)` —— 直接支持 QUIC TLS 指纹注入（这是 quic-go-utls 没用的 API，正是超越上游的点）
  - H3 应用层：消费画像 `GetHttp3*` 字段 → SETTINGS 帧（值+顺序+尾部 GREASE）、Priority Param（984832=0x0F0700）、伪头顺序、GREASE 帧
- 新增 `cloak/racer.go`：Protocol Racing（H2 vs H3 赛跑 + 域名协议缓存），失败降级 H2→H1
- 画像补全：81 画像逐个补 H3 数据（Chrome 系 5 字段一致，Firefox 不同——可从上游画像对照补全）
- verify-fingerprints 增加 H3 维度：quic.browserleaks.com（h3_text/h3_hash 断言）+ http3.is + Cloudflare trace + Akamai

**优点**：
- **QUIC TLS 层注入 Chrome 指纹**（UQUICClient）——比上游更完整，H3 全链路一致
- 官方 quic-go 维护活跃，无 fhttp 绑架，标准 net/http 接口不变
- 画像字段已存在，直接消费

**缺点**：
- 需要自己写 H3 应用层（SETTINGS/Priority/GREASE 帧编码）——工作量中等（参考 internal/http2 fork 的经验）
- quic-go 依赖较重（~50 个间接依赖）
- H3 走 UDP，代理支持需额外处理（SOCKS5 UDP-associate / CONNECT 隧道二期）

### 方案 B：先只做 H3 连接能力，不做指纹（最省）

**做法**：官方 quic-go 默认 H3，能访问纯 H3 站点。
**优点**：最快（1-2 天）。
**缺点**：H3 指纹 = quic-go 默认 = 一眼 bot；QUIC TLS 层也是 Go 默认。**不满足"过各种人机平台"目标，否决。**

### 方案 C：自己 fork quic-go 改 TLS 层（零外部 fork）

**做法**：把官方 quic-go 的 `internal/handshake/crypto_setup.go` 改为 `tls.UQUICClient`（参照 quic-go-utls 的 diff）。
**优点**：彻底零外部 fork，完全可控。
**缺点**：需要持续跟踪官方 quic-go 升级（每次上游改动都要 rebase）；工作量 2-3 倍；风险高。**仅当用户坚持零 fork 时才选。**

---

## 6. 推荐路线（方案 A2，超越上游）

考虑到仓库"零 fork 依赖"（H2 层已自 fork x/net/http2）与"标准 net/http 接口"双原则，推荐 A2：

1. **依赖**：官方 quic-go + uTLS（已有）
2. **新增 `cloak/transport_h3.go`**：quic-go 建连 + `UQUICClient` 注入画像的 `GetClientHelloId()` + H3 SETTINGS/Priority/伪头/GREASE 帧全按画像
3. **新增 `cloak/racer.go`**：仿上游 protocolRacer——H2+H3 并行，先到先用，域名级协议缓存；失败自动降级
4. **画像补全**：81 画像逐个补 H3 数据（从上游/curl_cffi 对照）
5. **验证**：verify-fingerprints 增加 H3 维度（quic.browserleaks.com h3_text/h3_hash 断言 + http3.is + Cloudflare trace + Akamai），`go test -race` 全绿
6. **代理**：H3 走 UDP，代理场景需 SOCKS5 UDP-associate 或 QUIC over CONNECT（可二期）

### 里程碑

| 阶段 | 内容 | 验收 |
|---|---|---|
| M1 | quic-go + UQUICClient 最小 H3 连接 | 本地 H3 server 200（已 PoC 验证 ✅） |
| M2 | H3 指纹全开（SETTINGS/Priority/伪头/GREASE + QUIC TLS） | browserleaks h3_text 匹配 Chrome 期望值；本地抓包确认 QUIC ClientHello 指纹 |
| M3 | Protocol Racing + 协议缓存 | Cloudflare/Akamai H3 通过；H2 行为不变 |
| M4 | 81 画像补全 + verify-fingerprints H3 维度 | 全画像 H3 指纹 hash 断言 |

### 已验证事实（本次 PoC）

```
✅ quic-go v0.61.0 + 本地 H3 server：HTTP/3.0 200 OK（回环 UDP 不受透明代理影响）
✅ quic-go 核心层不依赖 fhttp（仅 http3 包依赖）
✅ utls 提供 UQUICClient(config, clientHelloID) —— QUIC TLS 指纹注入入口
⚠️ 外网 QUIC 被环境透明代理(198.18.0.0/15)丢弃 → 验证需在无代理环境或本地回环
```

---

## 7. 风险与边界

- **DataDome 仍需 JS 引擎**（现状已知，非本项目能解决）
- H3 在部分企业网络（UDP 被禁）不可用 → 必须有 H2 降级兜底（Racing 天然覆盖）
- quic-go 依赖体积：权衡后接受（上游同款）
- 若坚持零 fork：选方案 B，但工期约 2-3 倍
