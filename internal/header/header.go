// Package header provides HTTP header constants and sorting utilities
// for browser fingerprint emulation.
//
// Pattern adapted from req (github.com/imroc/req). The key insight:
// H2 pseudo-header order and regular header order are passed as special
// HTTP headers that the H2 transport reads during encoding and strips
// from the wire. This avoids adding new API surface to the Transport.
//
//	req.Header.Set(header.HeaderOrderKey, "host,user-agent,accept,...")
//	req.Header.Set(header.PseudoHeaderOrderKey, ":method,:authority,:scheme,:path")
//
// When an H2-capable transport sees these keys, it sorts the output
// accordingly; when it doesn't, the keys are silently stripped (they're
// in the exclude list).
package header

import (
	"net/textproto"
	"strings"
)

const (
	HeaderOrderKey       = "__header_order__"
	PseudoHeaderOrderKey = "__pseudo_header_order__"
)

// ExcludedHeaders lists headers that should be stripped before sending.
// These are internal housekeeping keys.
var ExcludedHeaders = map[string]bool{
	"host":              true,
	"content-length":    true,
	"connection":        true,
	"proxy-connection":  true,
	"transfer-encoding": true,
	"upgrade":           true,
	"keep-alive":        true,
	HeaderOrderKey:       true,
	PseudoHeaderOrderKey: true,
}

// IsExcluded returns true if the header key should be stripped.
func IsExcluded(key string) bool {
	return ExcludedHeaders[strings.ToLower(key)]
}

// ─── Header Sorting ─────────────────────────────────────────────────────

// KeyValues is a key-value pair with multiple values (like http.Header).
type KeyValues struct {
	Key    string
	Values []string
}

// SortKeyValues sorts the slice in-place according to the ordered key list.
// Keys not in the ordered list are placed at the end in their original
// relative order.
func SortKeyValues(kvs []KeyValues, orderedKeys []string) {
	order := make(map[string]int, len(orderedKeys))
	for i, key := range orderedKeys {
		order[textproto.CanonicalMIMEHeaderKey(key)] = i
	}

	// Assign each element a sort position.
	type indexed struct {
		kv  KeyValues
		pos int
	}
	items := make([]indexed, len(kvs))
	defaultPos := len(order)
	for i, kv := range kvs {
		canonical := textproto.CanonicalMIMEHeaderKey(kv.Key)
		if pos, ok := order[canonical]; ok {
			items[i] = indexed{kv, pos}
		} else {
			items[i] = indexed{kv, defaultPos}
			defaultPos++
		}
	}

	// Stable sort by position.
	for i := 1; i < len(items); i++ {
		j := i
		for j > 0 && items[j].pos < items[j-1].pos {
			items[j], items[j-1] = items[j-1], items[j]
			j--
		}
	}

	// Write back.
	for i, item := range items {
		kvs[i] = item.kv
	}
}
