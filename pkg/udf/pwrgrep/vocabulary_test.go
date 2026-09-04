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

// taintedC is the same shape in C, where a value almost always arrives in a
// declaration rather than in an assignment. `char *p = getenv("X");` is a
// declaration with an initialiser - the grammar calls the two halves
// declarator and value - so a build that only knew left/right, name/value and
// target/value followed nothing at all in ordinary C.
const taintedC = `#include <stdlib.h>

void direct(void) {
    char *name = getenv("N");
    system(name);
}

void through_another(void) {
    char *a = getenv("N");
    char *b = a;
    system(b);
}

void constant(void) {
    char *name = "ls -la";
    system(name);
}

void inline_(void) {
    system(getenv("N"));
}
`

// TestAValueIsFollowedThroughACDeclaration is the C half of the operator
// above. It is worth its own test because C reaches a sink through a
// different node than every other language here: an init_declarator, whose
// halves the grammar names declarator and value.
func TestAValueIsFollowedThroughACDeclaration(t *testing.T) {
	dir := write(t, "app.c", taintedC)
	got := run(t, `
	["getenv($$$_)"] as $sources
	| ["system($$$_)"] as $sinks
	| (`+quoted(dir)+` | scan_ast("*.c"; $sources + $sinks)) as $all
	| ($all | of($sinks) | reaching($all | of($sources); []))
	| map(.LineNumber)`)

	want := []any{5.0, 11.0, 20.0}
	if !equal(got, want) {
		t.Errorf("reached lines %v, want %v\n  5 through a declaration, 11 through two of them,\n"+
			"  20 written into the call; 16 is a constant", got, want)
	}
}

// taintedThroughAClosure is the shape that made an SSRF rule report a test
// server's own address.
//
// The source sits inside a handler written as a closure, and the closure is
// the right-hand side of an assignment. Walking up from the source to find
// what it was given to used to walk straight out of the function and find
// that assignment, so `inner` was tainted - and then `ts`, which is built
// from it, and then `ts.URL`, which mentions `ts`. None of those holds the
// header's value. What `inner` was given is a function.
const taintedThroughAClosure = `package main

import (
	"net/http"
	"net/http/httptest"
)

func serve() {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Fail") != "" {
			w.WriteHeader(500)
		}
	})
	ts := httptest.NewServer(inner)
	http.Get(ts.URL)
}

func leaks(r *http.Request) {
	u := r.Header.Get("X-Target")
	http.Get(u)
}
`

// TestAValueDoesNotEscapeTheClosureItIsReadIn is the boundary the walk up from
// a source has to respect. A source inside a function body is not what an
// assignment outside that function gives away, whatever the spans say.
func TestAValueDoesNotEscapeTheClosureItIsReadIn(t *testing.T) {
	dir := write(t, "app.go", taintedThroughAClosure)
	got := run(t, `
	["$R.Header.Get($_)"] as $sources
	| ["http.Get($U)"] as $sinks
	| (`+quoted(dir)+` | scan_ast("*.go"; $sources + $sinks)) as $all
	| ($all | of($sinks) | reaching($all | of($sources); []))
	| map(.LineNumber)`)

	want := []any{20.0}
	if !equal(got, want) {
		t.Errorf("reached lines %v, want %v\n  20 is the header read into a name and fetched;\n"+
			"  15 fetches a test server's own address and the header never reached it", got, want)
	}
}

// taintedThroughAClosureLocal is the second half of the same leak, and it
// survives the first fix on its own.
//
// Here the source is assigned, so it binds a name correctly - `hdr`, scoped to
// the handler. What went wrong next was propagation: the right-hand side of
// `srv := httptest.NewServer(http.HandlerFunc(func(w, r) { ... }))` contains
// every identifier in the handler, `hdr` among them, so `srv` was fed by a
// value that only exists inside the closure. `hdr` is not in scope where
// `srv` is declared, and that is what makes it not a path.
const taintedThroughAClosureLocal = `package main

import (
	"net/http"
	"net/http/httptest"
)

func serve() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("X-Fail")
		_ = hdr
	}))
	http.Get(srv.URL)
}

func fetches(r *http.Request) {
	u := r.Header.Get("X-Target")
	http.Get(u)
}
`

// TestAValueDoesNotFeedANameDeclaredOutsideItsScope is the propagation half.
// Fixing only the walk up from the source makes this worse rather than better:
// the bogus early binding used to shadow the name, and removing it let
// propagation reach further.
func TestAValueDoesNotFeedANameDeclaredOutsideItsScope(t *testing.T) {
	dir := write(t, "app.go", taintedThroughAClosureLocal)
	got := run(t, `
	["$R.Header.Get($_)"] as $sources
	| ["http.Get($U)"] as $sinks
	| (`+quoted(dir)+` | scan_ast("*.go"; $sources + $sinks)) as $all
	| ($all | of($sinks) | reaching($all | of($sources); []))
	| map(.LineNumber)`)

	want := []any{18.0}
	if !equal(got, want) {
		t.Errorf("reached lines %v, want %v\n  18 is the header read into a name and fetched;\n"+
			"  13 fetches the test server's own address, which no header decided", got, want)
	}
}

// boundByALoop is the shape a loop binding makes, in the two languages whose
// grammars spell it most differently.
//
// A `for` is an assignment - `name` is given `value` - and its node encloses
// the body that uses the name. Recording the binding at the end of the node
// therefore put it after every use of the loop variable, and a value that
// arrived through a loop was never followed anywhere. Both halves of every
// zip-extraction and every multi-valued parameter read like this.
const boundByALoop = `import os
import subprocess


def each(request):
    for name in request.args.getlist("f"):
        subprocess.run(name, shell=True)


def once(request):
    name = request.args.get("f")
    subprocess.run(name, shell=True)


def never(request):
    subprocess.run("ls", shell=True)
`

// TestAValueFollowedIntoALoopBody is the fix for that: the binding is recorded
// at the end of the value, and between the value and the body there is nothing
// a name can be used in.
func TestAValueFollowedIntoALoopBody(t *testing.T) {
	dir := write(t, "app.py", boundByALoop)
	got := run(t, `
	["request.args.getlist($$$_)", "request.args.get($$$_)"] as $sources
	| ["subprocess.run($C, $$$_)"] as $sinks
	| (`+quoted(dir)+` | scan_ast("*.py"; $sources + $sinks)) as $all
	| ($all | of($sinks) | focus("C") | reaching($all | of($sources); []))
	| map(.LineNumber)`)

	want := []any{7.0, 12.0}
	if !equal(got, want) {
		t.Errorf("reached lines %v, want %v\n  7 is fed by the loop variable, 12 by the plain\n"+
			"  assignment; 16 runs a constant", got, want)
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

// boundWithoutAFieldName is the shape C# makes of a declaration: the
// declarator gives its name a field and leaves the initialiser an ordinary
// child, so none of the field pairs in bindingFields match and a declaration -
// which is how nearly every value in a C# program is introduced - carried no
// flow at all. Every rule in the corpus that follows a value through C# found
// nothing, silently.
const boundWithoutAFieldName = `using System.Data.SqlClient;

namespace Demo {
    public class Api {
        public void Read() {
            string name = Request.Query["name"];
            var cmd = new SqlCommand("SELECT * FROM t WHERE n = '" + name + "'", _c);
            cmd.ExecuteNonQuery();
        }

        public void Fixed() {
            string name = "constant";
            var cmd = new SqlCommand("SELECT * FROM t WHERE n = '" + name + "'", _c);
            cmd.ExecuteNonQuery();
        }
    }
}
`

func TestAValueIsFollowedThroughADeclarationWithNoValueField(t *testing.T) {
	dir := write(t, "Api.cs", boundWithoutAFieldName)
	got := run(t, `
	["$T $V = $S;"] as $assigned
	| ["new SqlCommand($Q, $$$_)"] as $sinks
	| (`+quoted(dir)+` | scan_ast("*.cs"; $assigned + $sinks)) as $all
	| ($all | of($assigned) | where_capture("S"; "Request\\.Query") | focus("S")) as $untrusted
	| ($all | of($sinks) | focus("Q") | reaching($untrusted; []))
	| map(.LineNumber)`)

	want := []any{7.0}
	if !equal(got, want) {
		t.Errorf("reached lines %v, want %v\n  7 is the query built from the request;\n"+
			"  13 builds the same string from a constant", got, want)
	}
}

// arrivingAsAParameter is the other half, and it is how every C# web framework
// written since about 2016 spells a request. ASP.NET binds a controller
// action's arguments from the query string, the route and the body, so the
// parameter *is* the untrusted value and there is no accessor in the method to
// point at instead. A source that is a name was given to nothing, so the walk
// up from it found no flow and the sink two lines later went unreported.
const arrivingAsAParameter = `using System.Data.SqlClient;

namespace Demo {
    public class Api {
        public void ByCity(string city) {
            var cmd = new SqlCommand("SELECT * FROM t WHERE c = '" + city + "'", _c);
            cmd.ExecuteNonQuery();
        }

        public void Elsewhere() {
            var cmd = new SqlCommand("SELECT * FROM t WHERE c = '" + city + "'", _c);
            cmd.ExecuteNonQuery();
        }
    }
}
`

func TestAValueIsFollowedFromTheParameterItArrivesIn(t *testing.T) {
	dir := write(t, "Api.cs", arrivingAsAParameter)
	got := run(t, `
	["public $T $M(string $P) { $$$_ }"] as $bound
	| ["new SqlCommand($Q, $$$_)"] as $sinks
	| (`+quoted(dir)+` | scan_ast("*.cs"; $bound + $sinks)) as $all
	| ($all | of($bound) | focus("P")) as $untrusted
	| ($all | of($sinks) | focus("Q") | reaching($untrusted; []))
	| map(.LineNumber)`)

	want := []any{6.0}
	if !equal(got, want) {
		t.Errorf("reached lines %v, want %v\n  6 uses the parameter it was given;\n"+
			"  11 spells a name the same way in a method that has no such parameter", got, want)
	}
}

// boundPositionally is Kotlin, and it is the grammar that names none of the
// three children a binding has.
//
// A property, an assignment and a `for` are all written positionally there, so
// none of the field-name pairs the taint engine knows matched anything and no
// Kotlin file carried any flow at all - in the language whose whole security
// literature is "an intent's extra reaches a sink". What makes it readable is
// that a grammar with no fields still marks the target by wrapping it in a
// node of its own.
//
// The second method is the scope test and it is the one that used to fail
// loudly: `name` there is a constant, and without a scope every Kotlin file
// with two methods that spell a local the same way reported the careful one.
const boundPositionally = `package app

class Sink {
    fun tainted(intent: Intent) {
        val name = intent.getStringExtra("n")
        exec(name)
    }

    fun clean() {
        val name = "constant"
        exec(name)
    }

    fun each(intent: Intent) {
        for (e in intent.getStringArrayListExtra("f")) {
            exec(e)
        }
    }
}
`

// TestAValueIsFollowedThroughAPositionalBinding is the fix: the wrapper the
// grammar puts round the target is the mark, measured from a probe rather than
// listed per language.
func TestAValueIsFollowedThroughAPositionalBinding(t *testing.T) {
	dir := write(t, "Sink.kt", boundPositionally)
	got := run(t, `
	["$I.getStringExtra($_)", "$I.getStringArrayListExtra($_)"] as $sources
	| ["exec($C)"] as $sinks
	| (`+quoted(dir)+` | scan_ast("*.kt"; $sources + $sinks)) as $all
	| ($all | of($sinks) | reaching($all | of($sources); []))
	| map(.LineNumber)`)

	want := []any{6.0, 16.0}
	if !equal(got, want) {
		t.Errorf("reached lines %v, want %v\n  6 is the extra read into a name and run,\n"+
			"  16 the same through a loop variable;\n"+
			"  11 spells the name the same way in a method that reads no intent", got, want)
	}
}

// closedOverInKotlin is the other half, and it is the boundary a grammar with
// no `body` field could not draw. A value read inside a lambda is not what the
// assignment outside it gives away.
const closedOverInKotlin = `package app

class Handler {
    fun serve(intent: Intent) {
        val inner = Runnable { tag -> val hdr = intent.getStringExtra("X"); use(hdr) }
        val started = wrap(inner)
        exec(started)
    }

    fun direct(intent: Intent) {
        val hdr = intent.getStringExtra("X")
        exec(hdr)
    }
}
`

func TestAValueDoesNotEscapeAKotlinLambda(t *testing.T) {
	dir := write(t, "Handler.kt", closedOverInKotlin)
	got := run(t, `
	["$I.getStringExtra($_)"] as $sources
	| ["exec($C)"] as $sinks
	| (`+quoted(dir)+` | scan_ast("*.kt"; $sources + $sinks)) as $all
	| ($all | of($sinks) | reaching($all | of($sources); []))
	| map(.LineNumber)`)

	want := []any{12.0}
	if !equal(got, want) {
		t.Errorf("reached lines %v, want %v\n  12 is the extra read into a name and run;\n"+
			"  7 runs what a lambda was wrapped into, and the extra never left it", got, want)
	}
}
