package tls_client

import (
	"context"
	"net"
	"sync"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/fhttp/http2"
	tls "github.com/bogdanfinn/utls"
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
	cachedTransports     map[string]http.RoundTripper
	transportCache       *transportCacheMeta
	cachedConnectionsLck sync.Mutex
	cachedTransportsLck  sync.RWMutex
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
