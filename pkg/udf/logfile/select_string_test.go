package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

// srun collects every value a query emits. select_string streams, so a test
// that read only the first result would pass while reporting one match out of
// however many there are.
func srun(t *testing.T, query string) []any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	var out []any
	iter := code.Run(nil)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if e, isErr := v.(error); isErr {
			t.Fatalf("%q: %v", query, e)
		}
		out = append(out, v)
	}
	return out
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
	rows := srun(t, fmt.Sprintf(`select_string(%q; "TODO")`, dir))
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

// TestSelectStringStreams pins the output contract itself: one value per match,
// not one array of them. get_childitem and find stream, and select_string not
// streaming was the inconsistency callers tripped over.
func TestSelectStringStreams(t *testing.T) {
	dir := searchTreeFixture(t)

	rows := srun(t, fmt.Sprintf(`select_string(%q; "TODO")`, dir))
	if len(rows) != 2 {
		t.Fatalf("select_string emitted %d values, want 2 - one per match", len(rows))
	}
	for i, row := range rows {
		if _, ok := row.(map[string]any); !ok {
			t.Errorf("value %d is %T, want a match object", i, row)
		}
	}

	// And it collects with [...] like any other stream.
	collected := srun(t, fmt.Sprintf(`[select_string(%q; "TODO")] | length`, dir))
	if len(collected) != 1 || fmt.Sprint(collected[0]) != "2" {
		t.Errorf("[select_string(...)] | length = %v, want [2]", collected)
	}
}

// TestSelectStringNoMatchesEmitsNothing checks the empty case is an empty
// stream rather than one empty array, which is what `[...] | length == 0`
// depends on.
func TestSelectStringNoMatchesEmitsNothing(t *testing.T) {
	dir := searchTreeFixture(t)
	rows := srun(t, fmt.Sprintf(`select_string(%q; "no-such-text-anywhere")`, dir))
	if len(rows) != 0 {
		t.Errorf("emitted %d values for a pattern that matches nothing, want none: %v", len(rows), rows)
	}
}

// TestSelectStringIsLazy checks the walk stops when the caller does. A stream
// that computed every match up front would still return the right answer here,
// so the test watches the work rather than the result: the file that would be
// scanned next is unreadable, and reading only the first match must not touch
// it.
func TestSelectStringIsLazy(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable files")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Sorted after a.txt, so the walk reaches it only if it keeps going.
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("needle\n"), 0o000); err != nil {
		t.Fatal(err)
	}

	q, err := gojq.Parse(fmt.Sprintf(`first(select_string(%q; "needle")) | .Path`, dir))
	if err != nil {
		t.Fatal(err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := code.Run(nil).Next()
	if !ok {
		t.Fatal("first(select_string(...)) produced no result")
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("first(select_string(...)) reached the unreadable file: %v", e)
	}
	if got, want := fmt.Sprint(v), filepath.Join(dir, "a.txt"); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestSelectStringCaseSensitive(t *testing.T) {
	dir := searchTreeFixture(t)
	rows := srun(t, fmt.Sprintf(`select_string(%q; "TODO"; {CaseSensitive: true})`, dir))
	if len(rows) != 1 {
		t.Errorf("case-sensitive matches = %d, want 1", len(rows))
	}
}

func TestSelectStringInclude(t *testing.T) {
	dir := searchTreeFixture(t)
	rows := srun(t, fmt.Sprintf(`select_string(%q; "todo"; {Include: "*.go"})`, dir))
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
	rows := srun(t, fmt.Sprintf(`select_string(%q; "TODO"; {Include: "*.txt", Context: 1})`, dir))
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

// TestSelectStringContextWindow covers the sliding window the streaming scan
// keeps: several matches close enough together that their context overlaps,
// plus matches at the very first and very last line, where the window is short.
func TestSelectStringContextWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	body := "hit one\nfiller\nhit two\nhit three\nfiller\nhit four\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	rows := srun(t, fmt.Sprintf(`select_string(%q; "hit"; {Context: 2})`, path))
	if len(rows) != 4 {
		t.Fatalf("matches = %d, want 4", len(rows))
	}

	want := []struct{ before, after string }{
		{"[]", "[filler hit two]"},                 // line 1: nothing before it
		{"[hit one filler]", "[hit three filler]"}, // line 3
		{"[filler hit two]", "[filler hit four]"},  // line 4, its window overlapping line 3's
		{"[hit three filler]", "[]"},               // line 6: nothing after it
	}

	for i, row := range rows {
		m := row.(map[string]any)
		if got := fmt.Sprint(m["Before"]); got != want[i].before {
			t.Errorf("match %d Before = %v, want %v", i, got, want[i].before)
		}
		if got := fmt.Sprint(m["After"]); got != want[i].after {
			t.Errorf("match %d After = %v, want %v", i, got, want[i].after)
		}
	}
}

// TestSelectStringPipedPath is the form that lets find feed the search.
func TestSelectStringPipedPath(t *testing.T) {
	dir := searchTreeFixture(t)
	rows := srun(t, fmt.Sprintf(`%q | select_string("TODO")`, dir))
	if len(rows) != 2 {
		t.Errorf("piped matches = %d, want 2", len(rows))
	}
}

func TestSelectStringSingleFile(t *testing.T) {
	dir := searchTreeFixture(t)
	rows := srun(t, fmt.Sprintf(`select_string(%q; "TODO")`, filepath.Join(dir, "a.txt")))
	if len(rows) != 1 {
		t.Errorf("single-file matches = %d, want 1", len(rows))
	}
}

// TestSelectStringList reports the files that contain a match, one per file.
func TestSelectStringList(t *testing.T) {
	dir := searchTreeFixture(t)
	rows := srun(t, fmt.Sprintf(`select_string(%q; "todo"; {List: true})`, dir))
	if len(rows) != 2 {
		t.Fatalf("list matches = %d, want 2 (one per file, not one per line)", len(rows))
	}
	for _, row := range rows {
		m := row.(map[string]any)
		if _, ok := m["LineNumber"]; ok {
			t.Errorf("list mode should report the file, not a line: %v", m)
		}
	}
}

// TestSelectStringWalkOrder pins the order matches arrive in. The lazy walk
// replaced filepath.WalkDir, and a stream is only useful if it is predictable.
func TestSelectStringWalkOrder(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"b_dir", "d_dir"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{"a.txt", "b_dir/inner.txt", "c.txt", "d_dir/inner.txt", "e.txt"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("mark\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rows := srun(t, fmt.Sprintf(`select_string(%q; "mark")`, dir))
	if len(rows) != len(files) {
		t.Fatalf("matches = %d, want %d", len(rows), len(files))
	}
	for i, row := range rows {
		want := filepath.Join(dir, files[i])
		if got := row.(map[string]any)["Path"].(string); got != want {
			t.Errorf("match %d Path = %q, want %q (lexical, depth-first)", i, got, want)
		}
	}
}

// TestSelectStringSkipsNoiseDirectories keeps the .git/node_modules/vendor skip
// that the WalkDir version had.
func TestSelectStringSkipsNoiseDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "x.txt"), []byte("mark\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kept.txt"), []byte("mark\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rows := srun(t, fmt.Sprintf(`select_string(%q; "mark")`, dir))
	if len(rows) != 1 {
		t.Fatalf("matches = %d, want 1 - node_modules should be skipped", len(rows))
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

// TestSelectStringMissingPathErrors keeps the failure a failure: a stream that
// simply ended would look like "no matches".
func TestSelectStringMissingPathErrors(t *testing.T) {
	q, _ := gojq.Parse(`select_string("/no/such/directory/anywhere"; "x")`)
	code, _ := gojq.Compile(q, RegisterAll()...)
	v, ok := code.Run(nil).Next()
	if !ok {
		t.Fatal("a missing path produced an empty stream, not an error")
	}
	if _, isErr := v.(error); !isErr {
		t.Errorf("a missing path should be an error, got %v", v)
	}
}
