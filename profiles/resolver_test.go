package profiles

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomBrowserProfileKeys_ExcludeCustomProfiles(t *testing.T) {
	keys := RandomBrowserProfileKeys()
	assert.NotEmpty(t, keys)

	for _, key := range keys {
		assert.NotContains(t, key, "zalando_")
		assert.NotContains(t, key, "nike_")
		assert.NotContains(t, key, "cloudscraper")
		assert.NotContains(t, key, "mms_")
		assert.NotContains(t, key, "mesh_")
		assert.NotContains(t, key, "confirmed_")
		assert.NotContains(t, strings.ToLower(key), "_psk")

		metadata, ok := GetProfileMetadata(key)
		if ok {
			assert.Empty(t, metadata.KnownGaps)
		}
	}
}

func TestResolveClientProfileWithKey_RandomAndChaos(t *testing.T) {
	randomKey, randomProfile := ResolveClientProfileWithKey(RandomProfileIdentifier)
	assert.NotEmpty(t, randomKey)
	assert.Equal(t, MappedTLSClients[randomKey].GetClientHelloStr(), randomProfile.GetClientHelloStr())

	chaosKey, chaosProfile := ResolveClientProfileWithKey(ChaosProfileIdentifier)
	assert.NotEmpty(t, chaosKey)
	assert.Equal(t, MappedTLSClients[chaosKey].GetClientHelloStr(), chaosProfile.GetClientHelloStr())
}

func TestResolveClientProfileWithKey_UnknownFallsBackToDefault(t *testing.T) {
	key, profile := ResolveClientProfileWithKey("unknown-profile")
	assert.Empty(t, key)
	assert.Equal(t, DefaultClientProfile.GetClientHelloStr(), profile.GetClientHelloStr())
}

func TestResolveClientProfileWithKeyStrict_IsCaseInsensitive(t *testing.T) {
	key, profile, err := ResolveClientProfileWithKeyStrict("  CHROME_146_psk  ")
	assert.NoError(t, err)
	assert.Equal(t, "chrome_146_PSK", key)
	assert.Equal(t, MappedTLSClients[key].GetClientHelloStr(), profile.GetClientHelloStr())
}

func TestResolveClientProfileWithKeyStrict_RejectsUnknown(t *testing.T) {
	_, _, err := ResolveClientProfileWithKeyStrict("unknown-profile")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnknownClientProfile))
}
