package tls_client

import (
	"github.com/bogdanfinn/fhttp/http2"
	"github.com/bogdanfinn/tls-client/bandwidth"
	tls "github.com/bogdanfinn/utls"
)

// protocolRacerConfig groups all parameters needed to construct a protocolRacer.
// This avoids the 16-parameter constructor and makes the dependency graph explicit.
type protocolRacerConfig struct {
	clientSessionCache  tls.ClientSessionCache
	insecureSkipVerify  bool
	serverNameOverwrite string
	transportOptions    *TransportOptions
	settings            map[http2.SettingID]uint32
	shardedCache        *shardedTransportCache
	transportInit       *keyedLockPool
	certificatePinner   CertificatePinner
	badPinHandlerFunc   BadPinHandlerFunc
	bandwidthTracker    bandwidth.BandwidthTracker

	// HTTP/3 specific settings
	http3Settings          map[uint64]uint64
	http3SettingsOrder     []uint64
	http3PriorityParam     uint32
	http3PseudoHeaderOrder []string
	http3SendGreaseFrames  bool
}

func (c *protocolRacerConfig) toRacer() *protocolRacer {
	return &protocolRacer{
		protocolCache:          make(map[string]string),
		clientSessionCache:     c.clientSessionCache,
		insecureSkipVerify:     c.insecureSkipVerify,
		serverNameOverwrite:    c.serverNameOverwrite,
		transportOptions:       c.transportOptions,
		settings:               c.settings,
		shardedCache:           c.shardedCache,
		transportInit:          c.transportInit,
		certificatePinner:      c.certificatePinner,
		badPinHandlerFunc:      c.badPinHandlerFunc,
		bandwidthTracker:       c.bandwidthTracker,
		http3Settings:          c.http3Settings,
		http3SettingsOrder:     c.http3SettingsOrder,
		http3PriorityParam:     c.http3PriorityParam,
		http3PseudoHeaderOrder: c.http3PseudoHeaderOrder,
		http3SendGreaseFrames:  c.http3SendGreaseFrames,
	}
}
