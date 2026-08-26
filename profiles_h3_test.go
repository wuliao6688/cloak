package cloak

import (
	"strings"
	"testing"

	"github.com/wuliao6688/cloak/profiles"
)

// TestAllBrowserProfilesH3DataValid verifies every browser profile with
// H3 data builds a valid http3.Transport (no nil maps, sane values).
// Profiles without H3 data (Safari, custom) are skipped by design —
// see docs/profile-audit.md §3.4.
func TestAllBrowserProfilesH3DataValid(t *testing.T) {
	checked := 0
	for name, profile := range profiles.AllClientProfiles() {
		// Only browser profiles should carry H3 data (audit doc §3.3).
		if isCustomProfile(name) {
			if profile.GetHttp3Settings() != nil {
				t.Errorf("%s: custom profile should not have H3 settings (audit §3.4)", name)
			}
			continue
		}

		// Safari keeps no H3 data by design (curl_cffi h3_fingerprints=False).
		if strings.HasPrefix(name, "safari") {
			if profile.GetHttp3Settings() != nil {
				t.Errorf("%s: Safari should not have H3 settings (audit §3.4)", name)
			}
			continue
		}

		// Browser profiles (Chrome/Firefox/Opera/Brave) must have H3.
		if profile.GetHttp3Settings() == nil {
			t.Errorf("%s: browser profile missing H3 settings (audit §3.3)", name)
			continue
		}
		if len(profile.GetHttp3SettingsOrder()) == 0 {
			t.Errorf("%s: H3 settings order empty", name)
		}
		if len(profile.GetHttp3PseudoHeaderOrder()) == 0 {
			t.Errorf("%s: H3 pseudo header order empty", name)
		}

		// Build the actual transport to prove the data is usable.
		tr := NewH3Transport(profile)
		rt := tr.buildHTTP3("localhost:443")
		if rt == nil {
			t.Errorf("%s: buildHTTP3 returned nil", name)
		}
		tr.CloseIdleConnections()
		checked++
	}
	t.Logf("验证 %d 个浏览器画像的 H3 数据", checked)
}

func isCustomProfile(name string) bool {
	// Exact registry keys for non-browser profiles (profiles/profiles.go).
	customs := map[string]bool{
		"zalando_android_mobile": true, "zalando_ios_mobile": true,
		"nike_ios_mobile": true, "nike_android_mobile": true,
		"cloudscraper": true,
		"mms_ios": true, "mms_ios_1": true, "mms_ios_2": true, "mms_ios_3": true,
		"mesh_ios": true, "mesh_ios_1": true, "mesh_ios_2": true,
		"mesh_android": true, "mesh_android_1": true, "mesh_android_2": true,
		"confirmed_ios": true, "confirmed_android": true,
		"okhttp4_android_7": true, "okhttp4_android_8": true, "okhttp4_android_9": true,
		"okhttp4_android_10": true, "okhttp4_android_11": true, "okhttp4_android_12": true,
		"okhttp4_android_13": true,
	}
	return customs[name]
}
