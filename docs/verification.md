# 验证矩阵

本项目的质量保障分三层：**单元/并发测试**（`go test -race`）、**外部平台验证**
（`cmd/verify-fingerprints`）、**客户场景回归**（真实用户问题复现）。

## 1. 测试基线

```bash
go test -race -count=1 ./...    # 全部通过
```

覆盖范围：

| 类别 | 测试 | 内容 |
|---|---|---|
| H2/H1 协商 | `TestTransportH2*` | H2 正常、H1 兜底、并发、POST、多画像 |
| 并发安全 | `TestTransportThreadSafety` | 多 goroutine 共享 Transport |
| 压力测试 | `TestStressSustained*` | 20 并发 / 10 分钟 / 内存无增长 / goroutine 零泄漏 |
| 代理 | `TestTransportProxy` / `TestProxy*` | HTTP 代理、CONNECT 隧道、并发、健康检查 |
| 指纹自检 | `TestFingerprint*` / `TestSelfCheck` | JA3/JA4 验证 |
| **H3/QUIC** | `TestH3Transport*` / `TestH3Race*` | 本地 H3 200、指纹注入、GREASE、racing 选协议、降级 |
| **画像有效性** | `TestAllBrowserProfilesH3DataValid` | 47 个 H3 画像全部能构建 transport |
| **客户场景** | `TestCustomerIssue*` | gzip、重定向、超时、100 并发稳定性、证书穿透 |

## 2. 外部平台验证（14 平台）

```bash
go run ./cmd/verify-fingerprints -profiles chrome_150
go run ./cmd/verify-fingerprints -all          # 全部画像
```

| 分类 | 平台 | 验证内容 |
|---|---|---|
| 🔬 TLS API | tls.peet.ws | JA3 / JA4 |
| | browserleaks.com | JA3 / JA3N |
| | browserscan.net | TLS 信息 |
| 🛡️ WAF/CDN | cloudflare.com | 200 / 不触发 1020 |
| | imperva.com | 200 |
| | f5.com | 200 |
| | akamai.com | 200（TLS+HTTP 一体） |
| | datadome.co | 200（无 JS 挑战） |
| | hcaptcha.com | 200 |
| | recaptcha-demo | 200 |
| | sannysoft.com | PASS |
| 📡 HTTP | httpbin.org | UA / 头注入 |
| 🚀 HTTP/3 | http3.is | H3 协商（QUIC 连通） |
| | quic.browserleaks.com | H3 SETTINGS 指纹（h3_hash） |

> 注意：🚀 H3 平台需要无 UDP 拦截的网络环境（企业代理/透明代理会丢弃 UDP，
> 此时自动降级 H2——这也验证了降级逻辑）。

### 已知基线（无 UDP 拦截环境）

| 层 | 通过率 |
|---|---|
| TLS APIs（3 平台） | 100% |
| WAF/CDN（8 平台） | 100% |
| Akamai 一体式 | ✅ 200 |
| DataDome | 需 JS 引擎（纯 TLS 无法过 JS 挑战，与所有库一致） |

## 3. 客户场景验证

从三个主流库（同类 Go 库、curl_cffi、imroc/req）的 **46 个真实
issue** 中提取高频问题，逐项复现验证。详见 [客户场景报告](customer-scenarios.md)。

### 动态验证结果

| 客户问题 | 测试 | 结果 |
|---|---|---|
| gzip 响应乱码 | `TestCustomerIssueGzipResponse` | ✅ |
| 重定向无限循环 | `TestCustomerIssueRedirects` | ✅ |
| 无超时挂起 | `TestCustomerIssueTimeout` | ✅ 500ms 精确超时 |
| 100 并发崩溃 | `TestCustomerIssueConcurrencyStability` | ✅ 5000 请求 0 失败 |
| 证书配置静默失效 | `TestCustomerIssueSetInsecureSkipVerifyThroughChain` | ✅（已修复） |

### 已修复的真实缺陷

**`Request.SetInsecureSkipVerify` 静默失效**（2026-08-26 修复）：
- 根因：类型断言 `*http.Transport` 不匹配自定义 Transport 包装链
- 修复：`InsecureSkipVerrifier` 接口 + `Unwrap()` 递归穿透
- 影响：自签证书场景（内部服务/测试环境）此前会静默失败

## 4. 质量门禁

提交前必须通过：

```bash
go build ./...                        # 编译
go vet ./...                          # 静态检查
go test -race -count=1 ./...          # 全部测试
gofmt -l .                            # 格式（不含 third_party）
```

## 5. 回归记录

| 日期 | 内容 | 结果 |
|---|---|---|
| 2026-08-26 | H3/QUIC 全链路（UQUICClient 指纹注入 + H3 racing） | ✅ |
| 2026-08-26 | 画像 H3 数据补全（42 个）+ 全量论证 | ✅ |
| 2026-08-26 | 清理 867 行死代码/过时内容 | ✅ |
| 2026-08-26 | SetInsecureSkipVerify 穿透修复 | ✅ |
