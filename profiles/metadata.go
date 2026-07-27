package profiles

import (
	"slices"
	"strings"
)

type ClientProfileMetadata struct {
	Key             string
	TLSBase         string
	KnownGaps       []string
	VerifiedAgainst []string
}

var profileMetadata = map[string]ClientProfileMetadata{
	// ── Chrome ──────────────────────────────────────────────────────────
	"chrome_103":        {Key: "chrome_103", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_104":        {Key: "chrome_104", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_105":        {Key: "chrome_105", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_106":        {Key: "chrome_106", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_107":        {Key: "chrome_107", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_108":        {Key: "chrome_108", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_109":        {Key: "chrome_109", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_110":        {Key: "chrome_110", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_111":        {Key: "chrome_111", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_112":        {Key: "chrome_112", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_116_PSK":    {Key: "chrome_116_PSK", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_116_PSK_PQ": {Key: "chrome_116_PSK_PQ", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_117":        {Key: "chrome_117", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_120":        {Key: "chrome_120", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_124":        {Key: "chrome_124", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_130_PSK":    {Key: "chrome_130_PSK", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_131":        {Key: "chrome_131", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_131_PSK":    {Key: "chrome_131_PSK", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_133":        {Key: "chrome_133", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_133_PSK":    {Key: "chrome_133_PSK", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_144":        {Key: "chrome_144", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_144_PSK":    {Key: "chrome_144_PSK", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_146":        {Key: "chrome_146", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_146_PSK":    {Key: "chrome_146_PSK", VerifiedAgainst: []string{"tls.peet.ws"}},
	"chrome_150": {
		Key:             "chrome_150",
		TLSBase:         "chrome_146",
		KnownGaps:       []string{"ml-dsa signature algorithms pending upstream utls support"},
		VerifiedAgainst: []string{"tls.peet.ws"},
	},
	"chrome_150_PSK": {
		Key:             "chrome_150_PSK",
		TLSBase:         "chrome_146_PSK",
		KnownGaps:       []string{"ml-dsa signature algorithms pending upstream utls support"},
		VerifiedAgainst: []string{"tls.peet.ws"},
	},

	// ── Brave ───────────────────────────────────────────────────────────
	"brave_146":     {Key: "brave_146", VerifiedAgainst: []string{"tls.peet.ws"}},
	"brave_146_PSK": {Key: "brave_146_PSK", VerifiedAgainst: []string{"tls.peet.ws"}},

	// ── Firefox ─────────────────────────────────────────────────────────
	"firefox_102":     {Key: "firefox_102", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_104":     {Key: "firefox_104", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_105":     {Key: "firefox_105", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_106":     {Key: "firefox_106", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_108":     {Key: "firefox_108", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_110":     {Key: "firefox_110", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_117":     {Key: "firefox_117", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_120":     {Key: "firefox_120", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_123":     {Key: "firefox_123", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_132":     {Key: "firefox_132", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_133":     {Key: "firefox_133", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_135":     {Key: "firefox_135", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_146_PSK": {Key: "firefox_146_PSK", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_147":     {Key: "firefox_147", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_147_PSK": {Key: "firefox_147_PSK", VerifiedAgainst: []string{"tls.peet.ws"}},
	"firefox_148":     {Key: "firefox_148", VerifiedAgainst: []string{"tls.peet.ws"}},

	// ── Safari ──────────────────────────────────────────────────────────
	"safari_15_6_1":    {Key: "safari_15_6_1", VerifiedAgainst: []string{"tls.peet.ws"}},
	"safari_16_0":      {Key: "safari_16_0", VerifiedAgainst: []string{"tls.peet.ws"}},
	"safari_ipad_15_6": {Key: "safari_ipad_15_6", VerifiedAgainst: []string{"tls.peet.ws"}},
	"safari_ios_15_5":  {Key: "safari_ios_15_5", VerifiedAgainst: []string{"tls.peet.ws"}},
	"safari_ios_15_6":  {Key: "safari_ios_15_6", VerifiedAgainst: []string{"tls.peet.ws"}},
	"safari_ios_16_0":  {Key: "safari_ios_16_0", VerifiedAgainst: []string{"tls.peet.ws"}},
	"safari_ios_17_0":  {Key: "safari_ios_17_0", VerifiedAgainst: []string{"tls.peet.ws"}},
	"safari_ios_18_0":  {Key: "safari_ios_18_0", VerifiedAgainst: []string{"tls.peet.ws"}},
	"safari_ios_18_5":  {Key: "safari_ios_18_5", VerifiedAgainst: []string{"tls.peet.ws"}},
	"safari_ios_26_0":  {Key: "safari_ios_26_0", VerifiedAgainst: []string{"tls.peet.ws"}},

	// ── Opera ───────────────────────────────────────────────────────────
	"opera_89": {Key: "opera_89", VerifiedAgainst: []string{"tls.peet.ws"}},
	"opera_90": {Key: "opera_90", VerifiedAgainst: []string{"tls.peet.ws"}},
	"opera_91": {Key: "opera_91", VerifiedAgainst: []string{"tls.peet.ws"}},

	// ── OkHttp ──────────────────────────────────────────────────────────
	"okhttp4_android_7":  {Key: "okhttp4_android_7", VerifiedAgainst: []string{"tls.peet.ws"}},
	"okhttp4_android_8":  {Key: "okhttp4_android_8", VerifiedAgainst: []string{"tls.peet.ws"}},
	"okhttp4_android_9":  {Key: "okhttp4_android_9", VerifiedAgainst: []string{"tls.peet.ws"}},
	"okhttp4_android_10": {Key: "okhttp4_android_10", VerifiedAgainst: []string{"tls.peet.ws"}},
	"okhttp4_android_11": {Key: "okhttp4_android_11", VerifiedAgainst: []string{"tls.peet.ws"}},
	"okhttp4_android_12": {Key: "okhttp4_android_12", VerifiedAgainst: []string{"tls.peet.ws"}},
	"okhttp4_android_13": {Key: "okhttp4_android_13", VerifiedAgainst: []string{"tls.peet.ws"}},
}

var normalizedProfileMetadataKeys = func() map[string]string {
	keys := make(map[string]string, len(profileMetadata))
	for key := range profileMetadata {
		keys[strings.ToLower(key)] = key
	}
	return keys
}()

func GetProfileMetadata(key string) (ClientProfileMetadata, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	metadata, ok := profileMetadata[key]
	if !ok {
		normalizedKey := strings.TrimSpace(strings.ToLower(key))
		if registeredKey, found := normalizedProfileMetadataKeys[normalizedKey]; found {
			metadata = profileMetadata[registeredKey]
			ok = true
		}
	}
	return cloneClientProfileMetadata(metadata), ok
}

// AllProfileMetadata returns a defensive copy of the metadata registry.
func AllProfileMetadata() map[string]ClientProfileMetadata {
	registryMu.RLock()
	defer registryMu.RUnlock()
	metadata := make(map[string]ClientProfileMetadata, len(profileMetadata))
	for key, value := range profileMetadata {
		metadata[key] = cloneClientProfileMetadata(value)
	}
	return metadata
}

// ProfilesWithKnownGaps returns keys that have non-empty KnownGaps.
func ProfilesWithKnownGaps() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	var keys []string
	for k, m := range profileMetadata {
		if len(m.KnownGaps) > 0 {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

func cloneClientProfileMetadata(metadata ClientProfileMetadata) ClientProfileMetadata {
	metadata.KnownGaps = slices.Clone(metadata.KnownGaps)
	metadata.VerifiedAgainst = slices.Clone(metadata.VerifiedAgainst)
	return metadata
}
