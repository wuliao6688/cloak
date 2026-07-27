// Package profiles provides integer-based profile identifiers for CFFI
// callers. Each constant maps to a pre-verified TLS fingerprint profile.
// These IDs are stable and safe for use in C shared libraries and FFI.
package profiles

// ProfileID is a C-compatible integer identifier for a TLS fingerprint profile.
type ProfileID int

const (
	ProfileChrome150       ProfileID = 1
	ProfileChrome146       ProfileID = 2
	ProfileChrome131       ProfileID = 3
	ProfileChrome120       ProfileID = 4
	ProfileChrome117       ProfileID = 5
	ProfileChrome116       ProfileID = 6
	ProfileFirefox148      ProfileID = 10
	ProfileFirefox147      ProfileID = 11
	ProfileFirefox132      ProfileID = 12
	ProfileFirefox120      ProfileID = 13
	ProfileSafariIOS18_5   ProfileID = 20
	ProfileSafariIOS17_0   ProfileID = 21
	ProfileSafariIOS16_0   ProfileID = 22
	ProfileSafari15_6_1    ProfileID = 23
	ProfileSafariIPad15_6  ProfileID = 24
	ProfileOpera91         ProfileID = 30
	ProfileBrave146        ProfileID = 31
	ProfileOkHttp4Android13 ProfileID = 40
	ProfileEdge120         ProfileID = 50
)

// profileIDMap maps integer ProfileID to the string key used by ResolveClientProfileStrict.
var profileIDMap = map[ProfileID]string{
	ProfileChrome150:       "chrome_150",
	ProfileChrome146:       "chrome_146",
	ProfileChrome131:       "chrome_131",
	ProfileChrome120:       "chrome_120",
	ProfileChrome117:       "chrome_117",
	ProfileChrome116:       "chrome_116",
	ProfileFirefox148:      "firefox_148",
	ProfileFirefox147:      "firefox_147",
	ProfileFirefox132:      "firefox_132",
	ProfileFirefox120:      "firefox_120",
	ProfileSafariIOS18_5:   "safari_ios_18_5",
	ProfileSafariIOS17_0:   "safari_ios_17_0",
	ProfileSafariIOS16_0:   "safari_16_0",
	ProfileSafari15_6_1:    "safari_15_6_1",
	ProfileSafariIPad15_6:  "safari_ipad_15_6",
	ProfileOpera91:         "opera_91",
	ProfileBrave146:        "brave_146",
	ProfileOkHttp4Android13: "okhttp4_android_13",
	ProfileEdge120:         "edge_120",
}

// ResolveProfileID returns the ClientProfile for a ProfileID.
// Returns error if the ID is unknown.
func ResolveProfileID(id ProfileID) (ClientProfile, error) {
	key, ok := profileIDMap[id]
	if !ok {
		return ClientProfile{}, ErrUnknownClientProfile
	}
	return ResolveClientProfileStrict(key)
}

// String returns the string key for this ProfileID (e.g. "chrome_150").
// Returns "" if the ID is unknown.
func (id ProfileID) String() string {
	return profileIDMap[id]
}

// ─── Rotation Groups ──────────────────────────────────────

// RotateGroup identifies a family of profiles for automatic rotation.
type RotateGroup int

const (
	RotateGroupChrome  RotateGroup = 1  // Chrome 116-150
	RotateGroupFirefox RotateGroup = 2  // Firefox 120-148
	RotateGroupSafari  RotateGroup = 3  // Safari iOS 15-18
	RotateGroupMobile  RotateGroup = 4  // OkHttp, mobile clients
	RotateGroupAll     RotateGroup = 5  // All verified profiles
)

// rotatePools maps each RotateGroup to a pool of profile IDs for rotation.
var rotatePools = map[RotateGroup][]ProfileID{
	RotateGroupChrome: {
		ProfileChrome150, ProfileChrome146, ProfileChrome131,
		ProfileChrome120, ProfileChrome117, ProfileChrome116,
	},
	RotateGroupFirefox: {
		ProfileFirefox148, ProfileFirefox147, ProfileFirefox132, ProfileFirefox120,
	},
	RotateGroupSafari: {
		ProfileSafariIOS18_5, ProfileSafariIOS17_0, ProfileSafariIOS16_0,
		ProfileSafari15_6_1, ProfileSafariIPad15_6,
	},
	RotateGroupMobile: {
		ProfileOkHttp4Android13,
	},
	RotateGroupAll: {
		ProfileChrome150, ProfileChrome146, ProfileChrome131, ProfileChrome120,
		ProfileFirefox148, ProfileFirefox147, ProfileFirefox132,
		ProfileSafariIOS18_5, ProfileSafariIOS17_0,
		ProfileOpera91, ProfileBrave146, ProfileOkHttp4Android13, ProfileEdge120,
	},
}

// NextRotateProfile returns the next profile in the rotation group.
// idx should be a request counter; it cycles through the pool.
func NextRotateProfile(group RotateGroup, idx int) (ProfileID, error) {
	pool, ok := rotatePools[group]
	if !ok {
		return 0, ErrUnknownClientProfile
	}
	return pool[idx%len(pool)], nil
}
