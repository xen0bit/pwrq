package path

import (
	"fmt"
	"testing"
)

func TestNormalizeAndStem(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"/a//b/../c" | normalize_path`, "/a/c"},
		{`"./a/./b" | normalize_path`, "a/b"},
		{`"/tmp/data/report.pdf" | stem`, "report"},
		{`"/tmp/data/archive.tar.gz" | stem`, "archive.tar"},
		{`"a.txt" | has_extension`, "true"},
		{`"README" | has_extension`, "false"},
		{`"/a/b/" | is_dir_path`, "true"},
		{`"/a/b" | is_dir_path`, "false"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}

func TestRelativeAndWithExtension(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"/a/b/c" | relative_path("/a")`, "b/c"},
		{`"c.txt" | with_extension(".md")`, "c.md"},
		{`"c.txt" | with_extension("md")`, "c.md"},
		{`"/tmp/a.txt" | with_extension(".log")`, "/tmp/a.log"},
		{`"archive.tar.gz" | with_extension("")`, "archive.tar"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}

func TestHomeAndSep(t *testing.T) {
	// These need no filesystem but expand_home needs the user database, which
	// is why they are CLI-only.
	if got := run(t, `path_sep`); fmt.Sprint(got) != "/" {
		t.Errorf("path_sep = %v, want /", got)
	}
	home := run(t, `home_dir`)
	if fmt.Sprint(home) == "" {
		t.Error("home_dir returned empty")
	}
	expanded := run(t, `"~/x" | expand_home`)
	if fmt.Sprint(expanded) == "~/x" {
		t.Errorf("expand_home did not expand: %v", expanded)
	}
}
