package astsearch

import (
	"fmt"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
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

// TestAFragmentCIsReadWhereItCouldStand is the control on that. A bare `$A +
// $B` is not a C program, and there are two ways to make the grammar accept
// it: put it where an expression can stand, or let the grammar repair it. The
// first is the pattern - an addition, anywhere one is written - and the second
// is an update_expression whose `++` the grammar invented and the caller never
// wrote, which would match `a++` and not `a + b`.
//
// So the reading has to be the addition, and the repair has to be refused. A
// pattern is only ever read as something the caller could have typed.
func TestAFragmentCIsReadWhereItCouldStand(t *testing.T) {
	c, err := compilePattern("$A + $B", "c")
	if err != nil {
		t.Fatal(err)
	}
	if !c.valid() {
		t.Fatalf("`$A + $B` was refused for c: %s", c.problem)
	}
	if got := c.query().SExpr; !strings.Contains(got, "binary_expression") ||
		strings.Contains(got, "update_expression") {
		t.Errorf("`$A + $B` compiled to %s, want the addition it spells", got)
	}
}

// sameness is source where the same comparison is written both ways, which is
// what a pattern with one name on two holes cannot tell apart.
const sameness = `package main

func f(a, b int) bool {
	if a == a {
		return true
	}
	if a == b {
		return false
	}
	return b == b
}
`

// TestAHoleWrittenTwiceMeansTheSameCodeTwice covers the pattern the corpus
// this was ported against writes most often after a plain call: a name used in
// two places, meaning the same code in both.
//
// A tree-sitter query with one capture name on two nodes does not require the
// two to be equal - it keeps whichever it bound last - so `$X == $X` used to
// be refused outright, and a caller had to name the holes apart and compare
// the captures in a later stage. The query says it directly with a predicate,
// and now does.
func TestAHoleWrittenTwiceMeansTheSameCodeTwice(t *testing.T) {
	var got []string
	for _, m := range matches(t, sameness, "$X == $X") {
		got = append(got, fmt.Sprint(field(t, m, "Text")))
	}
	want := []string{"a == a", "b == b"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("`$X == $X` matched %v, want %v; `a == b` is not a comparison of one thing "+
			"with itself", got, want)
	}
}

// TestARepeatedHoleIsNotReportedTwice keeps the rewrite out of the caller's
// results: the second half of a repeated hole is a capture nobody wrote.
func TestARepeatedHoleIsNotReportedTwice(t *testing.T) {
	got := matches(t, sameness, "$X == $X")
	if len(got) == 0 {
		t.Fatal("no matches to read captures from")
	}
	captures, ok := field(t, got[0], "Captures").(map[string]any)
	if !ok {
		t.Fatalf("Captures is %T, want an object", field(t, got[0], "Captures"))
	}
	if len(captures) != 1 || captures["X"] != "a" {
		t.Errorf("captures are %v, want just X bound to a", captures)
	}
}

// setThenUsed is source where the same variable is set and then used, once with the
// call the pattern names and once with another.
const setThenUsed = `import os

def handler(request):
    name = request.args
    log("hello")
    foo(name)
    return name

def other(request):
    x = request.args
    return bar(x)
`

// TestAPatternOfSeveralStatementsMatchesInsideAnyBlock is the second half of
// the same defect, and the reason the first was not enough.
//
// A pattern of more than one statement has no single node to compile to, so
// the engine gives it the node the grammar wraps a whole file in. Read
// literally that query says "a file whose first statement is this and whose
// last is that", and the statements it is looking for are nearly always in a
// function body - a different node - so it matched nothing anywhere, silently.
// See openUnit.
func TestAPatternOfSeveralStatementsMatchesInsideAnyBlock(t *testing.T) {
	dir := tree(t, map[string]string{"v.py": setThenUsed})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; %q)]`, dir, `$D = request.args
$$$_
foo($D)`))
	if len(got) != 1 {
		t.Fatalf("matched %d times, want 1: the second function assigns request.args too, "+
			"but passes it to bar", len(got))
	}
	captures, _ := field(t, got[0], "Captures").(map[string]any)
	if captures["D"] != "name" {
		t.Errorf("$D caught %v, want name", captures["D"])
	}
}

// TestASequenceSpansTheStatementsItNamed says where such a match starts and
// stops. Capturing the node the statements share would report the whole
// enclosing block; the span runs from the first statement to the last.
func TestASequenceSpansTheStatementsItNamed(t *testing.T) {
	dir := tree(t, map[string]string{"v.py": setThenUsed})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; %q)]`, dir, `$D = request.args
$$$_
foo($D)`))
	if len(got) != 1 {
		t.Fatalf("matched %d times, want 1", len(got))
	}
	if line := fmt.Sprint(field(t, got[0], "LineNumber")); line != "4" {
		t.Errorf("starts at line %s, want 4", line)
	}
	if line := fmt.Sprint(field(t, got[0], "EndLineNumber")); line != "6" {
		t.Errorf("ends at line %s, want 6", line)
	}
	if text := fmt.Sprint(field(t, got[0], "Text")); !strings.HasPrefix(text, "name = request.args") ||
		!strings.HasSuffix(text, "foo(name)") {
		t.Errorf("matched text is %q, want the three statements the pattern named", text)
	}
}

// terraform is a file of the shape a quarter of the rule corpus is written
// against: resources whose bodies the pattern does not want to enumerate.
const terraform = `resource "aws_lambda_function" "encrypted" {
  function_name = "ok"
  kms_key_arn   = aws_kms_key.a.arn
  environment {
    variables = { A = "1" }
  }
}

resource "aws_lambda_function" "plain" {
  function_name = "bad"
  environment {
    variables = { B = "2" }
  }
}

resource "aws_s3_bucket" "b" {
  bucket = "x"
}
`

// hclMatches runs a pattern over one Terraform file.
func hclMatches(t *testing.T, pattern string) []any {
	t.Helper()
	dir := tree(t, map[string]string{"main.tf": terraform})
	return collected(t, fmt.Sprintf(`[select_ast(%q; %q)]`, dir, pattern))
}

// TestAnEllipsisStandsWhereAnItemGoes is the defect that kept every Terraform
// rule in the corpus from doing anything.
//
// `...` inside a block body becomes an identifier, and an HCL body holds
// attributes and blocks - an identifier is neither, so the whole pattern was
// an ERROR parse. The error node then carried HCL's significant whitespace as
// literals, so the query text ended up with a raw newline inside a string and
// would not parse as a query at all. What the caller was told about was the
// newline. See bodyReading.
func TestAnEllipsisStandsWhereAnItemGoes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pattern string
		want    []string
	}{
		{"a body the pattern does not enumerate", "resource \"aws_lambda_function\" $A {\n  $$$_\n}",
			[]string{"1", "9"}},
		{"an ellipsis between braces on one line", "environment { $$$_ }", []string{"4", "11"}},
		{"an item named among items that are not", "resource $A $B {\n  $$$_\n  kms_key_arn = $$$_\n  $$$_\n}",
			[]string{"1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := hclMatches(t, tc.pattern)
			var lines []string
			for _, m := range got {
				lines = append(lines, fmt.Sprint(field(t, m, "LineNumber")))
			}
			if strings.Join(lines, ",") != strings.Join(tc.want, ",") {
				t.Errorf("%s\n  matched lines %v\n  want          %v", tc.pattern, lines, tc.want)
			}
		})
	}
}

// java is source where a rule's three-statement pattern has somewhere to be.
// The two cookies differ only in the argument to setSecure, which is what a
// pattern of statements in sequence is written to tell apart.
const java = `class Session {
  void open(HttpServletResponse resp) {
    Cookie a, b;

    a = new Cookie("a", "1");
    a.setPath("/");
    a.setSecure(false);
    resp.addCookie(a);

    b = new Cookie("b", "2");
    b.setSecure(true);
    resp.addCookie(b);
  }
}
`

// TestAPatternIsReadWhereItCouldBeWritten is the wrapper half of a scaffolded
// reading, and it is what a quarter of a rule corpus is written as:
//
//	$COOKIE = new Cookie(...);
//	...
//	$COOKIE.setSecure(false);
//
// Java has nowhere to put a statement outside a method, so as written this is
// an ERROR parse and every rule shaped like it found nothing. Compiled inside
// a method it is three statements in a body, which is what it says - and the
// method has to be walked back off afterwards, or the pattern would be a claim
// that the code sits in a method with that name and that return type.
//
// The `...` between the statements has to survive that: it says the two need
// not be adjacent, and the run it stands for is the second cookie's setPath.
func TestAPatternIsReadWhereItCouldBeWritten(t *testing.T) {
	dir := tree(t, map[string]string{"S.java": java})
	pattern := "$C = new Cookie($$$_);\n$$$_\n$C.setSecure(false);"
	got := collected(t, fmt.Sprintf(`[select_ast(%q; %q)]`, dir, pattern))
	if len(got) != 1 {
		t.Fatalf("matched %d times, want 1 - the cookie left insecure", len(got))
	}
	if line := fmt.Sprint(field(t, got[0], "LineNumber")); line != "5" {
		t.Errorf("matched at line %s, want 5", line)
	}
}

// TestAnEllipsisDoesNotTakeTheStatementBelowIt is the check on a reading that
// parses.
//
// Java reads `...` on a line of its own as an identifier, and an identifier
// followed by `HttpServletRequest $REQ = ...;` as one declaration - the
// ellipsis becomes the type and the pattern's own statement becomes the name.
// The query compiles, reports no error, and is a search for a declaration
// whose name is the word HttpServletRequest, which no Java has. Every rule
// written this way found nothing and said nothing.
//
// What settles it is what the ellipsis means: nothing in particular is there.
// So the pattern with the line struck out has to compile to the same query.
func TestAnEllipsisDoesNotTakeTheStatementBelowIt(t *testing.T) {
	pattern := "$X $M($$$_) {\n  $$$_\n  HttpServletRequest $REQ = $$$_;\n  $$$_\n}"
	c, err := compilePattern(pattern, "java")
	if err != nil {
		t.Fatal(err)
	}
	if !c.valid() {
		t.Fatalf("refused: %s", c.problem)
	}
	for i, q := range c.queries {
		if strings.Contains(q.SExpr, `name: (identifier) @_lit`) {
			t.Errorf("reading %d read the ellipsis as the declaration's type:\n%s", i, q.SExpr)
		}
	}
	dir := tree(t, map[string]string{"S.java": java})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; %q)]`, dir,
		"$X $M($$$_) {\n  $$$_\n  Cookie $A, $B;\n  $$$_\n}"))
	if len(got) != 1 {
		t.Fatalf("matched %d times, want the one method", len(got))
	}
}

// TestAQueryOfNothingIsRefused is the inverse of the failure this package
// exists to prevent, and the worse half of it.
//
// `return ...` followed by a statement is how a rule says "code after a
// return". Python folds the two lines into one hole, and the query becomes
// `(_) @S` - every node in every file. The pattern parses, reports no problem,
// and the rule built on it fires on a file with no return statement in it at
// all. A search that finds nothing is a rule that says nothing; a search that
// finds everything is a rule that is wrong.
func TestAQueryOfNothingIsRefused(t *testing.T) {
	c, err := compilePattern("return $$$_\n$S", "python")
	if err != nil {
		t.Fatal(err)
	}
	if c.valid() {
		t.Errorf("`return $$$_\\n$S` was accepted for python, compiling to %s", c.query().SExpr)
	}
	// The half of the same pattern that does say something still does.
	ok, err := compilePattern("return $$$_", "python")
	if err != nil {
		t.Fatal(err)
	}
	if !ok.valid() {
		t.Errorf("`return $$$_` was refused for python: %s", ok.problem)
	}
}

// TestAPatternsOwnBracketsAreNotScaffolding is the other half of finding the
// file node, and the reason it is measured by doubling the pattern rather than
// by parsing an empty file.
//
// `[$X]` compiles to a list holding one thing, which has exactly the shape a
// grammar's file node has - one child, wrapping what the caller wrote. Opening
// it would turn "a list of one element" into "anything containing $X". The
// difference is that doubling the pattern gives a file node two children and
// leaves a list with one.
func TestAPatternsOwnBracketsAreNotScaffolding(t *testing.T) {
	dir := tree(t, map[string]string{"a.py": "x = [1]\ny = 1\n"})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; %q)]`, dir, "[$X]"))
	if len(got) != 1 {
		t.Fatalf("`[$X]` matched %d times; the source has one list", len(got))
	}
	if text := fmt.Sprint(field(t, got[0], "Text")); text != "[1]" {
		t.Errorf("matched %q, want the list itself", text)
	}
}

// TestASubscriptWithALiteralOnTheLeftSpansTheWholeSubscript is the shape that
// showed the scaffold test could not tell a construct from the wrapper around
// one. `subscript_expression` appears in the scaffold chain, so `table[$I]`
// was opened up as though it were a list of statements: the head became `_`,
// the form lost its capture, and the span fell back to the union of the holes,
// which stops at the index and leaves the closing bracket out.
//
// `$A[$I]` was always right. That is what makes it a good test - the two
// spellings of one pattern have to agree, and the wrong one reported the same
// line, the same captures and a span one byte short, which nothing about the
// match said out loud.
func TestASubscriptWithALiteralOnTheLeftSpansTheWholeSubscript(t *testing.T) {
	dir := tree(t, map[string]string{"a.c": "int main(void) { table[3] = 1; }\n"})
	for _, pattern := range []string{"table[$I]", "$A[$I]"} {
		t.Run(pattern, func(t *testing.T) {
			got := collected(t, fmt.Sprintf(`[select_ast(%q; %q)]`, dir, pattern))
			if len(got) != 1 {
				t.Fatalf("matched %d times, want 1", len(got))
			}
			if text := fmt.Sprint(field(t, got[0], "Text")); text != "table[3]" {
				t.Errorf("matched %q, want %q - a span that stops short of the "+
					"bracket is what within, outside and not_at then compare",
					text, "table[3]")
			}
		})
	}
}

// TestAJavaDeclarationIsReadWhereverOneCanStand is the Java half of what
// TestACPatternIsReadAsAStatement is for C: the same characters mean two things
// and the grammar names them differently.
//
// `String $F = $E;` parses standing alone, because tree-sitter-java takes a
// bare statement at the top of a file, so it compiles cleanly on the first
// reading to a local_variable_declaration and no scaffolded reading is ever
// reached for. The identical text inside a class is a field_declaration and
// inside an interface a constant_declaration - three node types, one construct
// - and a query for one matches neither of the others.
//
// The cost was not a rule that fired in the wrong place. It was a rule nobody
// could write: `private static final String PASSWORD = "hunter2";` is the shape
// every hardcoded-credential rule is about, and no pattern could name it.
func TestAJavaDeclarationIsReadWhereverOneCanStand(t *testing.T) {
	const source = `package t;

class Secrets {
    private static final String PASSWORD = "hunter2";
    String instance = "b";

    void run() {
        String local = "c";
        System.out.println(local);
    }
}

interface Names {
    String CONSTANT = "d";
}
`
	dir := tree(t, map[string]string{"Secrets.java": source})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; %q) | .Captures.F]`, dir, "$T $F = $E;"))
	want := []string{"PASSWORD", "instance", "local", "CONSTANT"}
	if len(got) != len(want) {
		t.Fatalf("matched %d declarations %v, want %d %v", len(got), got, len(want), want)
	}
	for i, name := range want {
		if fmt.Sprint(got[i]) != name {
			t.Errorf("match %d bound %v, want %q", i, got[i], name)
		}
	}
}

// TestAModifiedJavaFieldKeepsItsTextComparison is the half that the capture
// numbering could quietly break. The second reading takes only the node type
// from the grammar and every other part of the query from the first reading,
// so a pattern that compares text has to still compare it - and has to compare
// the right text, not the text some other reading of it counted from.
func TestAModifiedJavaFieldKeepsItsTextComparison(t *testing.T) {
	const source = `package t;

class Secrets {
    private static final String PASSWORD = "hunter2";
    public String other = "b";
}
`
	dir := tree(t, map[string]string{"Secrets.java": source})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; %q) | .Captures.F]`,
		dir, "private static final $T $F = $E;"))
	if len(got) != 1 || fmt.Sprint(got[0]) != "PASSWORD" {
		t.Fatalf("matched %v, want [PASSWORD] - the modifiers are compared as text and only one "+
			"field has those", got)
	}
}

// TestAJavaCallIsNotReadAsAMember is the control. A grammar that cannot read
// the pattern inside a class has not given it a second name, and the reading
// that comes back from a failed parse - an ERROR node, whose type is a name
// like any other - must not be taken for one.
func TestAJavaCallIsNotReadAsAMember(t *testing.T) {
	c, err := compilePattern("foo($X);", "java")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.queries) != 1 {
		t.Errorf("a call compiled to %d readings, want 1:\n%v", len(c.queries), readingHeads(c))
	}
}

// readingHeads is the node type each reading is rooted at, for a failure
// message that says which readings were made rather than how many.
func readingHeads(c *compiled) []string {
	var out []string
	for _, q := range c.queries {
		head, _, ok := splitHead(q.SExpr)
		if !ok {
			head = q.SExpr
		}
		out = append(out, head)
	}
	return out
}

// The C# grammar wraps every statement that stands at the top of a file in a
// node of its own, so a pattern read there anchors to something no file with a
// class in it contains. See bodyReadings.
func TestACSharpStatementIsReadInsideAMethod(t *testing.T) {
	const source = `using System;

namespace Demo {
    public class Greeter {
        public void Run(string who) {
            Console.WriteLine(who);
        }
    }
}
`
	dir := tree(t, map[string]string{"Greeter.cs": source})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; %q) | .LineNumber]`, dir, "Console.WriteLine($X);"))
	if len(got) != 1 {
		t.Fatalf("a call inside a method: got %v, want the one on line 6", got)
	}
}

// And the reading it had before is still one of its readings: a file that is
// nothing but statements is C# too, and the rule should still find it.
func TestACSharpStatementIsStillReadAtTheTopOfAFile(t *testing.T) {
	dir := tree(t, map[string]string{"Program.cs": "using System;\nConsole.WriteLine(who);\n"})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; %q) | .LineNumber]`, dir, "Console.WriteLine($X);"))
	if len(got) != 1 {
		t.Fatalf("a top-level statement: got %v, want the one on line 2", got)
	}
}

// A declaration is reached wherever one can stand, which in C# is the same
// reading in both places - a field and a local both hold a variable_declaration
// - rather than the three node types Java has. See TestAJavaDeclarationIsRead.
func TestACSharpDeclarationIsReadWhereverOneCanStand(t *testing.T) {
	const source = `class Secrets {
    private const string PASSWORD = "hunter2";

    void Run() {
        string local = "c";
        Use(local);
    }
}
`
	dir := tree(t, map[string]string{"Secrets.cs": source})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; %q) | .Captures.V]`, dir, "$T $V = $E;"))
	want := []string{"PASSWORD", "local"}
	if len(got) != len(want) {
		t.Fatalf("declarations: got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("declaration %d: got %v, want %q", i, got[i], w)
		}
	}
}

// The gate is the whole reason this is affordable, and the reason it is safe:
// every other grammar in the build reads a statement standing alone as the same
// thing it reads one inside a body, and must be left alone. JavaScript is the
// one that a containment test rather than an exact-wrapper test got wrong.
func TestOnlyCSharpWrapsAStatementThatStandsAlone(t *testing.T) {
	var wrapped []string
	for _, name := range []string{
		"csharp", "java", "python", "go", "ruby", "javascript", "typescript",
		"c", "cpp", "php", "kotlin", "scala", "rust", "swift", "solidity",
	} {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			continue
		}
		if wrapsTopLevelStatements(entry.Language(), name) != "" {
			wrapped = append(wrapped, name)
		}
	}
	if len(wrapped) != 1 || wrapped[0] != "csharp" {
		t.Fatalf("grammars that wrap a top-level statement: got %v, want [csharp]", wrapped)
	}
}

// A pattern the grammar did not wrap is left alone, which is what keeps this
// off every C# pattern that is already a declaration. The reading it does have
// besides the class one is statementReading's and predates this.
func TestACSharpClassIsNotReadAsAStatement(t *testing.T) {
	c, err := compilePattern("class $C { $$$B }", "csharp")
	if err != nil {
		t.Fatal(err)
	}
	for _, head := range readingHeads(c) {
		if head == "global_statement" {
			t.Fatalf("a class declaration was read as a top-level statement: %v", readingHeads(c))
		}
	}
	dir := tree(t, map[string]string{"C.cs": "namespace N {\n  class Greeter { void Run() {} }\n}\n"})
	got := collected(t, fmt.Sprintf(`[select_ast(%q; %q) | .Captures.C]`, dir, "class $C { $$$B }"))
	if len(got) != 1 || got[0] != "Greeter" {
		t.Fatalf("the class: got %v, want [Greeter]", got)
	}
}

// Anchoring descends past the nodes a grammar wraps a construct in, and it has
// no way to tell those from the nodes that say something. In C# it descended
// past two that do, turning a pattern about a `throw` into one about every
// `new` in the program - a wrong answer, where the reading it replaced merely
// found nothing. See startsWhereThePatternDoes.
func TestACSharpKeywordIsNotDroppedFromAReading(t *testing.T) {
	const source = `class T {
    void Run(string q) {
        var made = new Widget(q);
        using (var s = Open(q)) { Use(s); }
    }
}
`
	dir := tree(t, map[string]string{"T.cs": source})
	// The throw matches nothing here, and the point is that it matches nothing
	// rather than every `new` in the file.
	got := collected(t, fmt.Sprintf(`[select_ast(%q; %q) | .LineNumber]`, dir, "throw new $E($$$_);"))
	if len(got) != 0 {
		t.Fatalf("throw: got %v, want nothing - the keyword was dropped", got)
	}
	// The using does match, and on the using rather than on the declaration
	// inside it, which is the same mistake the other way round.
	got = collected(t, fmt.Sprintf(`[select_ast(%q; %q) | .Text]`, dir, "using ($T $V = $E) { $$$_ }"))
	if len(got) != 1 || !strings.HasPrefix(fmt.Sprint(got[0]), "using (") {
		t.Fatalf("using: got %v, want the one using statement", got)
	}
}
