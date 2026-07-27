package profiles

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetProfileMetadata_Chrome150(t *testing.T) {
	metadata, ok := GetProfileMetadata("chrome_150")
	assert.True(t, ok)
	assert.Equal(t, "chrome_146", metadata.TLSBase)
	assert.NotEmpty(t, metadata.KnownGaps)
	assert.Contains(t, metadata.VerifiedAgainst, "tls.peet.ws")
}

func TestGetProfileMetadata_Unknown(t *testing.T) {
	_, ok := GetProfileMetadata("unknown_profile")
	assert.False(t, ok)
}

func TestAllBrowserProfilesHaveMetadata(t *testing.T) {
	browserKeys := RandomBrowserProfileKeys()
	allMeta := AllProfileMetadata()

	for _, key := range browserKeys {
		_, ok := allMeta[key]
		assert.True(t, ok, "browser profile %q is missing metadata entry", key)
	}
}

func TestProfilesWithKnownGaps(t *testing.T) {
	gaps := ProfilesWithKnownGaps()
	assert.NotEmpty(t, gaps)
	assert.Contains(t, gaps, "chrome_150")
	assert.Contains(t, gaps, "chrome_150_PSK")
}

func TestProfileMetadataReturnsDefensiveCopies(t *testing.T) {
	metadata, ok := GetProfileMetadata("chrome_150")
	assert.True(t, ok)
	metadata.KnownGaps[0] = "modified"
	metadata.VerifiedAgainst[0] = "modified"

	again, ok := GetProfileMetadata("chrome_150")
	assert.True(t, ok)
	assert.NotEqual(t, "modified", again.KnownGaps[0])
	assert.NotEqual(t, "modified", again.VerifiedAgainst[0])

	all := AllProfileMetadata()
	delete(all, "chrome_150")
	_, ok = GetProfileMetadata("chrome_150")
	assert.True(t, ok)
}
