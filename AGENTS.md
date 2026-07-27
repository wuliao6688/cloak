# AGENTS.md

## 适用范围

本文件适用于整个仓库。任何自动化工具、AI Agent 或维护者在修改代码，尤其是同步上游 `bogdanfinn/tls-client` 时，都必须先阅读本文件。

本文件的核心目的不是阻止上游更新，而是让上游的新功能、安全修复和画像更新能够进入本仓库，同时保留已经验证过的本地增强语义，避免因为简单选择 `ours` 或 `theirs` 造成回归。

## 项目身份

- Go module：`github.com/bogdanfinn/tls-client`
- 上游仓库：`https://github.com/bogdanfinn/tls-client.git`
- 上游默认分支（编写本文件时）：`master`
- 当前上游基线（编写本文件时）：`b790a31 Chrome150 (#259)`
- Go 版本：`1.26.5`
- 设计参考：`https://gitee.com/hqs666/kurl-client`

上述基线只是历史记录。每次同步前必须重新执行 `git remote -v`、`git branch -a` 和 `git log` 验证，不能假设远程名称、默认分支或提交哈希永久不变。

当前 `origin` 在编写本文件时直接指向上游仓库。若用户建立了自己的 GitHub fork，推荐的远程布局是：

```text
origin    -> 用户自己的 fork，只向这里推送
upstream  -> bogdanfinn/tls-client，只拉取，不推送
```

不要在未确认用户权限和意图时向当前 `origin` 或 `upstream` 推送。

## 总体维护原则

1. **上游优先，但行为不变量优先于旧实现细节。**
   - 接受上游的新画像、协议修复、安全修复、依赖更新和 API 演进。
   - 本地增强如果已被上游以更完整的方案实现，应迁移到上游方案，而不是机械保留两套实现。
   - 删除本地方案前，必须用测试证明下面记录的不变量仍成立。

2. **禁止整仓或热点文件无脑选边。**
   - 禁止使用 `git checkout --ours .`、`git checkout --theirs .`、批量覆盖整个冲突文件。
   - `client.go`、`roundtripper.go`、`racer.go`、`cffi_src/factory.go` 等文件必须逐个函数合并。

3. **保留用户工作。**
   - 同步前先检查 `git status --short`、未跟踪文件和 staged 内容。
   - 不得擅自执行 `git reset --hard`、`git clean -fd`、删除 stash 或覆盖用户未提交修改。
   - 工作树不干净时，默认先让用户确认备份方式：创建 patch，或由用户自行提交。用户明确要求跳过备份时，可以在记录该授权后直接继续，但仍不得覆盖任务范围外的用户修改。
   - 本项目的日常修改和上游同步直接在用户当前选择的本地维护分支上进行（编写本条时为 `main`；上游默认分支仍为 `master`），自动化工具不得自行创建额外工作分支或临时分支。

4. **以运行行为和测试为准。**
   - 证据优先级：实际测试/运行行为 > 当前构建产物 > 配置 > 源码注释 > 历史文档。
   - 上游注释与本地测试冲突时，先查清实际行为，不要仅凭注释决定。

5. **一次只解决一个冲突域。**
   - 建议顺序：依赖与画像 → Client API → Transport → Racing → CFFI → 测试/CI → 文档。
   - 每解决一个冲突域，先运行该域的最小测试，再继续下一个。

## 本地增强清单与不变量

以下内容是后续上游同步时必须重点核对的本地行为。文件名可以变化，实现可以重构，但语义不能在无说明、无测试的情况下消失。

### 1. 画像解析、元数据与不可变性

相关文件：

- `profiles/profiles.go`
- `profiles/resolver.go`
- `profiles/metadata.go`
- `profiles/resolver_test.go`
- `profiles/metadata_test.go`
- `profiles/profiles_immutability_test.go`
- `tests/ja3_integration_test.go`

必须保留的行为：

- `ResolveClientProfile` / `ResolveClientProfileWithKey` 保持旧版兼容：未知标识符回退默认画像。
- `ResolveClientProfileStrict` / `ResolveClientProfileWithKeyStrict` 对未知画像返回 `ErrUnknownClientProfile`。
- 严格解析忽略首尾空白并大小写不敏感，必须正确处理注册表中大写 `_PSK` key。
- `random` 与 `chaos` 从真实注册画像中选择，不随机拼接 TLS 参数。
- 随机候选必须排除：
  - Zalando、Nike、Cloudscraper、MMS、Mesh、Confirmed 等业务定制画像；
  - `_PSK`、`_PSK_PQ` 等显式恢复画像；
  - `KnownGaps` 非空画像。
- `ClientProfile` 构造器和 map/slice/指针 Getter 必须使用防御性复制，调用方不能污染全局画像。
- `GetProfileMetadata` 和 `AllProfileMetadata` 必须返回深度足够的副本，至少不能泄漏 `KnownGaps`、`VerifiedAgainst` slice。
- 新增浏览器/移动客户端画像时，应同步：
  - 注册表；
  - metadata；
  - 是否允许 random；
  - `TLSBase`、`KnownGaps`、`VerifiedAgainst`；
  - 必要的 JA3/HTTP2/HTTP3 验证数据。
- 上游修改 `DefaultClientProfile` 后，必须同步 README 的已知边界和相关测试。

如果上游新增自己的画像解析器，应优先复用上游注册机制，但必须补齐严格错误语义、随机过滤和不可变性测试。

### 2. HTTP Client 动态状态和输入隔离

相关文件：

- `client.go`
- `client_options.go`
- `client_internal_test.go`
- `client_options_internal_test.go`

必须保留的行为：

- 不得复制已经投入使用的 `http.Client` 值，因为其内部可能包含 mutex、请求取消表等同步状态。
- 动态修改代理、重定向和 Cookie Jar 时，使用新的 `*http.Client` 状态指针替换；正在执行的请求继续使用自己的快照。
- `SetProxy` 成功后：
  - `Transport` 和 `dialer` 必须同步切换；
  - 回到直连时必须使用带超时、本地地址和自定义 `net.Dialer` 的 direct dialer，不能退化成裸 `proxy.Direct`；
  - 旧 Transport 的空闲连接在状态锁之外关闭。
- `SetProxy` 失败时保留旧 Client、旧 Transport 和旧 Dialer，并恢复配置值。
- 默认 Header 按字段合并，请求自身字段优先；字段名比较必须大小写不敏感。
- 请求只设置一个 Header 时，不能导致所有其他默认 Header 消失。
- Header、CONNECT Header、证书 Pin 列表和 `TransportOptions` 在构建时防御性复制。
- `WithCatchPanics` 必须把 panic 转成显式 error，不能出现 `(nil, nil)`。
- Cookie Jar 获取和切换必须通过状态锁快照；不要长时间持锁执行网络 I/O。

如果上游把 `httpClient` 改成其他状态容器，应保留“请求快照 + 原子/加锁切换”的语义，并继续避免复制活跃 `http.Client`。

### 3. Transport 缓存和 TLS 握手并发

相关文件：

- `roundtripper.go`
- `roundtripper_internal_test.go`

必须保留的行为：

- 不得用一个全局 mutex 覆盖完整 TCP/TLS 握手。
- `cachedConnections`、`cachedTransports` 使用独立锁；Transport 读路径可使用 `RWMutex`。
- Transport 初始化按目标 key 单飞，同一目标只初始化一次，不同目标可以并行握手。
- map 锁只保护短时读写，不能在锁内执行 Dial、TLS Handshake、证书 Pinning 或关闭大量连接。
- 协议探测缓存的连接必须有清晰所有权：取出即删除，覆盖旧连接时关闭旧连接。
- `CloseIdleConnections` 先在锁内快照/切换状态，再在锁外关闭连接和 Transport。
- Transport 缓存必须有可配置 LRU 上限；回收只在 map 锁外关闭空闲连接，不得中断正在使用的响应体。
- HTTP/2 后续重连握手必须继承等待该 host 的活动请求 context，不能永久退化为 `context.Background()`。
- Transport 初始化在请求交给下游 RoundTripper 之前失败时，本层负责关闭原请求体。
- “理论上不可能”的分支返回正常 error，不使用 panic 作为常规控制流。

合并上游 `roundtripper.go` 时，特别检查 `dialTLS`、`getTransport`、`getOrCreateTransport` 是否形成锁递归或重新扩大临界区。

### 4. HTTP/3 与 HTTP/2 Protocol Racing

相关文件：

- `racer.go`
- `racer_test.go`
- `roundtripper.go`

必须保留的行为：

- 只有 `GET`、`HEAD`、`OPTIONS` 可以参与双协议竞速。
- 有 Body 时必须有 `GetBody`；HTTP/2 和 HTTP/3 使用互相独立的 request/body clone。
- POST、PUT、PATCH、DELETE 等请求不得因竞速被重复发送。
- 原请求 Body 如果不会交给下游 Transport，本层必须关闭；未进入 RoundTrip 的 clone 也必须关闭。
- HTTP/3 立即尝试，HTTP/2 的约 300ms 延迟必须可以被 context 取消。
- Racing 延迟或等待超时允许配置时，默认值必须继续保持约 300ms/10s，负延迟和非正超时必须在构建阶段拒绝。
- 竞速继承原请求 context，并有独立的等待超时。
- 获胜协议只取消输家；获胜请求 context 保持到响应 Body EOF 或 Close，不能在返回 Response 时立即取消。
- 输家 Response Body 必须关闭；输家的临时 HTTP/3 Transport 必须关闭。
- 缓存实际获胜的 HTTP/3 Transport，不得获胜后重新构造一个未参与请求的 Transport。
- 同一 host 首次竞速单飞，其他 host 仍能并行。
- 缓存协议失败时清除失效协议和 Transport，再允许安全请求重新竞速。
- 清理协程必须能够随两个尝试结束而退出，不能依赖错误的固定结果计数永久阻塞。

如果上游重写 Racing，优先采用上游成熟实现，但以上场景必须有等价测试。

### 5. CFFI session 生命周期

相关文件：

- `cffi_src/factory.go`
- `cffi_src/factory_concurrency_test.go`
- `cffi_dist/main.go`
- `cffi_dist/go.mod`

必须保留的行为：

- client registry 使用读写锁；普通读取不占写锁。
- 构建客户端和网络 I/O 不能持有全局 client map 写锁。
- 不同 `sessionId` 可以并行。
- 同一 `sessionId` 具有 flight 租约，覆盖：
  - 查找/创建 client；
  - 动态代理和重定向修改；
  - BuildRequest、Cookie 更新、Do；
  - BuildResponse 和响应体读取。
- 只有完成当前 session 请求后，下一个同 session 请求才能改变动态代理，避免 A 请求使用 B 请求代理。
- session 租约在 success、error、panic 路径都必须释放，release 必须幂等。
- `RemoveSession` 与 `ClearSessionCache` 先从 map 分离 client，再在全局锁外关闭空闲连接。
- `ClearSessionCache` 是 session 生命周期边界：不能出现 clear 完成后，clear 前开始的构建又写回旧 client。
- 创建失败的 client 不能写入 session map。
- CFFI 对 `tlsClientIdentifier` 使用严格画像解析；已有 session 也不能绕过未知标识符校验。
- session 缓存使用可配置的 LRU 容量和 idle TTL；驱逐必须跳过正在执行或等待 flight 的 session，并在全局锁外关闭连接。
- 启用 idle TTL 后不得在每个 session 请求结束时扫描整个缓存；使用下一到期时间、单飞清理或等价的摊销 O(1) 快路径。
- CFFI JSON 返回缓冲区必须保持现有 `freeMemory` ABI；优化大响应时避免无必要的 payload 级中间字符串复制。

#### 嵌套 module 不变量

`cffi_dist` 是独立 Go module。根目录的 `go test ./...` 不会覆盖它。

`cffi_dist/go.mod` 中必须保留：

```go
replace github.com/bogdanfinn/tls-client => ../
```

否则 CFFI 动态库会链接已发布的旧上游版本，而不是当前工作树，本地 session/Racing/画像修复都可能静默丢失。

每次同步上游依赖或 CFFI 代码后，必须单独进入 `cffi_dist` 编译。

Windows 使用 Go 1.26.x 构建 `c-shared` 时，链接阶段的临时 DLL 基础名不能包含 `-`。Go 会把输出名写入未加引号的 `.def` `LIBRARY` 指令，MinGW 会把连字符解析为语法错误。`cffi_dist/build.sh` 必须先用仅含字母、数字和下划线的安全名称链接，再把 DLL 和 `.h` 一并重命名为兼容的分发名称。

### 6. 测试、CI 和文档

相关文件：

- `.github/workflows/ci.yml`
- `Readme.md`
- `AGENTS.md`
- 根目录、`profiles`、`cffi_src` 下新增的内部测试
- `tests/ja3_integration_test.go`

必须保留的行为：

- 默认 CI 不运行依赖公网的完整 `tests`。
- 使用 `go test -run '^$' ./...` 编译根 module 的全部 package。
- 单独编译 `cffi_dist` 嵌套 module。
- 核心 root、profiles、bandwidth、cffi_src 运行 race 测试。
- 本地集成测试只精确选择 `httptest`/离线用例。
- JA3 在线验证使用 `integration` build tag，不进入默认 CI。
- README 必须如实说明能力、限制、random 过滤、Racing 请求约束和 CFFI 并发模型。
- 上游同步改变本地不变量后，应同时更新本文件，而不是让文档继续描述已经不存在的实现。

## 上游同步标准流程

### 0. 明确任务范围

同步前先回答：

- 目标是合并上游全部更新，还是只取某个 PR/commit？
- 是否允许修改公开 API？
- 是否需要保持 CFFI ABI/JSON 字段兼容？
- 是否允许更新 Go 版本和依赖？
- 当前环境是否允许访问 GitHub、下载依赖和运行在线测试？

不明确时，默认：合并上游默认分支、保持公开 API/CFFI JSON 兼容、不运行在线测试。

### 1. 记录现场

至少记录：

```bash
git status --short
git remote -v
git branch --show-current
git rev-parse HEAD
git log -5 --oneline
```

如果工作树不干净，停止合并操作并先保护用户修改。不要把用户修改和上游 merge 冲突混在同一个不可恢复步骤中。

### 2. 配置远程

理想状态：

```bash
git remote -v
# origin   <用户 fork>
# upstream https://github.com/bogdanfinn/tls-client.git
```

若当前 `origin` 仍指向上游，只有在用户提供 fork 地址并同意后才能调整远程。不要猜测用户的 fork URL。

### 3. 确认本地维护分支

本项目明确要求直接在用户当前选择的本地维护分支上修改和同步，不创建 `codex/`、临时同步或其他工作分支：

```bash
git branch --show-current
# 当前仓库预期输出：main
```

如果当前分支不是用户指定的维护分支，必须先确认意图；不得擅自切换、创建或合并分支。不得把远程跟踪分支、WIP 分支或实验分支无差别合并。试验性修改默认使用仓库外 patch 备份和小步可审阅 diff；用户明确要求跳过备份时可以继续，但不通过新建分支隔离。

### 4. 获取和审阅上游变化

```bash
git fetch upstream --tags
git log --oneline --decorate HEAD..upstream/master
git diff --stat HEAD...upstream/master
```

先按目录分类变化：

- profiles / uTLS / dependencies；
- client API / options；
- transport / HTTP2 / HTTP3；
- CFFI；
- tests / CI / docs。

发现上游也实现了本地同类功能时，先比较行为和测试，再决定保留哪套实现。

### 5. 使用 merge 保留上游祖先关系

默认使用 merge，而不是把整段上游历史逐个 cherry-pick 或 rebase 到无法辨认：

```bash
git merge --no-commit upstream/master
```

只有用户明确要求线性历史时才考虑 rebase。长期 fork 周期性跟进上游时，保留 merge ancestry 通常更容易进行下一次同步。

### 6. 逐域解决冲突

推荐顺序：

1. `go.mod`、`go.sum`、Go 版本；
2. `profiles/`；
3. `client_options.go`、`client.go`；
4. `roundtripper.go`；
5. `racer.go`；
6. `cffi_src/`；
7. `cffi_dist/`；
8. tests、CI、README、AGENTS。

每个冲突文件都要回答：

- 上游新增了什么行为？
- 本地修改解决了什么问题？
- 两者是否可以组合？
- 上游是否已经覆盖本地问题？
- 哪些测试证明最终行为？

### 7. 处理依赖与画像更新

- 优先接受上游 uTLS/fhttp/quic-go 安全和兼容性更新。
- 更新 Go 版本时同步：根 `go.mod`、`.tool-versions`、`cffi_dist/go.mod`、Dockerfile、CI、README。
- 上游新增画像时补齐 metadata 和 random 策略。
- 上游删除/重命名画像时保留必要兼容 alias，或在发布说明中明确 breaking change。
- 不要因为 `cffi_dist/go.mod` 的 `replace` 而忽略其 require/go.sum 维护；replace 只负责指向当前源码。

### 8. 格式化与验证

先确认：

```bash
go version
# 应与 go.mod / .tool-versions 一致
```

格式化所有本次修改的 Go 文件：

```bash
gofmt -w <本次修改的 .go 文件>
```

根 module 基础验证：

```bash
go test . ./profiles ./bandwidth ./cffi_src
go test -run '^$' ./...
go vet ./...
go test -race . ./profiles ./bandwidth ./cffi_src
```

离线 integration 验证：

```bash
go test -race ./tests -run '^(TestConfigValidation_|TestRecorded|TestClient_UseSameConnection|TestClient_UseDifferentConnection|TestClient_UseCompressedResponse|TestClient_Redirect|TestClient_TestFailWithTimeout|TestWebSocketEcho$|TestWebSocketWithHeaderOrder|TestWebSocketWithoutHeaderOrder)'
```

CFFI 嵌套 module：

```bash
cd cffi_dist
go test -run '^$' ./...
cd ..
```

在线画像测试只在网络允许且明确需要时运行：

```bash
go test -tags=integration ./tests -run '^TestJA3Integration_'
```

Windows 本地环境如果不能运行 `-race`，必须至少完成编译，并让 Linux CI 执行 race。不得把“未安装 Go/缺少 C 编译器”描述成测试通过。

最后检查：

```bash
git diff --check
git status --short
git diff --stat
```

### 9. 提交前审阅

确认：

- 没有冲突标记：`<<<<<<<`、`=======`、`>>>>>>>`；
- 没有误删本地新增测试；
- 没有恢复全局握手锁；
- 没有复制活跃 `http.Client`；
- 没有让非安全请求进入双协议竞速；
- 没有在返回 winner response 时取消 winner context；
- 没有缩短 CFFI session flight 到仅 client lookup；
- 没有删除 `cffi_dist/go.mod` 的本地 replace；
- README、AGENTS 与实际行为一致。

## 冲突热点快速决策表

| 文件/区域 | 上游内容通常应接受 | 本地语义必须复核 |
| --- | --- | --- |
| `go.mod` / `go.sum` | 新依赖、安全升级、Go 版本 | 根与 CFFI module 版本一致，replace 保留 |
| `profiles/internal_browser_profiles.go` | 新/修正画像 | metadata、random、KnownGaps、测试同步 |
| `profiles/profiles.go` | 注册表与默认画像变化 | Getter 防御性复制、兼容 resolver |
| `client.go` | 新 API、Hook、客户端行为修复 | 指针快照切换、代理 Dialer、Header 合并、panic error |
| `client_options.go` | 新 option 和校验 | 输入对象防御性复制、Racing 约束文档 |
| `roundtripper.go` | ALPN、HTTP2/3、TLS 修复 | 短 map 锁、按 key 单飞、Body 所有权 |
| `racer.go` | 上游 Racing 重构 | 安全方法、独立 Body、winner context、输家清理 |
| `cffi_src/factory.go` | JSON 字段、新 options | session flight、严格画像、锁外关闭 |
| `cffi_dist/main.go` | ABI/导出函数变化 | `CreateClientForRequest` 租约覆盖完整请求 |
| `cffi_dist/go.mod` | 依赖版本 | `replace => ../` 不得丢失 |
| `.github/workflows/ci.yml` | 新平台/测试 | 默认离线、race、嵌套 module 编译 |

## 修改边界

- 不要为了减少冲突而大规模重命名上游公开 API、包名或目录。
- 不要无依据修改模块路径；若用户决定发布独立 fork module，这是单独迁移任务，需要同步所有 import、CFFI module、示例和文档。
- 不要自动更新所有画像指纹；画像变更必须有上游证据或实际抓包/指纹验证。
- 不要把 `random` 实现成随机 JA3/扩展拼接器。
- 不要在默认 CI 中加入不稳定公网依赖。
- 不要因为测试环境缺少 Go、网络、CGO 就删除测试或降低验证要求。

## 完成上游同步时的报告格式

向用户报告时至少包括：

1. 合并到的上游 commit/tag；
2. 接受的上游主要变化；
3. 发生冲突的热点文件和解决策略；
4. 保留、替换或删除了哪些本地增强，以及原因；
5. 实际运行的格式化、编译、单测、race、vet、CFFI 验证；
6. 未运行的在线测试和原因；
7. 剩余已知风险或需要人工确认的画像差异。

不要只说“合并完成”或“测试通过”，必须给出可复现的命令和真实结果。
