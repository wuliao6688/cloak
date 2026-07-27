package profiles

import (
	tls "github.com/bogdanfinn/utls"
)

var Chrome_150_PSK = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Chrome",
		RandomExtensionOrder: false,
		Version:              "150_PSK",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []byte{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.ApplicationSettingsExtensionNew{
						SupportedProtocols: []string{"h2"},
					},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.SCTExtension{},
					tls.BoringGREASEECH(),
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519MLKEM768},
						{Group: tls.X25519},
					}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.SignatureScheme(0x0904),
						tls.SignatureScheme(0x0905),
						tls.SignatureScheme(0x0906),
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519MLKEM768,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.ExtendedMasterSecretExtension{},
					&tls.SessionTicketExtension{},
					&tls.SNIExtension{},
					&tls.RenegotiationInfoExtension{
						Renegotiation: tls.RenegotiateOnceAsClient,
					},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.StatusRequestExtension{},
					&tls.ALPNExtension{AlpnProtocols: []string{
						"h2",
						"http/1.1",
					}},
					&tls.UtlsGREASEExtension{},
					&tls.UtlsPreSharedKeyExtension{},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_150 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Chrome",
		RandomExtensionOrder: false,
		Version:              "150",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []byte{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.ApplicationSettingsExtensionNew{
						SupportedProtocols: []string{"h2"},
					},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.SCTExtension{},
					tls.BoringGREASEECH(),
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519MLKEM768},
						{Group: tls.X25519},
					}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.SignatureScheme(0x0904),
						tls.SignatureScheme(0x0905),
						tls.SignatureScheme(0x0906),
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519MLKEM768,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.ExtendedMasterSecretExtension{},
					&tls.SessionTicketExtension{},
					&tls.SNIExtension{},
					&tls.RenegotiationInfoExtension{
						Renegotiation: tls.RenegotiateOnceAsClient,
					},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.StatusRequestExtension{},
					&tls.ALPNExtension{AlpnProtocols: []string{
						"h2",
						"http/1.1",
					}},
					&tls.UtlsGREASEExtension{},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_146_PSK = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Chrome",
		RandomExtensionOrder: false,
		Version:              "146_PSK",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []byte{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.ApplicationSettingsExtensionNew{
						SupportedProtocols: []string{"h2"},
					},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.SCTExtension{},
					tls.BoringGREASEECH(),
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519MLKEM768},
						{Group: tls.X25519},
					}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519MLKEM768,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.ExtendedMasterSecretExtension{},
					&tls.SessionTicketExtension{},
					&tls.SNIExtension{},
					&tls.RenegotiationInfoExtension{
						Renegotiation: tls.RenegotiateOnceAsClient,
					},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.StatusRequestExtension{},
					&tls.ALPNExtension{AlpnProtocols: []string{
						"h2",
						"http/1.1",
					}},
					&tls.GenericExtension{Id: 0xca34, Data: []byte{0x00, 0x00}}, // https://source.chromium.org/search?q=TLSEXT_TYPE_trust_anchors https://issues.chromium.org/issues/398275713
					&tls.UtlsGREASEExtension{},
					&tls.UtlsPreSharedKeyExtension{},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_146 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Chrome",
		RandomExtensionOrder: false,
		Version:              "146",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []byte{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.ApplicationSettingsExtensionNew{
						SupportedProtocols: []string{"h2"},
					},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.SCTExtension{},
					tls.BoringGREASEECH(),
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519MLKEM768},
						{Group: tls.X25519},
					}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519MLKEM768,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.ExtendedMasterSecretExtension{},
					&tls.SessionTicketExtension{},
					&tls.SNIExtension{},
					&tls.RenegotiationInfoExtension{
						Renegotiation: tls.RenegotiateOnceAsClient,
					},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.StatusRequestExtension{},
					&tls.ALPNExtension{AlpnProtocols: []string{
						"h2",
						"http/1.1",
					}},
					&tls.GenericExtension{Id: 0xca34, Data: []byte{0x00, 0x00}}, // https://source.chromium.org/search?q=TLSEXT_TYPE_trust_anchors https://issues.chromium.org/issues/398275713
					&tls.UtlsGREASEExtension{},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_144_PSK = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Chrome",
		RandomExtensionOrder: false,
		Version:              "144",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []byte{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.ApplicationSettingsExtensionNew{
						SupportedProtocols: []string{"h2"},
					},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.SCTExtension{},
					tls.BoringGREASEECH(),
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519MLKEM768},
						{Group: tls.X25519},
					}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519MLKEM768,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.ExtendedMasterSecretExtension{},
					&tls.SessionTicketExtension{},
					&tls.SNIExtension{},
					&tls.RenegotiationInfoExtension{
						Renegotiation: tls.RenegotiateOnceAsClient,
					},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.StatusRequestExtension{},
					&tls.ALPNExtension{AlpnProtocols: []string{
						"h2",
						"http/1.1",
					}},
					&tls.UtlsGREASEExtension{},
					&tls.UtlsPreSharedKeyExtension{OmitEmptyPsk: true},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
	http3Settings: map[uint64]uint64{
		1: 65536, // SETTINGS_QPACK_MAX_TABLE_CAPACITY
		7: 100,   // SETTINGS_QPACK_BLOCKED_STREAMS
	},
	http3SettingsOrder: []uint64{
		1,    // SETTINGS_QPACK_MAX_TABLE_CAPACITY
		0x6,  // SETTINGS_MAX_FIELD_SECTION_SIZE
		7,    // SETTINGS_QPACK_BLOCKED_STREAMS
		0x33, // SETTINGS_H3_DATAGRAM
	},
	http3PriorityParam: 984832,
	http3PseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	http3SendGreaseFrames: true,
}

var Chrome_144 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Chrome",
		RandomExtensionOrder: false,
		Version:              "144",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []byte{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519MLKEM768},
						{Group: tls.X25519},
					}},
					&tls.SNIExtension{},
					&tls.ApplicationSettingsExtensionNew{
						SupportedProtocols: []string{"h2"},
					},
					&tls.RenegotiationInfoExtension{
						Renegotiation: tls.RenegotiateOnceAsClient,
					},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519MLKEM768,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.SessionTicketExtension{},
					&tls.StatusRequestExtension{},
					&tls.ExtendedMasterSecretExtension{},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.SCTExtension{},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					tls.BoringGREASEECH(),
					&tls.ALPNExtension{AlpnProtocols: []string{
						"h2",
						"http/1.1",
					}},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.UtlsGREASEExtension{},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
	http3Settings: map[uint64]uint64{
		1: 65536, // SETTINGS_QPACK_MAX_TABLE_CAPACITY
		7: 100,   // SETTINGS_QPACK_BLOCKED_STREAMS
	},
	http3SettingsOrder: []uint64{
		1,    // SETTINGS_QPACK_MAX_TABLE_CAPACITY
		0x6,  // SETTINGS_MAX_FIELD_SECTION_SIZE
		7,    // SETTINGS_QPACK_BLOCKED_STREAMS
		0x33, // SETTINGS_H3_DATAGRAM
	},
	http3PriorityParam: 984832,
	http3PseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	http3SendGreaseFrames: true,
}

var Chrome_133_PSK = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Chrome",
		RandomExtensionOrder: false,
		Version:              "133",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []byte{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.SCTExtension{},
					&tls.SNIExtension{},
					tls.BoringGREASEECH(),
					&tls.RenegotiationInfoExtension{
						Renegotiation: tls.RenegotiateOnceAsClient,
					},
					&tls.ExtendedMasterSecretExtension{},
					&tls.StatusRequestExtension{},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.SessionTicketExtension{},
					&tls.ApplicationSettingsExtensionNew{
						SupportedProtocols: []string{"h3", "h2"},
					},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519MLKEM768},
						{Group: tls.X25519},
					}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519MLKEM768,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.ALPNExtension{AlpnProtocols: []string{
						"h3",
						"h2",
						"http/1.1",
					}},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.UtlsGREASEExtension{},
					&tls.UtlsPreSharedKeyExtension{},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_133 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Chrome",
		RandomExtensionOrder: false,
		Version:              "133",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []byte{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.SessionTicketExtension{},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.ApplicationSettingsExtensionNew{
						SupportedProtocols: []string{"h3", "h2"},
					},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519MLKEM768},
						{Group: tls.X25519},
					}},
					&tls.SCTExtension{},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.StatusRequestExtension{},
					&tls.ALPNExtension{AlpnProtocols: []string{
						"h3",
						"h2",
						"http/1.1",
					}},
					&tls.SNIExtension{},
					tls.BoringGREASEECH(),
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519MLKEM768,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.ExtendedMasterSecretExtension{},
					&tls.RenegotiationInfoExtension{
						Renegotiation: tls.RenegotiateOnceAsClient,
					},
					&tls.UtlsGREASEExtension{},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_117 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Chrome",
		RandomExtensionOrder: false,
		Version:              "117",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []uint8{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.SNIExtension{},
					&tls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.ApplicationSettingsExtension{
						SupportedProtocols: []string{"h2"},
					},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.CurveID(tls.GREASE_PLACEHOLDER),
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.ExtendedMasterSecretExtension{},
					&tls.SessionTicketExtension{},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.SCTExtension{},
					&tls.StatusRequestExtension{},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519},
					}},
					&tls.RenegotiationInfoExtension{Renegotiation: tls.RenegotiateOnceAsClient},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.UtlsGREASEExtension{},
					&tls.UtlsPaddingExtension{GetPaddingLen: tls.BoringPaddingStyle},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_124 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Chrome",
		RandomExtensionOrder: false,
		Version:              "124",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []uint8{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.SCTExtension{},
					&tls.ExtendedMasterSecretExtension{},
					&tls.ApplicationSettingsExtension{
						SupportedProtocols: []string{"h2"},
					},
					&tls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.SNIExtension{},
					&tls.SessionTicketExtension{},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519Kyber768Draft00,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					tls.BoringGREASEECH(),
					&tls.StatusRequestExtension{},
					&tls.RenegotiationInfoExtension{Renegotiation: tls.RenegotiateOnceAsClient},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519Kyber768Draft00},
						{Group: tls.X25519},
					}},
					&tls.UtlsGREASEExtension{},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_120 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Chrome",
		RandomExtensionOrder: false,
		Version:              "120",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []uint8{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.SNIExtension{},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.StatusRequestExtension{},
					&tls.ExtendedMasterSecretExtension{},
					&tls.SessionTicketExtension{},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.RenegotiationInfoExtension{Renegotiation: tls.RenegotiateOnceAsClient},
					&tls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
					tls.BoringGREASEECH(),
					&tls.SCTExtension{},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519},
					}},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.ApplicationSettingsExtension{
						SupportedProtocols: []string{"h2"},
					},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.UtlsGREASEExtension{},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_112 = ClientProfile{
	clientHelloId: tls.HelloChrome_112,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingEnablePush:           0,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_116_PSK = ClientProfile{
	clientHelloId: tls.HelloChrome_112_PSK,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingEnablePush:           0,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_116_PSK_PQ = ClientProfile{
	clientHelloId: tls.HelloChrome_115_PQ_PSK,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingEnablePush:           0,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_111 = ClientProfile{
	clientHelloId: tls.HelloChrome_111,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingEnablePush:           0,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_110 = ClientProfile{
	clientHelloId: tls.HelloChrome_110,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingEnablePush:           0,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_109 = ClientProfile{
	clientHelloId: tls.HelloChrome_109,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingEnablePush:           0,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_108 = ClientProfile{
	clientHelloId: tls.HelloChrome_108,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingEnablePush:           0,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_107 = ClientProfile{
	clientHelloId: tls.HelloChrome_107,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingEnablePush:           0,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_106 = ClientProfile{
	clientHelloId: tls.HelloChrome_106,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingEnablePush:           0,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_105 = ClientProfile{
	clientHelloId: tls.HelloChrome_105,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_104 = ClientProfile{
	clientHelloId: tls.HelloChrome_104,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Chrome_103 = ClientProfile{
	clientHelloId: tls.HelloChrome_103,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Safari_15_6_1 = ClientProfile{
	clientHelloId: tls.HelloSafari_15_6_1,
	settings: map[SettingID]uint32{
		SettingInitialWindowSize:    4194304,
		SettingMaxConcurrentStreams: 100,
	},
	settingsOrder: []SettingID{
		SettingInitialWindowSize,
		SettingMaxConcurrentStreams,
	},
	pseudoHeaderOrder: []string{
		":method",
		":scheme",
		":path",
		":authority",
	},
	connectionFlow: 10485760,
}

var Safari_16_0 = ClientProfile{
	clientHelloId: tls.HelloSafari_16_0,
	settings: map[SettingID]uint32{
		SettingInitialWindowSize:    4194304,
		SettingMaxConcurrentStreams: 100,
	},
	settingsOrder: []SettingID{
		SettingInitialWindowSize,
		SettingMaxConcurrentStreams,
	},
	pseudoHeaderOrder: []string{
		":method",
		":scheme",
		":path",
		":authority",
	},
	connectionFlow: 10485760,
}

var Safari_Ipad_15_6 = ClientProfile{
	clientHelloId: tls.HelloIPad_15_6,
	settings: map[SettingID]uint32{
		SettingInitialWindowSize:    2097152,
		SettingMaxConcurrentStreams: 100,
	},
	settingsOrder: []SettingID{
		SettingInitialWindowSize,
		SettingMaxConcurrentStreams,
	},
	pseudoHeaderOrder: []string{
		":method",
		":scheme",
		":path",
		":authority",
	},
	connectionFlow: 10485760,
}

var Safari_IOS_17_0 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "iOS",
		RandomExtensionOrder: false,
		Version:              "17.0",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_ECDSA_WITH_3DES_EDE_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
					tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
				},
				CompressionMethods: []uint8{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.SNIExtension{},
					&tls.ExtendedMasterSecretExtension{},
					&tls.RenegotiationInfoExtension{Renegotiation: tls.RenegotiateOnceAsClient},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
						tls.CurveP521,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
					&tls.StatusRequestExtension{},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.ECDSAWithSHA1,
						tls.PSSWithSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
						tls.PKCS1WithSHA1,
					}},
					&tls.SCTExtension{},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519},
					}},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
						tls.VersionTLS11,
						tls.VersionTLS10,
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionZlib,
					}},
					&tls.UtlsGREASEExtension{},
					&tls.UtlsPaddingExtension{GetPaddingLen: tls.BoringPaddingStyle},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingEnablePush:           0,
		SettingInitialWindowSize:    2097152,
		SettingMaxConcurrentStreams: 100,
	},
	settingsOrder: []SettingID{
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxConcurrentStreams,
	},
	pseudoHeaderOrder: []string{
		":method",
		":scheme",
		":path",
		":authority",
	},
	connectionFlow: 10485760,
}

var Safari_IOS_26_0 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "iOS",
		RandomExtensionOrder: false,
		Version:              "26.0",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_ECDSA_WITH_3DES_EDE_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
					tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
				},
				CompressionMethods: []uint8{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.SNIExtension{},
					&tls.ExtendedMasterSecretExtension{},
					&tls.RenegotiationInfoExtension{Renegotiation: tls.RenegotiateOnceAsClient},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519MLKEM768,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
						tls.CurveP521,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
					&tls.StatusRequestExtension{},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
						tls.PKCS1WithSHA1,
					}},
					&tls.SCTExtension{},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519MLKEM768},
						{Group: tls.X25519},
					}},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionZlib,
					}},
					&tls.UtlsGREASEExtension{},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingEnablePush:           0,
		SettingMaxConcurrentStreams: 100,
		SettingInitialWindowSize:    2097152,
		SettingNoRFC7540Priorities:  1,
	},
	settingsOrder: []SettingID{
		SettingEnablePush,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingNoRFC7540Priorities,
	},
	pseudoHeaderOrder: []string{
		":method",
		":scheme",
		":authority",
		":path",
	},
	connectionFlow: 10420225,
}

var Safari_IOS_18_5 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "iOS",
		RandomExtensionOrder: false,
		Version:              "18.5",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_ECDSA_WITH_3DES_EDE_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
					tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
				},
				CompressionMethods: []uint8{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.SNIExtension{},
					&tls.ExtendedMasterSecretExtension{},
					&tls.RenegotiationInfoExtension{Renegotiation: tls.RenegotiateOnceAsClient},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
						tls.CurveP521,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
					&tls.StatusRequestExtension{},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
						tls.PKCS1WithSHA1,
					}},
					&tls.SCTExtension{},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519},
					}},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
						tls.VersionTLS11,
						tls.VersionTLS10,
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionZlib,
					}},
					&tls.UtlsGREASEExtension{},
					&tls.UtlsPaddingExtension{GetPaddingLen: tls.BoringPaddingStyle},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingEnablePush:           0,
		SettingMaxConcurrentStreams: 100,
		SettingInitialWindowSize:    2097152,
		SettingNoRFC7540Priorities:  1,
	},
	settingsOrder: []SettingID{
		SettingEnablePush,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingNoRFC7540Priorities,
	},
	pseudoHeaderOrder: []string{
		":method",
		":scheme",
		":authority",
		":path",
	},
	connectionFlow: 10420225,
	headerPriority: &PriorityParam{
		StreamDep: 0,
		Exclusive: false,
		Weight:    255,
	},
}

var Safari_IOS_18_0 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "iOS",
		RandomExtensionOrder: false,
		Version:              "18.0",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_ECDSA_WITH_3DES_EDE_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
					tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
				},
				CompressionMethods: []uint8{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.SNIExtension{},
					&tls.ExtendedMasterSecretExtension{},
					&tls.RenegotiationInfoExtension{Renegotiation: tls.RenegotiateOnceAsClient},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
						tls.CurveP521,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
					&tls.StatusRequestExtension{},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.ECDSAWithSHA1,
						tls.PSSWithSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
						tls.PKCS1WithSHA1,
					}},
					&tls.SCTExtension{},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519},
					}},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
						tls.VersionTLS11,
						tls.VersionTLS10,
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionZlib,
					}},
					&tls.UtlsGREASEExtension{},
					&tls.UtlsPaddingExtension{GetPaddingLen: tls.BoringPaddingStyle},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingEnablePush:           0,
		SettingMaxConcurrentStreams: 100,
		SettingInitialWindowSize:    2097152,
		0x8:                               1,
		0x9:                               1,
	},
	settingsOrder: []SettingID{
		SettingEnablePush,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		0x8,
		0x9,
	},
	pseudoHeaderOrder: []string{
		":method",
		":scheme",
		":authority",
		":path",
	},
	connectionFlow: 10420225,
}

var Safari_IOS_16_0 = ClientProfile{
	clientHelloId: tls.HelloIOS_16_0,
	settings: map[SettingID]uint32{
		SettingInitialWindowSize:    2097152,
		SettingMaxConcurrentStreams: 100,
	},
	settingsOrder: []SettingID{
		SettingInitialWindowSize,
		SettingMaxConcurrentStreams,
	},
	pseudoHeaderOrder: []string{
		":method",
		":scheme",
		":path",
		":authority",
	},
	connectionFlow: 10485760,
}

var Safari_IOS_15_5 = ClientProfile{
	clientHelloId: tls.HelloIOS_15_5,
	settings: map[SettingID]uint32{
		SettingInitialWindowSize:    2097152,
		SettingMaxConcurrentStreams: 100,
	},
	settingsOrder: []SettingID{
		SettingInitialWindowSize,
		SettingMaxConcurrentStreams,
	},
	pseudoHeaderOrder: []string{
		":method",
		":scheme",
		":path",
		":authority",
	},
	connectionFlow: 10485760,
}

var Safari_IOS_15_6 = ClientProfile{
	clientHelloId: tls.HelloIOS_15_6,
	settings: map[SettingID]uint32{
		SettingInitialWindowSize:    2097152,
		SettingMaxConcurrentStreams: 100,
	},
	settingsOrder: []SettingID{
		SettingInitialWindowSize,
		SettingMaxConcurrentStreams,
	},
	pseudoHeaderOrder: []string{
		":method",
		":scheme",
		":path",
		":authority",
	},
	connectionFlow: 10485760,
}

var Firefox_117 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Firefox",
		RandomExtensionOrder: false,
		Version:              "117",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []byte{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					&tls.SNIExtension{},
					&tls.ExtendedMasterSecretExtension{},
					&tls.RenegotiationInfoExtension{Renegotiation: tls.RenegotiateOnceAsClient},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
						tls.CurveP521,
						tls.FAKEFFDHE2048,
						tls.FAKEFFDHE3072,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},

					&tls.SessionTicketExtension{},
					&tls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
					&tls.StatusRequestExtension{},
					&tls.DelegatedCredentialsExtension{
						SupportedSignatureAlgorithms: []tls.SignatureScheme{
							tls.ECDSAWithP256AndSHA256,
							tls.ECDSAWithP384AndSHA384,
							tls.ECDSAWithP521AndSHA512,
							tls.ECDSAWithSHA1,
						},
					},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.X25519},
						{Group: tls.CurveP256},
					}},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.ECDSAWithP521AndSHA512,
						tls.PSSWithSHA256,
						tls.PSSWithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA256,
						tls.PKCS1WithSHA384,
						tls.PKCS1WithSHA512,
						tls.ECDSAWithSHA1,
						tls.PKCS1WithSHA1,
					}},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.FakeRecordSizeLimitExtension{Limit: 0x4001},
					&tls.UtlsPaddingExtension{GetPaddingLen: tls.BoringPaddingStyle},
				},
			}, nil
		},
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingInitialWindowSize: 131072,
		SettingMaxFrameSize:      16384,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingInitialWindowSize,
		SettingMaxFrameSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":path",
		":authority",
		":scheme",
	},
	connectionFlow: 12517377,
	headerPriority: &PriorityParam{
		StreamDep: 13,
		Exclusive: false,
		Weight:    41,
	},
	priorities: []Priority{
		{StreamID: 3, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    200,
		}},
		{StreamID: 5, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    100,
		}},
		{StreamID: 7, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 9, PriorityParam: PriorityParam{
			StreamDep: 7,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 11, PriorityParam: PriorityParam{
			StreamDep: 3,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 13, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    240,
		}},
	},
}

var Firefox_110 = ClientProfile{
	clientHelloId: tls.HelloFirefox_110,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingInitialWindowSize: 131072,
		SettingMaxFrameSize:      16384,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingInitialWindowSize,
		SettingMaxFrameSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":path",
		":authority",
		":scheme",
	},
	connectionFlow: 12517377,
	headerPriority: &PriorityParam{
		StreamDep: 13,
		Exclusive: false,
		Weight:    41,
	},
	priorities: []Priority{
		{StreamID: 3, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    200,
		}},
		{StreamID: 5, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    100,
		}},
		{StreamID: 7, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 9, PriorityParam: PriorityParam{
			StreamDep: 7,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 11, PriorityParam: PriorityParam{
			StreamDep: 3,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 13, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    240,
		}},
	},
}

var Firefox_108 = ClientProfile{
	clientHelloId: tls.HelloFirefox_108,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingInitialWindowSize: 131072,
		SettingMaxFrameSize:      16384,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingInitialWindowSize,
		SettingMaxFrameSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":path",
		":authority",
		":scheme",
	},
	connectionFlow: 12517377,
	headerPriority: &PriorityParam{
		StreamDep: 13,
		Exclusive: false,
		Weight:    41,
	},
	priorities: []Priority{
		{StreamID: 3, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    200,
		}},
		{StreamID: 5, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    100,
		}},
		{StreamID: 7, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 9, PriorityParam: PriorityParam{
			StreamDep: 7,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 11, PriorityParam: PriorityParam{
			StreamDep: 3,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 13, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    240,
		}},
	},
}

var Firefox_106 = ClientProfile{
	clientHelloId: tls.HelloFirefox_106,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingInitialWindowSize: 131072,
		SettingMaxFrameSize:      16384,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingInitialWindowSize,
		SettingMaxFrameSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":path",
		":authority",
		":scheme",
	},
	connectionFlow: 12517377,
	headerPriority: &PriorityParam{
		StreamDep: 13,
		Exclusive: false,
		Weight:    41,
	},
	priorities: []Priority{
		{StreamID: 3, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    200,
		}},
		{StreamID: 5, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    100,
		}},
		{StreamID: 7, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 9, PriorityParam: PriorityParam{
			StreamDep: 7,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 11, PriorityParam: PriorityParam{
			StreamDep: 3,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 13, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    240,
		}},
	},
}

var Firefox_105 = ClientProfile{
	clientHelloId: tls.HelloFirefox_105,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingInitialWindowSize: 131072,
		SettingMaxFrameSize:      16384,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingInitialWindowSize,
		SettingMaxFrameSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":path",
		":authority",
		":scheme",
	},
	connectionFlow: 12517377,
	headerPriority: &PriorityParam{
		StreamDep: 13,
		Exclusive: false,
		Weight:    41,
	},
	priorities: []Priority{
		{StreamID: 3, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    200,
		}},
		{StreamID: 5, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    100,
		}},
		{StreamID: 7, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 9, PriorityParam: PriorityParam{
			StreamDep: 7,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 11, PriorityParam: PriorityParam{
			StreamDep: 3,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 13, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    240,
		}},
	},
}

var Firefox_104 = ClientProfile{
	clientHelloId: tls.HelloFirefox_104,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingInitialWindowSize: 131072,
		SettingMaxFrameSize:      16384,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingInitialWindowSize,
		SettingMaxFrameSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":path",
		":authority",
		":scheme",
	},
	connectionFlow: 12517377,
	headerPriority: &PriorityParam{
		StreamDep: 13,
		Exclusive: false,
		Weight:    41,
	},
	priorities: []Priority{
		{StreamID: 3, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    200,
		}},
		{StreamID: 5, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    100,
		}},
		{StreamID: 7, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 9, PriorityParam: PriorityParam{
			StreamDep: 7,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 11, PriorityParam: PriorityParam{
			StreamDep: 3,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 13, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    240,
		}},
	},
}

var Firefox_102 = ClientProfile{
	clientHelloId: tls.HelloFirefox_102,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingInitialWindowSize: 131072,
		SettingMaxFrameSize:      16384,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingInitialWindowSize,
		SettingMaxFrameSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":path",
		":authority",
		":scheme",
	},
	connectionFlow: 12517377,
	headerPriority: &PriorityParam{
		StreamDep: 13,
		Exclusive: false,
		Weight:    41,
	},
	priorities: []Priority{
		{StreamID: 3, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    200,
		}},
		{StreamID: 5, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    100,
		}},
		{StreamID: 7, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 9, PriorityParam: PriorityParam{
			StreamDep: 7,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 11, PriorityParam: PriorityParam{
			StreamDep: 3,
			Exclusive: false,
			Weight:    0,
		}},
		{StreamID: 13, PriorityParam: PriorityParam{
			StreamDep: 0,
			Exclusive: false,
			Weight:    240,
		}},
	},
}

var Opera_90 = ClientProfile{
	clientHelloId: tls.HelloOpera_90,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Opera_91 = ClientProfile{
	clientHelloId: tls.HelloOpera_91,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

var Opera_89 = ClientProfile{
	clientHelloId: tls.HelloOpera_89,
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:      65536,
		SettingMaxConcurrentStreams: 1000,
		SettingInitialWindowSize:    6291456,
		SettingMaxHeaderListSize:    262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingMaxConcurrentStreams,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

// Brave_146 is a TLS client profile for Brave Browser v146 (Chromium 146 based).
// Captured from tls.peet.ws/api/all on 2026-03-27 (5 samples verified).
// NOTE: Brave randomizes TLS extension order on every connection (anti-fingerprinting).
// JA3 hash: varies per connection (extension order changes)
// JA4: t13d1516h2_8daaf6152771_d8a2da3f94cd (consistent - JA4 sorts extensions)
// Peetprint hash: 1d4ffe9b0e34acac0bd883fa7f79d7b5 (consistent)
var Brave_146 = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Brave",
		RandomExtensionOrder: true,
		Version:              "146",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []byte{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					// Brave randomizes extension order each connection (RandomExtensionOrder=true).
					// GREASE bookends (first+last) stay fixed; inner extensions are shuffled.
					&tls.UtlsGREASEExtension{},
					&tls.SessionTicketExtension{},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.RenegotiationInfoExtension{
						Renegotiation: tls.RenegotiateOnceAsClient,
					},
					&tls.ALPNExtension{AlpnProtocols: []string{
						"h2",
						"http/1.1",
					}},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519MLKEM768,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.ExtendedMasterSecretExtension{},
					tls.BoringGREASEECH(),
					&tls.SNIExtension{},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519MLKEM768},
						{Group: tls.X25519},
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.ApplicationSettingsExtensionNew{
						SupportedProtocols: []string{"h2"},
					},
					&tls.SCTExtension{},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.StatusRequestExtension{},
					&tls.UtlsGREASEExtension{},
				},
			}, nil
		},
	},
	headerPriority: &PriorityParam{
		StreamDep: 0,
		Exclusive: true,
		Weight:    255,
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}

// Brave_146_PSK is a TLS client profile for Brave Browser v146 with PSK (Pre-Shared Key)
// for subsequent TLS 1.3 connections where session resumption is available.
// Extension order is randomized like the non-PSK variant.
var Brave_146_PSK = ClientProfile{
	clientHelloId: tls.ClientHelloID{
		Client:               "Brave",
		RandomExtensionOrder: true,
		Version:              "146_PSK",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []byte{
					tls.CompressionNone,
				},
				Extensions: []tls.TLSExtension{
					// Brave randomizes extension order each connection (RandomExtensionOrder=true).
					// GREASE bookends (first+last) stay fixed; inner extensions are shuffled.
					&tls.UtlsGREASEExtension{},
					&tls.SessionTicketExtension{},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
						tls.PskModeDHE,
					}},
					&tls.RenegotiationInfoExtension{
						Renegotiation: tls.RenegotiateOnceAsClient,
					},
					&tls.ALPNExtension{AlpnProtocols: []string{
						"h2",
						"http/1.1",
					}},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519MLKEM768,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.ExtendedMasterSecretExtension{},
					tls.BoringGREASEECH(),
					&tls.SNIExtension{},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519MLKEM768},
						{Group: tls.X25519},
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{
						tls.CertCompressionBrotli,
					}},
					&tls.ApplicationSettingsExtensionNew{
						SupportedProtocols: []string{"h2"},
					},
					&tls.SCTExtension{},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{
						tls.PointFormatUncompressed,
					}},
					&tls.StatusRequestExtension{},
					&tls.UtlsGREASEExtension{},
					&tls.UtlsPreSharedKeyExtension{},
				},
			}, nil
		},
	},
	headerPriority: &PriorityParam{
		StreamDep: 0,
		Exclusive: true,
		Weight:    255,
	},
	settings: map[SettingID]uint32{
		SettingHeaderTableSize:   65536,
		SettingEnablePush:        0,
		SettingInitialWindowSize: 6291456,
		SettingMaxHeaderListSize: 262144,
	},
	settingsOrder: []SettingID{
		SettingHeaderTableSize,
		SettingEnablePush,
		SettingInitialWindowSize,
		SettingMaxHeaderListSize,
	},
	pseudoHeaderOrder: []string{
		":method",
		":authority",
		":scheme",
		":path",
	},
	connectionFlow: 15663105,
}
