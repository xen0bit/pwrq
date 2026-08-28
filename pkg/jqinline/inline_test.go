package jqinline

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/itchyny/gojq"
)

// tcase is a query and the input to run it against. The input matters: the
// property that inlining preserves meaning is only worth anything if the
// queries are actually evaluated, so every case here runs.
type tcase struct {
	query string
	input any
}

func obj(pairs ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i].(string)] = pairs[i+1]
	}
	return m
}

func arr(xs ...any) []any { return xs }

// cases covers the grammar an inlined body can be made of and, more to the
// point, the ways a naive paste would go wrong: a body that binds a name its
// argument also uses, a body that reads a name from its own scope, a nested
// definition that would shadow the argument, a label an argument breaks to.
var cases = []tcase{
	// The plain shapes.
	{`def f: 1; f`, nil},
	{`def f: .a + 1; f | f`, obj("a", 1.0)},
	{`def f(a): a + a; f(.x)`, obj("x", 2.0)},
	{`def f($a): $a + $a; f(1, 2)`, nil},
	{`def f($a; b): [$a, b]; f(1; 2)`, nil},
	{`def f: 1; def g: f; g`, nil},
	{`def double: . * 2; def quad: double | double; [.[] | quad]`, arr(1.0, 2.0)},
	{`def f: 1; def f(a): a; f + f(2)`, nil},
	{`def f: .a; f[0]`, obj("a", arr(7.0, 8.0))},
	{`def f: {a: 1}; f.a`, nil},
	{`def f: [1, 2, 3]; f[]?`, nil},
	{`def f(a): [a]; f(.[] | select(. > 1))`, arr(1.0, 2.0, 3.0)},

	// A definition used nowhere: inlining is what would have placed it, so
	// it goes.
	{`def unused: 1; .`, 3.0},

	// Value parameters are jq's own shorthand for a binding, and the filter
	// form of the name stays usable beside the value form.
	{`def f($a): [a, $a]; f(1, 2)`, nil},
	{`def f($a): $a; f(.[] | . * 2)`, arr(1.0, 2.0)},

	// Hygiene: the body binds what the argument reads.
	{`def f(g): 9 as $x | [g]; 1 as $x | f($x)`, nil},
	{`def f($a; $b): [$a, $b]; 5 as $a | f(1; $a)`, nil},
	{`def f(g): reduce (1, 2) as $x (0; . + g); 10 as $x | f($x)`, nil},
	{`def f(g): foreach (1, 2) as $x (0; . + g; .); 10 as $x | f($x)`, nil},
	{`def f(g): . as {$x} | [g, $x]; 7 as $x | f($x)`, obj("x", 1.0)},
	{`def f(g): . as [$x, $y] | [g, $x, $y]; 7 as $x | f($x)`, arr(1.0, 2.0)},
	{`def f(g): . as [$x] ?// {$x} | [g, $x]; 7 as $x | f($x)`, obj("x", 1.0)},
	{`def f(g): def h: 5; [h, g]; def h: 9; f(h)`, nil},
	{`def f(g): def h(z): [z, g]; h(1); def h(z): 99; f(h(2))`, nil},
	{`def f(g): label $out | [1, 2, 3 | g]; label $out | f(if . == 2 then break $out else . end)`, nil},
	{`def f(g): 3 as $x | [g, $x]; f($__loc__.line)`, nil},

	// Free names that still mean the same thing at the call site, so the
	// body may travel.
	{`5 as $x | def f: $x; f + f`, nil},
	{`def base: 10; def f: base + 1; f`, nil},

	// Free names that do not, so the definition has to stay. A body only
	// keeps a free function name when that function is itself one inlining
	// cannot remove, so the case needs a recursive definition to set up.
	{`. as $x | def f: $x; 2 as $x | f`, 1.0},
	{shadowedCall, nil},
	{recursiveShadow, nil},

	// Recursion stays a definition. (`def f: def g: f; g; f` belongs here
	// too, but it never terminates when run, so it is checked in
	// TestInlineKeepsWhatItCannotExpand alone.)
	{`def fact: if . <= 1 then 1 else . * (. - 1 | fact) end; fact`, 5.0},
	{`def walkr: if type == "array" then map(walkr) else . + 1 end; walkr`, arr(1.0, arr(2.0))},

	// Definitions in the places other than the top of the program that jq
	// allows them.
	{`. | def f: 1; f`, nil},
	{`[def f: 2; f, 3] | .[0:1]`, nil},
	{`(def f: 3; f) + 1`, nil},
	{`if (def f: true; f) then "y" else "n" end`, nil},
	{`reduce (def f: 1, 2; f) as $x (0; . + $x)`, nil},
	{`def outer: def inner: 2; inner * 3; outer`, nil},

	// Calls buried in the corners of the grammar.
	{`def f: "k"; {(f): 1}`, nil},
	{`def f: 1; "\(f) and \(f)"`, nil},
	{`def f: 0; .[f]`, arr(9.0)},
	{`def f: 1; .[f:]`, arr(1.0, 2.0, 3.0)},
	{`def f: 2; [limit(f; 1, 2, 3)]`, nil},
	{`def f: .a; try f catch "no"`, obj("a", 1.0)},
	{`def f: -1; [f, 2 * f]`, nil},
	{`def f: @base64 "x"; [f]`, nil},
	{`def f: 1; [.[] | f]`, arr(1.0)},

	// A definition that shadows a builtin, and one whose body calls the
	// builtin it shadows the name of elsewhere.
	{`def length: 4; [length, ("ab" | length)]`, nil},

	// Several uses, which is the whole point: the body appears at each.
	{`def tag: {t: .}; [.[] | tag]`, arr(1.0, 2.0, 3.0)},
}

// shadowedCall: f's body calls r, and by the time f is called r means the
// other definition. Expanding f there would silently switch which r runs, so
// f stays a definition. Both r's recurse, so neither can be inlined away -
// which is what makes the shadowing permanent rather than something a later
// pass dissolves.
const shadowedCall = `def r: if . > 3 then . else . + 1 | r end; ` +
	`def f: r; ` +
	`def r: if . > 9 then . else . + 5 | r end; ` +
	`0 | [f, r]`

// recursiveShadow: the body's own rec recurses, so it travels to the call
// site as a definition - and the argument names rec too, meaning something
// else. One of them has to be renamed.
const recursiveShadow = `def rec: if . > 3 then . else . + 1 | rec end; ` +
	`def f(g): def rec: if . > 9 then . else . + 3 | rec end; [rec, g]; 0 | f(rec)`

func parse(t *testing.T, src string) *gojq.Query {
	t.Helper()
	q, err := gojq.Parse(src)
	if err != nil {
		t.Fatalf("case does not parse: %v\n%s", err, src)
	}
	return q
}

// results runs a query and collects everything it produces, errors included,
// as strings. Comparing these is how the tests below check that inlining did
// not change what a query means.
func results(q *gojq.Query, input any) []string {
	code, err := gojq.Compile(q)
	if err != nil {
		return []string{"compile error: " + err.Error()}
	}
	var out []string
	iter := code.Run(input)
	for i := 0; i < 1000; i++ {
		v, ok := iter.Next()
		if !ok {
			return out
		}
		if err, ok := v.(error); ok {
			out = append(out, "error: "+err.Error())
			continue
		}
		out = append(out, fmt.Sprintf("%#v", v))
	}
	return append(out, "...truncated")
}

// TestInlinePreservesMeaning is the property the whole package rests on: the
// inlined query answers exactly what the original did, output for output.
func TestInlinePreservesMeaning(t *testing.T) {
	for _, c := range cases {
		want := results(parse(t, c.query), c.input)
		got := results(Inline(parse(t, c.query)).Query, c.input)
		if strings.Join(want, "\x00") != strings.Join(got, "\x00") {
			t.Errorf("inlining changed the answer\n--- query:\n%s\n--- inlined:\n%s\n--- was: %v\n--- now: %v",
				c.query, Inline(parse(t, c.query)).Query, want, got)
		}
	}
}

// TestInlineOutputReparses: the result has to be a query a user can keep
// editing, so it must survive a round trip through the parser unchanged.
func TestInlineOutputReparses(t *testing.T) {
	for _, c := range cases {
		out := Inline(parse(t, c.query)).Query.String()
		again, err := gojq.Parse(out)
		if err != nil {
			t.Errorf("inlined query does not parse: %v\n--- query:\n%s\n--- inlined:\n%s", err, c.query, out)
			continue
		}
		if again.String() != out {
			t.Errorf("inlined query is not stable through the parser\n--- once:\n%s\n--- twice:\n%s", out, again.String())
		}
	}
}

// TestInlineIsIdempotent: pressing the button twice is the same as pressing
// it once. Anything left the first time was left for a reason that has not
// changed.
func TestInlineIsIdempotent(t *testing.T) {
	for _, c := range cases {
		once := Inline(parse(t, c.query)).Query.String()
		twice := Inline(parse(t, once)).Query.String()
		if once != twice {
			t.Errorf("inlining is not idempotent\n--- first:\n%s\n--- second:\n%s", once, twice)
		}
	}
}

// TestInlineLeavesTheInputAlone: the caller keeps its parse tree, because the
// page reuses it.
func TestInlineLeavesTheInputAlone(t *testing.T) {
	for _, c := range cases {
		q := parse(t, c.query)
		before := q.String()
		Inline(q)
		if after := q.String(); after != before {
			t.Errorf("Inline modified its argument\n--- before:\n%s\n--- after:\n%s", before, after)
		}
	}
}

// TestInlineRemovesWhatItExpanded: a definition every call site took a copy
// of has no reason to still be there.
func TestInlineRemovesWhatItExpanded(t *testing.T) {
	for _, src := range []string{
		`def f: 1; f`,
		`def f: 1; def g: f; g`,
		`def unused: 1; .`,
		`. | def f: 1; f`,
		`def outer: def inner: 2; inner * 3; outer`,
	} {
		r := Inline(parse(t, src))
		if got := r.Query.String(); strings.Contains(got, "def ") {
			t.Errorf("definition survived inlining\n--- query:\n%s\n--- inlined:\n%s", src, got)
		}
		if len(r.Kept) != 0 {
			t.Errorf("%s: unexpected notes %v", src, r.Kept)
		}
	}
}

// TestInlineExpansions pins the shape of the result for the cases whose
// output a reader would want to check by eye.
func TestInlineExpansions(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`def f: 1; f`, `1`},
		{`def f: .a; f[0]`, `.a[0]`},
		{`def f: .a + 1; f | f`, `(.a + 1) | (.a + 1)`},
		{`def f(a): a + a; f(.x)`, `(.x + .x)`},
		{`def f($a): $a + $a; f(1)`, `(1 as $a | $a + $a)`},
		// The rename that keeps an argument's $x out of the body's reach.
		{`def f(g): 9 as $x | [g]; 1 as $x | f($x)`, `1 as $x | (9 as $x_1 | [$x])`},
		{`def f(g): def h: 5; [h, g]; def h: 9; f(h)`, `[5, 9]`},
		// A nested definition that has to survive - it recurses - and whose
		// name the argument also uses, so it is renamed out of the way.
		{recursiveShadow, `def rec: if . > 3 then . else . + 1 | rec end; ` +
			`0 | (def rec_1: if . > 9 then . else . + 3 | rec_1 end; [rec_1, rec])`},
	} {
		if got := Inline(parse(t, c.in)).Query.String(); got != c.want {
			t.Errorf("Inline(%s)\n got: %s\nwant: %s", c.in, got, c.want)
		}
	}
}

// TestInlineKeepsWhatItCannotExpand: the two cases that stay definitions do
// stay, and say why.
func TestInlineKeepsWhatItCannotExpand(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def fact: if . <= 1 then 1 else . * (. - 1 | fact) end; fact`, "calls itself"},
		{`def f: def g: f; g; f`, "calls itself"},
		{`. as $x | def f: $x; 2 as $x | f`, "means something else"},
		{shadowedCall, "means something else"},
	} {
		r := Inline(parse(t, c.src))
		if !strings.Contains(r.Query.String(), "def ") {
			t.Errorf("%s: definition was expanded anyway:\n%s", c.src, r.Query)
		}
		if len(r.Kept) == 0 {
			t.Fatalf("%s: nothing explained why", c.src)
		}
		if !strings.Contains(strings.Join(r.Kept, "\n"), c.want) {
			t.Errorf("%s: notes %v do not mention %q", c.src, r.Kept, c.want)
		}
	}
}

// TestInlineCountsCallSites: the count is what the page reports, so it has to
// mean call sites replaced.
func TestInlineCountsCallSites(t *testing.T) {
	for _, c := range []struct {
		src  string
		want int
	}{
		{`.a`, 0},
		{`def unused: 1; .`, 0},
		{`def f: 1; f`, 1},
		{`def f: 1; f + f + f`, 3},
		{`def fact: if . <= 1 then 1 else . * (. - 1 | fact) end; fact`, 0},
	} {
		if got := Inline(parse(t, c.src)).Expanded; got != c.want {
			t.Errorf("Inline(%s).Expanded = %d, want %d", c.src, got, c.want)
		}
	}
}

// TestInlineOnQueryWithoutDefinitions leaves the query exactly as it was.
func TestInlineOnQueryWithoutDefinitions(t *testing.T) {
	for _, src := range []string{`.`, `.a | .b`, `[.[] | select(.x)] | length`, `label $out | break $out`} {
		r := Inline(parse(t, src))
		if got, want := r.Query.String(), parse(t, src).String(); got != want {
			t.Errorf("Inline(%s) = %s, want it untouched", src, got)
		}
		if r.Expanded != 0 || len(r.Kept) != 0 {
			t.Errorf("Inline(%s) reported %d expansions and %v", src, r.Expanded, r.Kept)
		}
	}
}

// TestInlineStopsBeforeItRunsAway: copying a body at every call site is
// duplication by design, and `def a: .; def b: a | a;` doubles per
// definition. Twenty-five of those is 33 million copies of `.`, which is not
// a query anything can open - so inlining stops, says so, and hands back
// something that still parses and still means the same thing.
func TestInlineStopsBeforeItRunsAway(t *testing.T) {
	var b strings.Builder
	b.WriteString("def d0: .;")
	for i := 1; i <= 25; i++ {
		fmt.Fprintf(&b, " def d%d: d%d | d%d;", i, i-1, i-1)
	}
	b.WriteString(" d25")

	done := make(chan Result, 1)
	go func() { done <- Inline(parse(t, b.String())) }()

	var r Result
	select {
	case r = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("inlining a doubling query did not finish")
	}

	out := r.Query.String()
	if len(out) > 4*maxGrowth {
		t.Errorf("the result is %d bytes, far past the %d-byte cap", len(out), maxGrowth)
	}
	if len(r.Kept) == 0 {
		t.Fatal("nothing said why the query was left part-expanded")
	}
	if !strings.Contains(strings.Join(r.Kept, "\n"), "too large") {
		t.Errorf("notes %v do not say the query got too large", r.Kept)
	}
	if _, err := gojq.Parse(out); err != nil {
		t.Errorf("the part-expanded query does not parse: %v", err)
	}
}

func TestInlineNilQuery(t *testing.T) {
	if r := Inline(nil); r.Query != nil || r.Expanded != 0 {
		t.Errorf("Inline(nil) = %+v", r)
	}
}
