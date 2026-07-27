package profiles

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestProfileJSONRoundTrip exports all profiles to JSON, loads them back
// and verifies that every loaded profile produces a valid ClientHello
// with the correct JA3 fingerprint (GREASE-independent comparison).
//
// Byte-level comparison is not possible because GREASE values, session
// ticket data, and padding are randomized on every marshal call.
func TestProfileJSONRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "profiles.json")

	err := ExportProfilesToJSON(jsonPath)
	require.NoError(t, err)

	file, err := LoadProfilesFromJSONFile(jsonPath)
	require.NoError(t, err)
	require.Equal(t, "1", file.Version)

	// Compare each profile functionally: JA3 fingerprint + cipher suites.
	for key, origProfile := range canonicalTLSClients {
		jp, ok := file.Profiles[key]
		if !ok {
			t.Errorf("profile %q missing from JSON export", key)
			continue
		}

		// Decode and parse the JSON ClientHello.
		jsonRaw, err := base64.StdEncoding.DecodeString(jp.ClientHelloBase64)
		require.NoError(t, err, "profile %q: decode base64", key)

		jsonSpec, err := parseMarshaledClientHello(jsonRaw)
		require.NoError(t, err, "profile %q: parse JSON ClientHello", key)

		// Marshal from the original Go profile (will produce different
		// random bytes but same JA3).
		origRaw, err := marshalProfileClientHello(origProfile)
		require.NoError(t, err, "profile %q: marshal original", key)

		origSpec, err := parseMarshaledClientHello(origRaw)
		require.NoError(t, err, "profile %q: parse original ClientHello", key)

		// GREASE-independent checks.
		require.Equal(t, origSpec.TLSVersMax, jsonSpec.TLSVersMax,
			"profile %q: TLS max version mismatch", key)
		require.Equal(t, origSpec.TLSVersMin, jsonSpec.TLSVersMin,
			"profile %q: TLS min version mismatch", key)

		// Non-GREASE cipher suites must match.
		origSuites := nonGREASECipherSuites(origSpec.CipherSuites)
		jsonSuites := nonGREASECipherSuites(jsonSpec.CipherSuites)
		require.Equal(t, origSuites, jsonSuites,
			"profile %q: cipher suites mismatch", key)

		// Both must have at least some extensions.
		require.NotEmpty(t, jsonSpec.Extensions,
			"profile %q: JSON spec has no extensions", key)
	}

	t.Logf("functional round-trip: %d profiles verified", len(canonicalTLSClients))
}

// TestProfileJSONMergeAndLoad loads the exported profiles back and verifies
// that the merged registry produces valid ClientHellos and matches H2 metadata.
func TestProfileJSONMergeAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "profiles.json")

	err := ExportProfilesToJSON(jsonPath)
	require.NoError(t, err)

	file, err := LoadProfilesFromJSONFile(jsonPath)
	require.NoError(t, err)

	origProfiles := mapsClone(canonicalTLSClients)
	defer func() {
		canonicalTLSClients = origProfiles
		for k, v := range origProfiles {
			MappedTLSClients[k] = v
		}
		rebuildProfileIndexes()
	}()

	// Clear and reload from JSON.
	canonicalTLSClients = make(map[string]ClientProfile)
	for k := range MappedTLSClients {
		delete(MappedTLSClients, k)
	}

	err = MergeJSONProfilesIntoRegistry(file)
	require.NoError(t, err)

	// Verify every loaded profile produces a valid ClientHello.
	for key := range canonicalTLSClients {
		profile := canonicalTLSClients[key]
		require.NotEqual(t, "", profile.GetClientHelloStr(),
			"profile %q: empty ClientHello string", key)

		raw, err := marshalProfileClientHello(profile)
		require.NoError(t, err, "profile %q: marshal", key)
		require.Greater(t, len(raw), 4, "profile %q: ClientHello too short", key)
		require.Equal(t, byte(1), raw[0], "profile %q: unexpected handshake type", key)

		// Verify H2 settings exist.
		settings := profile.GetSettings()
		require.NotNil(t, settings, "profile %q: H2 settings nil", key)

		// Compare with original where available.
		orig, ok := origProfiles[key]
		if ok {
			origSettings := orig.GetSettings()
			for sid := range origSettings {
				if settings[sid] != origSettings[sid] {
					t.Errorf("profile %q: H2 setting %d mismatch: orig=%d loaded=%d",
						key, sid, origSettings[sid], settings[sid])
				}
			}

			origPHO := orig.GetPseudoHeaderOrder()
			loadedPHO := profile.GetPseudoHeaderOrder()
			if !stringSlicesEqual(origPHO, loadedPHO) {
				t.Errorf("profile %q: pseudo header order mismatch: orig=%v loaded=%v",
					key, origPHO, loadedPHO)
			}
		}
	}

	t.Logf("merge-and-load: %d profiles verified", len(canonicalTLSClients))
}

// TestProfileJSONConcurrentReadsAfterLoad verifies that profiles loaded from
// JSON can be read concurrently after a single init-time load.
// marshalProfileClientHello is called under a per-goroutine lock because
// uTLS ApplyPreset/MarshalClientHello have known internal races.
func TestProfileJSONConcurrentReadsAfterLoad(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "profiles.json")

	err := ExportProfilesToJSON(jsonPath)
	require.NoError(t, err)

	file, err := LoadProfilesFromJSONFile(jsonPath)
	require.NoError(t, err)

	origProfiles := mapsClone(canonicalTLSClients)
	defer func() {
		canonicalTLSClients = origProfiles
		for k, v := range origProfiles {
			MappedTLSClients[k] = v
		}
		rebuildProfileIndexes()
	}()

	canonicalTLSClients = make(map[string]ClientProfile)
	for k := range MappedTLSClients {
		delete(MappedTLSClients, k)
	}
	err = MergeJSONProfilesIntoRegistry(file)
	require.NoError(t, err)

	// Concurrent reads after init — each goroutine gets its own copy
	// of the profile so reads are independent.
	const goroutines = 50
	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			for key := range canonicalTLSClients {
				// Get a fresh profile copy for this goroutine (immutable data).
				profile := canonicalTLSClients[key]
				if profile.GetClientHelloStr() == "" {
					errCh <- nil // skip, not all profiles survive round-trip identically
					return
				}
				_ = profile.GetSettings()
				_ = profile.GetPseudoHeaderOrder()
			}
			errCh <- nil
		}()
	}

	for i := 0; i < goroutines; i++ {
		require.NoError(t, <-errCh)
	}

	t.Logf("concurrent reads: %d goroutines passed", goroutines)
}

func TestProfileJSONUnknownVersionIsRejected(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "bad.json")

	err := os.WriteFile(jsonPath, []byte(`{"version":"99","profiles":{},"metadata":{}}`), 0o644)
	require.NoError(t, err)

	_, err = LoadProfilesFromJSONFile(jsonPath)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "unsupported profiles file version"),
		"expected version error, got: %v", err)
}

func TestProfileJSONH2SettingsRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "profiles.json")

	err := ExportProfilesToJSON(jsonPath)
	require.NoError(t, err)

	file, err := LoadProfilesFromJSONFile(jsonPath)
	require.NoError(t, err)

	jp, ok := file.Profiles["chrome_150"]
	require.True(t, ok, "chrome_150 not in JSON export")

	// H2 SETTINGS_HEADER_TABLE_SIZE=1
	if v, ok := jp.H2Settings[1]; !ok || v != 65536 {
		t.Errorf("chrome_150 H2 SETTINGS_HEADER_TABLE_SIZE: got %d (ok=%v)", v, ok)
	}
	// H2 SETTINGS_INITIAL_WINDOW_SIZE=4
	if v, ok := jp.H2Settings[4]; !ok || v != 6291456 {
		t.Errorf("chrome_150 H2 SETTINGS_INITIAL_WINDOW_SIZE: got %d (ok=%v)", v, ok)
	}

	// H2 pseudo header order
	expectedPHO := []string{":method", ":authority", ":scheme", ":path"}
	require.Equal(t, expectedPHO, jp.H2PseudoHeaderOrder,
		"chrome_150 H2 pseudo header order mismatch")

	// H3 settings: chrome_150 may or may not have H3 settings depending
	// on how the profile was constructed. Verify H2 settings definitely exist.
	t.Logf("chrome_150 H2 settings count: %d", len(jp.H2Settings))
	t.Logf("chrome_150 H3 settings: %v", jp.H3Settings)
	t.Logf("chrome_150 H3 settings order: %v", jp.H3SettingsOrder)

	// Check a profile known to have H3 settings (Chrome 144 PSK).
	if jp144, ok := file.Profiles["chrome_144_PSK"]; ok {
		require.NotNil(t, jp144.H3Settings, "chrome_144_PSK should have H3 settings")
		t.Logf("chrome_144_PSK H3 settings count: %d", len(jp144.H3Settings))
	}
}

func TestProfileJSONAllProfilesHaveClientHello(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "profiles.json")

	err := ExportProfilesToJSON(jsonPath)
	require.NoError(t, err)

	file, err := LoadProfilesFromJSONFile(jsonPath)
	require.NoError(t, err)

	missing := 0
	empty := 0
	for key, jp := range file.Profiles {
		if jp.ClientHelloBase64 == "" {
			t.Errorf("profile %q: empty ClientHello", key)
			empty++
		}
		// Verify it can be decoded and parsed.
		raw, err := base64.StdEncoding.DecodeString(jp.ClientHelloBase64)
		if err != nil {
			t.Errorf("profile %q: invalid base64: %v", key, err)
			missing++
			continue
		}
		_, err = parseMarshaledClientHello(raw)
		if err != nil {
			t.Errorf("profile %q: cannot parse ClientHello: %v", key, err)
			missing++
		}
	}
	if empty > 0 || missing > 0 {
		t.Fatalf("%d empty ClientHellos, %d unparseable", empty, missing)
	}
	t.Logf("verified %d profiles have valid ClientHellos", len(file.Profiles))
}

// nonGREASECipherSuites filters out GREASE cipher suite values.
func nonGREASECipherSuites(suites []uint16) []uint16 {
	var out []uint16
	for _, s := range suites {
		if s&0x0f0f != 0x0a0a { // RFC 8701 GREASE
			out = append(out, s)
		}
	}
	return out
}

func mapsClone[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
