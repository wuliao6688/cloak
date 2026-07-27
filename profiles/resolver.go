package profiles

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
)

const (
	RandomProfileIdentifier = "random"
	ChaosProfileIdentifier  = "chaos"
)

var ErrUnknownClientProfile = errors.New("unknown client profile")

var (
	normalizedProfileKeys    map[string]string
	randomBrowserProfileKeys []string
)

var excludedRandomProfilePrefixes = []string{
	"zalando_",
	"nike_",
	"cloudscraper",
	"mms_",
	"mesh_",
	"confirmed_",
}

func init() {
	normalizedProfileKeys = make(map[string]string, len(canonicalTLSClients))
	randomBrowserProfileKeys = make([]string, 0, len(canonicalTLSClients))
	for key := range canonicalTLSClients {
		normalizedProfileKeys[strings.ToLower(key)] = key
		if isRandomBrowserProfileKey(key) {
			randomBrowserProfileKeys = append(randomBrowserProfileKeys, key)
		}
	}
	slices.Sort(randomBrowserProfileKeys)
}

// ResolveClientProfile maps an identifier to a profile.
// Unknown identifiers fallback to DefaultClientProfile.
// "random" and "chaos" select a random real browser profile.
func ResolveClientProfile(identifier string) ClientProfile {
	_, profile := ResolveClientProfileWithKey(identifier)
	return profile
}

// ResolveClientProfileStrict maps an identifier to a profile and returns an
// error instead of silently falling back when the identifier is unknown.
func ResolveClientProfileStrict(identifier string) (ClientProfile, error) {
	_, profile, err := ResolveClientProfileWithKeyStrict(identifier)
	return profile, err
}

// ResolveClientProfileWithKey resolves identifier and returns the final key and profile.
func ResolveClientProfileWithKey(identifier string) (string, ClientProfile) {
	key, profile, err := ResolveClientProfileWithKeyStrict(identifier)
	if err != nil {
		return "", DefaultClientProfile
	}
	return key, profile
}

// ResolveClientProfileWithKeyStrict resolves identifier and returns the
// canonical registry key. Identifiers are matched case-insensitively.
func ResolveClientProfileWithKeyStrict(identifier string) (string, ClientProfile, error) {
	normalizedIdentifier := strings.TrimSpace(strings.ToLower(identifier))
	if normalizedIdentifier == "" {
		return "", ClientProfile{}, fmt.Errorf("%w: identifier is empty", ErrUnknownClientProfile)
	}

	if normalizedIdentifier == RandomProfileIdentifier || normalizedIdentifier == ChaosProfileIdentifier {
		key := pickRandomBrowserProfileKey()
		if key == "" {
			return "", ClientProfile{}, errors.New("no eligible browser profiles are registered")
		}
		return key, canonicalTLSClients[key], nil
	}

	profile, ok := canonicalTLSClients[identifier]
	if ok {
		return identifier, profile, nil
	}

	if key, ok := normalizedProfileKeys[normalizedIdentifier]; ok {
		return key, canonicalTLSClients[key], nil
	}

	return "", ClientProfile{}, fmt.Errorf("%w: %q", ErrUnknownClientProfile, identifier)
}

// RandomBrowserProfileKeys returns all keys eligible for random/chaos selection.
func RandomBrowserProfileKeys() []string {
	return slices.Clone(randomBrowserProfileKeys)
}

func isRandomBrowserProfileKey(key string) bool {
	normalizedKey := strings.ToLower(key)

	for _, excludedPrefix := range excludedRandomProfilePrefixes {
		if strings.HasPrefix(normalizedKey, excludedPrefix) {
			return false
		}
	}

	// PSK profiles describe a resumed connection. A randomly selected browser
	// session must start from its base profile and reach PSK naturally through
	// the session cache.
	if strings.HasSuffix(normalizedKey, "_psk") || strings.Contains(normalizedKey, "_psk_") {
		return false
	}

	if metadata, ok := GetProfileMetadata(key); ok && len(metadata.KnownGaps) > 0 {
		return false
	}

	return true
}

func pickRandomBrowserProfileKey() string {
	keys := RandomBrowserProfileKeys()
	if len(keys) == 0 {
		return ""
	}

	randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(keys))))
	if err != nil {
		return keys[0]
	}

	return keys[randomIndex.Int64()]
}
