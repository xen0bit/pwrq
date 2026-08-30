package astsearch

import (
	"fmt"
	"strings"
	"testing"
)

// A pattern is a claim about a node's children, and the query the engine
// compiles it to drops most of that claim: how many children there are, which
// one a hole stands for, and where the construct begins and ends. sexp.go puts
// all three back, and this file is what says so.
//
// Every case here failed before it did, and failed silently - the matches were
// well-formed and the captures populated, so nothing but the count and the
// text gave the answer away.

// arity is source with the same call written at four different lengths, which
// is what a pattern that does not say "and no others" reads wrongly.
const arity = `package main

func main() {
	g(1, 2)
	g(1, 2, 3)
	g(1, 2, 3, 4)
	g((7))
}
`

// matches runs a pattern over one Go file and returns the matches.
func matches(t *testing.T, source, pattern string) []any {
	t.Helper()
	dir := tree(t, map[string]string{"a.go": source})
	return collected(t, fmt.Sprintf(`[select_ast(%q; %q)]`, dir, pattern))
}

// TestAPatternSaysHowManyChildrenThereAre is the defect this file was written
// for. `g($A, $B)` asks for a call with two arguments, and the query the
// engine builds asks for two arguments somewhere in the list, in that order.
// Against a four-argument call that is six matches - every ordered pair - each
// binding $A and $B to different code. On pwrq's own source, `fmt.Errorf($A,
// $B)` reported 1939 matches at 1211 places.
func TestAPatternSaysHowManyChildrenThereAre(t *testing.T) {
	got := matches(t, arity, "g($A, $B)")
	if len(got) != 1 {
		t.Fatalf("`g($A, $B)` matched %d times; the source has one call with two arguments", len(got))
	}
	if line := fmt.Sprint(field(t, got[0], "LineNumber")); line != "4" {
		t.Errorf("matched line %s, want 4", line)
	}
}

// TestAnEllipsisSaysAndAnythingElse covers the other half: a pattern needs a
// way to name some of a node's children and leave the rest alone, which is
// what Semgrep spells `...` and pwrq spells $$$_.
func TestAnEllipsisSaysAndAnythingElse(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		want    []string
	}{
		// An ellipsis stands for nothing as readily as for something, which
		// is why the first two reach the one-argument call as well.
		{"g($A, $$$_)", []string{"g(1, 2)", "g(1, 2, 3)", "g(1, 2, 3, 4)", "g((7))"}},
		{"g($$$_, $A)", []string{"g(1, 2)", "g(1, 2, 3)", "g(1, 2, 3, 4)", "g((7))"}},
		{"g($A, $$$_, $B)", []string{"g(1, 2)", "g(1, 2, 3)", "g(1, 2, 3, 4)"}},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			var got []string
			for _, m := range matches(t, arity, tc.pattern) {
				got = append(got, fmt.Sprint(field(t, m, "Text")))
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("matched %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAHoleBindsTheNodeItWasWrittenFor covers the fold: with one hole in a
// list, the engine binds the list rather than the hole, so `g($A)` matched a
// call with any number of arguments and .Captures.A read "(1, 2, 3)" -
// parentheses and all. Every rule that filters on what a hole caught depends
// on this.
func TestAHoleBindsTheNodeItWasWrittenFor(t *testing.T) {
	got := matches(t, arity, "g($A)")
	if len(got) != 1 {
		t.Fatalf("`g($A)` matched %d times; the source has one call with one argument", len(got))
	}
	if a := fmt.Sprint(field(t, got[0], "Captures").(map[string]any)["A"]); a != "(7)" {
		t.Errorf("$A caught %q, want %q - the argument, not the list holding it", a, "(7)")
	}
}

// TestAMatchSpansTheConstruct covers the third: a match's span was the union
// of its captures, so a literal's closing brace fell outside it and a pattern
// with no holes at all had no span to report. Offset and EndOffset are what
// `within` is built on, so a short span is a rule that silently stops nesting.
func TestAMatchSpansTheConstruct(t *testing.T) {
	const source = `package main

import "crypto/tls"

func main() {
	_ = &tls.Config{MinVersion: 1, InsecureSkipVerify: true}
}
`
	got := matches(t, source, "tls.Config{$$$_, InsecureSkipVerify: true, $$$_}")
	if len(got) != 1 {
		t.Fatalf("matched %d times, want 1", len(got))
	}
	want := "tls.Config{MinVersion: 1, InsecureSkipVerify: true}"
	if text := fmt.Sprint(field(t, got[0], "Text")); text != want {
		t.Errorf("match text is %q, want %q", text, want)
	}
}

// TestTheEngineOwnCapturesAreNotReported checks that the two captures the
// rewrite adds for its own use stay out of a caller's way.
func TestTheEngineOwnCapturesAreNotReported(t *testing.T) {
	got := matches(t, arity, "g($A, $$$_)")
	for _, m := range got {
		captures := field(t, m, "Captures").(map[string]any)
		for _, name := range []string{rootCapture, ellipsisName} {
			if _, ok := captures[name]; ok {
				t.Errorf("match reports a capture named %q, which nobody wrote", name)
			}
		}
	}
}

// TestANamedVariadicBesideSiblingsIsRefused covers the one shape that cannot
// be honoured. $$$NAME alone in a list binds the list; beside a sibling the
// engine compiles it to a single node, so `f($A, $$$REST)` quietly means "two
// arguments" and $REST holds the second one.
func TestANamedVariadicBesideSiblingsIsRefused(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		refused bool
	}{
		{"f($A, $$$REST)", true},
		{"f($$$REST, $A)", true},
		{"f($A, $$$_)", false},
		{"f($$$REST)", false},
		{"func $N($$$P) error { $$$B }", false},
		{"for $$$H { $$$B }", false},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			c, err := compilePattern(tc.pattern, "go")
			if err != nil {
				t.Fatal(err)
			}
			if c.valid() == tc.refused {
				t.Errorf("valid=%v, want %v; problem was %q", c.valid(), !tc.refused, c.problem)
			}
			if tc.refused && !strings.Contains(c.problem, "$$$_") {
				t.Errorf("the refusal does not name the form that works: %q", c.problem)
			}
		})
	}
}

// TestQueryTextSurvivesARoundTrip checks the parser against the shapes the
// engine actually emits. A query it cannot read is returned unchanged, so a
// parser that quietly loses a piece would leave the pattern working and every
// fix above silently off.
func TestQueryTextSurvivesARoundTrip(t *testing.T) {
	for _, sexp := range []string{
		`(call_expression function: (identifier) @_lit_1 arguments: (_) @P)`,
		`(binary_expression left: (_) @A operator: "+" right: (_) @B)`,
		`(composite_literal type: (qualified_type package: (package_identifier) @_lit_1))
(#eq? @_lit_1 "tls")`,
		`(argument_list . (_) @A . (_) @B .)`,
	} {
		t.Run(sexp, func(t *testing.T) {
			forms, err := parseSexp(sexp)
			if err != nil {
				t.Fatalf("cannot read a query the engine produced: %v", err)
			}
			var parts []string
			for _, f := range forms {
				parts = append(parts, f.render())
			}
			// Anchors are dropped on the way in and re-derived, so they are
			// compared without them.
			got := strings.ReplaceAll(strings.Join(parts, "\n"), ". ", "")
			want := strings.ReplaceAll(strings.ReplaceAll(sexp, ". ", ""), " .)", ")")
			if got != strings.ReplaceAll(want, " .)", ")") {
				t.Errorf("round trip changed the query:\n got %s\nwant %s", got, want)
			}
		})
	}
}

// TestACPatternIsReadAsAStatement covers the language where the same
// characters mean two things. `gets($BUF)` where a C file begins is a
// declaration - `gets` the type, `($BUF)` the declarator - so the query
// compiled, reported no problem, and matched nothing in any C ever written.
func TestACPatternIsReadAsAStatement(t *testing.T) {
	const source = `#include <stdio.h>

void read_name(char *dst)
{
    char buf[64];
    gets(buf);
    strcpy(dst, buf);
}
`
	dir := tree(t, map[string]string{"a.c": source})
	for _, tc := range []struct {
		pattern string
		want    string
	}{
		{"gets($BUF)", "gets(buf)"},
		{"strcpy($D, $S)", "strcpy(dst, buf)"},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			got := collected(t, fmt.Sprintf(`[select_ast(%q; %q)]`, dir, tc.pattern))
			if len(got) != 1 {
				t.Fatalf("matched %d times, want 1", len(got))
			}
			if text := fmt.Sprint(field(t, got[0], "Text")); text != tc.want {
				t.Errorf("matched %q, want %q", text, tc.want)
			}
		})
	}
}

// TestAFragmentCIsRefusedRatherThanRescued is the control on that. A bare
// `$A + $B` is not a C program in any context, and reading it as a statement
// makes the grammar produce something - an update_expression with an empty
// operator - rather than nothing. The terminated reading is only reached for a
// pattern that already parses, so the refusal stands.
func TestAFragmentCIsRefusedRatherThanRescued(t *testing.T) {
	c, err := compilePattern("$A + $B", "c")
	if err != nil {
		t.Fatal(err)
	}
	if c.valid() {
		t.Errorf("`$A + $B` was accepted for c, compiling to %s", c.query().SExpr)
	}
}
