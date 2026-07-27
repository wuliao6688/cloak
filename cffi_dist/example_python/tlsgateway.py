"""tlsgateway SDK for Python — 商用级 TLS 指纹 HTTP 客户端

快速开始:
    from tlsgateway import Session, PROFILE_CHROME_150
    s = Session(PROFILE_CHROME_150, timeout=30)
    r = s.get("https://httpbin.org/ip")
    print(r.status_code, r.text)
"""

import ctypes
import os
import sys
from dataclasses import dataclass
from typing import Optional


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


# ─── C Types ───────────────────────────────────────────────

class TgResponse(ctypes.Structure):
    _fields_ = [
        ("status", ctypes.c_int),
        ("body", ctypes.c_char_p),
        ("bodyLen", ctypes.c_int),
        ("error", ctypes.c_char_p),
    ]


def _load_library() -> ctypes.CDLL:
    """Load the shared library from python/ or system path."""
    candidates = [
        os.path.join(os.path.dirname(__file__), "libtlsgateway.so"),
        os.path.join(os.path.dirname(__file__), "tlsgateway.dll"),
        "libtlsgateway.so",
        "tlsgateway.dll",
        "libtlsgateway.dylib",
    ]
    for path in candidates:
        try:
            return ctypes.CDLL(path)
        except OSError:
            continue
    raise RuntimeError(
        "Cannot find libtlsgateway.so/.dll. "
        "Place it alongside this file or in the system library path."
    )


_lib = _load_library()

# Session lifecycle
_lib.tg_session_create.argtypes = [ctypes.c_int, ctypes.c_int, ctypes.c_char_p]
_lib.tg_session_create.restype = ctypes.c_char_p

_lib.tg_session_free.argtypes = [ctypes.c_char_p]
_lib.tg_session_free.restype = None

# Requests
_lib.tg_get.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
_lib.tg_get.restype = ctypes.POINTER(TgResponse)

_lib.tg_post.argtypes = [ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p]
_lib.tg_post.restype = ctypes.POINTER(TgResponse)

_lib.tg_request.argtypes = [ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p]
_lib.tg_request.restype = ctypes.POINTER(TgResponse)

# Response access
_lib.tg_response_free.argtypes = [ctypes.POINTER(TgResponse)]
_lib.tg_response_free.restype = None


# ─── Pythonic Wrapper ──────────────────────────────────────

@dataclass
class Response:
    """HTTP response with decoded body."""
    status_code: int
    text: str
    body: bytes
    error: Optional[str] = None

    @property
    def ok(self) -> bool:
        return 200 <= self.status_code < 300


class Session:
    """A TLS session with a specific fingerprint profile.

    Usage:
        s = Session(PROFILE_CHROME_150, timeout=30)
        r = s.get("https://httpbin.org/ip")
        print(r.status_code, r.text)
    """

    def __init__(self, profile_id: int = PROFILE_CHROME_150,
                 timeout: int = 30, proxy: Optional[str] = None):
        proxy_bytes = proxy.encode("utf-8") if proxy else None
        sid = _lib.tg_session_create(profile_id, timeout, proxy_bytes)
        if sid is None:
            raise RuntimeError("tg_session_create returned NULL")
        sid_str = sid.decode("utf-8")
        if sid_str.startswith("ERR:"):
            raise RuntimeError(sid_str)
        self._sid = sid_str
        self._session_id_bytes = sid_str.encode("utf-8")

    def __del__(self):
        if hasattr(self, '_session_id_bytes'):
            _lib.tg_session_free(self._session_id_bytes)

    def close(self):
        """Explicitly close the session."""
        if hasattr(self, '_session_id_bytes'):
            _lib.tg_session_free(self._session_id_bytes)
            del self._session_id_bytes

    def _handle_response(self, r_ptr) -> Response:
        if not r_ptr:
            return Response(0, "", b"", "NULL response pointer")
        r = r_ptr.contents
        error = r.error.decode("utf-8") if r.error else None
        body_bytes = bytes(r.body[:r.bodyLen]) if r.body and r.bodyLen > 0 else b""
        text = body_bytes.decode("utf-8", errors="replace")
        status = r.status
        _lib.tg_response_free(r_ptr)
        return Response(status, text, body_bytes, error)

    def get(self, url: str) -> Response:
        r_ptr = _lib.tg_get(self._session_id_bytes, url.encode("utf-8"))
        return self._handle_response(r_ptr)

    def post(self, url: str, body: Optional[str] = None) -> Response:
        body_bytes = body.encode("utf-8") if body else None
        r_ptr = _lib.tg_post(self._session_id_bytes, url.encode("utf-8"), body_bytes)
        return self._handle_response(r_ptr)

    def request(self, method: str, url: str,
                headers: Optional[str] = None,
                body: Optional[str] = None) -> Response:
        header_bytes = headers.encode("utf-8") if headers else None
        body_bytes = body.encode("utf-8") if body else None
        r_ptr = _lib.tg_request(
            self._session_id_bytes,
            method.encode("utf-8"),
            url.encode("utf-8"),
            header_bytes,
            body_bytes,
        )
        return self._handle_response(r_ptr)


# ─── Fallback —─────────────────────────────────────────────

def _fallback_test():
    """Test using the Go HTTP proxy as fallback when DLL not available."""
    import requests
    return requests


__all__ = [
    "Session", "Response",
    "PROFILE_CHROME_150", "PROFILE_CHROME_146", "PROFILE_CHROME_131",
    "PROFILE_FIREFOX_148", "PROFILE_FIREFOX_147", "PROFILE_FIREFOX_132",
    "PROFILE_SAFARI_IOS_18_5", "PROFILE_SAFARI_IOS_17_0",
    "PROFILE_OPERA_91", "PROFILE_OKHTTP4_ANDROID_13",
]
