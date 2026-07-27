' tlsgateway 易语言调用示例
' 
' 构建 DLL:
'   go build -buildmode=c-shared -o libtlsgateway.so ./cffi_dist
'
' 编译本示例:
'   将 libtlsgateway.so 放在程序目录

.版本 2
.支持库 spec

.程序集 窗口程序集_启动窗口

' ─── DLL 声明 ─────────────────────────────────────────────

.DLL命令 tg_session_create, 文本型, "libtlsgateway.so", "tg_session_create"
    .参数 profileID, 整数型
    .参数 timeoutSec, 整数型
    .参数 proxyURL, 文本型

.DLL命令 tg_session_free, , "libtlsgateway.so", "tg_session_free"
    .参数 sessionID, 文本型

.DLL命令 tg_get, 整数型, "libtlsgateway.so", "tg_get"
    .参数 sessionID, 文本型
    .参数 url, 文本型

.DLL命令 tg_post, 整数型, "libtlsgateway.so", "tg_post"
    .参数 sessionID, 文本型
    .参数 url, 文本型
    .参数 body, 文本型

.DLL命令 tg_request, 整数型, "libtlsgateway.so", "tg_request"
    .参数 sessionID, 文本型
    .参数 method, 文本型
    .参数 url, 文本型
    .参数 headers, 文本型
    .参数 body, 文本型

.DLL命令 tg_response_status, 整数型, "libtlsgateway.so", "tg_response_status"
    .参数 response, 整数型

.DLL命令 tg_response_body, 文本型, "libtlsgateway.so", "tg_response_body"
    .参数 response, 整数型

.DLL命令 tg_response_body_len, 整数型, "libtlsgateway.so", "tg_response_body_len"
    .参数 response, 整数型

.DLL命令 tg_response_error, 文本型, "libtlsgateway.so", "tg_response_error"
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

' ─── 示例：GET 请求 ───────────────────────────────────────

.子程序 示例_GET请求
    .局部变量 session, 文本型
    .局部变量 resp, 整数型
    .局部变量 状态码, 整数型
    .局部变量 返回文本, 文本型
    .局部变量 错误信息, 文本型
    
    ' 创建 session（Chrome 150, 30秒超时, 无代理）
    session = tg_session_create(PROFILE_CHROME_150, 30, "")
    
    如果 (取文本左边(session, 4) = "ERR:") 则
        调试输出("创建失败: " + session)
        返回
    结束如果
    
    ' GET 请求
    resp = tg_get(session, "https://httpbin.org/ip")
    
    如果 (resp = 0) 则
        调试输出("请求失败")
        返回
    结束如果
    
    状态码 = tg_response_status(resp)
    返回文本 = tg_response_body(resp)
    错误信息 = tg_response_error(resp)
    
    如果 (错误信息 = "") 则
        调试输出("状态: " + 到文本(状态码))
        调试输出("内容: " + 返回文本)
    否则
        调试输出("错误: " + 错误信息)
    结束如果
    
    tg_response_free(resp)
    tg_session_free(session)

' ─── 示例：POST 请求 ─────────────────────────────────────

.子程序 示例_POST请求
    .局部变量 session, 文本型
    .局部变量 resp, 整数型
    .局部变量 状态码, 整数型
    .局部变量 返回文本, 文本型
    
    session = tg_session_create(PROFILE_FIREFOX_148, 60, "http://127.0.0.1:8080")
    
    如果 (取文本左边(session, 4) = "ERR:") 则
        调试输出("创建失败: " + session)
        返回
    结束如果
    
    resp = tg_post(session, "https://httpbin.org/post", "name=test&value=123")
    
    状态码 = tg_response_status(resp)
    返回文本 = tg_response_body(resp)
    
    调试输出("POST 状态: " + 到文本(状态码))
    调试输出("POST 返回: " + 返回文本)
    
    tg_response_free(resp)
    tg_session_free(session)

' ─── 示例：完整控制 ───────────────────────────────────────

.子程序 示例_完整请求
    .局部变量 session, 文本型
    .局部变量 resp, 整数型
    
    session = tg_session_create(PROFILE_SAFARI_IOS_18_5, 30, "")
    
    ' 自定义方法 + 请求头 + body
    resp = tg_request(session, "PUT", "https://httpbin.org/put", "Content-Type: application/json" + 字符(10) + "Authorization: Bearer xxx", "{\"key\":\"value\"}")
    
    调试输出("状态: " + 到文本(tg_response_status(resp)))
    
    tg_response_free(resp)
    tg_session_free(session)
