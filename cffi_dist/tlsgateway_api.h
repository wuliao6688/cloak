/* tlsgateway_api.h — 商用级 TLS 指纹 SDK
 *
 * 零 JSON，纯 C 类型，支持画像/代理/TLS 三重轮换防检测。
 *
 * === 快速开始 ===
 *   char* s = tg_session_create(TLS_PROFILE_CHROME_150, 30, "http://proxy:8080");
 *
 *   // 开启双重防检测
 *   tg_session_set_rotate(s, 1, 3, 20);  // 每3次请求切换Chrome画像
 *   tg_session_set_proxy_list(s, "http://ip1:8080\nhttp://ip2:8080", 5);
 *
 *   TgResponse* r = tg_get(s, "https://httpbin.org/ip");
 *   printf("status=%d\n", tg_response_status(r));
 *   tg_response_free(r);
 *   tg_session_free(s);
 */

#ifndef TLSGATEWAY_API_H
#define TLSGATEWAY_API_H

#include "tlsgateway_profiles.h"

#ifdef __cplusplus
extern "C" {
#endif

/* ─── Types ─────────────────────────────────────────────── */

typedef struct {
    int    status;      // HTTP 状态码
    int    errorCode;   // 0=成功, 1=网络, 2=HTTP, 3=会话, 4=超时, 5=画像
    char*  body;        // 响应体
    int    bodyLen;     // 响应体长度
    char*  headers;     // 响应头（"Key: Value\n..." 格式）
    char*  error;       // 错误描述（NULL=成功）
} TgResponse;

/* ─── Session Lifecycle ─────────────────────────────────── */

/** 创建会话（返回字符串 ID）。 */
char* tg_session_create(int profileID, int timeoutSec, const char* proxyURL);

/** 创建会话（返回整数 handle，-1=失败）。易语言/C 友好。 */
int   tg_session_create_int(int profileID, int timeoutSec, const char* proxyURL);

void  tg_session_free(const char* sessionID);

/* ─── Proxy Management ──────────────────────────────────── */

/**
 * 设置/切换代理。支持 HTTP/SOCKS5/认证。
 * 格式: "http://user:pass@host:port" / "socks5://host:port" / "" (清除)
 */
int   tg_session_set_proxy(const char* sessionID, const char* proxyURL);
char* tg_session_get_proxy(const char* sessionID);

/**
 * 设置代理池。格式: "http://ip1:8080\nhttp://ip2:8080\nsocks5://ip3:1080"
 * rotateEveryN: 每N次请求切换代理，0=每请求。
 */
int   tg_session_set_proxy_list(const char* sessionID, const char* proxyList, int rotateEveryN);

/* ─── Anti-Detection ─────────────────────────────────────── */

/**
 * 画像轮换 + TLS 刷新。
 * group: 1=Chrome 2=Firefox 3=Safari 4=Mobile 5=All 6=Chaos(每请求随机画像)
 * everyN: 每N次请求切换画像 (Chaos忽略)  tlsRefresh: 每N次刷新TLS(推荐50,0=禁用)
 */
int   tg_session_set_rotate(const char* sessionID, int group, int everyN, int tlsRefresh);

/* ─── Profile Management ────────────────────────────────── */
int   tg_session_set_profile(const char* sessionID, int profileID);
int   tg_session_get_profile(const char* sessionID);

/* ─── Cookie Management ─────────────────────────────────── */
char* tg_session_get_cookies(const char* sessionID, const char* url);
int   tg_session_set_cookies(const char* sessionID, const char* url, const char* cookies);
int   tg_session_clear_cookies(const char* sessionID);

/** 启用/禁用 Cookie jar（0=禁用, 非0=启用）。禁用后请求不带 Cookie。 */
int   tg_session_set_cookie_store(const char* sessionID, int enable);

/** H2 随机化（Chaos 模式自动启用）。随机化 TLS 扩展顺序 + H2 Settings。 */
int   tg_session_set_h2_randomize(const char* sessionID, int enable);

/** 设置自定义 CA 证书路径（PEM 格式）。 */
int   tg_session_set_ca_cert(const char* sessionID, const char* certPath);

/* ─── HTTP Requests ─────────────────────────────────────── */
TgResponse* tg_get(const char* sessionID, const char* url);
TgResponse* tg_post(const char* sessionID, const char* url, const char* body);

/** 二进制 POST（data=NULL / dataLen=0 允许）。 */
TgResponse* tg_post_bin(const char* sessionID, const char* url,
                        const void* data, int dataLen);

/** Multipart 文件上传。filePath=文件路径 fieldName=表单字段名。 */
TgResponse* tg_post_multipart(const char* sessionID, const char* url,
                              const char* filePath, const char* fieldName);

TgResponse* tg_request(const char* sessionID, const char* method, const char* url,
                       const char* headers, const char* body);

/* ─── Convenience ───────────────────────────────────────── */
char* tg_get_body(const char* sessionID, const char* url);
int   tg_get_status(const char* sessionID, const char* url);

/* ─── Response Access ───────────────────────────────────── */
int    tg_response_status(TgResponse* r);
char*  tg_response_body(TgResponse* r);
int    tg_response_body_len(TgResponse* r);
char*  tg_response_headers(TgResponse* r);
char*  tg_response_header(TgResponse* r, const char* name);
char*  tg_response_error(TgResponse* r);
int    tg_response_error_code(TgResponse* r);
void   tg_response_free(TgResponse* r);

#ifdef __cplusplus
}
#endif

#endif
