package cloak

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wuliao6688/cloak/profiles"
	"golang.org/x/text/encoding/korean"
)

// TestDecodeEUCKR verifies EUC-KR (Korean) response bodies decode to
// UTF-8 correctly (customer issue E7 / upstream #207).
func TestDecodeEUCKR(t *testing.T) {
	// "안녕하세요, 세상!" encoded as EUC-KR
	original := "안녕하세요, 세상!"
	enc := korean.EUCKR.NewEncoder()
	eucBytes, err := enc.Bytes([]byte(original))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=EUC-KR")
		w.Write(eucBytes)
	}))
	defer srv.Close()

	req := ImpersonateRequest(profiles.Chrome_150)
	r, err := req.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	got := r.String()
	if got != original {
		t.Fatalf("EUC-KR 解码失败: got %q want %q", got, original)
	}
	t.Logf("EUC-KR 解码正确: %q", got)
}

// TestDecodeCharsetLookup verifies charset name resolution.
func TestDecodeCharsetLookup(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"euc-kr", true},
		{"EUC-KR", true},
		{"gbk", true},
		{"big5", true},
		{"shift_jis", true},
		{"utf-8", true},
		{"bogus-charset", false},
	}
	for _, c := range cases {
		if c.want {
			if lookupEncoding(c.name) == nil {
				t.Errorf("lookupEncoding(%s) = nil, want encoding", c.name)
			}
		} else {
			if lookupEncoding(c.name) != nil {
				t.Errorf("lookupEncoding(%s) = non-nil, want nil", c.name)
			}
		}
	}
	t.Log("charset 查找正确")
}
