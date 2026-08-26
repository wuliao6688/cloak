# 已知问题登记册（Known Issues Log）

> **规则**：本项目遇到任何新的坑/缺陷/环境陷阱，**立即追加到本文件**（按下面格式），
> 然后再修复。修复完成后更新状态。这条规则由 AGENTS.md 引用，所有后续工作必须遵守。

---

## 登记格式

```markdown
### [日期] 标题
- **分类**: 泄漏 | 验证环境 | 工具误操作 | 兼容性 | 性能 | 其他
- **症状**: 发生了什么
- **根因**: 为什么
- **修复**: 怎么解决的（或"待修复"/"规避"）
- **防止复发**: 后续怎么做（规则）
- **状态**: ✅已修复 / ⚠️规避 / ❌待修复
```

---

## 已登记问题

### [2026-08-26] ImpersonateRequest 每次新建 Transport 导致连接泄漏
- **分类**: 泄漏
- **症状**: 持续使用下泄漏 keep-alive 连接及 goroutine（3 分钟 37K goroutine / 586MB 内存暴涨）；1 小时压测触发
- **根因**: `ImpersonateRequest()` 每次调用创建**全新 Transport + 全新连接池**，无生命周期管理
- **修复**: 全局 Transport 池（pool.go）——相同画像共享同一 Transport，refs 引用计数
- **防止复发**: 新增 API 若创建 client/transport，必须考虑连接生命周期（池化 or 文档化关闭义务）
- **状态**: ✅已修复（pool.go, commit 845fe93）

### [2026-08-26] Transport 池 refs=0 时删除条目导致 create/destroy 循环
- **分类**: 泄漏
- **症状**: 并发下连接无限累积（37K goroutine），清理反而加剧泄漏
- **根因**: `releasePooledClient` 在 refs=0 时 `delete(transportPool, key)`——其他 worker 还在用该 Transport，删除后重建，循环往复
- **修复**: 池条目**永不删除**；refs=0 只 CloseIdleConnections 释放资源，保留条目供复用
- **防止复发**: 池化设计时考虑并发用户；"引用计数 + 删除"模式在共享资源上会竞态
- **状态**: ✅已修复（pool.go）

### [2026-08-26] SetRetry 重试循环中非 2xx body 未关闭
- **分类**: 泄漏
- **症状**: 每个 429 响应泄漏连接 + setRequestCancel goroutine（独立复现 10s 泄漏 2893 goroutine）
- **根因**: `executeWithRetry` 决定重试后直接丢弃 resp，body 从未读取/关闭
- **修复**: 决定重试前 drain + close body
- **防止复发**: 任何读取响应的代码路径，body 必须被消费（drain）或显式关闭
- **状态**: ✅已修复（request.go executeWithRetry）

### [2026-08-26] 响应 body 只 Close 不 ReadAll 导致连接无法复用
- **分类**: 泄漏
- **症状**: setRequestCancel goroutine 挂起；连接池无法复用连接
- **根因**: net/http 要求连接复用前读完 body（或完全丢弃），只 Close 不读无法回收
- **修复**: 压测工具所有场景改为 `io.Copy(io.Discard, resp.Body)` 再 Close
- **防止复发**: 标准 net/http 铁律——drain 再 Close；压测场景同样遵守
- **状态**: ✅已修复（stress-varied）

### [2026-08-26] 透明代理冷启动导致验证工具 0/14 全败
- **分类**: 验证环境
- **症状**: verify-fingerprints 0/14 全败（handshake EOF/超时），但最小复现 200 OK
- **根因**: 本机透明代理对**进程第一个 TLS 连接**做 MITM 检查（10-15s 慢），后续快；并发 4 使每个平台都像"第一个连接"
- **修复**: verify-fingerprints 加 warm-up（先打 tls.peet.ws 建立代理缓存）+ 并发降到 2 → 0/14 变 10/14
- **防止复发**: 验证工具必须先 warm-up 再判定；失败要分类（环境 vs 指纹 vs JS 挑战）
- **状态**: ✅已修复（cmd/verify-fingerprints warm-up）

### [2026-08-26] 外网 UDP 被透明代理丢弃，H3 平台无法验证
- **分类**: 验证环境
- **症状**: http3.is / quic.browserleaks.com 永远失败（context deadline / 无响应）
- **根因**: 外网 UDP 被 198.18.0.0/15 fake-ip 透明代理丢弃
- **修复**: 无法本地修复——H3 线上验证需无代理环境（云主机）
- **防止复发**: 明确区分"环境限制"与"指纹问题"；本地 H3 回环测试不受影响
- **状态**: ⚠️规避（环境限制，非缺陷）

### [2026-08-26] 间歇性 DNS 抖动（127.0.0.53）
- **分类**: 验证环境
- **症状**: 部分域名（如 tls.browserleaks.com）解析失败（server misbehaving / i/o timeout）
- **根因**: 本机 systemd-resolved(127.0.0.53) 对特定域间歇性异常
- **修复**: 重试机制；不归因为指纹问题
- **防止复发**: 验证结果按"环境/DNS/指纹"分类，不一律归因
- **状态**: ⚠️规避（环境问题）

### [2026-08-26] curl 000/403 不代表网络不通——WAF 应用层指纹检测
- **分类**: 验证环境
- **症状**: curl 访问 akamai.com 稳定 403（5/5），cloak 稳定 200（5/5）；curl 部分站点 000
- **根因**: 网络可达（git push 证明），拦截发生在应用层——Akamai 检测 TLS 指纹判定 curl 为机器人
- **修复**: 无需修复——这是指纹有效性的**反向证明**；用 curl vs cloak 同环境对照判断
- **防止复发**: 判断指纹有效性看**服务端判定结果**（JA3/JA4 匹配 + WAF 放行），不是连通性
- **状态**: ✅已论证（docs/fingerprint-verification.md §6）

### [2026-08-26] 编译产物二进制误入库
- **分类**: 工具误操作
- **症状**: verify-sites、stress-varied 二进制被 `git add -A` 提交进库（mode 100755）
- **根因**: 构建产物与源码同目录，`git add -A` 一并提交
- **修复**: `git rm --cached` 移除 + 加入 .gitignore
- **防止复发**: 新增 cmd/ 工具后先检查 git status；二进制加 .gitignore
- **状态**: ✅已修复（两次，已形成规则）

### [2026-08-26] 批量替换把函数体写错（newReq 递归调用自己）
- **分类**: 工具误操作
- **症状**: stack overflow 被误判为 OOM（exit 137），排查浪费大量时间
- **根因**: execute_code 批量替换 `cloak.ImpersonateRequest(randProfile())` → `newReq()` 时，把 newReq 定义本身也替换了
- **修复**: 修正为 `cloak.ImpersonateRequest(randProfile())`
- **防止复发**: 批量替换后**必须**：grep 检查关键函数体 + go build + 冒烟测试
- **状态**: ✅已修复

### [2026-08-26] 压测工具服务器缺 handler 导致场景误判
- **分类**: 工具误操作
- **症状**: req_retry 场景失败 3239 次（attempts=1），误判为库 bug
- **根因**: 场景调用 `/status/429` 但服务器 mux 没注册该 handler → 返回 404 → 重试条件不满足
- **修复**: 补上 `/status/404/500/429` handlers
- **防止复发**: 场景与服务器端点必须一一对应，先冒烟再全量
- **状态**: ✅已修复

---

## 新增问题的自动记录流程

遇到新问题（缺陷/环境陷阱/工具误操作/任何值得记住的坑）时：

1. **立即**按上方格式追加到本文件（症状+根因+修复+防止复发）
2. 修复完成后更新状态（✅/⚠️/❌）
3. 如果是通用规则（非本项目特有），同步考虑是否需要写入 Hermes 技能/记忆

> 记录优先于修复——先记下来，再动手。防止"修完就忘"。
