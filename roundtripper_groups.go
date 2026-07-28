package tls_client

import (
	"context"
	"net"
	"sync"

	"github.com/bogdanfinn/fhttp/http2"
	tls "github.com/bogdanfinn/utls"

	"github.com/bogdanfinn/tls-client/profiles"
)

// --- RoundTripper grouped sub-structs ---
// These groups partition the 28-field roundTripper into logical domains,
// embedded anonymously so existing field references remain unchanged.

// rtTLSParams groups TLS handshake and certificate parameters.
type rtTLSParams struct {
	clientHelloId               tls.ClientHelloID
	clientSessionCache          tls.ClientSessionCache
	serverNameOverwrite         string
	insecureSkipVerify          bool
	withRandomTlsExtensionOrder bool
	certificatePinner           CertificatePinner
	badPinHandlerFunc           BadPinHandlerFunc
}

// rtH2Params groups HTTP/2 protocol parameters.
type rtH2Params struct {
	initialStreamID   uint32
	allowHTTP         bool
	settings          map[http2.SettingID]uint32
	settingsOrder     []http2.SettingID
	headerPriority    *http2.PriorityParam
	priorities        []http2.Priority
	pseudoHeaderOrder []string
	connectionFlow    uint32
}

// profileSettings converts profiles.SettingID map to fhttp/http2.SettingID map.
func profileSettings(m map[profiles.SettingID]uint32) map[http2.SettingID]uint32 {
	out := make(map[http2.SettingID]uint32, len(m))
	for k, v := range m {
		out[http2.SettingID(k)] = v
	}
	return out
}

// profileSettingsOrder converts profiles.SettingID slice to fhttp/http2.SettingID slice.
func profileSettingsOrder(s []profiles.SettingID) []http2.SettingID {
	out := make([]http2.SettingID, len(s))
	for i, v := range s {
		out[i] = http2.SettingID(v)
	}
	return out
}

// profilePriorityParam converts profiles.PriorityParam to fhttp/http2.PriorityParam.
func profilePriorityParam(p *profiles.PriorityParam) *http2.PriorityParam {
	if p == nil {
		return nil
	}
	return &http2.PriorityParam{
		StreamDep: p.StreamDep,
		Exclusive: p.Exclusive,
		Weight:    p.Weight,
	}
}

// profilePriorities converts profiles.Priority slice to fhttp/http2.Priority slice.
func profilePriorities(pp []profiles.Priority) []http2.Priority {
	out := make([]http2.Priority, len(pp))
	for i, p := range pp {
		out[i] = http2.Priority{
			StreamID:      p.StreamID,
			PriorityParam: http2.PriorityParam(p.PriorityParam),
		}
	}
	return out
}

// rtH3Params groups HTTP/3 protocol parameters.
type rtH3Params struct {
	http3Settings          map[uint64]uint64
	http3SettingsOrder     []uint64
	http3PriorityParam     uint32
	http3PseudoHeaderOrder []string
	http3SendGreaseFrames  bool
}

// rtCacheState groups connection and transport caches with their locks.
type rtCacheState struct {
	cachedConnections    map[string]net.Conn
	cachedConnectionsLck sync.Mutex
	shardedCache         *shardedTransportCache // replaces cachedTransports + lock + meta
	transportInit        keyedLockPool
}

// rtH2DialState groups HTTP/2 dial context tracking.
type rtH2DialState struct {
	http2DialContexts    map[string]map[uint64]context.Context
	http2DialCancels     map[string]map[uint64]context.CancelFunc
	http2DialContextSeq  uint64
	http2DialContextsLck sync.Mutex
}

// rtProtoFlags groups boolean protocol toggles.
type rtProtoFlags struct {
	forceHttp1   bool
	disableHttp3 bool
	disableIPV4  bool
	disableIPV6  bool
}
