package cloak

import (
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/ianaindex"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
)

// decodeCharset converts raw body bytes from the response's charset
// (from Content-Type header) into UTF-8. Unknown/unsupported charsets
// return the raw bytes unchanged.
//
// Supported: EUC-KR (Korean), GBK/GB2312 (simplified Chinese),
// Big5 (traditional Chinese), Shift_JIS/EUC-JP/ISO-2022-JP (Japanese),
// ISO-8859-* (Latin), UTF-16/32, windows-125x.
func decodeCharset(body []byte, contentType string) []byte {
	charset := charsetFromContentType(contentType)
	if charset == "" || charset == "utf-8" || charset == "utf8" || charset == "us-ascii" {
		return body
	}
	enc := lookupEncoding(charset)
	if enc == nil {
		return body
	}
	decoded, err := enc.NewDecoder().Bytes(body)
	if err != nil {
		return body // fall back to raw
	}
	return decoded
}

// charsetFromContentType extracts the charset=... parameter from a
// Content-Type header, e.g. "text/html; charset=EUC-KR" → "euc-kr".
func charsetFromContentType(ct string) string {
	for _, part := range strings.Split(ct, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "charset=") {
			return strings.Trim(strings.ToLower(strings.TrimPrefix(part, "charset=")), "\"'")
		}
	}
	return ""
}

// lookupEncoding maps a charset name to a Go text encoding.
func lookupEncoding(name string) encoding.Encoding {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	// Korean
	case "euc-kr", "euckr", "ks_c_5601-1987", "cp949", "korean":
		return korean.EUCKR
	// Simplified Chinese
	case "gbk", "gb2312", "gb18030", "cp936":
		return simplifiedchinese.GBK
	// Traditional Chinese
	case "big5", "big-5", "cp950":
		return traditionalchinese.Big5
	// Japanese
	case "shift_jis", "shift-jis", "sjis", "cp932":
		return japanese.ShiftJIS
	case "euc-jp", "eucjp":
		return japanese.EUCJP
	case "iso-2022-jp", "iso2022-jp":
		return japanese.ISO2022JP
	// Latin / Western
	case "iso-8859-1", "latin1", "latin-1":
		return charmap.ISO8859_1
	case "iso-8859-2":
		return charmap.ISO8859_2
	case "iso-8859-5":
		return charmap.ISO8859_5
	case "iso-8859-15":
		return charmap.ISO8859_15
	case "windows-1250":
		return charmap.Windows1250
	case "windows-1251":
		return charmap.Windows1251
	case "windows-1252", "cp1252":
		return charmap.Windows1252
	case "windows-1256":
		return charmap.Windows1256
	// Unicode
	case "utf-16", "utf16", "utf-16le", "utf16le":
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	case "utf-16be", "utf16be":
		return unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
	case "utf-32", "utf32":
		return utf32.UTF32(utf32.LittleEndian, utf32.IgnoreBOM)
	}
	// Fallback: try IANA index (handles variants like "ISO_8859-1:1987").
	if enc, err := ianaindex.IANA.Encoding(name); err == nil && enc != nil {
		return enc
	}
	return nil
}
