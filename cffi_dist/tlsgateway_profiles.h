/* tlsgateway_profiles.h — TLS 指纹画像常量
 *
 * 与 profiles/consts.go 同步。所有常量值稳定，不会因版本变更而重编号。
 * 配合 tlsgateway_api.h 的 tg_session_create(profileID, ...) 使用。
 */

#ifndef TLSGATEWAY_PROFILES_H
#define TLSGATEWAY_PROFILES_H

#ifdef __cplusplus
extern "C" {
#endif

/* Chrome (100-series) */
#define TLS_PROFILE_CHROME_150          1
#define TLS_PROFILE_CHROME_146          2
#define TLS_PROFILE_CHROME_131          3
#define TLS_PROFILE_CHROME_120          4
#define TLS_PROFILE_CHROME_117          5
#define TLS_PROFILE_CHROME_116          6

/* Firefox (100-series) */
#define TLS_PROFILE_FIREFOX_148        10
#define TLS_PROFILE_FIREFOX_147        11
#define TLS_PROFILE_FIREFOX_132        12
#define TLS_PROFILE_FIREFOX_120        13

/* Safari (iOS + macOS) */
#define TLS_PROFILE_SAFARI_IOS_18_5    20
#define TLS_PROFILE_SAFARI_IOS_17_0    21
#define TLS_PROFILE_SAFARI_IOS_16_0    22
#define TLS_PROFILE_SAFARI_15_6_1      23
#define TLS_PROFILE_SAFARI_IPAD_15_6   24

/* Chromium-based (Opera/Brave/Edge) */
#define TLS_PROFILE_OPERA_91           30
#define TLS_PROFILE_BRAVE_146          31
#define TLS_PROFILE_EDGE_120           50

/* Mobile */
#define TLS_PROFILE_OKHTTP4_ANDROID_13 40

/* String-based profiles (for JSON API) */
#define TLS_PROFILE_STRING_CHROME_150  "chrome_150"
#define TLS_PROFILE_STRING_FIREFOX_148 "firefox_148"
#define TLS_PROFILE_STRING_SAFARI_18_5 "safari_ios_18_5"

#ifdef __cplusplus
}
#endif

#endif /* TLSGATEWAY_PROFILES_H */
