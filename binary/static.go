package binary

import "regexp"

// staticPatterns matches version-banner strings that libraries commonly embed
// in read-only data. A hit means the library was probably compiled into the
// object rather than dynamically linked, since dynamic dependencies would
// show up in Needed instead. Matches are heuristic; callers should treat them
// as low confidence.
//
// Each pattern's first submatch, if present, is the version. When several
// patterns share a library name only the first match is reported, so put the
// version-capturing pattern first.
var staticPatterns = []struct {
	library string
	re      *regexp.Regexp
}{
	{"zlib", regexp.MustCompile(`(?:deflate|inflate) (1\.[0-9]+(?:\.[0-9]+)?) Copyright`)},
	{"openssl", regexp.MustCompile(`OpenSSL ([0-9]+\.[0-9]+\.[0-9]+[a-z]?)`)},
	{"boringssl", regexp.MustCompile(`BoringSSL`)},
	// SQLITE_SOURCE_ID: "YYYY-MM-DD HH:MM:SS <sha>" is a distinctive
	// fingerprint; the version string itself is a bare "3.x.y" that is too
	// generic to match safely.
	{"sqlite", regexp.MustCompile(`20[0-9]{2}-[01][0-9]-[0-3][0-9] [0-2][0-9]:[0-5][0-9]:[0-5][0-9] [0-9a-f]{40,}`)},
	{"libcurl", regexp.MustCompile(`libcurl/([0-9]+\.[0-9]+\.[0-9]+)`)},
	{"pcre2", regexp.MustCompile(`PCRE2 (10\.[0-9]+)`)},
	{"libpng", regexp.MustCompile(`libpng version ([0-9]+\.[0-9]+\.[0-9]+)`)},
	{"libjpeg-turbo", regexp.MustCompile(`libjpeg-turbo version ([0-9]+\.[0-9]+\.[0-9]+)`)},
	{"libwebp", regexp.MustCompile(`libwebp ([0-9]+\.[0-9]+\.[0-9]+)`)},
	{"libxml2", regexp.MustCompile(`libxml2 (2\.[0-9]+\.[0-9]+)`)},
	{"libxml2", regexp.MustCompile(`xmlParseDoc : `)},
	{"brotli", regexp.MustCompile(`[Bb]rotli/([0-9]+\.[0-9]+\.[0-9]+)`)},
	{"lz4", regexp.MustCompile(`LZ4 v([0-9]+\.[0-9]+\.[0-9]+)`)},
	{"zstd", regexp.MustCompile(`Zstandard v([0-9]+\.[0-9]+\.[0-9]+)`)},
	{"libffi", regexp.MustCompile(`libffi ([0-9]+\.[0-9]+(?:\.[0-9]+)?)`)},
	{"mbedtls", regexp.MustCompile(`[Mm]bed ?TLS ([0-9]+\.[0-9]+\.[0-9]+)`)},
	{"expat", regexp.MustCompile(`expat_([0-9]+\.[0-9]+\.[0-9]+)`)},
	{"cares", regexp.MustCompile(`c-ares ([0-9]+\.[0-9]+\.[0-9]+)`)},
}

const maxMatchLen = 128

func scanStatic(rodata []byte) []Hint {
	var hints []Hint
	seen := make(map[string]bool)
	for _, p := range staticPatterns {
		m := p.re.FindSubmatchIndex(rodata)
		if m == nil {
			continue
		}
		match := clip(rodata[m[0]:m[1]])
		var version string
		if len(m) >= 4 && m[2] >= 0 {
			version = string(rodata[m[2]:m[3]])
		}
		if seen[p.library] {
			continue
		}
		seen[p.library] = true
		hints = append(hints, Hint{
			Library: p.library,
			Version: version,
			Match:   match,
		})
	}
	return hints
}

func clip(b []byte) string {
	if len(b) > maxMatchLen {
		b = b[:maxMatchLen]
	}
	return string(b)
}
