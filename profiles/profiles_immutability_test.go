package profiles

import (
	"testing"

	tls "github.com/wuliao6688/utls"
	"github.com/stretchr/testify/assert"
)

func TestClientProfileGettersReturnDefensiveCopies(t *testing.T) {
	profile := Chrome_150

	originalSettings := profile.GetSettings()
	settings := profile.GetSettings()
	for key := range settings {
		settings[key]++
		break
	}
	assert.Equal(t, originalSettings, profile.GetSettings())

	originalSettingsOrder := profile.GetSettingsOrder()
	settingsOrder := profile.GetSettingsOrder()
	if len(settingsOrder) > 0 {
		settingsOrder[0] = 0
	}
	assert.Equal(t, originalSettingsOrder, profile.GetSettingsOrder())

	originalPseudoHeaderOrder := profile.GetPseudoHeaderOrder()
	pseudoHeaderOrder := profile.GetPseudoHeaderOrder()
	if len(pseudoHeaderOrder) > 0 {
		pseudoHeaderOrder[0] = "modified"
	}
	assert.Equal(t, originalPseudoHeaderOrder, profile.GetPseudoHeaderOrder())

	originalHTTP3Settings := profile.GetHttp3Settings()
	http3Settings := profile.GetHttp3Settings()
	for key := range http3Settings {
		http3Settings[key]++
		break
	}
	assert.Equal(t, originalHTTP3Settings, profile.GetHttp3Settings())
}

func TestProfileResolverDoesNotUseExportedMutableRegistry(t *testing.T) {
	original := MappedTLSClients["chrome_146"]
	defer func() { MappedTLSClients["chrome_146"] = original }()
	MappedTLSClients["chrome_146"] = Chrome_103

	key, resolved, err := ResolveClientProfileWithKeyStrict("chrome_146")
	assert.NoError(t, err)
	assert.Equal(t, "chrome_146", key)
	assert.Equal(t, original.GetClientHelloStr(), resolved.GetClientHelloStr())
}

func TestClientHelloIDPointerFieldsAreDefensivelyCopied(t *testing.T) {
	seed := tls.PRNGSeed{}
	weights := tls.Weights{}
	id := tls.ClientHelloID{Seed: &seed, Weights: &weights}
	profile := NewClientProfile(id, nil, nil, nil, 0, nil, nil, 0, false, nil, nil, 0, nil, false)

	first := profile.GetClientHelloId()
	second := profile.GetClientHelloId()
	assert.NotSame(t, id.Seed, first.Seed)
	assert.NotSame(t, id.Weights, first.Weights)
	assert.NotSame(t, first.Seed, second.Seed)
	assert.NotSame(t, first.Weights, second.Weights)
}
