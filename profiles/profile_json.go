// Package profiles provides JSON-based profile serialization.
//
// Profiles can be exported to a JSON file and loaded back, enabling
// (1) external profile editing without recompilation and (2) hot-reload
// when new browser versions are released.
//
// The TLS ClientHello is serialized as raw wire-format bytes (base64),
// preserving byte-level fidelity. HTTP/2 and HTTP/3 parameters are
// stored as JSON-native maps, slices and scalars.
package profiles

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	tls "github.com/wuliao6688/utls"
)

// ─── JSON Schema types ────────────────────────────────────────────────

// ProfilesFile is the top-level structure of a profiles.json file.
type ProfilesFile struct {
	Version          string                `json:"version"`
	DefaultProfile   string                `json:"defaultProfile"`
	RandomExclusions []string              `json:"randomExclusions"`
	Profiles         map[string]JSONProfile `json:"profiles"`
	Metadata         map[string]ClientProfileMetadata `json:"metadata"`
}

// JSONProfile is the JSON representation of a single client profile.
type JSONProfile struct {
	// TLS: raw ClientHello bytes (base64-encoded).  Kept as opaque
	// bytes so we don't need to define every uTLS extension type in JSON.
	ClientHelloBase64 string `json:"clientHelloBase64"`

	// TLS metadata (informational, not used for fingerprinting).
	TLSClient  string `json:"tlsClient,omitempty"`
	TLSVersion string `json:"tlsVersion,omitempty"`

	// HTTP/2
	H2Settings         map[uint16]uint32 `json:"h2Settings,omitempty"`
	H2SettingsOrder    []uint16          `json:"h2SettingsOrder,omitempty"`
	H2PseudoHeaderOrder []string         `json:"h2PseudoHeaderOrder,omitempty"`
	H2ConnectionFlow   uint32            `json:"h2ConnectionFlow,omitempty"`
	H2StreamID         uint32            `json:"h2StreamID,omitempty"`

	// HTTP/3
	H3Settings         map[uint64]uint64 `json:"h3Settings,omitempty"`
	H3SettingsOrder    []uint64          `json:"h3SettingsOrder,omitempty"`
	H3PseudoHeaderOrder []string         `json:"h3PseudoHeaderOrder,omitempty"`
	H3PriorityParam    uint32            `json:"h3PriorityParam,omitempty"`
	H3SendGreaseFrames bool              `json:"h3SendGreaseFrames,omitempty"`
}

// ─── Export ────────────────────────────────────────────────────────────

// ExportProfilesToJSON exports all profiles in the canonical registry to
// the given file path. The ClientHello is marshaled to wire format.
func ExportProfilesToJSON(path string) error {
	file := ProfilesFile{
		Version:          "1",
		DefaultProfile:   defaultProfileKey(),
		RandomExclusions: excludedRandomProfilePrefixes,
		Profiles:         make(map[string]JSONProfile),
		Metadata:         AllProfileMetadata(),
	}

	for key := range canonicalTLSClients {
		profile := canonicalTLSClients[key]
		jp, err := profileToJSON(profile)
		if err != nil {
			return fmt.Errorf("export profile %q: %w", key, err)
		}
		file.Profiles[key] = jp
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func defaultProfileKey() string {
	for k, v := range MappedTLSClients {
		if v.GetClientHelloStr() == DefaultClientProfile.GetClientHelloStr() {
			return k
		}
	}
	return "chrome_150"
}

func profileToJSON(profile ClientProfile) (JSONProfile, error) {
	// Marshal the ClientHello to wire format.
	raw, err := marshalProfileClientHello(profile)
	if err != nil {
		return JSONProfile{}, fmt.Errorf("marshal ClientHello: %w", err)
	}

	jp := JSONProfile{
		ClientHelloBase64:   base64.StdEncoding.EncodeToString(raw),
		TLSClient:           profile.GetClientHelloId().Client,
		TLSVersion:          profile.GetClientHelloId().Version,
		H2Settings:          convertH2Settings(profile.GetSettings()),
		H2SettingsOrder:     convertH2SettingsOrder(profile.GetSettingsOrder()),
		H2PseudoHeaderOrder: profile.GetPseudoHeaderOrder(),
		H2ConnectionFlow:    profile.GetConnectionFlow(),
		H2StreamID:          profile.GetStreamID(),
		H3Settings:          profile.GetHttp3Settings(),
		H3SettingsOrder:     profile.GetHttp3SettingsOrder(),
		H3PseudoHeaderOrder: profile.GetHttp3PseudoHeaderOrder(),
		H3PriorityParam:    profile.GetHttp3PriorityParam(),
		H3SendGreaseFrames:  profile.GetHttp3SendGreaseFrames(),
	}
	return jp, nil
}

// convertH2Settings converts SettingID keys to uint16 for JSON.
func convertH2Settings(settings map[SettingID]uint32) map[uint16]uint32 {
	if len(settings) == 0 {
		return nil
	}
	out := make(map[uint16]uint32, len(settings))
	for k, v := range settings {
		out[uint16(k)] = v
	}
	return out
}

// convertH2SettingsOrder converts []SettingID to []uint16 for JSON.
func convertH2SettingsOrder(order []SettingID) []uint16 {
	if len(order) == 0 {
		return nil
	}
	out := make([]uint16, len(order))
	for i, id := range order {
		out[i] = uint16(id)
	}
	return out
}

// ─── Import / Load ─────────────────────────────────────────────────────

// LoadProfilesFromJSONFile reads a profiles JSON file and registers all
// profiles from it into the canonical registry.
//
// The caller is responsible for ensuring thread safety — call this once
// during initialization before any goroutines access the registry.
func LoadProfilesFromJSONFile(path string) (*ProfilesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profiles file: %w", err)
	}

	var file ProfilesFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("unmarshal profiles JSON: %w", err)
	}

	if file.Version != "1" {
		return nil, fmt.Errorf("unsupported profiles file version: %q", file.Version)
	}

	return &file, nil
}

// MergeJSONProfilesIntoRegistry takes the data from a ProfilesFile and
// merges it into the current canonical registry. Existing profiles with
// the same key are replaced. The resolver's normalized key index and
// random-exclusion lists are rebuilt.
func MergeJSONProfilesIntoRegistry(file *ProfilesFile) error {
	if file == nil {
		return fmt.Errorf("profiles file is nil")
	}

	newProfiles := make(map[string]ClientProfile)

	for key, jp := range file.Profiles {
		profile, err := jsonToProfile(key, jp)
		if err != nil {
			return fmt.Errorf("profile %q: %w", key, err)
		}
		newProfiles[key] = profile
	}

	// Merge into canonical registry — hold write lock for the entire mutation.
	LockRegistry()
	defer UnlockRegistry()

	for key, profile := range newProfiles {
		canonicalTLSClients[key] = profile
		MappedTLSClients[key] = profile
	}

	// Merge metadata.
	for key, meta := range file.Metadata {
		profileMetadata[key] = meta
	}

	// Update exclusion list if present.
	if len(file.RandomExclusions) > 0 {
		excludedRandomProfilePrefixes = file.RandomExclusions
	}

	// Rebuild lookup indexes.
	rebuildProfileIndexes()

	// Update default profile if specified.
	if file.DefaultProfile != "" {
		if p, ok := canonicalTLSClients[file.DefaultProfile]; ok {
			DefaultClientProfile = p
		}
	}

	return nil
}

// RebuildProfileIndexes is exported so that callers who modify
// canonicalTLSClients directly can regenerate the resolver indexes.
// It acquires the write lock automatically.
func RebuildProfileIndexes() {
	LockRegistry()
	defer UnlockRegistry()
	rebuildProfileIndexes()
}

func rebuildProfileIndexes() {
	normalizedProfileKeys = make(map[string]string, len(canonicalTLSClients))
	randomBrowserProfileKeys = make([]string, 0, len(canonicalTLSClients))
	for key := range canonicalTLSClients {
		normalizedProfileKeys[strings.ToLower(key)] = key
		if isRandomBrowserProfileKey(key) {
			randomBrowserProfileKeys = append(randomBrowserProfileKeys, key)
		}
	}
	sort.Strings(randomBrowserProfileKeys)
}

func jsonToProfile(key string, jp JSONProfile) (ClientProfile, error) {
	if jp.ClientHelloBase64 == "" {
		return ClientProfile{}, fmt.Errorf("clientHelloBase64 is empty")
	}

	raw, err := base64.StdEncoding.DecodeString(jp.ClientHelloBase64)
	if err != nil {
		return ClientProfile{}, fmt.Errorf("decode ClientHello base64: %w", err)
	}

	// Parse back using uTLS Fingerprinter (same approach as fingerprint_validity_test.go).
	spec, err := parseMarshaledClientHello(raw)
	if err != nil {
		return ClientProfile{}, fmt.Errorf("parse ClientHello from wire bytes: %w", err)
	}

	clientHelloID := tls.ClientHelloID{
		Client:  jp.TLSClient,
		Version: jp.TLSVersion,
		Seed:    nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return *spec, nil
		},
	}

	// Build HTTP/2 settings.
	settings := make(map[SettingID]uint32, len(jp.H2Settings))
	for k, v := range jp.H2Settings {
		settings[SettingID(k)] = v
	}

	settingsOrder := make([]SettingID, len(jp.H2SettingsOrder))
	for i, id := range jp.H2SettingsOrder {
		settingsOrder[i] = SettingID(id)
	}

	return NewClientProfile(
		clientHelloID,
		settings,
		settingsOrder,
		jp.H2PseudoHeaderOrder,
		jp.H2ConnectionFlow,
		nil, // priorities — not serialized yet (complex type)
		nil, // headerPriority — not serialized yet
		jp.H2StreamID,
		false, // allowHTTP — always false for HTTPS profiles
		jp.H3Settings,
		jp.H3SettingsOrder,
		jp.H3PriorityParam,
		jp.H3PseudoHeaderOrder,
		jp.H3SendGreaseFrames,
	), nil
}
