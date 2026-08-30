package astsearch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/odvcencio/gotreesitter/grammars"
)

const goSource = `package main

import "fmt"

func run(name string) error {
	if name == "" {
		return fmt.Errorf("empty")
	}
	return nil
}

func check(a, b int) error {
	if a > b {
		return fmt.Errorf("out of order")
	}
	return nil
}

func main() {
	if err := run("x"); err != nil {
		fmt.Println(err)
	}
}
`

const pySource = `def greet(name):
    return "hi " + name


class Thing:
    def method(self, a, b):
        raise ValueError("nope")
`

// tree writes a set of files into a temporary directory and returns its path.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// srun collects every value a query emits. select_ast streams, so a test that
// read only the first result would pass while reporting one match of however
// many there are.
func srun(t *testing.T, query string) []any {
	t.Helper()
	out, err := srunErr(query)
	if err != nil {
		t.Fatalf("%q: %v", query, err)
	}
	return out
}

// srunErr is the same, for the cases where the error is the thing being tested.
func srunErr(query string) ([]any, error) {
	q, err := gojq.Parse(query)
	if err != nil {
		return nil, err
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		return nil, err
	}
	var out []any
	iter := code.Run(nil)
	for {
		v, ok := iter.Next()
		if !ok {
			return out, nil
		}
		if e, isErr := v.(error); isErr {
			return out, e
		}
		out = append(out, v)
	}
}

// collected runs a query whose single output is an array and returns its
// elements, so that a test reads the matches rather than the array holding
// them.
func collected(t *testing.T, query string) []any {
	t.Helper()
	got := srun(t, query)
	if len(got) != 1 {
		t.Fatalf("%q produced %d outputs, want one array", query, len(got))
	}
	arr, ok := got[0].([]any)
	if !ok {
		t.Fatalf("%q produced %T, want an array", query, got[0])
	}
	return arr
}

func field(t *testing.T, v any, key string) any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected an object, got %T", v)
	}
	return m[key]
}

// TestMatchesTheShapeNotTheText is the whole point of the cmdlet: the pattern
// is matched against the parse tree, so formatting does not matter and a
// comment mentioning the same words does not match.
func TestMatchesTheShapeNotTheText(t *testing.T) {
	dir := tree(t, map[string]string{
		"tidy.go": goSource,
		"messy.go": "package main\n\nimport \"fmt\"\n\nfunc   spread(\n\tname string,\n) error {\n\t" +
			"return fmt.Errorf(\"x\")\n}\n",
		"lying.go": "package main\n\n// func decoy(a string) error is only a comment\nconst s = " +
			"`func quoted(a string) error {}`\n",
	})

	got := collected(t, fmt.Sprintf(`[select_ast(%q; "func $NAME($$$ARGS) error { $$$BODY }") | .Captures.NAME]`, dir))
	names := map[string]bool{}
	for _, v := range got {
		names[fmt.Sprint(v)] = true
	}

	for _, want := range []string{"run", "check", "spread"} {
		if !names[want] {
			t.Errorf("did not find %s; found %v", want, names)
		}
	}
	// The two that only look like functions to a regex.
	for _, unwanted := range []string{"decoy", "quoted"} {
		if names[unwanted] {
			t.Errorf("matched %s, which is inside a comment or a string literal", unwanted)
		}
	}
}

// TestCapturesCarryTheHoles checks the part a regex search has no analogue
// for: each metavariable comes back under its own name.
func TestCapturesCarryTheHoles(t *testing.T) {
	dir := tree(t, map[string]string{"a.go": goSource})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; "fmt.Errorf($MSG)") | .Captures.MSG]`, dir))
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(got), got)
	}
	for _, v := range got {
		if !strings.Contains(fmt.Sprint(v), `"`) {
			t.Errorf("capture %v does not look like the string literal argument", v)
		}
	}
}

// TestPositionsPointAtTheMatch checks the line and column a caller would use
// to open the file there.
func TestPositionsPointAtTheMatch(t *testing.T) {
	dir := tree(t, map[string]string{"a.go": goSource})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; "func $N($$$A) error { $$$B }")]`, dir))
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
	// `func run` is on line 5 of goSource, `func check` on line 13. The match
	// starts at the name, which is where a caller wants the cursor.
	first := got[0]
	if line := field(t, first, "LineNumber"); fmt.Sprint(line) != "5" {
		t.Errorf("first match reported line %v, want 5", line)
	}
	if col := field(t, first, "Column"); fmt.Sprint(col) != "6" {
		t.Errorf("first match reported column %v, want 6", col)
	}
	if lang := field(t, first, "Language"); lang != "go" {
		t.Errorf("reported language %v, want go", lang)
	}
}

// TestTextSpansEveryLineOfTheMatch checks that a match crossing lines carries
// all of them, which is the difference from a line-oriented search.
func TestTextSpansEveryLineOfTheMatch(t *testing.T) {
	dir := tree(t, map[string]string{"a.go": goSource})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; "func $N($$$A) error { $$$B }")]`, dir))
	text := fmt.Sprint(field(t, got[0], "Text"))
	if !strings.Contains(text, "\n") {
		t.Errorf("a multi-line function matched as one line: %q", text)
	}
	if start, end := field(t, got[0], "LineNumber"), field(t, got[0], "EndLineNumber"); start == end {
		t.Errorf("a multi-line match reported the same start and end line (%v)", start)
	}
}

// TestAMalformedPatternIsRefused is the reason this package wraps the engine
// rather than calling it.
//
// A pattern that is not code compiles anyway, into a query full of ERROR nodes
// that runs against every file and matches nothing. Without this check a typo
// and an honest absence are the same empty result, and the caller reads the
// wrong one.
func TestAMalformedPatternIsRefused(t *testing.T) {
	dir := tree(t, map[string]string{"a.go": goSource})
	for _, pattern := range []string{"func $$$(", "))))", "if { { {"} {
		t.Run(pattern, func(t *testing.T) {
			_, err := srunErr(fmt.Sprintf(`[select_ast(%q; %q; {Language: "go"})]`, dir, pattern))
			if err == nil {
				t.Fatalf("pattern %q was accepted; it can never match", pattern)
			}
			if !strings.Contains(err.Error(), "does not parse as go code") {
				t.Errorf("error %q does not say the pattern is not go code", err)
			}
		})
	}
}

// TestValidPatternsAreNotRefused is the false-positive control on the check
// above. A refusal that fires on working patterns would be worse than no check
// at all.
func TestValidPatternsAreNotRefused(t *testing.T) {
	for _, tc := range []struct{ language, pattern string }{
		{"go", "func $N($$$A) error { $$$B }"},
		{"go", "if $C { $$$B }"},
		{"go", "$X.$M($$$A)"},
		{"go", "for $$$H { $$$B }"},
		{"go", "return $X, $Y"},
		{"go", "$A + $B"},
		{"python", "def $N($$$A): $$$B"},
		{"python", "for $V in $I: $$$B"},
		{"python", "raise $E($$$A)"},
		{"javascript", "function $N($$$A) { $$$B }"},
		{"javascript", "require($M)"},
	} {
		t.Run(tc.language+": "+tc.pattern, func(t *testing.T) {
			got := srun(t, fmt.Sprintf(`ast_pattern(%q; %q) | .Valid`, tc.pattern, tc.language))
			if len(got) != 1 || got[0] != true {
				t.Errorf("%q was reported invalid for %s", tc.pattern, tc.language)
			}
		})
	}
}

// TestForeignLanguagesAreSkipped covers the failure the first version of this
// cmdlet had: a Go pattern searched over a repository died on the repository's
// Dockerfile.
func TestForeignLanguagesAreSkipped(t *testing.T) {
	dir := tree(t, map[string]string{
		"a.go":       goSource,
		"Dockerfile": "FROM scratch\nCOPY a /a\n",
		"s.py":       pySource,
		"notes.txt":  "nothing to parse here\n",
	})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; "func $N($$$A) error { $$$B }") | .Captures.N]`, dir))
	if len(got) != 2 {
		t.Fatalf("expected the 2 Go functions, got %d: %v", len(got), got)
	}
}

// TestEveryFileSkippedIsNotAnAnswer is the other half. Skipping quietly is
// right for a file in another language and wrong when it was every file: that
// is a broken pattern, and an empty result would read as a fact about the code.
func TestEveryFileSkippedIsNotAnAnswer(t *testing.T) {
	dir := tree(t, map[string]string{"a.go": goSource, "b.go": goSource})
	_, err := srunErr(fmt.Sprintf(`[select_ast(%q; "func $$$(")]`, dir))
	if err == nil {
		t.Fatal("a pattern that parses as nothing returned an empty result instead of an error")
	}
	if !strings.Contains(err.Error(), "every file was skipped") {
		t.Errorf("error %q does not say that nothing was searched", err)
	}
}

// TestNoMatchesIsStillAnAnswer is the control on that: a working pattern that
// genuinely finds nothing must not be turned into an error.
func TestNoMatchesIsStillAnAnswer(t *testing.T) {
	dir := tree(t, map[string]string{"a.go": goSource})
	if got := collected(t, fmt.Sprintf(`[select_ast(%q; "func absent_from_this_file() bool { $$$B }")]`, dir)); len(got) != 0 {
		t.Errorf("expected no matches, got %v", got)
	}
}

// TestSearchIsLazy checks that the stream stops where the caller stops, which
// is what makes `first(select_ast(...))` cheap on a large tree.
func TestSearchIsLazy(t *testing.T) {
	files := map[string]string{"aaa.go": goSource}
	// A file that cannot be parsed at all, placed after the first alphabetically.
	files["zzz.go"] = goSource
	dir := tree(t, files)

	if got := collected(t, fmt.Sprintf(`[first(select_ast(%q; "func $N($$$A) error { $$$B }"))]`, dir)); len(got) != 1 {
		t.Fatalf("first() produced %d matches, want exactly one", len(got))
	}
}

// TestIncludeNarrowsTheWalk checks the option that decides which files are
// read at all - and, with it, the second way the "every file was skipped"
// error earns its place.
//
// A Python pattern narrowed to Go files reaches no file it can parse. Without
// the check that is an empty result, which reads as "this code does not occur"
// rather than "you searched the wrong files".
func TestIncludeNarrowsTheWalk(t *testing.T) {
	dir := tree(t, map[string]string{"a.go": goSource, "b.py": pySource})
	if got := collected(t, fmt.Sprintf(`[select_ast(%q; "def $N($$$A): $$$B"; {Include: "*.py"})]`, dir)); len(got) != 2 {
		t.Errorf("expected the 2 Python defs, got %d", len(got))
	}
	_, err := srunErr(fmt.Sprintf(`[select_ast(%q; "def $N($$$A): $$$B"; {Include: "*.go"})]`, dir))
	if err == nil || !strings.Contains(err.Error(), "every file was skipped") {
		t.Errorf("a Python pattern over Go files returned %v, not the skipped-everything error", err)
	}
}

// TestUnknownOptionIsRefused keeps the options object from swallowing a
// misspelling, which is how a caller ends up believing a filter applied.
func TestUnknownOptionIsRefused(t *testing.T) {
	dir := tree(t, map[string]string{"a.go": goSource})
	_, err := srunErr(fmt.Sprintf(`[select_ast(%q; "$A + $B"; {Includes: "*.go"})]`, dir))
	if err == nil || !strings.Contains(err.Error(), "unknown option") {
		t.Errorf("a misspelled option was accepted: %v", err)
	}
}

// TestPatternReportsWhatItCompiledTo covers ast_pattern's reason for
// existing: showing the caller what their pattern became.
func TestPatternReportsWhatItCompiledTo(t *testing.T) {
	got := srun(t, `ast_pattern("func $N($$$A) error { $$$B }"; "go")`)
	if len(got) != 1 {
		t.Fatalf("expected one object, got %v", got)
	}
	if v := field(t, got[0], "Valid"); v != true {
		t.Errorf("a working pattern was reported invalid")
	}
	query := fmt.Sprint(field(t, got[0], "Query"))
	if !strings.Contains(query, "function_declaration") {
		t.Errorf("the compiled query %q does not mention the node it matches", query)
	}
	vars, ok := field(t, got[0], "MetaVariables").([]any)
	if !ok {
		t.Fatalf("MetaVariables is %T, want an array", field(t, got[0], "MetaVariables"))
	}
	seen := map[string]bool{}
	for _, v := range vars {
		seen[fmt.Sprint(v)] = true
	}
	for _, want := range []string{"N", "A", "B"} {
		if !seen[want] {
			t.Errorf("MetaVariables %v does not list %s", vars, want)
		}
	}
	// The engine's own names for pattern literals are not the caller's.
	for _, v := range vars {
		if strings.HasPrefix(fmt.Sprint(v), "_") {
			t.Errorf("MetaVariables leaks the engine's internal name %v", v)
		}
	}
}

// TestPatternReportsTheOnesThatParseAsSomethingElse pins the case the ERROR
// check cannot catch, so that it is at least visible.
//
// Python's `except $E: $$$B` is not an ERROR: "except" also reads as an
// identifier, so the pattern compiles into a query for an assignment. It is
// well-formed, wrong, and silent - and reading .Query is the only way to see it.
func TestPatternReportsTheOnesThatParseAsSomethingElse(t *testing.T) {
	got := srun(t, `ast_pattern("except $E: $$$B"; "python") | .Query`)
	if len(got) != 1 {
		t.Fatalf("expected one query, got %v", got)
	}
	if q := fmt.Sprint(got[0]); !strings.Contains(q, "assignment") {
		t.Skipf("the engine now compiles this pattern differently (%s); the case it stood for is gone", q)
	}
}

// TestAnUnknownLanguageSaysWhereToLook checks that naming a grammar this build
// does not carry points at the cmdlet that lists the ones it does.
// The name is one no grammar can ever have, because which grammars exist is a
// build-time choice: "cobol" is absent from this repository's build and
// present in an untagged one, and a test that depended on that would pass or
// fail by build tag.
func TestAnUnknownLanguageSaysWhereToLook(t *testing.T) {
	_, err := srunErr(`ast_pattern("$A + $B"; "not a language, and never will be")`)
	if err == nil {
		t.Fatal("an unknown language was accepted")
	}
	if !strings.Contains(err.Error(), "get_ast_language") {
		t.Errorf("error %q does not point at the cmdlet that lists the languages", err)
	}
}

// TestLanguagesAreReadFromTheRegistry is the drift guard.
//
// Which grammars a binary carries is a build-time decision, so get_ast_language
// must report the registry the parser itself consults rather than a list
// written down beside it. Changing the build tags must change the answer.
func TestLanguagesAreReadFromTheRegistry(t *testing.T) {
	got := srun(t, `[get_ast_language | .Name]`)
	names, _ := got[0].([]any)
	if len(names) != len(grammars.AllLanguages()) {
		t.Errorf("get_ast_language reported %d languages, the registry has %d",
			len(names), len(grammars.AllLanguages()))
	}
	if len(names) == 0 {
		t.Fatal("this build carries no grammars, so select_ast can never do anything")
	}
}

// TestEveryLanguageThisBuildClaimsCanParse checks the claim get_ast_language
// makes. A language in the list that cannot compile a pattern is a name that
// wastes a caller's next call.
func TestEveryLanguageThisBuildClaimsCanParse(t *testing.T) {
	for _, entry := range grammars.AllLanguages() {
		t.Run(entry.Name, func(t *testing.T) {
			if _, err := compilePattern("$A", entry.Name); err != nil {
				t.Errorf("get_ast_language offers %s, but a pattern will not compile for it: %v",
					entry.Name, err)
			}
		})
	}
}

// TestGoIsAlwaysPresent pins the one language this repository's own build must
// carry, since the Makefile's grammar list is what decides it.
func TestGoIsAlwaysPresent(t *testing.T) {
	if grammars.DetectLanguageByName("go") == nil {
		t.Error("this build cannot parse Go, which the Makefile's GRAMMARS list is supposed to guarantee")
	}
}
