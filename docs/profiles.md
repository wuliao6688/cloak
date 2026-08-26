# 画像体系

## 总览

77 个预置画像，分浏览器画像（59）和移动/定制画像（18）。

```
├── 浏览器画像 (59)
│   ├── Chrome 系列 (23): Chrome_103 ~ Chrome_150 (+PSK 变体)
│   ├── Firefox 系列 (15): Firefox_102 ~ Firefox_148
│   ├── Safari 系列 (10): Safari_15_6_1 ~ Safari_IOS_26_0
│   ├── Opera (3): Opera_89/90/91
│   └── Brave (2): Brave_146 (+PSK)
├── 移动/定制画像 (18)
│   ├── OkHttp4 Android (7): Android 7~13
│   ├── 电商 App (4): Nike / Zalando (Android+iOS)
│   ├── 社交 App (7): Mesh / MMS / Confirmed (iOS+Android)
│   └── Cloudflare 定制 (1): CloudflareCustom
```

## 浏览器画像清单

### Chrome（23 个）

```
Chrome_150 / Chrome_150_PSK
Chrome_146 / Chrome_146_PSK
Chrome_144 / Chrome_144_PSK
Chrome_133 / Chrome_133_PSK
Chrome_131 / Chrome_131_PSK
Chrome_130_PSK
Chrome_124
Chrome_120
Chrome_117
Chrome_116_PSK / Chrome_116_PSK_PQ
Chrome_112
Chrome_111 / Chrome_110 / Chrome_109 / Chrome_108 / Chrome_107 / Chrome_106 / Chrome_105 / Chrome_104 / Chrome_103
```

### Firefox（15 个）

```
Firefox_148 / Firefox_147 / Firefox_147_PSK / Firefox_146_PSK
Firefox_135 / Firefox_133 / Firefox_132
Firefox_123 / Firefox_120 / Firefox_117
Firefox_110 / Firefox_108 / Firefox_106 / Firefox_105 / Firefox_104 / Firefox_102
```

### Safari（10 个）

```
Safari_15_6_1 / Safari_16_0 / Safari_Ipad_15_6
Safari_IOS_15_5 / Safari_IOS_15_6 / Safari_IOS_16_0 / Safari_IOS_17_0
Safari_IOS_18_0 / Safari_IOS_18_5 / Safari_IOS_26_0
```

### Opera / Brave（5 个）

```
Opera_89 / Opera_90 / Opera_91
Brave_146 / Brave_146_PSK
```

## 移动/定制画像（18 个）

| 画像 | 场景 |
|---|---|
| Okhttp4Android7~13 | Android 原生 OkHttp 4.10 指纹（7 个 Android 版本） |
| NikeIosMobile / NikeAndroidMobile | Nike App |
| ZalandoIosMobile / ZalandoAndroidMobile | Zalando App |
| MeshIos / MeshIos2 / MeshAndroid / MeshAndroid2 | Mesh 社交 App |
| MMSIos / MMSIos2 / MMSIos3 | MMS App |
| ConfirmedIos / ConfirmedAndroid / ConfirmedAndroid2 | Confirmed App |
| CloudflareCustom | Cloudflare 场景定制 |

## 画像内容

每个画像包含完整的三层指纹：

```go
var Chrome_150 = ClientProfile{
	// TLS 层：uTLS ClientHelloID + SpecFactory（字节级规格）
	clientHelloId: tls.ClientHelloID{
		Client:  "Chrome",
		Version: "150",
		SpecFactory: func() (tls.ClientHelloSpec, error) { ... },
	},

	// HTTP/2 层
	settings:          map[SettingID]uint32{ ... },   // SETTINGS 键值
	settingsOrder:     []SettingID{ ... },             // SETTINGS 顺序
	pseudoHeaderOrder: []string{ ... },                // 伪头顺序
	connectionFlow:    15663105,                        // WINDOW_UPDATE

	// HTTP/3 层（QUIC）
	http3Settings:            map[uint64]uint64{ ... }, // H3 SETTINGS
	http3SettingsOrder:       []uint64{ ... },
	http3PriorityParam:       984832,
	http3PseudoHeaderOrder:   []string{ ... },
	http3SendGreaseFrames:    true,
}
```

## H3 数据覆盖（47/77）

| 画像组 | H3 数据 | 说明 |
|---|---|---|
| Chrome 系（23） | ✅ | `1:65536,7:100 + GREASE + priority 984832 + masp` |
| Firefox 系（15） | ✅ | `1:65536,7:20 + GREASE + 51:1 + 8:1 + msap` |
| Opera / Brave（5） | ✅ | Chromium 内核，同 Chrome |
| Safari（10） | ⚠️ 无 | 按论证不提供（见下） |
| 移动/定制（18） | ⚠️ 无 | 移动 App 基本走 H2，无 QUIC |

**为什么 Safari 不提供 H3 数据**：curl_cffi（业界最强伪装库）的 11 个
Safari 画像全部 `h3_fingerprints: False`——Safari 的 QUIC 实现是 Apple 私有
且参数未公开，套用 Chrome 的 H3 数据反而自曝（UA 说 Safari、H3 指纹是
Chrome，检测方直接判 bot）。保持无 H3 → 自动降级 H2，与真实 Safari 一致。

## 数据来源与论证

画像数据经**全量论证**，对照权威数据源：

| 数据源 | 内容 |
|---|---|
| curl-impersonate（lexiforest） | 真实浏览器抓包，12009 行 patch，含 chrome99~150 / firefox133~147 / safari15~26 的 H2+H3+QUIC 指纹 |
| curl_cffi | 画像清单 + h3_fingerprints 标记 |
| 同类 Go 库 | 本项目画像源头（同源） |

**论证结论**：
1. H2 SETTINGS 与 curl 参考**逐版本一致**（含 Chrome 99→119 的演变）
2. H3 数据与 curl chrome145/146/150、firefox147 参考**完全一致**
3. 画像数据有效性由 `TestAllBrowserProfilesH3DataValid` 测试守卫
   （每个画像的 H3 数据必须能构建 http3.Transport）

详见 [画像数据论证报告](profile-audit.md)。

## 使用画像

```go
import "github.com/wuliao6688/cloak/profiles"

// 直接引用画像常量
client := cloak.Impersonate(profiles.Chrome_150)

// 按 key 动态解析（key 是注册表名，小写蛇形）
p, err := profiles.ResolveClientProfileStrict("chrome_150")

// 遍历全部
for key, profile := range profiles.AllClientProfiles() {
	// key = "chrome_150", "firefox_147", "okhttp4_android_13", ...
}

// 自定义画像（完整签名，14 个参数：TLS + H2 + H3 全层）
p := profiles.NewClientProfile(
	tls.ClientHelloID{Client: "Chrome", Version: "150"}, // TLS 指纹
	map[SettingID]uint32{SettingHeaderTableSize: 65536}, // H2 SETTINGS
	[]SettingID{SettingHeaderTableSize},                  // H2 SETTINGS 顺序
	[]string{":method", ":authority", ":scheme", ":path"}, // 伪头顺序
	15663105,                                            // connectionFlow
	[]Priority{},                                        // 优先级帧
	nil,                                                 // headerPriority
	3,                                                   // streamID
	false,                                               // allowHTTP
	nil, nil, 0, nil, false,                             // H3 字段（可空）
)
```
