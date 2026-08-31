package pwrgrep_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// The operators a rule is written with, each against the thing it was added
// for. Every one of these is a rule that could not be ported before it
// existed, so the case here is the shape the corpus writes rather than a
// minimal example.

// TestMain loads the grammars before the clock starts on any one query.
//
// The first pattern compiled in a process decodes every grammar the binary
// carries, which is two hundred of them in a test build and takes fifteen
// seconds under the race detector. Left where it falls it is charged to
// whichever test runs first, which then times out on a slower machine for a
// reason that has nothing to do with what it is testing.
func TestMain(m *testing.M) {
	runner := &queryrun.Runner{Options: udf.DefaultRegistry().Options()}
	runner.Run(context.Background(), &queryrun.Request{
		Query: `"f($X)" | ast_pattern("python") | .Valid`, NullInput: true, MaxResults: 1,
	})
	os.Exit(m.Run())
}

// run evaluates a query against the cmdlet vocabulary and decodes its one
// result.
func run(t *testing.T, query string) any {
	t.Helper()
	runner := &queryrun.Runner{Options: udf.DefaultRegistry().Options()}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res := runner.Run(ctx, &queryrun.Request{Query: query, NullInput: true, Compact: true, MaxResults: 4})
	if res.Error != "" {
		t.Fatalf("%s\n  %s", query, res.Error)
	}
	if len(res.Values) == 0 {
		t.Fatalf("%s produced nothing", query)
	}
	var out any
	if err := json.Unmarshal([]byte(res.Values[0]), &out); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return out
}

// write puts one file in a temporary directory and returns the directory.
func write(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// tainted is the shape a taint rule is about: a value that arrives from
// outside and ends up somewhere it decides what the program does.
const tainted = `import subprocess, shlex
from flask import request

def direct():
    name = request.args.get("n")
    subprocess.run(name, shell=True)

def through_another():
    a = request.args.get("n")
    b = a
    subprocess.run(b, shell=True)

def sanitized():
    name = shlex.quote(request.args.get("n"))
    subprocess.run(name, shell=True)

def constant():
    name = "ls -la"
    subprocess.run(name, shell=True)

def inline():
    subprocess.run(request.args.get("n"), shell=True)
`

// TestAValueIsFollowedFromWhereItArrivesToWhereItIsUsed is the operator a
// quarter of any rule corpus needs. Neither end is a finding on its own.
func TestAValueIsFollowedFromWhereItArrivesToWhereItIsUsed(t *testing.T) {
	dir := write(t, "app.py", tainted)
	got := run(t, `
	["request.args.get($$$_)"] as $sources
	| ["subprocess.run($$$_)"] as $sinks
	| ["shlex.quote($$$_)"] as $clean
	| (`+quoted(dir)+` | scan_ast("*.py"; $sources + $sinks + $clean)) as $all
	| ($all | of($sinks) | reaching($all | of($sources); $all | of($clean)))
	| map(.LineNumber)`)

	want := []any{6.0, 11.0, 22.0}
	if !equal(got, want) {
		t.Errorf("reached lines %v, want %v\n  6 direct, 11 through another name, 22 written into the call;\n"+
			"  15 is sanitized and 19 is a constant", got, want)
	}
}

// TestASanitizedValueIsNotFollowed pins the half of the previous test that is
// easiest to lose: a rule that ignored its sanitizers would still pass on the
// lines it does report.
func TestASanitizedValueIsNotFollowed(t *testing.T) {
	dir := write(t, "app.py", tainted)
	got := run(t, `
	["request.args.get($$$_)"] as $sources
	| ["subprocess.run($$$_)"] as $sinks
	| (`+quoted(dir)+` | scan_ast("*.py"; $sources + $sinks)) as $all
	| ($all | of($sinks) | reaching($all | of($sources); []))
	| map(.LineNumber)`)

	// With nothing named as a sanitizer, the call that quotes its argument is
	// reached like any other.
	want := []any{6.0, 11.0, 15.0, 22.0}
	if !equal(got, want) {
		t.Errorf("reached lines %v, want %v", got, want)
	}
}

// TestFocusMovesTheFindingOntoTheHole is what a rule means by naming a hole to
// focus on: the same call is found, and the caret sits on the argument that
// makes it worth reporting.
func TestFocusMovesTheFindingOntoTheHole(t *testing.T) {
	dir := write(t, "app.py", "import subprocess\nsubprocess.run(cmd, shell=True)\n")
	got := run(t, `(`+quoted(dir)+` | scan_ast("*.py"; ["subprocess.run($CMD, shell=$TRUE)"]))
	| focus("TRUE") | map({L: .LineNumber, C: .Column, T: .Text})`)

	want := []any{map[string]any{"L": 2.0, "C": 27.0, "T": "True"}}
	if !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestAHoleIsComparedAsANumberInWhateverBaseItWasWritten is what the file-mode
// rules need. 0o777 and 511 are one value, and a rule that compared the text
// would report neither.
func TestAHoleIsComparedAsANumberInWhateverBaseItWasWritten(t *testing.T) {
	dir := write(t, "app.py", "import os\nos.chmod(\"/a\", 0o777)\nos.chmod(\"/b\", 0o600)\nos.chmod(\"/c\", 511)\n")
	got := run(t, `(`+quoted(dir)+` | scan_ast("*.py"; ["os.chmod($P, $MODE)"]))
	| where_capture_compare("$MODE >= 0o666") | map(.LineNumber)`)

	want := []any{2.0, 4.0}
	if !equal(got, want) {
		t.Errorf("got %v, want %v; 0o777 and 511 are the same mode and 0o600 is not", got, want)
	}
}

// TestAHoleIsConstrainedByAPatternOfItsOwn covers the constraint a rule writes
// as syntax rather than as a regex over the text.
func TestAHoleIsConstrainedByAPatternOfItsOwn(t *testing.T) {
	dir := write(t, "app.py", "import os, subprocess\nsubprocess.run(\"ls\")\nsubprocess.run(os.getenv(\"CMD\"))\n")
	got := run(t, `(`+quoted(dir)+` | scan_ast("*.py"; ["subprocess.run($CMD)"]))
	| where_capture_ast("CMD"; "os.getenv($$$_)") | map(.LineNumber)`)

	want := []any{3.0}
	if !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestARuleCanSearchTextWhereThereIsNoGrammar is the other third of a corpus:
// a template with a directive in it that no language parses.
func TestARuleCanSearchTextWhereThereIsNoGrammar(t *testing.T) {
	dir := write(t, "page.html", "<html>\n{% autoescape off %}\n{{ bio|safe }}\n</html>\n")
	got := run(t, `(`+quoted(dir)+` | scan_regex("*.html"; ["\\{%\\s*autoescape\\s+off\\s*%\\}"]))
	| map({L: .LineNumber, T: .Text})`)

	want := []any{map[string]any{"L": 2.0, "T": "{% autoescape off %}"}}
	if !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestANamedGroupIsAHole is what lets a text rule say what it found in the
// same terms a structural one does.
func TestANamedGroupIsAHole(t *testing.T) {
	dir := write(t, "page.html", "{% autoescape off %}\n")
	got := run(t, `(`+quoted(dir)+` | scan_regex("*.html"; ["autoescape\\s+(?P<MODE>off|on)"]))
	| finding("x"; "autoescaping is $MODE here") | map(.Message)`)

	want := []any{"autoescaping is off here"}
	if !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// quoted renders a path as a jq string literal.
func quoted(s string) string {
	body, _ := json.Marshal(s)
	return string(body)
}

// equal compares two decoded JSON values.
func equal(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
