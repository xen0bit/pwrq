package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query string) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	v, ok := code.Run(nil).Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

func runErr(t *testing.T, query string) error {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		return err
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		return err
	}
	v, ok := code.Run(nil).Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		return e
	}
	return nil
}

// tree builds a small source directory and returns its path.
func tree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"src/a.txt":        "alpha\n",
		"src/nested/b.txt": "beta\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRoundTrip(t *testing.T) {
	for _, ext := range []string{".zip", ".tar", ".tar.gz"} {
		t.Run(ext, func(t *testing.T) {
			dir := tree(t)
			src := filepath.Join(dir, "src")
			arc := filepath.Join(dir, "out"+ext)
			dest := filepath.Join(dir, "restored"+strings.ReplaceAll(ext, ".", "_"))

			run(t, fmt.Sprintf(`compress_archive(%q; %q)`, src, arc))
			if _, err := os.Stat(arc); err != nil {
				t.Fatalf("archive not created: %v", err)
			}

			entries := run(t, fmt.Sprintf(`read_archive(%q) | map(.Name)`, arc)).([]any)
			var names []string
			for _, e := range entries {
				names = append(names, e.(string))
			}
			joined := strings.Join(names, ",")
			for _, want := range []string{"src/a.txt", "src/nested/b.txt"} {
				if !strings.Contains(joined, want) {
					t.Errorf("%s: entries %v missing %s", ext, names, want)
				}
			}

			run(t, fmt.Sprintf(`expand_archive(%q; %q)`, arc, dest))
			got, err := os.ReadFile(filepath.Join(dest, "src", "nested", "b.txt"))
			if err != nil {
				t.Fatalf("%s: extracted file missing: %v", ext, err)
			}
			if string(got) != "beta\n" {
				t.Errorf("%s: extracted content = %q", ext, got)
			}
		})
	}
}

func TestCompressMultiplePaths(t *testing.T) {
	dir := tree(t)
	arc := filepath.Join(dir, "pair.zip")
	a := filepath.Join(dir, "src", "a.txt")
	b := filepath.Join(dir, "src", "nested", "b.txt")
	run(t, fmt.Sprintf(`[%q, %q] | compress_archive(%q)`, a, b, arc))
	entries := run(t, fmt.Sprintf(`read_archive(%q) | length`, arc))
	if fmt.Sprint(entries) != "2" {
		t.Errorf("entries = %v, want 2", entries)
	}
}

func TestUnsupportedExtension(t *testing.T) {
	dir := tree(t)
	err := runErr(t, fmt.Sprintf(`compress_archive(%q; %q)`, filepath.Join(dir, "src"), filepath.Join(dir, "x.rar")))
	if err == nil || !strings.Contains(err.Error(), "unsupported archive") {
		t.Errorf("expected an unsupported-archive error, got %v", err)
	}
}

// TestSafeJoinRefusesEscape covers Zip Slip: an entry name is attacker
// controlled, and "../../etc/passwd" must not be written where it asks.
func TestSafeJoinRefusesEscape(t *testing.T) {
	dest := t.TempDir()
	for _, name := range []string{"../escape.txt", "a/../../escape.txt", "../../etc/passwd"} {
		if _, err := safeJoin(dest, name); err == nil {
			t.Errorf("safeJoin(%q) should be refused", name)
		}
	}
	// An absolute entry name is not an escape: filepath.Join re-roots it under
	// the destination, which is the outcome we want.
	for _, name := range []string{"ok.txt", "a/b/ok.txt", "./ok.txt", "/absolute.txt"} {
		got, err := safeJoin(dest, name)
		if err != nil {
			t.Errorf("safeJoin(%q) should be allowed: %v", name, err)
			continue
		}
		if !strings.HasPrefix(got, dest) {
			t.Errorf("safeJoin(%q) = %q, outside %q", name, got, dest)
		}
	}
}
