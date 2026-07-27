"""tlsgateway SDK for Python — 商用级 TLS 指纹 HTTP 客户端

零 JSON，Session 模型，支持画像/代理/TLS 三重轮换防检测。

快速开始:
    from tlsgateway import Session
    s = Session(PROFILE_CHROME_150, proxy="http://127.0.0.1:8080")
    s.set_rotate(ROTATE_CHROME, 3, 20)
    r = s.get("https://httpbin.org/ip")
    print(r.status_code, r.text)

代理管理:
    s.set_proxy("http://user:pass@host:8080")      # HTTP 认证代理
    s.set_proxy("socks5://host:1080")               # SOCKS5 代理
    s.set_proxy_list("http://ip1\\nhttp://ip2", 5)  # 每5次请求轮换IP
    s.set_proxy("")                                  # 清除代理
"""

import ctypes
import os
from dataclasses import dataclass, field
from typing import Optional, Dict


# ─── Profile Constants ─────────────────────────────────────

PROFILE_CHROME_150, PROFILE_CHROME_146, PROFILE_CHROME_131 = 1, 2, 3
PROFILE_CHROME_120, PROFILE_CHROME_117, PROFILE_CHROME_116 = 4, 5, 6
PROFILE_FIREFOX_148, PROFILE_FIREFOX_147, PROFILE_FIREFOX_132, PROFILE_FIREFOX_120 = 10, 11, 12, 13
PROFILE_SAFARI_IOS_18_5, PROFILE_SAFARI_IOS_17_0 = 20, 21
PROFILE_SAFARI_IOS_16_0, PROFILE_SAFARI_15_6_1, PROFILE_SAFARI_IPAD_15_6 = 22, 23, 24
PROFILE_OPERA_91, PROFILE_BRAVE_146 = 30, 31
PROFILE_OKHTTP4_ANDROID_13, PROFILE_EDGE_120 = 40, 50

ROTATE_CHROME, ROTATE_FIREFOX, ROTATE_SAFARI, ROTATE_MOBILE, ROTATE_ALL, ROTATE_CHAOS = 1, 2, 3, 4, 5, 6

ERR_OK, ERR_NETWORK, ERR_HTTP, ERR_SESSION, ERR_TIMEOUT, ERR_PROFILE = 0, 1, 2, 3, 4, 5


# ─── C Types ───────────────────────────────────────────────

class TgResponse(ctypes.Structure):
    _fields_ = [
        ("status", ctypes.c_int), ("errorCode", ctypes.c_int),
        ("body", ctypes.c_char_p), ("bodyLen", ctypes.c_int),
        ("headers", ctypes.c_char_p), ("error", ctypes.c_char_p),
    ]


def _load():
    for p in [os.path.join(os.path.dirname(__file__), "libtlsgateway.so"),
              "libtlsgateway.so", "tlsgateway.dll", "libtlsgateway.dylib"]:
        try: return ctypes.CDLL(p)
        except OSError: continue
    raise RuntimeError("libtlsgateway.so not found")

_lib = _load()

_s = _lib.tg_session_create; _s.argtypes = [ctypes.c_int]*2 + [ctypes.c_char_p]; _s.restype = ctypes.c_char_p
_lib.tg_session_free.argtypes = [ctypes.c_char_p]
_lib.tg_session_set_proxy.argtypes = [ctypes.c_char_p]*2; _lib.tg_session_set_proxy.restype = ctypes.c_int
_lib.tg_session_get_proxy.argtypes = [ctypes.c_char_p]; _lib.tg_session_get_proxy.restype = ctypes.c_char_p
_lib.tg_session_set_proxy_list.argtypes = [ctypes.c_char_p]*2 + [ctypes.c_int]; _lib.tg_session_set_proxy_list.restype = ctypes.c_int
_lib.tg_session_set_rotate.argtypes = [ctypes.c_char_p] + [ctypes.c_int]*3; _lib.tg_session_set_rotate.restype = ctypes.c_int
_lib.tg_session_set_profile.argtypes = [ctypes.c_char_p, ctypes.c_int]; _lib.tg_session_set_profile.restype = ctypes.c_int
_lib.tg_session_get_profile.argtypes = [ctypes.c_char_p]; _lib.tg_session_get_profile.restype = ctypes.c_int
_lib.tg_session_get_cookies.argtypes = [ctypes.c_char_p]*2; _lib.tg_session_get_cookies.restype = ctypes.c_char_p
_lib.tg_session_set_cookies.argtypes = [ctypes.c_char_p]*3; _lib.tg_session_set_cookies.restype = ctypes.c_int
_lib.tg_session_clear_cookies.argtypes = [ctypes.c_char_p]; _lib.tg_session_clear_cookies.restype = ctypes.c_int
_lib.tg_session_set_cookie_store.argtypes = [ctypes.c_char_p, ctypes.c_int]; _lib.tg_session_set_cookie_store.restype = ctypes.c_int
_lib.tg_post_bin.argtypes = [ctypes.c_char_p]*2 + [ctypes.c_void_p, ctypes.c_int]; _lib.tg_post_bin.restype = ctypes.POINTER(TgResponse)
_lib.tg_get.argtypes = [ctypes.c_char_p]*2; _lib.tg_get.restype = ctypes.POINTER(TgResponse)
_lib.tg_post.argtypes = [ctypes.c_char_p]*3; _lib.tg_post.restype = ctypes.POINTER(TgResponse)
_lib.tg_request.argtypes = [ctypes.c_char_p]*5; _lib.tg_request.restype = ctypes.POINTER(TgResponse)
_lib.tg_get_body.argtypes = [ctypes.c_char_p]*2; _lib.tg_get_body.restype = ctypes.c_char_p
_lib.tg_get_status.argtypes = [ctypes.c_char_p]*2; _lib.tg_get_status.restype = ctypes.c_int
_lib.tg_response_free.argtypes = [ctypes.POINTER(TgResponse)]


# ─── Python Types ──────────────────────────────────────────

@dataclass
class Response:
    status_code: int; error_code: int; text: str; body: bytes
    headers: Dict[str, str] = field(default_factory=dict)
    error: Optional[str] = None
    @property
    def ok(self) -> bool: return 200 <= self.status_code < 300
    @property
    def success(self) -> bool: return self.error_code == 0

class TgError(Exception):
    def __init__(self, code, msg): self.code = code; super().__init__(f"[{code}] {msg}")


# ─── Session ───────────────────────────────────────────────

class Session:
    """TLS 指纹会话。维护 Cookie、连接池、画像/代理/指纹三重轮换。"""

    def __init__(self, profile_id=PROFILE_CHROME_150, timeout=30, proxy=None):
        sid = _lib.tg_session_create(profile_id, timeout, proxy.encode() if proxy else None)
        s = sid.decode()
        if s.startswith("ERR:"):
            parts = s.split(":", 2)
            raise TgError(int(parts[1]) if len(parts) > 1 else -1, parts[2] if len(parts) > 2 else s)
        self._s = s; self._b = s.encode(); self._closed = False

    def __enter__(self): return self
    def __exit__(self, *a): self.close()
    def close(self):
        if not self._closed: _lib.tg_session_free(self._b); self._closed = True
    def __del__(self): self.close()

    # ─── Proxy ────────────────────────────────────────
    def set_proxy(self, url: str):
        """设置/切换/清除代理。支持 http:// socks5:// 及 user:pass@ 认证。"""
        c = _lib.tg_session_set_proxy(self._b, url.encode())
        if c: raise TgError(c, "set_proxy")
    @property
    def proxy(self) -> str:
        r = _lib.tg_session_get_proxy(self._b); return r.decode() if r else ""
    def set_proxy_list(self, proxies: str, rotate_every=1):
        """代理池轮换。格式: "http://ip1:8080\\nhttp://ip2:8080\\nsocks5://ip3:1080" """
        c = _lib.tg_session_set_proxy_list(self._b, proxies.encode(), rotate_every)
        if c: raise TgError(c, "set_proxy_list")

    # ─── Anti-Detection ───────────────────────────────
    def set_rotate(self, group=ROTATE_CHROME, every_n=3, tls_refresh=50):
        c = _lib.tg_session_set_rotate(self._b, group, every_n, tls_refresh)
        if c: raise TgError(c, "set_rotate")

    # ─── Profile / Cookies ────────────────────────────
    def switch_profile(self, pid): c = _lib.tg_session_set_profile(self._b, pid); assert not c, f"switch_profile({pid})"
    @property
    def profile_id(self): return _lib.tg_session_get_profile(self._b)
    def get_cookies(self, url): r = _lib.tg_session_get_cookies(self._b, url.encode()); return r.decode() if r else ""
    def set_cookies(self, url, ck): _lib.tg_session_set_cookies(self._b, url.encode(), ck.encode())
    def clear_cookies(self): _lib.tg_session_clear_cookies(self._b)
    def set_cookie_store(self, enable=True):
        c = _lib.tg_session_set_cookie_store(self._b, 1 if enable else 0); assert not c, "set_cookie_store"

    # ─── Requests ─────────────────────────────────────
    def _r(self, p):
        if not p: return Response(0, ERR_NETWORK, "", b"", {}, "NULL")
        r = p.contents
        err = r.error.decode() if r.error else None
        body = bytes(r.body[:r.bodyLen]) if r.body and r.bodyLen > 0 else b""
        h_raw = r.headers.decode() if r.headers else ""
        headers = {}
        for line in h_raw.split("\n"):
            parts = line.split(":", 1)
            if len(parts) == 2: headers[parts[0].strip()] = parts[1].strip()
        _lib.tg_response_free(p)
        return Response(r.status, r.errorCode, body.decode(errors="replace"), body, headers, err)

    def get(self, url): return self._r(_lib.tg_get(self._b, url.encode()))
    def post(self, url, body=None): return self._r(_lib.tg_post(self._b, url.encode(), body.encode() if body else None))
    def post_bin(self, url, data: bytes):
        return self._r(_lib.tg_post_bin(self._b, url.encode(), data, len(data) if data else 0))
    def request(self, method, url, headers=None, body=None):
        return self._r(_lib.tg_request(self._b, method.encode(), url.encode(),
                                        headers.encode() if headers else None,
                                        body.encode() if body else None))
    def get_body(self, url):
        r = _lib.tg_get_body(self._b, url.encode()); return r.decode(errors="replace") if r else None
    def get_status(self, url): return _lib.tg_get_status(self._b, url.encode())
