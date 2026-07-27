' tlsgateway 易语言完整调用示例
'
' 构建 DLL:
'   go build -buildmode=c-shared -o libtlsgateway.so ./cffi_dist
'
' 编译本示例: 将 libtlsgateway.so 放在程序目录

.版本 2
.支持库 spec

.程序集 窗口程序集_启动窗口

' ─── DLL 声明 ─────────────────────────────────────────────
' 只需声明 18 个函数，覆盖所有场景

' === Session ===
.DLL命令 tg_session_create, 文本型, "libtlsgateway.so", "tg_session_create"
    .参数 profileID, 整数型
    .参数 timeoutSec, 整数型
    .参数 proxyURL, 文本型

.DLL命令 tg_session_free, , "libtlsgateway.so", "tg_session_free"
    .参数 sessionID, 文本型

' === Profile ===
.DLL命令 tg_session_set_profile, 整数型, "libtlsgateway.so", "tg_session_set_profile"
    .参数 sessionID, 文本型
    .参数 profileID, 整数型

.DLL命令 tg_session_get_profile, 整数型, "libtlsgateway.so", "tg_session_get_profile"
    .参数 sessionID, 文本型

' === Cookies ===
.DLL命令 tg_session_get_cookies, 文本型, "libtlsgateway.so", "tg_session_get_cookies"
    .参数 sessionID, 文本型
    .参数 url, 文本型

.DLL命令 tg_session_set_cookies, 整数型, "libtlsgateway.so", "tg_session_set_cookies"
    .参数 sessionID, 文本型
    .参数 url, 文本型
    .参数 cookies, 文本型

.DLL命令 tg_session_clear_cookies, 整数型, "libtlsgateway.so", "tg_session_clear_cookies"
    .参数 sessionID, 文本型

.DLL命令 tg_session_set_cookie_store, 整数型, "libtlsgateway.so", "tg_session_set_cookie_store"
    .参数 sessionID, 文本型
    .参数 enable, 整数型

.DLL命令 tg_session_set_h2_randomize, 整数型, "libtlsgateway.so", "tg_session_set_h2_randomize"
    .参数 sessionID, 文本型
    .参数 enable, 整数型

.DLL命令 tg_session_set_ca_cert, 整数型, "libtlsgateway.so", "tg_session_set_ca_cert"
    .参数 sessionID, 文本型
    .参数 certPath, 文本型

' === Proxy ===
.DLL命令 tg_session_set_proxy, 整数型, "libtlsgateway.so", "tg_session_set_proxy"
    .参数 sessionID, 文本型
    .参数 proxyURL, 文本型

.DLL命令 tg_session_get_proxy, 文本型, "libtlsgateway.so", "tg_session_get_proxy"
    .参数 sessionID, 文本型

.DLL命令 tg_session_set_proxy_list, 整数型, "libtlsgateway.so", "tg_session_set_proxy_list"
    .参数 sessionID, 文本型
    .参数 proxyList, 文本型
    .参数 rotateEveryN, 整数型

' === Anti-Detection ===
.DLL命令 tg_session_set_rotate, 整数型, "libtlsgateway.so", "tg_session_set_rotate"
    .参数 sessionID, 文本型
    .参数 rotateGroup, 整数型
    .参数 everyN, 整数型
    .参数 tlsRefreshEvery, 整数型

' === Requests ===
.DLL命令 tg_get, 整数型, "libtlsgateway.so", "tg_get"
    .参数 sessionID, 文本型
    .参数 url, 文本型

.DLL命令 tg_post, 整数型, "libtlsgateway.so", "tg_post"
    .参数 sessionID, 文本型
    .参数 url, 文本型
    .参数 body, 文本型

.DLL命令 tg_post_bin, 整数型, "libtlsgateway.so", "tg_post_bin"
    .参数 sessionID, 文本型
    .参数 url, 文本型
    .参数 data, 整数型
    .参数 dataLen, 整数型

.DLL命令 tg_post_multipart, 整数型, "libtlsgateway.so", "tg_post_multipart"
    .参数 sessionID, 文本型
    .参数 url, 文本型
    .参数 filePath, 文本型
    .参数 fieldName, 文本型

.DLL命令 tg_request, 整数型, "libtlsgateway.so", "tg_request"
    .参数 sessionID, 文本型
    .参数 method, 文本型
    .参数 url, 文本型
    .参数 headers, 文本型
    .参数 body, 文本型

' === Convenience ===
.DLL命令 tg_get_body, 文本型, "libtlsgateway.so", "tg_get_body"
    .参数 sessionID, 文本型
    .参数 url, 文本型

.DLL命令 tg_get_status, 整数型, "libtlsgateway.so", "tg_get_status"
    .参数 sessionID, 文本型
    .参数 url, 文本型

' === Response ===
.DLL命令 tg_response_status, 整数型, "libtlsgateway.so", "tg_response_status"
    .参数 response, 整数型

.DLL命令 tg_response_body, 文本型, "libtlsgateway.so", "tg_response_body"
    .参数 response, 整数型

.DLL命令 tg_response_body_len, 整数型, "libtlsgateway.so", "tg_response_body_len"
    .参数 response, 整数型

.DLL命令 tg_response_headers, 文本型, "libtlsgateway.so", "tg_response_headers"
    .参数 response, 整数型

.DLL命令 tg_response_header, 文本型, "libtlsgateway.so", "tg_response_header"
    .参数 response, 整数型
    .参数 name, 文本型

.DLL命令 tg_response_error, 文本型, "libtlsgateway.so", "tg_response_error"
    .参数 response, 整数型

.DLL命令 tg_response_error_code, 整数型, "libtlsgateway.so", "tg_response_error_code"
    .参数 response, 整数型

.DLL命令 tg_response_free, , "libtlsgateway.so", "tg_response_free"
    .参数 response, 整数型

' ─── 画像常量 ─────────────────────────────────────────────

.常量 PROFILE_CHROME_150, 1
.常量 PROFILE_CHROME_146, 2
.常量 PROFILE_FIREFOX_148, 10
.常量 PROFILE_FIREFOX_147, 11
.常量 PROFILE_SAFARI_IOS_18_5, 20
.常量 PROFILE_OPERA_91, 30
.常量 PROFILE_OKHTTP4_ANDROID_13, 40

' ─── 错误码 ───────────────────────────────────────────────

.常量 ERR_OK, 0
.常量 ERR_NETWORK, 1
.常量 ERR_HTTP, 2
.常量 ERR_SESSION, 3
.常量 ERR_TIMEOUT, 4
.常量 ERR_PROFILE, 5

' ─── 轮换组 ───────────────────────────────────────────────

.常量 ROTATE_CHROME, 1
.常量 ROTATE_FIREFOX, 2
.常量 ROTATE_SAFARI, 3
.常量 ROTATE_MOBILE, 4
.常量 ROTATE_ALL, 5
.常量 ROTATE_CHAOS, 6

' ═══════════════════════════════════════════════════════════
' 场景一：最简单的 GET 请求
' ═══════════════════════════════════════════════════════════

.子程序 场景1_简单GET
    .局部变量 session, 文本型
    .局部变量 返回文本, 文本型
    
    session = tg_session_create(PROFILE_CHROME_150, 30, "")
    返回文本 = tg_get_body(session, "https://httpbin.org/ip")
    
    如果 (返回文本 = "") 则
        调试输出("请求失败")
    否则
        调试输出("返回: " + 返回文本)
    结束如果
    
    tg_session_free(session)

' ═══════════════════════════════════════════════════════════
' 场景二：完整响应（状态码 + body + 响应头）
' ═══════════════════════════════════════════════════════════

.子程序 场景2_完整响应
    .局部变量 session, 文本型
    .局部变量 resp, 整数型
    .局部变量 状态码, 整数型
    .局部变量 错误码, 整数型
    .局部变量 返回文本, 文本型
    .局部变量 响应头, 文本型
    .局部变量 ContentType, 文本型
    
    session = tg_session_create(PROFILE_FIREFOX_148, 30, "")
    resp = tg_get(session, "https://httpbin.org/json")
    
    错误码 = tg_response_error_code(resp)
    
    如果 (错误码 = ERR_OK) 则
        状态码 = tg_response_status(resp)
        返回文本 = tg_response_body(resp)
        响应头 = tg_response_headers(resp)
        ContentType = tg_response_header(resp, "Content-Type")
        
        调试输出("状态: " + 到文本(状态码))
        调试输出("Content-Type: " + ContentType)
        调试输出("所有响应头: " + 响应头)
        调试输出("内容: " + 取文本左边(返回文本, 200))
    否则
        .局部变量 错误信息, 文本型
        错误信息 = tg_response_error(resp)
        调试输出("错误(" + 到文本(错误码) + "): " + 错误信息)
    结束如果
    
    tg_response_free(resp)
    tg_session_free(session)

' ═══════════════════════════════════════════════════════════
' 场景三：登录流程（Cookie 延续）
' ═══════════════════════════════════════════════════════════

.子程序 场景3_登录流程
    .局部变量 session, 文本型
    .局部变量 resp, 整数型
    .局部变量 cookies, 文本型
    .局部变量 错误码, 整数型
    
    session = tg_session_create(PROFILE_CHROME_150, 30, "")
    
    ' 第一步：GET 登录页
    resp = tg_get(session, "https://httpbin.org/cookies/set?session=abc123")
    
    错误码 = tg_response_error_code(resp)
    如果 (错误码 = ERR_OK) 则
        调试输出("登录页状态: " + 到文本(tg_response_status(resp)))
        调试输出("Set-Cookie: " + tg_response_header(resp, "Set-Cookie"))
    结束如果
    tg_response_free(resp)
    
    ' 第二步：查看保存的 Cookie
    cookies = tg_session_get_cookies(session, "https://httpbin.org")
    调试输出("当前Cookie: " + cookies)
    
    ' 第三步：带着 Cookie 请求内部页
    resp = tg_get(session, "https://httpbin.org/cookies")
    
    如果 (tg_response_error_code(resp) = ERR_OK) 则
        调试输出("Cookie验证: " + tg_response_body(resp))
    结束如果
    tg_response_free(resp)
    
    tg_session_free(session)

' ═══════════════════════════════════════════════════════════
' 场景四：POST 请求 + 自定义 Header
' ═══════════════════════════════════════════════════════════

.子程序 场景4_POST请求
    .局部变量 session, 文本型
    .局部变量 resp, 整数型
    
    session = tg_session_create(PROFILE_SAFARI_IOS_18_5, 30, "")
    
    ' POST JSON 数据 + 自定义请求头
    resp = tg_request(session, "POST", "https://httpbin.org/post", "Content-Type: application/json" + 字符(10) + "X-Custom: hello", "{\"name\":\"test\"}")
    
    调试输出("POST 状态: " + 到文本(tg_response_status(resp)))
    调试输出("POST 返回: " + tg_response_body(resp))
    
    tg_response_free(resp)
    tg_session_free(session)

' ═══════════════════════════════════════════════════════════
' 场景五：切换指纹 + 代理
' ═══════════════════════════════════════════════════════════

.子程序 场景5_切换指纹
    .局部变量 session, 文本型
    .局部变量 resp, 整数型
    .局部变量 ret, 整数型
    
    ' 创建 session，带代理
    session = tg_session_create(PROFILE_CHROME_150, 30, "http://127.0.0.1:8080")
    
    ' 第一次请求（Chrome 指纹）
    resp = tg_get(session, "https://httpbin.org/ip")
    调试输出("[Chrome] " + tg_response_body(resp))
    tg_response_free(resp)
    
    ' 切换到 Firefox 指纹（保留 Cookie + 代理）
    ret = tg_session_set_profile(session, PROFILE_FIREFOX_148)
    如果 (ret = ERR_OK) 则
        调试输出("指纹已切换: " + 到文本(tg_session_get_profile(session)))
    结束如果
    
    ' 第二次请求（Firefox 指纹，同样的 Cookie）
    resp = tg_get(session, "https://httpbin.org/ip")
    调试输出("[Firefox] " + tg_response_body(resp))
    tg_response_free(resp)
    
    tg_session_free(session)

' ═══════════════════════════════════════════════════════════
' 场景六：仅需状态码（超轻量）
' ═══════════════════════════════════════════════════════════

.子程序 场景6_仅状态码
    .局部变量 session, 文本型
    .局部变量 状态码, 整数型
    
    session = tg_session_create(PROFILE_CHROME_150, 10, "")
    状态码 = tg_get_status(session, "https://httpbin.org/status/404")
    调试输出("状态码: " + 到文本(状态码))
    tg_session_free(session)

' ═══════════════════════════════════════════════════════════
' 场景七：画像轮换防检测（人机平台专用）
' ═══════════════════════════════════════════════════════════

.子程序 场景7_画像轮换
    .局部变量 session, 文本型
    .局部变量 resp, 整数型
    .局部变量 i, 整数型
    
    session = tg_session_create(PROFILE_CHROME_150, 30, "")
    
    ' 开启轮换: Chrome 组, 每 3 次请求切换画像, 每 20 次请求刷新 TLS
    tg_session_set_rotate(session, ROTATE_CHROME, 3, 20)
    
    ' 发 10 次请求——自动在 Chrome116~150 之间轮换
    .计次循环首 (10, i)
        resp = tg_get(session, "https://httpbin.org/ip")
        调试输出("请求 #" + 到文本(i) + " 状态: " + 到文本(tg_response_status(resp)))
        tg_response_free(resp)
    .计次循环尾 ()
    
    tg_session_free(session)

' ═══════════════════════════════════════════════════════════
' 场景八：代理管理
' ═══════════════════════════════════════════════════════════

.子程序 场景8_代理管理
    .局部变量 session, 文本型
    
    ' 创建时不设代理
    session = tg_session_create(PROFILE_CHROME_150, 30, "")
    
    ' 动态设置代理（HTTP 认证）
    tg_session_set_proxy(session, "http://user:pass@127.0.0.1:8080")
    调试输出("当前代理: " + tg_session_get_proxy(session))
    
    ' 切换 SOCKS5
    tg_session_set_proxy(session, "socks5://127.0.0.1:1080")
    
    ' 代理池（每5次请求轮换）
    tg_session_set_proxy_list(session, "http://ip1:8080" + 字符(10) + "http://ip2:8080", 5)
    
    ' 清除
    tg_session_set_proxy(session, "")
    
    tg_session_free(session)

' ═══════════════════════════════════════════════════════════
' 场景九：Chaos 模式 — 每请求随机画像（最强防检测）
' ═══════════════════════════════════════════════════════════

.子程序 场景9_Chaos模式
    .局部变量 session, 文本型
    .局部变量 resp, 整数型
    .局部变量 i, 整数型
    
    session = tg_session_create(PROFILE_CHROME_150, 30, "")
    tg_session_set_rotate(session, ROTATE_CHAOS, 0, 0)
    
    .计次循环首 (10, i)
        resp = tg_get(session, "https://httpbin.org/ip")
        调试输出("请求 #" + 到文本(i) + " 画像=" + 到文本(tg_session_get_profile(session)) + " 状态=" + 到文本(tg_response_status(resp)))
        tg_response_free(resp)
    .计次循环尾 ()
    
    tg_session_free(session)
