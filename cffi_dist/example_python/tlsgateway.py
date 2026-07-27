"""tlsgateway SDK for Python — 商用级 TLS 指纹 HTTP 客户端

零 JSON，Session 模型，类型安全。

快速开始:
    from tlsgateway import Session
    s = Session(PROFILE_CHROME_150)
    r = s.get("https://httpbin.org/ip")
    print(r.status_code, r.text)

登录场景:
    s = Session(PROFILE_CHROME_150)
    s.get("https://example.com/login")
    s.post("https://example.com/login", "user=admin&pass=123")
    s.get("https://example.com/dashboard")

Context manager:
    with Session(PROFILE_CHROME_150) as s:
        r = s.get("https://httpbin.org/ip")
"""

import ctypes
import os
import time
from dataclasses import dataclass, field
from typing import Optional, Dict, List


# ─── Profile Constants ─────────────────────────────────────

PROFILE_CHROME_150 = 1
PROFILE_CHROME_146 = 2
PROFILE_CHROME_131 = 3
PROFILE_CHROME_120 = 4
PROFILE_CHROME_117 = 5
PROFILE_CHROME_116 = 6
PROFILE_FIREFOX_148 = 10
PROFILE_FIREFOX_147 = 11
PROFILE_FIREFOX_132 = 12
PROFILE_FIREFOX_120 = 13
PROFILE_SAFARI_IOS_18_5 = 20
PROFILE_SAFARI_IOS_17_0 = 21
PROFILE_SAFARI_IOS_16_0 = 22
PROFILE_SAFARI_15_6_1 = 23
PROFILE_SAFARI_IPAD_15_6 = 24
PROFILE_OPERA_91 = 30
PROFILE_BRAVE_146 = 31
PROFILE_OKHTTP4_ANDROID_13 = 40
PROFILE_EDGE_120 = 50

# Rotation groups
ROTATE_CHROME = 1
ROTATE_FIREFOX = 2
ROTATE_SAFARI = 3
ROTATE_MOBILE = 4
ROTATE_ALL = 5

# ─── Error Codes ───────────────────────────────────────────

ERR_OK = 0
ERR_NETWORK = 1
ERR_HTTP = 2
ERR_SESSION = 3
ERR_TIMEOUT = 4
ERR_PROFILE = 5

_ERROR_NAMES = {
    0: "OK", 1: "NETWORK", 2: "HTTP", 3: "SESSION", 4: "TIMEOUT", 5: "PROFILE",
}


# ─── C Types ───────────────────────────────────────────────

class TgResponse(ctypes.Structure):
    _fields_ = [
        ("status", ctypes.c_int),
        ("errorCode", ctypes.c_int),
        ("body", ctypes.c_char_p),
        ("bodyLen", ctypes.c_int),
        ("headers", ctypes.c_char_p),
        ("error", ctypes.c_char_p),
    ]


def _load_library() -> ctypes.CDLL:
    candidates = [
        os.path.join(os.path.dirname(__file__), "libtlsgateway.so"),
        os.path.join(os.path.dirname(__file__), "tlsgateway.dll"),
        "libtlsgateway.so", "tlsgateway.dll", "libtlsgateway.dylib",
    ]
    for path in candidates:
        try:
            return ctypes.CDLL(path)
        except OSError:
            continue
    raise RuntimeError("Cannot find libtlsgateway.so/.dll")


_lib = _load_library()

# Session
_lib.tg_session_create.argtypes = [ctypes.c_int, ctypes.c_int, ctypes.c_char_p]
_lib.tg_session_create.restype = ctypes.c_char_p
_lib.tg_session_free.argtypes = [ctypes.c_char_p]
_lib.tg_session_free.restype = None

# Profile
_lib.tg_session_set_profile.argtypes = [ctypes.c_char_p, ctypes.c_int]
_lib.tg_session_set_profile.restype = ctypes.c_int
_lib.tg_session_get_profile.argtypes = [ctypes.c_char_p]
_lib.tg_session_get_profile.restype = ctypes.c_int

# Cookies
_lib.tg_session_get_cookies.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
_lib.tg_session_get_cookies.restype = ctypes.c_char_p
_lib.tg_session_set_cookies.argtypes = [ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p]
_lib.tg_session_set_cookies.restype = ctypes.c_int
_lib.tg_session_clear_cookies.argtypes = [ctypes.c_char_p]
_lib.tg_session_clear_cookies.restype = ctypes.c_int

# Anti-detection
_lib.tg_session_set_rotate.argtypes = [ctypes.c_char_p, ctypes.c_int, ctypes.c_int, ctypes.c_int]
_lib.tg_session_set_rotate.restype = ctypes.c_int

# Requests
_lib.tg_get.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
_lib.tg_get.restype = ctypes.POINTER(TgResponse)
_lib.tg_post.argtypes = [ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p]
_lib.tg_post.restype = ctypes.POINTER(TgResponse)
_lib.tg_request.argtypes = [ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p]
_lib.tg_request.restype = ctypes.POINTER(TgResponse)

# Convenience
_lib.tg_get_body.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
_lib.tg_get_body.restype = ctypes.c_char_p
_lib.tg_get_status.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
_lib.tg_get_status.restype = ctypes.c_int

# Response
_lib.tg_response_status.argtypes = [ctypes.POINTER(TgResponse)]
_lib.tg_response_status.restype = ctypes.c_int
_lib.tg_response_body.argtypes = [ctypes.POINTER(TgResponse)]
_lib.tg_response_body.restype = ctypes.c_char_p
_lib.tg_response_body_len.argtypes = [ctypes.POINTER(TgResponse)]
_lib.tg_response_body_len.restype = ctypes.c_int
_lib.tg_response_headers.argtypes = [ctypes.POINTER(TgResponse)]
_lib.tg_response_headers.restype = ctypes.c_char_p
_lib.tg_response_header.argtypes = [ctypes.POINTER(TgResponse), ctypes.c_char_p]
_lib.tg_response_header.restype = ctypes.c_char_p
_lib.tg_response_error.argtypes = [ctypes.POINTER(TgResponse)]
_lib.tg_response_error.restype = ctypes.c_char_p
_lib.tg_response_error_code.argtypes = [ctypes.POINTER(TgResponse)]
_lib.tg_response_error_code.restype = ctypes.c_int
_lib.tg_response_free.argtypes = [ctypes.POINTER(TgResponse)]
_lib.tg_response_free.restype = None


# ─── Pythonic Types ────────────────────────────────────────

@dataclass
class Response:
    """HTTP 响应，含解析后的 header dict 和 body。"""
    status_code: int
    error_code: int
    text: str
    body: bytes
    headers: Dict[str, str] = field(default_factory=dict)
    error: Optional[str] = None

    @property
    def ok(self) -> bool:
        return 200 <= self.status_code < 300

    @property
    def success(self) -> bool:
        return self.error_code == 0

    def __repr__(self) -> str:
        if self.error:
            return f"Response({self.status_code}, err={self.error})"
        return f"Response({self.status_code}, {len(self.body)}B)"


class TgError(Exception):
    """tlsgateway 错误。"""
    def __init__(self, code: int, message: str):
        self.code = code
        self.message = message
        super().__init__(f"[{_ERROR_NAMES.get(code, 'UNKNOWN')}] {message}")


def _parse_headers(raw: Optional[str]) -> Dict[str, str]:
    if not raw:
        return {}
    result = {}
    for line in raw.split("\n"):
        parts = line.split(":", 1)
        if len(parts) == 2:
            result[parts[0].strip()] = parts[1].strip()
    return result


# ─── Session ───────────────────────────────────────────────

class Session:
    """TLS 指纹会话。

    维护 Cookie jar、连接池、指纹画像。线程安全。

    Usage:
        s = Session(PROFILE_CHROME_150, timeout=30, proxy="http://127.0.0.1:8080")
        r = s.get("https://httpbin.org/ip")
        s.switch_profile(PROFILE_FIREFOX_148)  # 切换指纹，保留 Cookie

    Context manager:
        with Session(PROFILE_CHROME_150) as s:
            r = s.get("https://httpbin.org/ip")
    """

    def __init__(self, profile_id: int = PROFILE_CHROME_150,
                 timeout: int = 30, proxy: Optional[str] = None):
        proxy_bytes = proxy.encode("utf-8") if proxy else None
        sid = _lib.tg_session_create(profile_id, timeout, proxy_bytes)
        if sid is None:
            raise RuntimeError("tg_session_create returned NULL")
        sid_str = sid.decode("utf-8")
        if sid_str.startswith("ERR:"):
            parts = sid_str.split(":", 2)
            code = int(parts[1]) if len(parts) > 1 else -1
            msg = parts[2] if len(parts) > 2 else sid_str
            raise TgError(code, msg)
        self._sid_str = sid_str
        self._sid_bytes = sid_str.encode("utf-8")
        self._closed = False

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def close(self):
        if not self._closed:
            _lib.tg_session_free(self._sid_bytes)
            self._closed = True

    def __del__(self):
        self.close()

    # ─── Profile ───────────────────────────────────────

    def switch_profile(self, profile_id: int):
        """切换指纹画像（保留 Cookie jar）。"""
        code = _lib.tg_session_set_profile(self._sid_bytes, profile_id)
        if code != 0:
            raise TgError(code, f"switch_profile({profile_id}) failed")

    @property
    def profile_id(self) -> int:
        return _lib.tg_session_get_profile(self._sid_bytes)

    # ─── Cookies ───────────────────────────────────────

    def get_cookies(self, url: str) -> str:
        """获取指定 URL 下的 Cookie。"name=value; name2=value2" """
        result = _lib.tg_session_get_cookies(self._sid_bytes, url.encode("utf-8"))
        if result is None:
            return ""
        return result.decode("utf-8")

    def set_cookies(self, url: str, cookies: str):
        """设置 Cookie。格式: "name=value; name2=value2" """
        code = _lib.tg_session_set_cookies(self._sid_bytes, url.encode("utf-8"), cookies.encode("utf-8"))
        if code != 0:
            raise TgError(code, "set_cookies failed")

    def clear_cookies(self):
        code = _lib.tg_session_clear_cookies(self._sid_bytes)
        if code != 0:
            raise TgError(code, "clear_cookies failed")

    # ─── Anti-Detection ────────────────────────────────

    def set_rotate(self, group: int, every_n: int = 3, tls_refresh: int = 50):
        """启用画像自动轮换。
        
        Args:
            group: 1=Chrome, 2=Firefox, 3=Safari, 4=Mobile, 5=All
            every_n: 每 N 次请求切换画像
            tls_refresh: 每 N 次请求刷新 TLS 握手
        """
        code = _lib.tg_session_set_rotate(self._sid_bytes, group, every_n, tls_refresh)
        if code != 0:
            raise TgError(code, "set_rotate failed")

    # ─── Requests ──────────────────────────────────────

    def _handle(self, r_ptr) -> Response:
        if not r_ptr:
            return Response(0, ERR_NETWORK, "", b"", {}, "NULL response")
        r = r_ptr.contents
        error = r.error.decode("utf-8") if r.error else None
        body_bytes = bytes(r.body[:r.bodyLen]) if r.body and r.bodyLen > 0 else b""
        text = body_bytes.decode("utf-8", errors="replace")
        headers_raw = r.headers.decode("utf-8") if r.headers else None
        headers = _parse_headers(headers_raw)
        status = r.status
        error_code = r.errorCode
        _lib.tg_response_free(r_ptr)
        return Response(status, error_code, text, body_bytes, headers, error)

    def get(self, url: str) -> Response:
        return self._handle(_lib.tg_get(self._sid_bytes, url.encode("utf-8")))

    def post(self, url: str, body: Optional[str] = None) -> Response:
        body_bytes = body.encode("utf-8") if body else None
        return self._handle(_lib.tg_post(self._sid_bytes, url.encode("utf-8"), body_bytes))

    def request(self, method: str, url: str,
                headers: Optional[str] = None, body: Optional[str] = None) -> Response:
        h_bytes = headers.encode("utf-8") if headers else None
        b_bytes = body.encode("utf-8") if body else None
        return self._handle(_lib.tg_request(self._sid_bytes, method.encode("utf-8"),
                                             url.encode("utf-8"), h_bytes, b_bytes))

    # ─── Convenience ───────────────────────────────────

    def get_body(self, url: str) -> Optional[str]:
        """GET 请求，直接返回响应体（失败返回 None）。"""
        result = _lib.tg_get_body(self._sid_bytes, url.encode("utf-8"))
        if result is None:
            return None
        return result.decode("utf-8", errors="replace")

    def get_status(self, url: str) -> int:
        """GET 请求，直接返回 HTTP 状态码（失败返回 0）。"""
        return _lib.tg_get_status(self._sid_bytes, url.encode("utf-8"))


# ─── Convenience Functions ─────────────────────────────────

def quick_get(url: str, profile_id: int = PROFILE_CHROME_150,
              timeout: int = 30) -> Optional[str]:
    """单次 GET 请求，自动创建/销毁 session。返回响应体。"""
    with Session(profile_id, timeout) as s:
        r = s.get(url)
        return r.text if r.success else None


# ─── Module Interface ──────────────────────────────────────

__all__ = [
    "Session", "Response", "TgError",
    "PROFILE_CHROME_150", "PROFILE_CHROME_146", "PROFILE_CHROME_131",
    "PROFILE_FIREFOX_148", "PROFILE_FIREFOX_147", "PROFILE_FIREFOX_132",
    "PROFILE_SAFARI_IOS_18_5", "PROFILE_SAFARI_IOS_17_0",
    "PROFILE_OPERA_91", "PROFILE_OKHTTP4_ANDROID_13",
    "ERR_OK", "ERR_NETWORK", "ERR_HTTP", "ERR_SESSION", "ERR_TIMEOUT",
    "quick_get",
]
