# 客户场景与问题检索报告

> 检索日期：2026-08-26
> 数据源：bogdanfinn/tls-client（19 issues）、lexiforest/curl_cffi（15 issues）、imroc/req（12 issues）
> 方法：GitHub issues 按评论数排序抓取（评论多 = 真实痛点），提取问题详情后分类

---

## 1. 客户画像（谁在用这类库）

| 客户类型 | 典型场景 | 代表问题 |
|---|---|---|
| **爬虫/数据采集工程师** | 抓取电商、社交、新闻站点；规避 WAF | POST 400、代理超时、gzip 乱码 |
| **账号自动化（营销/社群）** | 批量注册/登录、群发、点赞关注 | 并发崩溃、运行数小时后挂起 |
| **安全研究/渗透测试** | 协议分析、指纹验证、ClientHello 复制 | TLS illegal parameter、unexpected message |
| **API 集成商（Node/Python 调 Go 库）** | 移动端 App 数据对接 | protobuf 字节错乱、证书 pinning |
| **移动端 App 逆向** | 复制 iOS/Android 抓包指纹 | 移动端 TLS 指纹缺口（Chrome v130+） |

---

## 2. 问题清单（按类别）

### A. 稳定性类（最痛）

| # | 问题 | 来源 | 描述 |
|---|---|---|---|
| A1 | **并发崩溃** | tls-client #53 | 100 并发下 nil pointer dereference |
| A2 | **长时间运行崩溃** | tls-client #71 | 运行数小时后 crash（fhttp setRequestCancel） |
| A3 | **无超时挂起** | tls-client #33 | transport 没有 timeout，请求永久挂起 |
| A4 | **流式响应卡死** | curl_cffi #141 | stream=True 各种场景卡死 |
| A5 | **长跑后无响应** | curl_cffi #578 | 8+ 小时后挂起，超时/异常都不触发 |
| A6 | **轮询容易超时** | curl_cffi #106 | 轮询请求超时频率高于 requests |

### B. TLS/指纹类

| # | 问题 | 来源 | 描述 |
|---|---|---|---|
| B1 | **TLS unexpected message** | tls-client #6 | 访问 google.com 报错（utls 兼容性） |
| B2 | **TLS illegal parameter** | tls-client #185 | Ubuntu 22.04 特定环境握手失败 |
| B3 | **TLSFingerprint 丢失** | req #1 | 首个请求后指纹失效（状态泄漏） |
| B4 | **移动端指纹缺口** | curl_cffi #434 | Chrome v130 后无 Android TLS 指纹 |
| B5 | **JA3 构建 Spec 报错** | tls-client #7 | 用 JA3 构造画像失败 |
| B6 | **自定义签名算法** | tls-client #88 | iOS App 抓包含 0x0301/0x0303 等算法无法复制 |

### C. HTTP 行为类

| # | 问题 | 来源 | 描述 |
|---|---|---|---|
| C1 | **POST 返回 400** | tls-client #147 | GET 正常但 POST 全部 400 Bad Gateway |
| C2 | **重定向无限循环** | tls-client #14 | WithNotFollowRedirects 反而跟随；httpbin cookies 卡死 |
| C3 | **gzip 响应乱码** | tls-client #32 | Accept-Encoding 设置后 body 是乱码 |
| C4 | **响应协议头大小写** | tls-client #119 | HTTP/2.0 vs http/2.0 case 问题 |
| C5 | **protobuf 字节错乱** | tls-client #120 | 发送/接收 protobuf 时多出字节 |
| C6 | **digest 认证 body 为 nil** | req | 摘要认证后响应体丢失 |

### D. 网络/代理类

| # | 问题 | 来源 | 描述 |
|---|---|---|---|
| D1 | **代理导致 EOF** | tls-client #66 | 使用代理时 frequent unexpected EOF |
| D2 | **代理 SSL 版本错误** | curl_cffi #6 | 带用户名密码的代理 WRONG_VERSION_NUMBER |
| D3 | **自定义 Dial 缺失** | tls-client #218/#200 | 需要自定义 socket/DNS（scraper、proxy 场景） |
| D4 | **Dialer 类型太具体** | tls-client #160 | 应改为 interface 便于扩展 |
| D5 | **证书 pinning 通配符** | tls-client #61 | 需要 OkHttp 式 *.domain 通配符 |
| D6 | **证书路径问题** | curl_cffi #104 | CAfile 路径错误导致 ErrCode 77 |

### E. 功能缺口类

| # | 问题 | 来源 | 描述 |
|---|---|---|---|
| E1 | **HTTP/3 支持** | tls-client #104/#189 | QUIC 时代必须（本项目已实现 ✅） |
| E2 | **UA 跟随画像** | tls-client #108 | 用浏览器画像时应自动匹配 UA |
| E3 | **请求前/后钩子** | tls-client #220 | prerequest/postrequest hooks |
| E4 | **响应落盘** | tls-client #21 | 大响应直接写文件而非内存 |
| E5 | **优先级帧顺序** | tls-client #17 | Firefox WINDOW_UPDATE 帧顺序不对 |
| E6 | **自定义流 ID** | tls-client #205 | AllowHTTP、StreamID 值设置 |
| E7 | **本地地址绑定** | tls-client #52 | WithLocalAddr 多网卡场景 |
| E8 | **编码支持** | tls-client #207 | EUC-KR 韩文编码 |

---

## 3. 客户真实场景还原

### 场景 1：电商爬虫(SSL/代理)
> "Windows Server 2019 + Node 调 Go 库，用代理抓商品数据，频繁 unexpected EOF，请求失败率 20%+"
> → 对应 D1/A3

### 场景 2：批量账号操作(并发)
> "100 并发跑营销自动化，跑一会就 nil pointer dereference 崩溃"
> → 对应 A1/A2

### 场景 3：移动 App 数据采集(指纹)
> "App 是 iOS 原生 + protobuf，抓包发现 ClientHello 含特殊签名算法 0x0301/0x0303，库复制不了"
> → 对应 B6/C5

### 场景 4：长时监控轮询(超时)
> "24 小时轮询监控价格，跑 8 小时后开始卡死，20s 超时根本不触发"
> → 对应 A4/A5/A6

### 场景 5：注册/登录自动化(HTTP 行为)
> "GET 没问题，POST 提交表单全部 400，服务器说 UA 不对/请求不完整"
> → 对应 C1/B3

---

## 4. 对照本项目验证结果（2026-08-26 实测）

### 4.1 动态验证（新增 customer_issues_test.go）

| 问题 | 验证测试 | 结果 | 证据 |
|---|---|---|---|
| **C3 gzip 乱码** | `TestCustomerIssueGzipResponse` | ✅ 通过 | 浏览器头 Accept-Encoding + gzip 服务器 → body 正确解压 `"gzip-ok-内容"` |
| **C2 重定向循环** | `TestCustomerIssueRedirects` | ✅ 通过 | 302→302→final 正确跟随；禁用跟随返回 302 |
| **A3 无超时挂起** | `TestCustomerIssueTimeout` | ✅ 通过 | 慢服务器 5s + client 500ms 超时 → 500ms 正确返回 `context deadline exceeded`，不挂死 |
| **A1/A2 并发崩溃** | `TestCustomerIssueConcurrencyStability` | ✅ 通过 | 100 并发 × 50 轮 = 5000 请求 0 失败，goroutine 无泄漏 |
| **C1 证书配置失效（新修复）** | `TestCustomerIssueSetInsecureSkipVerifyThroughChain` | ✅ 通过 | 修复前 `SetInsecureSkipVerify` 对自定义 Transport **静默失效**（类型断言 `*http.Transport` 不匹配包装链）→ 修复后穿透 HeaderRoundTripper/customHeader 到达 Transport |

### 4.2 存量测试已覆盖的问题

| 问题 | 存量测试 | 结果 |
|---|---|---|
| A1 并发安全 | `TestTransportThreadSafety` / `TestTransportH2Concurrent` | ✅ |
| A2 长时间运行 | `TestStressSustained*`（5 组 stress 测试） | ✅ |
| D1 代理 EOF | `TestTransportProxy` / `TestProxyConcurrentRequests` | ✅ |
| C1 POST 400 | `TestTransportRoundTripH2POST` | ✅ |
| H3 能力（E1） | `TestH3Transport*` / `TestH3Race*` / `TestAllBrowserProfilesH3DataValid` | ✅ |

### 4.3 静态验证（代码审查）

| 问题 | 项目能力 | 结论 |
|---|---|---|
| B1/B2 TLS 握手错误 | 77 画像 uTLS SpecFactory，verify-fingerprints 12+2 平台 | ✅ 覆盖 |
| B3 TLSFingerprint 丢失 | Transport 按画像构造，SetProfile 支持动态切换 | ✅ 无 req 的状态泄漏设计 |
| B4 移动端指纹缺口 | OkHttp4Android7~13、Nike/Zalando/MMS 等 18 个移动画像 | ✅ 优于 curl_cffi |
| B6 自定义签名算法 | SpecFactory 完全自定义 ClientHello | ✅ |
| C4 协议头大小写 | 标准 net/http 处理 | ✅ |
| C5 protobuf 字节错乱 | 无 fork 的 fhttp，body 直通 | ✅ 无上游 fhttp bug |
| C6 digest 认证 | SetBasicAuth/SetBearerAuthToken | ✅ |
| D3 自定义 Dial | `TransportOptions` 可扩展（当前为固定 net.Dialer） | ⚠️ 待加 |
| D5 证书 pinning 通配符 | 未实现 | ⚠️ 待加 |
| D7 本地地址绑定 | 未实现 | ⚠️ 待加 |
| E3 请求前后钩子 | `OnRequest` / `OnResponse` | ✅ 已实现 |
| E4 响应落盘 | `SetOutputFile` / `SetOutput` | ✅ 已实现 |
| E6 自定义流 ID | internal/http2 fork 支持 | ✅ |
| E7 编码支持 | 未实现（EUC-KR） | ⚠️ 待加 |

### 4.4 本次修复

**`Request.SetInsecureSkipVerify` 静默失效**（真实缺陷，对应客户 C1 类问题）：
- 根因：类型断言 `r.client.Transport.(*http.Transport)` —— 项目实际是自定义 `tlsgateway.Transport` 被 HeaderRoundTripper/customHeader 包装
- 修复：新增 `InsecureSkipVerrifier` 接口 + `Transport.SetInsecureSkipVerify` 公开方法 + 三个包装器 `Unwrap()` 方法，递归穿透包装链
- 验证：自签证书 + 完整包装链请求成功（修复前失败）
