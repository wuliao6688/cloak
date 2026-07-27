// Package profiles provides TLS ClientHello profiles and HTTP/2
// fingerprint parameters. H2 types are defined locally so the profiles
// package has zero external dependencies beyond uTLS — downstream
// packages (e.g. tlsgateway) do not transitively pull in fhttp/http2.
package profiles

// SettingID identifies an HTTP/2 SETTINGS parameter.
type SettingID uint16

// HTTP/2 SETTINGS identifiers (RFC 7540 §6.5.2).
const (
	SettingHeaderTableSize      SettingID = 1
	SettingEnablePush           SettingID = 2
	SettingMaxConcurrentStreams SettingID = 3
	SettingInitialWindowSize    SettingID = 4
	SettingMaxFrameSize         SettingID = 5
	SettingMaxHeaderListSize    SettingID = 6

	// Chrome-specific SETTINGS (borrowed from fhttp).
	SettingNoRFC7540Priorities SettingID = 0x9
)

// PriorityParam describes an HTTP/2 stream priority (RFC 7540 §5.3).
type PriorityParam struct {
	StreamDep uint32
	Exclusive bool
	Weight    uint8
}

// Priority assigns a PriorityParam to a stream ID.
type Priority struct {
	StreamID      uint32
	PriorityParam PriorityParam
}
