# uTLS (cloak 本地 fork)

本目录是 uTLS 的本地 fork,由 `github.com/wuliao6688/cloak` 维护使用。

uTLS 是 "crypto/tls" 的 fork,提供:
- ClientHello 指纹伪装(TLS 指纹)
- 握手底层访问
- 假 session ticket

## 相对上游的改动

- 支持 `ClientHelloID` 注入(通过 `UClient` / `UQUICClient`)
- QUIC TLS 握手指纹注入(`UQUICClient` + `HelloCustom` + `ApplyPreset`)
- 与 cloak 的 HTTP/3 引擎(third_party/quic-go-utls)配套

## 使用

本 fork 不直接对外发布,通过 cloak 主模块的 `replace` 指令引用:

```
replace github.com/wuliao6688/utls => ./third_party/utls
```

## 上游

上游项目:[refraction-networking/utls](https://github.com/refraction-networking/utls)

## License

MIT(与原 uTLS 一致)
