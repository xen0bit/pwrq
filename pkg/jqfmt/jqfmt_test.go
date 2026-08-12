package jqfmt

import (
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

// cases are queries the formatter has to survive. They cover every node type
// the renderer spreads across lines, plus the ones that never spread, so the
// property checks below are exercised over the whole grammar rather than the
// handful of examples a reader would write by hand.
var cases = []string{
	// A bare term and a short pipeline: nothing to spread.
	`.`,
	`.a[0:3]`,
	`"a1 b22 c333" | [scan("[0-9]+")]`,
	`map_values(select(type == "number") | . * 2)`,
	// The pipeline that started this: every stage its own line, the final
	// object filled and the parenthesised sub-pipeline left whole.
	`. as $all
	 | ([rncd_compare($all)] | sort_by(.Hybrid) | .[0]) as $closest
	 | shared_chunks($all[$closest.IndexB]; $all[$closest.IndexA])
	 | {match: [$closest.NameA, $closest.NameB],
	    hybrid: $closest.Hybrid, coverage: .Coverage,
	    tampered_bytes: ([.Chunks[] | select(.Matched | not) | .Length] | add),
	    tampered_regions: [.Chunks[] | select(.Matched | not)
	                        | {start: .Start, length: .Length}]}`,
	// Objects: fill when every entry is single-line, one-per-line when an
	// entry spreads, and the empty form.
	`{names: (map(.Name) | {duplicated: contains_duplicates, distinct: (dedupe | length)}), versions: (map(.Version) | {all_same: all_equal, distinct: dedupe})}`,
	`{a: {b: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20]}}`,
	`{($label): .}`,
	`{ }`,
	// Arrays: a short body inline, a pipeline body hanging, a comma list
	// spreading, and the empty form.
	`[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20]`,
	`[.Chunks[] | select(.Matched | not) | select(.Length > 5) | {start: .Start, length: .Length, offset: .RefOffset}]`,
	`[]`,
	// Conditionals, both compact and spread.
	`if .age >= 18 then "adult" elif .age >= 13 then "teen" else "minor" end`,
	`if .status >= 500 then "critical: " + .message elif .status >= 400 then "warning: " + .message elif .status >= 300 then "redirect: " + .message else "ok" end`,
	// The loop forms.
	`reduce .[] as $x ({}; .[$x.name] = $x.v)`,
	`foreach [1, 2, 3, 4, 5, 6, 7, 8, 9, 10][] as $x (0; . + $x; if . > 100 then "big" else . end)`,
	// try/catch, labels and breaks.
	`try .a catch "nope"`,
	`label $out | [.[] | if . then . | break $out else empty end]`,
	// Operators that must never be rewritten.
	`.foo |= . + 1`,
	`.a, .b, .c`,
	`[1] | add`,
	`([1] | add)`,
	`.x // "default"`,
	`.a.b.c[]?.d[0:2]`,
	`.foo[] |= (select(.bar) | .baz)`,
	`."quoted.key"[0:1]`,
	`{key: ("a" + "b"), "str-key": 1, ($k): 2, "interp-\(.x)": 3}`,
	`first(.[] | select(.enabled)) // null`,
	// Definitions and a string with interpolation.
	`def bytes($p): hex_encode($p; true) | hex_decode; bytes("/usr/bin/mv") | shared_chunks(bytes("/usr/bin/cp")) | {Coverage, MatchedBytes, Spans}`,
	`"\(.name) is \(.age)"`,
	`@base64`,
	`walk(if type == "string" then ascii_upcase else . end)`,
	`[range(1; 100) | select(. % 7 == 0)]`,
	`{a: null, b: true, c: false}`,
	`{a: {b: 1}} | .a.b`,
}

// parse is a test helper: a case that does not parse is a bug in the list.
func parse(t *testing.T, src string) *gojq.Query {
	t.Helper()
	q, err := gojq.Parse(src)
	if err != nil {
		t.Fatalf("case does not parse: %v\n%s", err, src)
	}
	return q
}

// TestFormatRoundTrips is the meaning-preservation property: formatting must
// not change what the query does. The canonical String form of a reparse
// equals the original's String form, which proves the same parse tree came
// back — parens neither gained nor lost.
func TestFormatRoundTrips(t *testing.T) {
	for _, src := range cases {
		got := Format(parse(t, src))
		reparsed := parse(t, got)
		if reparsed.String() != parse(t, src).String() {
			t.Errorf("formatting changed the query\n--- input:\n%s\n--- output:\n%s", src, got)
		}
	}
}

// TestFormatIsIdempotent is what makes Format safe to press repeatedly: a
// second pass over its own output changes nothing.
func TestFormatIsIdempotent(t *testing.T) {
	for _, src := range cases {
		once := Format(parse(t, src))
		twice := Format(parse(t, once))
		if once != twice {
			t.Errorf("formatting is not idempotent\n--- first:\n%s\n--- second:\n%s", once, twice)
		}
	}
}

// TestFormatSpreadsPipelines pins the layout that motivated Format: a
// top-level pipeline puts each stage on its own line.
func TestFormatSpreadsPipelines(t *testing.T) {
	got := Format(parse(t, `.a | .b | .c`))
	if got != ".a\n| .b\n| .c" {
		t.Errorf("got:\n%s", got)
	}
}

// TestFormatKeepsShortThingsInline: nothing about a query that already fits
// on one line should change, which is what makes the button idempotent for
// the common case. A top-level pipeline is the deliberate exception - those
// spread by rule - so it is not in this list.
func TestFormatKeepsShortThingsInline(t *testing.T) {
	for _, src := range []string{`.a`, `{a: 1}`, `[1, 2]`, `if .a then .b else .c end`} {
		if got := Format(parse(t, src)); got != src {
			t.Errorf("short query %q was spread:\n%s", src, got)
		}
	}
}

// TestFormatHangsArrayPipelines pins the one place a nested pipeline spreads:
// an array whose body is a pipeline hangs its continuation stages under the
// opening bracket rather than flattening them.
func TestFormatHangsArrayPipelines(t *testing.T) {
	got := Format(parse(t, `[.Chunks[] | select(.Matched | not) | select(.Length > 5) | {start: .Start, length: .Length, offset: .RefOffset}]`))
	want := "[.Chunks[]\n  | select(.Matched | not)\n  | select(.Length > 5)\n  | {start: .Start, length: .Length, offset: .RefOffset}]"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatFillsObjects pins the object layout: entries run across lines
// with a trailing comma, and a continuation line aligns under the opening
// brace.
func TestFormatFillsObjects(t *testing.T) {
	src := `{first: 1, second: 2, third: 3, fourth: 4, fifth: 5, sixth: 6, seventh: 7, eighth: 8, ninth: 9, tenth: 10, eleventh: 11, twelfth: 12}`
	got := Format(parse(t, src))
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("object was not filled:\n%s", got)
	}
	if !strings.HasPrefix(lines[0], "{first: 1") || !strings.HasSuffix(lines[0], ",") {
		t.Errorf("first fill line is %q, want entries with a trailing comma", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  ") {
		t.Errorf("continuation line %q is not indented under the brace", lines[1])
	}
	if !strings.HasSuffix(lines[len(lines)-1], "}") {
		t.Errorf("last line %q does not close the object", lines[len(lines)-1])
	}
}

// TestFormatBreaksIfWhenItDoesNotFit pins the conditional layout.
func TestFormatBreaksIfWhenItDoesNotFit(t *testing.T) {
	src := `if .status >= 500 then "critical: " + .message elif .status >= 400 then "warning: " + .message else "ok" end`
	got := Format(parse(t, src))
	want := "if .status >= 500 then\n  \"critical: \" + .message\nelif .status >= 400 then\n  \"warning: \" + .message\nelse\n  \"ok\"\nend"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestMinifyMatchesCanonical: Minify is the existing single-line behaviour,
// so it has to equal the parser's own rendering exactly.
func TestMinifyMatchesCanonical(t *testing.T) {
	for _, src := range cases {
		q := parse(t, src)
		if got := Minify(q); got != q.String() {
			t.Errorf("Minify(%q) = %q, want %q", src, got, q.String())
		}
	}
}
