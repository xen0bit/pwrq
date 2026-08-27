package queryrun

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"
)

// run evaluates a request against plain jq. The vocabulary a host adds is its
// own business; what this package owes every host is jq's semantics, and that
// is what these tests pin.
func run(t *testing.T, req *Request) Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return (&Runner{}).Run(ctx, req)
}

func wantValues(t *testing.T, res Result, want ...string) {
	t.Helper()
	if res.Error != "" {
		t.Fatalf("unexpected error (%s): %s", res.Kind, res.Error)
	}
	if len(res.Values) != len(want) {
		t.Fatalf("got %d values %q, want %d %q", len(res.Values), res.Values, len(want), want)
	}
	for i := range want {
		if res.Values[i] != want[i] {
			t.Fatalf("value %d = %q, want %q", i, res.Values[i], want[i])
		}
	}
	if res.Count != len(want) {
		t.Fatalf("Count = %d, want %d", res.Count, len(want))
	}
}

// TestInputModes pins the jq flags. Each case is what the command line would
// print for the same input and program.
func TestInputModes(t *testing.T) {
	cases := []struct {
		name string
		req  Request
		want []string
	}{
		{
			// A stream of values is many inputs, not a syntax error.
			name: "stream",
			req:  Request{Query: ".a", Input: `{"a":1} {"a":2}`, Compact: true},
			want: []string{"1", "2"},
		},
		{
			name: "slurp",
			req:  Request{Query: ".", Input: "1 2 3", Slurp: true, Compact: true},
			want: []string{"[1,2,3]"},
		},
		{
			// -n runs the program once on null, and the input stays readable.
			name: "null input",
			req:  Request{Query: "[., inputs]", Input: "1 2", NullInput: true, Compact: true},
			want: []string{"[null,1,2]"},
		},
		{
			// -n -s: `inputs` sees the slurped array as the one value it is.
			name: "null input slurped",
			req:  Request{Query: "[inputs]", Input: "1 2", NullInput: true, Slurp: true, Compact: true},
			want: []string{"[[1,2]]"},
		},
		{
			// `input` draws from the same cursor the program is walking, so
			// the value it consumed does not come back as a second run.
			name: "input builtin",
			req:  Request{Query: "[., input]", Input: "1 2", Compact: true},
			want: []string{"[1,2]"},
		},
		{
			name: "raw input",
			req:  Request{Query: ".", Input: "a\nb\n", RawInput: true, Compact: true},
			want: []string{`"a"`, `"b"`},
		},
		{
			// -R -s is the input verbatim, trailing newline included.
			name: "raw input slurped",
			req:  Request{Query: ".", Input: "a\nb\n", RawInput: true, Slurp: true, Compact: true},
			want: []string{`"a\nb\n"`},
		},
		{
			// CRLF is a line ending, not part of the line.
			name: "raw input crlf",
			req:  Request{Query: ".", Input: "a\r\nb\r\n", RawInput: true, Compact: true},
			want: []string{`"a"`, `"b"`},
		},
		{
			// Empty input is a single null, so a generator still runs.
			name: "empty input",
			req:  Request{Query: "[limit(3; repeat(1))]", Compact: true},
			want: []string{"[1,1,1]"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantValues(t, run(t, &c.req), c.want...)
		})
	}
}

// TestInputExhausted pins what the CLI does when a program reads past the end
// of its input: the runs that completed are reported, and the one that ran out
// fails. Driving the program from the same cursor `input` reads is what makes
// this match - an independent loop would run the program again on a value that
// had already been consumed.
func TestInputExhausted(t *testing.T) {
	res := run(t, &Request{Query: "[., input]", Input: "1 2 3 4 5", Compact: true})
	if res.Kind != KindRuntime {
		t.Fatalf("kind = %q, want %q", res.Kind, KindRuntime)
	}
	if len(res.Values) != 2 || res.Values[0] != "[1,2]" || res.Values[1] != "[3,4]" {
		t.Fatalf("got %q, want [1,2] and [3,4] before the failure", res.Values)
	}
}

// TestOutputModes pins how a value is rendered. Values go through gojq's own
// marshaller, so numbers, escapes and key order match the command line;
// encoding/json would escape <, > and & and refuse NaN outright.
func TestOutputModes(t *testing.T) {
	cases := []struct {
		name string
		req  Request
		want string
	}{
		{"compact", Request{Query: "{b:1,a:2}", Compact: true}, `{"a":2,"b":1}`},
		{"indent default", Request{Query: "{a:1}"}, "{\n  \"a\": 1\n}"},
		{"indent four", Request{Query: "{a:1}", Indent: 4}, "{\n    \"a\": 1\n}"},
		{"tab", Request{Query: "{a:1}", Tab: true}, "{\n\t\"a\": 1\n}"},
		{"raw string", Request{Query: `"hi"`, Raw: true}, "hi"},
		{"raw non-string", Request{Query: `[1]`, Raw: true, Compact: true}, "[1]"},
		{"html untouched", Request{Query: `"<a>&b"`, Compact: true}, `"<a>&b"`},
		{"nan is null", Request{Query: "nan", Compact: true}, "null"},
		{"big number kept", Request{Query: ".", Input: "123456789012345678901234567890", Compact: true}, "123456789012345678901234567890"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantValues(t, run(t, &c.req), c.want)
		})
	}
}

func TestArgs(t *testing.T) {
	res := run(t, &Request{
		Query:   "$x + $y",
		Compact: true,
		// A name binds with or without the dollar.
		Args: []Arg{{Name: "x", Value: "1"}, {Name: "$y", Value: "2"}},
	})
	wantValues(t, res, "3")

	// An argument is JSON text, so any value can be bound.
	wantValues(t, run(t, &Request{
		Query:   "$o.a",
		Compact: true,
		Args:    []Arg{{Name: "o", Value: `{"a":[1,2]}`}},
	}), "[1,2]")

	for _, c := range []struct {
		name string
		args []Arg
	}{
		{"not a name", []Arg{{Name: "1x", Value: "1"}}},
		{"bound twice", []Arg{{Name: "x", Value: "1"}, {Name: "x", Value: "2"}}},
		{"not JSON", []Arg{{Name: "x", Value: "{"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := run(t, &Request{Query: "$x", Args: c.args})
			if res.Kind != KindArgs {
				t.Fatalf("kind = %q, want %q (error: %s)", res.Kind, KindArgs, res.Error)
			}
		})
	}
}

func TestFailureKinds(t *testing.T) {
	cases := []struct {
		name string
		req  Request
		kind string
	}{
		{"empty", Request{Query: "  "}, KindParse},
		{"unparseable", Request{Query: "foo | | bar"}, KindParse},
		{"unknown function", Request{Query: "no_such_function"}, KindCompile},
		{"runtime", Request{Query: `1 + "x"`}, KindRuntime},
		{"bad input", Request{Query: ".", Input: "{not json"}, KindInput},
		{"halt", Request{Query: `"boom" | halt_error`}, KindHalt},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := run(t, &c.req)
			if res.Kind != c.kind {
				t.Fatalf("kind = %q, want %q (error: %s)", res.Kind, c.kind, res.Error)
			}
			if res.Error == "" {
				t.Fatal("a failure should carry a message")
			}
			// Values is never nil, so a caller can range over it without
			// checking, and a JSON encoding of it is [] rather than null.
			if res.Values == nil {
				t.Fatal("Values should be empty, not nil")
			}
		})
	}
}

// TestHalt pins that a deliberate stop is reported as one. error("boom") also
// satisfies gojq's error interface, so matching too loosely would report a
// genuine failure as a clean halt.
func TestHalt(t *testing.T) {
	res := run(t, &Request{Query: `1, ("boom" | halt_error)`, Compact: true})
	if !res.Halted || res.Kind != KindHalt || res.Error != "boom" {
		t.Fatalf("got halted=%v kind=%q error=%q", res.Halted, res.Kind, res.Error)
	}
	// The value emitted before the halt is still reported.
	if res.Count != 1 || res.Values[0] != "1" {
		t.Fatalf("got %d values %q, want [1]", res.Count, res.Values)
	}

	if res := run(t, &Request{Query: `error("boom")`}); res.Halted {
		t.Fatalf("error() is a runtime failure, not a halt: %+v", res)
	}
}

// TestLimits pins the bounds a host with no user to interrupt it depends on.
func TestLimits(t *testing.T) {
	res := run(t, &Request{Query: "repeat(1)", Compact: true, MaxResults: 5})
	if !res.Truncated || res.Count != 5 || res.Kind != KindLimit {
		t.Fatalf("got truncated=%v count=%d kind=%q", res.Truncated, res.Count, res.Kind)
	}

	// A single enormous value is bounded too: few results, many bytes.
	res = run(t, &Request{Query: "repeat([range(1000)])", Compact: true, MaxOutputBytes: 4096})
	if !res.Truncated || res.Kind != KindLimit || !strings.Contains(res.Error, "output") {
		t.Fatalf("got truncated=%v kind=%q error=%q", res.Truncated, res.Kind, res.Error)
	}

	// Zero means unbounded, which is what a host that can stop the run some
	// other way asks for.
	res = run(t, &Request{Query: "limit(3; repeat(1))", Compact: true})
	if res.Truncated || res.Count != 3 {
		t.Fatalf("got truncated=%v count=%d", res.Truncated, res.Count)
	}
}

// TestTimeout pins that the deadline is the context's, and that the message
// names the limit that was actually set.
func TestTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	started := time.Now()
	res := (&Runner{}).Run(ctx, &Request{Query: "[repeat(1)]", Compact: true})
	if res.Kind != KindTimeout {
		t.Fatalf("kind = %q, want %q (error: %s)", res.Kind, KindTimeout, res.Error)
	}
	// The message names the budget left when the run started, not the 50ms
	// the deadline was built from - the two differ by however long it took to
	// get from WithTimeout to Run, which is sub-millisecond idle and more than
	// that under load. Assert the figure is a duration in the right
	// neighbourhood rather than the exact string, which made this test fail in
	// CI at 49ms.
	m := regexp.MustCompile(`stopped after ([0-9.]+(?:ns|µs|ms|s))`).FindStringSubmatch(res.Error)
	if m == nil {
		t.Fatalf("the message should name the deadline, got: %s", res.Error)
	}
	named, err := time.ParseDuration(m[1])
	if err != nil {
		t.Fatalf("could not parse %q out of %q: %v", m[1], res.Error, err)
	}
	if named <= 40*time.Millisecond || named > 50*time.Millisecond {
		t.Fatalf("named deadline %s is not near the 50ms configured, got: %s", named, res.Error)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("the run took %s; the deadline did not stop it promptly", elapsed)
	}
}

func TestInputCount(t *testing.T) {
	if got := run(t, &Request{Query: ".", Input: "1 2 3", Compact: true}).InputCount; got != 3 {
		t.Fatalf("InputCount = %d, want 3", got)
	}
	// Slurped, the whole input is one value.
	if got := run(t, &Request{Query: ".", Input: "1 2 3", Slurp: true, Compact: true}).InputCount; got != 1 {
		t.Fatalf("slurped InputCount = %d, want 1", got)
	}
}
