package common

import (
	"os"
	"strings"
	"testing"
)

func TestExcerptDescribesWhatWasPassed(t *testing.T) {
	cases := []struct {
		name  string
		input any
		want  string
	}{
		{"a short string", "hello world", `; input was "hello world"`},
		{"the empty string", "", "; input was the empty string"},
		{"a boolean", true, "; input was true"},
		{"null", nil, "; input was null"},
		{"a number", 42, "; input was 42"},
		{"an array", []any{1, 2, 3}, "; input was an array of 3"},
		{"an object", map[string]any{"a": 1}, "; input was an object with 1 keys"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Excerpt(tc.input); got != tc.want {
				t.Errorf("Excerpt(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestExcerptStaysOnOneLine keeps a stream of errors scannable. A decoder's
// input is often a whole document, and pasting its newlines into the message
// would break the one-error-one-line reading.
func TestExcerptStaysOnOneLine(t *testing.T) {
	got := Excerpt("first line\nsecond line\nthird line")
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("the excerpt spans lines: %q", got)
	}
	if !strings.Contains(got, "first line second line") {
		t.Errorf("the excerpt lost the content: %q", got)
	}
}

// TestExcerptIsBounded stops a large input becoming the error.
func TestExcerptIsBounded(t *testing.T) {
	got := Excerpt(strings.Repeat("x", 5000))
	if len(got) > 200 {
		t.Errorf("the excerpt is %d bytes long: %q", len(got), got)
	}
	if !strings.Contains(got, "5000 characters in all") {
		t.Errorf("the excerpt does not say how much was left out: %q", got)
	}
}

// TestExcerptDoesNotPrintBytesAsText covers the input a decoder is most often
// handed and least able to describe. Compressed or encrypted bytes render as
// replacement characters, which read as corruption when they are an artefact
// of the display; the length is what a caller holding them actually needs.
func TestExcerptDoesNotPrintBytesAsText(t *testing.T) {
	got := Excerpt(string([]byte{0x78, 0x9c, 0xff, 0xfe, 0x00, 0x01}))
	if strings.ContainsRune(got, '�') {
		t.Errorf("the excerpt printed raw bytes as text: %q", got)
	}
	if !strings.Contains(got, "6 bytes, not valid text") {
		t.Errorf("the excerpt does not describe the bytes: %q", got)
	}
}

// TestExcerptCutsOnRuneBoundaries stops a truncated excerpt ending in half a
// UTF-8 sequence, which would render as the very replacement character the
// rest of this file works to avoid.
func TestExcerptCutsOnRuneBoundaries(t *testing.T) {
	got := Excerpt(strings.Repeat("héllo wörld ", 40))
	if strings.ContainsRune(got, '�') {
		t.Errorf("the cut landed mid-rune: %q", got)
	}
}

// TestInputHintPrefersTheFileAdvice checks the two clauses never both fire.
// When the input turns out to be a path the caller meant as data, quoting it
// again under "input was" would restate what the hint just explained.
func TestInputHintPrefersTheFileAdvice(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/payload.gz"
	if err := os.WriteFile(path, []byte("not really gzip"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	got := InputHint("gzip_decompress", path)
	if !strings.Contains(got, "pass the file flag") {
		t.Fatalf("a path on disk did not get the file-flag advice: %q", got)
	}
	if strings.Contains(got, "input was") {
		t.Errorf("both clauses fired: %q", got)
	}

	// And a value that is plainly data still gets the excerpt.
	if got := InputHint("gzip_decompress", "hello world"); !strings.Contains(got, "input was") {
		t.Errorf("data got no excerpt: %q", got)
	}
}
