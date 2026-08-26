# quic-go (cloak 本地 fork)

本目录是 quic-go-utls 的本地 fork,由 `github.com/wuliao6688/cloak` 维护使用,
作为 cloak 的 HTTP/3 (QUIC) 引擎。

## 相对上游的改动

- **QUIC TLS 指纹注入**:`crypto_setup.go` 使用 `UQUICClient` + `HelloCustom` +
  `ApplyPreset`,把浏览器 ClientHello 完整注入 QUIC TLS 1.3 握手
- **HTTP/3 应用层指纹**:`http3Settings` / `http3SettingsOrder` /
  `http3PriorityParam` / `http3PseudoHeaderOrder` / `http3SendGreaseFrames`
  全部由画像驱动
- **标准 net/http**:http3 包使用标准库 net/http(不使用任何 fork),符合
  cloak 的"零 fork 依赖"原则
- `quic.Config` 增加 `ClientHelloID` 字段,沿 dial 链传递到 crypto setup

## 使用

不直接对外发布,通过 cloak 主模块的 `replace` 指令引用:

```
replace github.com/wuliao6688/quic-go-utls => ./third_party/quic-go-utls
```

## 上游

上游项目:[quic-go/quic-go](https://github.com/quic-go/quic-go)(uTLS 集成思路参考 refraction-networking/utls)

## License

MIT(与原 quic-go 一致)
