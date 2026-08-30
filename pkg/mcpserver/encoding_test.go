package mcpserver

import (
	"strings"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf"
)

func warnings(t *testing.T, query string) []encodingWarning {
	t.Helper()
	udf.DefaultRegistry()
	parsed, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	return checkEncodings(parsed)
}

// TestDoubleEncodingIsReported pins the failure this check exists for. The
// query runs, returns a plausible string, and is wrong: the base64 covers the
// hex text rather than the compressed bytes, so the result is twice the size it
// should be. Nothing errors, so without this nothing is said.
func TestDoubleEncodingIsReported(t *testing.T) {
	got := warnings(t, `"x" | zlib_compress | base64_encode`)
	if len(got) != 1 {
		t.Fatalf("got %d warnings, want 1: %+v", len(got), got)
	}
	for _, want := range []string{"zlib_compress", "base64_encode", "hex_decode"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("warning does not mention %s: %s", want, got[0].Message)
		}
	}
}

// TestMismatchedDecoderIsReported covers the case that is not merely wasteful
// but meaningless: an md5 digest is hex, and hex made of the characters 0-9a-f
// is also valid base64, so base64_decode accepts it and returns bytes that mean
// nothing. There is no error to notice.
func TestMismatchedDecoderIsReported(t *testing.T) {
	got := warnings(t, `"x" | md5 | base64_decode`)
	if len(got) != 1 {
		t.Fatalf("got %d warnings, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "md5 returns a hex string") {
		t.Errorf("warning does not say what md5 returns: %s", got[0].Message)
	}
}

// TestCorrectPipelinesAreNotWarnedAbout is the half of the check that decides
// whether it is usable. A warning on a correct query is worse than no warning
// at all: it sends a caller to rewrite a stage that was right, and after one of
// those the rest stop being read.
func TestCorrectPipelinesAreNotWarnedAbout(t *testing.T) {
	for _, query := range []string{
		// The declared round trips.
		`"x" | base64_encode | base64_decode`,
		`"x" | hex_encode | hex_decode`,
		`"x" | base32_encode | base32_decode`,
		`"x" | base85_encode | base85_decode`,
		`"x" | binary_encode | binary_decode`,
		`"x" | zlib_compress | zlib_decompress`,
		`"x" | gzip_compress | gzip_decompress`,
		`"x" | deflate_compress | deflate_decompress`,
		// The wire format written correctly.
		`"x" | zlib_compress | hex_decode | base64_encode`,
		`"x" | zlib_compress | hex_decode | base64_encode | base64_decode | zlib_decompress`,
		// A left side that is not a cmdlet at all.
		`"x" | base64_encode`,
		`.Content | base64_decode`,
		// A caller who sliced the hex knows it is hex.
		`"x" | sha256 | .[0:8]`,
		// A consumer that declared nothing.
		`"x" | sha256 | ascii_upcase`,
		`"x" | md5 | length`,
	} {
		if got := warnings(t, query); len(got) != 0 {
			t.Errorf("%s warned when it should not: %+v", query, got)
		}
	}
}

// TestWarningsReachInsideExpressions checks that a mismatch buried in an object
// value or a function argument is found, since that is where it is hardest to
// see by eye and most likely to survive into a result.
func TestWarningsReachInsideExpressions(t *testing.T) {
	for _, query := range []string{
		`{wire: ("x" | zlib_compress | base64_encode)}`,
		`[("x" | zlib_compress | base64_encode)]`,
		`("x" | zlib_compress | base64_encode) | length`,
		`def f: zlib_compress | base64_encode; "x" | f`,
	} {
		if got := warnings(t, query); len(got) != 1 {
			t.Errorf("%s produced %d warnings, want 1", query, len(got))
		}
	}
}

// TestEveryReconcilerIsACmdlet checks that a warning never names a cmdlet that
// is not there. Advice to pipe through something that does not exist is worse
// than no advice: it costs the caller a call to find out.
func TestEveryReconcilerIsACmdlet(t *testing.T) {
	udf.DefaultRegistry()
	known := map[string]bool{}
	for _, c := range catalogue() {
		known[c.Name] = true
	}
	for _, query := range []string{
		`"x" | zlib_compress | base64_encode`,
		`"x" | md5 | base32_decode`,
		`"x" | aes_encrypt("k") | hex_decode`,
		`"x" | base64url_encode | hex_decode`,
		`"x" | binary_encode | base64_decode`,
	} {
		for _, w := range warnings(t, query) {
			for _, word := range strings.Fields(strings.ReplaceAll(w.Message, ";", " ")) {
				if strings.HasSuffix(word, "_decode") && !known[word] {
					t.Errorf("%s advises %s, which is not a cmdlet", query, word)
				}
			}
		}
	}
}
