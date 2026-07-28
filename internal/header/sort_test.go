package header

import (
	"strings"
	"testing"
)

func TestSortKeyValues(t *testing.T) {
	order := []string{":method", ":authority", ":scheme", ":path"}
	kvs := []KeyValues{
		{Key: ":scheme", Values: []string{"https"}},
		{Key: ":path", Values: []string{"/api"}},
		{Key: ":method", Values: []string{"GET"}},
		{Key: ":authority", Values: []string{"example.com"}},
	}

	SortKeyValues(kvs, order)

	expected := []string{":method", ":authority", ":scheme", ":path"}
	for i, kv := range kvs {
		if kv.Key != expected[i] {
			t.Errorf("position %d: got %q want %q", i, kv.Key, expected[i])
		}
	}
}

func TestSortKeyValuesUnorderedFallback(t *testing.T) {
	order := []string{"host", "user-agent"}
	kvs := []KeyValues{
		{Key: "accept", Values: []string{"*/*"}},
		{Key: "user-agent", Values: []string{"test"}},
		{Key: "host", Values: []string{"example.com"}},
	}

	SortKeyValues(kvs, order)

	// host and user-agent should be at positions 0,1; accept at 2
	expected := []string{"host", "user-agent", "accept"}
	for i, kv := range kvs {
		if kv.Key != expected[i] {
			t.Errorf("position %d: got %q want %q", i, kv.Key, expected[i])
		}
	}
}

func TestIsExcluded(t *testing.T) {
	tests := []struct {
		key      string
		excluded bool
	}{
		{"Host", true},
		{"Content-Length", true},
		{"__header_order__", true},
		{"__pseudo_header_order__", true},
		{"User-Agent", false},
		{"Accept", false},
	}
	for _, tt := range tests {
		got := IsExcluded(tt.key)
		if got != tt.excluded {
			t.Errorf("IsExcluded(%q) = %v, want %v", tt.key, got, tt.excluded)
		}
	}
}

func TestSortKeyValuesPreservesValues(t *testing.T) {
	order := []string{"accept", "user-agent"}
	kvs := []KeyValues{
		{Key: "user-agent", Values: []string{"test-agent"}},
		{Key: "accept", Values: []string{"text/html", "application/json"}},
	}

	SortKeyValues(kvs, order)

	// Check values preserved after sort
	if len(kvs[0].Values) != 2 || kvs[0].Values[0] != "text/html" || kvs[0].Values[1] != "application/json" {
		t.Errorf("accept values incorrect: %v", kvs[0].Values)
	}
	if len(kvs[1].Values) != 1 || kvs[1].Values[0] != "test-agent" {
		t.Errorf("user-agent values incorrect: %v", kvs[1].Values)
	}
}

func FuzzSortKeyValues(f *testing.F) {
	f.Add(":method", ":scheme", ":path")
	f.Fuzz(func(t *testing.T, a, b, c string) {
		order := strings.Split(a+","+b+","+c, ",")
		kvs := []KeyValues{
			{Key: order[1], Values: []string{"v1"}},
			{Key: order[0], Values: []string{"v0"}},
			{Key: order[2], Values: []string{"v2"}},
		}
		SortKeyValues(kvs, order)
		// Verify no panic and all KVs still present
		if len(kvs) != 3 {
			t.Errorf("lost keys: got %d want 3", len(kvs))
		}
	})
}
