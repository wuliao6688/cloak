# 指纹体系

tls-client 在**三个网络层**复刻浏览器指纹。理解这三层，就知道为什么它能过
Cloudflare / Akamai / Imperva 等 WAF。

## 1. 为什么需要指纹

当你的程序通过 HTTPS 访问网站时，服务端（或中间的 WAF）能看到：

```
TCP/IP 层      → 操作系统特征（TTL、窗口大小）
TLS 层         → ClientHello 内容（JA3/JA4 指纹）
HTTP/2 层      → SETTINGS 帧、伪头顺序、优先级
HTTP/3 层      → QUIC 参数、H3 SETTINGS 帧
应用层         → UA、Accept、Sec-CH-UA 等头
```

`curl`、`requests`、`net/http` 的默认指纹一眼可辨。tls-client 让每一层都与
真实浏览器逐字节一致。

## 2. TLS 层（uTLS）

### 原理

TLS 1.3 ClientHello 包含：密码套件、扩展列表、扩展顺序、supported_groups、
signature_algorithms 等。每个浏览器（甚至版本）的组合都不同 → JA3/JA4 哈希。

tls-client 用 [uTLS](https://github.com/refraction-networking/utls)（Tor 团队）
构造**与浏览器完全相同的 ClientHello 字节**，而非"看起来像"。

### 实现

```go
// 每个画像 = 一个 ClientHelloID + SpecFactory（字节级规格）
clientHelloId: tls.ClientHelloID{
	Client:  "Chrome",
	Version: "150",
	SpecFactory: func() (tls.ClientHelloSpec, error) {
		return tls.ClientHelloSpec{
			CipherSuites: []uint16{ tls.GREASE_PLACEHOLDER, tls.TLS_AES_128_GCM_SHA256, ... },
			Extensions:   []tls.TLSExtension{ ... },
		}, nil
	},
}
```

- 77 个画像的 Spec 全部来自真实浏览器抓包
- 支持 GREASE 值、随机扩展顺序、自定义 SpecFactory（复制任意 ClientHello）
- `Transport.SelfCheck()` 可以验证 JA3/JA4 是否匹配预期

### 验证

```go
info := tr.SelfCheck("https://tls.peet.ws/api/all")
// info.JA4 = t13d1516h2_8daaf6152771_...
```

## 3. HTTP/2 层

### 原理

HTTP/2 连接建立后，客户端先发 SETTINGS 帧。SETTINGS 的**键值对和顺序**、
伪头顺序（`:method :authority :scheme :path` vs `:method :scheme :authority :path`）、
WINDOW_UPDATE 增量值（Chrome 15663105 vs Firefox 12517377）都是指纹。

### 画像字段

```go
settings: map[SettingID]uint32{
	SettingHeaderTableSize:   65536,
	SettingEnablePush:        0,
	SettingInitialWindowSize: 6291456,
	SettingMaxHeaderListSize: 262144,
},
settingsOrder: []SettingID{ SettingHeaderTableSize, SettingEnablePush, ... },
pseudoHeaderOrder: []string{ ":method", ":authority", ":scheme", ":path" },
connectionFlow: 15663105,   // WINDOW_UPDATE 增量
```

### 版本演变（真实浏览器）

| 版本 | SETTINGS | 说明 |
|---|---|---|
| Chrome 99-104 | `1:65536;3:1000;4:6291456;6:262144` | 无 EnablePush |
| Chrome 107-116 | `1:65536;2:0;3:1000;4:6291456;6:262144` | 加 EnablePush |
| Chrome 119+ | `1:65536;2:0;4:6291456;6:262144` | 去 MaxConcurrentStreams |
| Firefox 102-123 | `1:65536;4:131072;5:16384` | |
| Firefox 133+ | `1:65536;2:0;4:131072;5:16384` | 加 EnablePush |
| Safari 15 | `4:4194304;3:100` | 无 HeaderTableSize |
| Safari 18+ | `2:0;3:100;4:2097152;8:1;9:1` | 加 NoRFC7540Priorities |

> 项目画像数据与 curl-impersonate 真实抓包**逐版本一致**（见 [画像论证](profiles.md)）。

## 4. HTTP/3 层（QUIC）🔥

### 为什么 H3 指纹是难点

HTTP/3 基于 QUIC（UDP）。QUIC 内部也做 TLS 1.3 握手，但**加密层独立于 TCP**，
而且握手方式不同（Transport Parameters 扩展、ALPN=h3、TLS 1.3 only）。

- `curl` 的 H3：QUIC TLS 是 ngtcp2 默认 → 指纹暴露
- 上游 bogdanfinn：用了 quic-go-utls，但 **QUIC TLS 层仍是 Go 默认指纹**
- **tls-client：用 `UQUICClient` 把浏览器 ClientHello 注入 QUIC TLS 握手** ✅

### 实现（third_party/quic-go-utls fork）

```
http3.Transport → quic.DialEarly → crypto_setup
                                    │
                    HelloCustom + ApplyPreset(浏览器 Spec)
                    ├─ ALPN → "h3"（TCP 画像原本是 h2）
                    ├─ SupportedVersions → TLS1.3 only
                    ├─ QUIC Transport Parameters 扩展注入（extension 57）
                    └─ 其余扩展/顺序/密码套件 = 浏览器原样
```

### H3 画像字段

```go
http3Settings: map[uint64]uint64{
	1: 65536,  // SETTINGS_QPACK_MAX_TABLE_CAPACITY
	7: 100,    // SETTINGS_QPACK_BLOCKED_STREAMS
},
http3SettingsOrder: []uint64{ 1, 0x6, 7, 0x33 },
http3PriorityParam: 984832,
http3PseudoHeaderOrder: []string{ ":method", ":authority", ":scheme", ":path" },
http3SendGreaseFrames: true,
```

参考数据（curl-impersonate 真实抓包，与项目一致）：

| 浏览器 | H3 SETTINGS | 伪头 | Priority |
|---|---|---|---|
| Chrome 145/146/150 | `1:65536;6:262144;7:100;51:1;GREASE` | masp | 984832 |
| Firefox 147 | `1:65536;7:20;727725890:0;16765559:1;51:1;8:1` | msap | 0 |

### H2 vs H3 协议赛跑

Chrome 会**同时发起 H2 和 H3 连接，谁先响应用谁**（Happy Eyeballs）。

```go
// tls-client 的 H3RaceTransport 完全复刻这一行为
rt := tlsgateway.NewH3RaceTransport(profiles.Chrome_150)
// 1. 首个请求：H3 + H2 并行赛跑
// 2. 胜者按域名缓存（h3 或 h2）
// 3. 后续请求直接走缓存协议
// 4. H3 失败（UDP 被禁/服务器无 QUIC）→ 自动降级 H2
```

## 5. 应用层

浏览器头自动注入（`HeaderRoundTripper` / Transport 内置）：

```
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 ...
Accept: text/html,application/xhtml+xml,...
Sec-CH-UA: "Google Chrome";v="150", "Chromium";v="150", ...
Sec-Fetch-Mode: navigate
Accept-Language: en-US,en;q=0.9
Accept-Encoding: gzip, deflate, br
```

- 按画像注入对应浏览器的头（Chrome 用 Sec-CH-UA，Firefox 不用）
- `SetHeaderNonCanonical` 支持精确大小写（部分服务器看头大小写）
- `WithOrderedHeaders` 支持 H1 头排序
