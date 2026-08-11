package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

func srun(t *testing.T, query string) any {
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

func searchTreeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"a.txt":     "alpha\nTODO: fix\ngamma\n",
		"b.go":      "package main\n// todo: later\n",
		"sub/c.txt": "nothing here\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSelectString(t *testing.T) {
	dir := searchTreeFixture(t)
	rows := srun(t, fmt.Sprintf(`select_string(%q; "TODO")`, dir)).([]any)
	if len(rows) != 2 {
		t.Fatalf("matches = %d, want 2 (case-insensitive by default)", len(rows))
	}
	first := rows[0].(map[string]any)
	for _, key := range []string{"Path", "LineNumber", "Line", "Match"} {
		if _, ok := first[key]; !ok {
			t.Errorf("match object missing %s: %v", key, first)
		}
	}
}

func TestSelectStringCaseSensitive(t *testing.T) {
	dir := searchTreeFixture(t)
	rows := srun(t, fmt.Sprintf(`select_string(%q; "TODO"; {CaseSensitive: true})`, dir)).([]any)
	if len(rows) != 1 {
		t.Errorf("case-sensitive matches = %d, want 1", len(rows))
	}
}

func TestSelectStringInclude(t *testing.T) {
	dir := searchTreeFixture(t)
	rows := srun(t, fmt.Sprintf(`select_string(%q; "todo"; {Include: "*.go"})`, dir)).([]any)
	if len(rows) != 1 {
		t.Fatalf("filtered matches = %d, want 1", len(rows))
	}
	path := rows[0].(map[string]any)["Path"].(string)
	if !strings.HasSuffix(path, "b.go") {
		t.Errorf("Path = %q, want the .go file", path)
	}
}

func TestSelectStringContext(t *testing.T) {
	dir := searchTreeFixture(t)
	rows := srun(t, fmt.Sprintf(`select_string(%q; "TODO"; {Include: "*.txt", Context: 1})`, dir)).([]any)
	if len(rows) != 1 {
		t.Fatalf("matches = %d, want 1", len(rows))
	}
	m := rows[0].(map[string]any)
	if fmt.Sprint(m["Before"]) != "[alpha]" {
		t.Errorf("Before = %v, want [alpha]", m["Before"])
	}
	if fmt.Sprint(m["After"]) != "[gamma]" {
		t.Errorf("After = %v, want [gamma]", m["After"])
	}
}

// TestSelectStringPipedPath is the form that lets find feed the search.
func TestSelectStringPipedPath(t *testing.T) {
	dir := searchTreeFixture(t)
	rows := srun(t, fmt.Sprintf(`%q | select_string("TODO")`, dir)).([]any)
	if len(rows) != 2 {
		t.Errorf("piped matches = %d, want 2", len(rows))
	}
}

func TestSelectStringSingleFile(t *testing.T) {
	dir := searchTreeFixture(t)
	rows := srun(t, fmt.Sprintf(`select_string(%q; "TODO")`, filepath.Join(dir, "a.txt"))).([]any)
	if len(rows) != 1 {
		t.Errorf("single-file matches = %d, want 1", len(rows))
	}
}

func TestSelectStringRejectsUnknownOption(t *testing.T) {
	dir := searchTreeFixture(t)
	q, _ := gojq.Parse(fmt.Sprintf(`select_string(%q; "TODO"; {Nonsense: 1})`, dir))
	code, _ := gojq.Compile(q, RegisterAll()...)
	v, _ := code.Run(nil).Next()
	if _, isErr := v.(error); !isErr {
		t.Errorf("unknown option should be an error, got %v", v)
	}
}
