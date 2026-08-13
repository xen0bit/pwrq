package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileFlagHint_ExistingFileGetsHint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.gz")
	if err := os.WriteFile(path, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	hint := FileFlagHint("gzip_decompress", path)
	if hint == "" {
		t.Fatal("expected a hint for a path naming an existing file")
	}
	if !strings.Contains(hint, `gzip_decompress("`+path+`"; true)`) {
		t.Errorf("hint should show the corrected call, got: %s", hint)
	}
}

// The hint exists to explain a specific mistake. Firing it on anything else
// would turn a clear error about bad data into a misleading suggestion, so
// these are the cases it must stay quiet for.
func TestFileFlagHint_StaysQuiet(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name  string
		input any
	}{
		{"non-string input", []byte("bytes")},
		{"empty string", ""},
		{"path that does not exist", filepath.Join(dir, "absent.gz")},
		{"a directory", dir},
		{"payload containing NUL", "\x1f\x8b\x00binary"},
		{"payload spanning lines", "line one\nline two"},
		{"payload longer than any path", strings.Repeat("a", 4097)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if hint := FileFlagHint("gzip_decompress", tc.input); hint != "" {
				t.Errorf("expected no hint, got: %s", hint)
			}
		})
	}
}
