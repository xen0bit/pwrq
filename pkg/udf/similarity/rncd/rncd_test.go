package rncd_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/similarity/rncd"
)

// The corpus every query below is written against: two documents that share
// most of their text, and one with content of its own. They are bound as jq
// variables rather than written to disk, because nothing in this package
// touches the filesystem — a value is the whole input.
var (
	sharedText = strings.Repeat("shared configuration block\n", 40)
	docA       = strings.Repeat("alpha alpha alpha\n", 60) + sharedText
	docB       = strings.Repeat("alpha alpha alpha\n", 58) + sharedText
	docC       = strings.Repeat("unrelated content entirely\n", 60)
)

// withCorpus prefixes a query with the bindings it is written against.
func withCorpus(query string) string {
	return strconv.Quote(docA) + " as $a | " +
		strconv.Quote(docB) + " as $b | " +
		strconv.Quote(docC) + " as $c | " + query
}

// run evaluates a query against just this package's cmdlets and collects every
// output, so a cmdlet that streams is exercised as a stream.
func run(t *testing.T, query string) ([]any, error) {
	t.Helper()
	q, err := gojq.Parse(withCorpus(query))
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, rncd.RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	var out []any
	iter := code.Run(nil)
	for {
		v, ok := iter.Next()
		if !ok {
			return out, nil
		}
		if err, isErr := v.(error); isErr {
			return out, err
		}
		out = append(out, v)
	}
}

func mustRun(t *testing.T, query string) []any {
	t.Helper()
	out, err := run(t, query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return out
}

// ---------------------------------------------------------------------------
// rncd_compare

func TestCompareEmitsOneObjectPerPair(t *testing.T) {
	results := mustRun(t, `rncd_compare([$a, $b, $c])`)
	if len(results) != 3 { // C(3,2)
		t.Fatalf("got %d pairs from 3 values, want 3", len(results))
	}
	first, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("a pair came back as %T, want an object", results[0])
	}
	for _, key := range []string{
		"PSTypeName", "IndexA", "IndexB", "NameA", "NameB", "LengthA", "LengthB",
		"EntropyA", "EntropyB", "Ncd", "NcdFingerprint", "EntropyGlobal",
		"EntropyProfile", "Hybrid",
	} {
		if _, present := first[key]; !present {
			t.Errorf("pair object has no %s: %v", key, first)
		}
	}
	if first["PSTypeName"] != "Pwrq.RncdPair" {
		t.Errorf("PSTypeName = %v", first["PSTypeName"])
	}
	// A bare string carries no label, and the shape says so with null rather
	// than by leaving the property out.
	if first["NameA"] != nil {
		t.Errorf("NameA = %v, want null for an unnamed value", first["NameA"])
	}
	if first["LengthA"] != len(docA) {
		t.Errorf("LengthA = %v, want %d", first["LengthA"], len(docA))
	}
}

// TestCompareIndexesIntoTheCallersArray is what makes an unnamed corpus usable:
// the pair says which two elements it scored, so the caller can look them back
// up in the array they built.
func TestCompareIndexesIntoTheCallersArray(t *testing.T) {
	results := mustRun(t, `[rncd_compare([$a, $b, $c]) | [.IndexA, .IndexB]]`)
	got := results[0].([]any)
	want := [][2]int{{0, 1}, {0, 2}, {1, 2}}
	if len(got) != len(want) {
		t.Fatalf("got %d pairs, want %d", len(got), len(want))
	}
	for i, pair := range got {
		p := pair.([]any)
		if p[0] != want[i][0] || p[1] != want[i][1] {
			t.Errorf("pair %d is %v, want %v", i, p, want[i])
		}
	}
}

func TestCompareRanksTheLikePairFirst(t *testing.T) {
	results := mustRun(t, `[rncd_compare([$a, $b, $c])] | sort_by(.Hybrid) | .[0] | [.IndexA, .IndexB]`)
	got := results[0].([]any)
	if got[0] != 0 || got[1] != 1 {
		t.Errorf("closest pair is %v, want the two like documents at 0 and 1", got)
	}
}

// TestCompareCallingFormsAgree pins the binding rule: the same corpus scored
// through the pipeline and as a leading argument is the same corpus.
func TestCompareCallingFormsAgree(t *testing.T) {
	piped := mustRun(t, `[$a, $b, $c] | [rncd_compare] | map(.Hybrid)`)
	explicit := mustRun(t, `[rncd_compare([$a, $b, $c])] | map(.Hybrid)`)
	if len(piped) != 1 || len(explicit) != 1 {
		t.Fatalf("piped %v, explicit %v", piped, explicit)
	}
	p, e := piped[0].([]any), explicit[0].([]any)
	if len(p) != len(e) {
		t.Fatalf("piped gave %d pairs, explicit gave %d", len(p), len(e))
	}
	for i := range p {
		if p[i] != e[i] {
			t.Errorf("pair %d scored %v piped and %v explicit", i, p[i], e[i])
		}
	}
}

// TestCompareLabelsNamedValues covers the ByPropertyName form: an object binds
// its bytes from Content and its label from Name, which is how a corpus built
// out of files reports paths instead of array offsets.
func TestCompareLabelsNamedValues(t *testing.T) {
	results := mustRun(t, `rncd_compare([{Name: "doc_a", Content: $a}, {Name: "doc_b", Content: $b}])`)
	if len(results) != 1 {
		t.Fatalf("got %d pairs, want 1", len(results))
	}
	pair := results[0].(map[string]any)
	if pair["NameA"] != "doc_a" || pair["NameB"] != "doc_b" {
		t.Errorf("names are %v/%v, want doc_a/doc_b", pair["NameA"], pair["NameB"])
	}
	// The label must not change the score: it is not part of the bytes.
	bare := mustRun(t, `rncd_compare([$a, $b]) | .Hybrid`)
	if pair["Hybrid"] != bare[0] {
		t.Errorf("named corpus scored %v, unnamed scored %v", pair["Hybrid"], bare[0])
	}
}

// TestCompareAcceptsAnyByteSource is the point of taking values rather than
// paths: a string computed in the query is a corpus element like any other,
// with no file behind it.
func TestCompareAcceptsAnyByteSource(t *testing.T) {
	results := mustRun(t, `[rncd_compare([$a, ($a + "epilogue\n")])] | length`)
	if results[0] != 1 {
		t.Fatalf("got %v pairs from two derived strings, want 1", results[0])
	}
	scores := mustRun(t, `[rncd_compare([$a, ($a + "epilogue\n"), $c])] | sort_by(.Hybrid) | .[0] | [.IndexA, .IndexB]`)
	got := scores[0].([]any)
	if got[0] != 0 || got[1] != 1 {
		t.Errorf("closest pair is %v, want the string and its extended self", got)
	}
}

func TestCompareOptionsBind(t *testing.T) {
	// Alpha=1 puts the whole score on the compression distance, so the two
	// have to come back equal.
	results := mustRun(t, `[rncd_compare([$a, $b, $c]; {Alpha: 1, Beta: 0}) | select(.Hybrid != .Ncd)] | length`)
	if results[0] != 0 {
		t.Errorf("%v pairs scored differently from their Ncd with Alpha=1", results[0])
	}
	// Parameter names bind case-insensitively, as PowerShell's do.
	if _, err := run(t, `rncd_compare([$a, $b]; {alpha: 1, beta: 0})`); err != nil {
		t.Errorf("lowercase option names: %v", err)
	}
	// MaxPairs: 0 lifts the limit rather than forbidding everything.
	if _, err := run(t, `rncd_compare([$a, $b, $c]; {MaxPairs: 0})`); err != nil {
		t.Errorf("MaxPairs: 0 should lift the limit: %v", err)
	}
}

// TestCompareShortCorpusIsNotAnError: fewer than two values is a corpus with no
// pairs in it, which is an empty result, not a failure.
func TestCompareShortCorpusIsNotAnError(t *testing.T) {
	for _, query := range []string{`rncd_compare([])`, `rncd_compare([$a])`} {
		out, err := run(t, query)
		if err != nil {
			t.Errorf("%s: %v", query, err)
		}
		if len(out) != 0 {
			t.Errorf("%s emitted %d pairs, want none", query, len(out))
		}
	}
}

func TestCompareRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"unknown option":       `rncd_compare([$a, $b]; {Alpah: 0.9})`,
		"weights above one":    `rncd_compare([$a, $b]; {Alpha: 0.9, Beta: 0.9})`,
		"negative weight":      `rncd_compare([$a, $b]; {Alpha: -1})`,
		"no workers":           `rncd_compare([$a, $b]; {Workers: 0})`,
		"too many pairs":       `rncd_compare([$a, $b, $c]; {MaxPairs: 1})`,
		"not an array":         `rncd_compare($a)`,
		"null input":           `rncd_compare(null)`,
		"element not bytes":    `rncd_compare([$a, 42])`,
		"element has no bytes": `rncd_compare([$a, {Length: 10}])`,
		"empty element":        `rncd_compare([$a, ""])`,
		"options not object":   `rncd_compare([$a, $b]; "Alpha")`,
	}
	for name, query := range cases {
		if _, err := run(t, query); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

// TestCompareNamesTheOffendingElement: a corpus is assembled element by
// element, so an error that says only "bad input" leaves the caller bisecting
// their own array.
func TestCompareNamesTheOffendingElement(t *testing.T) {
	_, err := run(t, `rncd_compare([$a, $b, 42])`)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "element 2") {
		t.Errorf("error does not say which element: %v", err)
	}
}

// ---------------------------------------------------------------------------
// shared_chunks

func TestSharedChunksReportsTheSharedSpan(t *testing.T) {
	results := mustRun(t, `shared_chunks($b; $a)`)
	obj := results[0].(map[string]any)

	if obj["PSTypeName"] != "Pwrq.SharedChunks" {
		t.Errorf("PSTypeName = %v", obj["PSTypeName"])
	}
	if obj["TargetLength"] != len(docB) || obj["ReferenceLength"] != len(docA) {
		t.Errorf("lengths are %v/%v, want %d/%d",
			obj["TargetLength"], obj["ReferenceLength"], len(docB), len(docA))
	}
	coverage, _ := obj["Coverage"].(float64)
	if coverage < 0.9 {
		t.Errorf("two documents sharing most of their text covered only %.3f", coverage)
	}

	chunks := obj["Chunks"].([]any)
	if len(chunks) == 0 {
		t.Fatal("no chunks reported")
	}
	// The chunks must tile the target, which is what makes Coverage a
	// fraction of the whole rather than of some subset.
	total := 0
	for _, c := range chunks {
		chunk := c.(map[string]any)
		length, _ := chunk["Length"].(int)
		total += length
		matched, _ := chunk["Matched"].(bool)
		if matched {
			if _, ok := chunk["RefOffset"].(int); !ok {
				t.Errorf("a matched chunk has no RefOffset: %v", chunk)
			}
		} else if chunk["RefOffset"] != nil {
			t.Errorf("a literal chunk reported RefOffset %v, want null", chunk["RefOffset"])
		}
	}
	if targetLen, _ := obj["TargetLength"].(int); total != targetLen {
		t.Errorf("chunks total %d bytes, target is %d", total, targetLen)
	}
}

// TestSharedChunksOffsetsAreExact is the difference between this cmdlet and the
// compression distances: a reported span can be cut out of both values and
// compared byte for byte, and it has to hold.
func TestSharedChunksOffsetsAreExact(t *testing.T) {
	results := mustRun(t, `shared_chunks($b; $a) | .Chunks | map(select(.Matched))`)
	spans := results[0].([]any)
	if len(spans) == 0 {
		t.Fatal("no matched spans")
	}
	for _, s := range spans {
		span := s.(map[string]any)
		start, _ := span["Start"].(int)
		length, _ := span["Length"].(int)
		off, _ := span["RefOffset"].(int)
		if docB[start:start+length] != docA[off:off+length] {
			t.Fatalf("span at target %d (len %d) does not match reference %d", start, length, off)
		}
	}
}

func TestSharedChunksCallingFormsAgree(t *testing.T) {
	piped := mustRun(t, `$b | shared_chunks($a) | .Coverage`)
	explicit := mustRun(t, `shared_chunks($b; $a) | .Coverage`)
	if piped[0] != explicit[0] {
		t.Errorf("piped coverage %v, explicit coverage %v", piped[0], explicit[0])
	}
}

// TestSharedChunksTargetIsTheInput pins the direction: coverage is a fraction
// of the *target*, so swapping the two values is a different question and
// generally a different answer.
func TestSharedChunksTargetIsTheInput(t *testing.T) {
	forward := mustRun(t, `shared_chunks($b; $a) | .TargetLength`)
	reverse := mustRun(t, `shared_chunks($a; $b) | .TargetLength`)
	if forward[0] != len(docB) || reverse[0] != len(docA) {
		t.Errorf("target lengths %v and %v, want %d and %d",
			forward[0], reverse[0], len(docB), len(docA))
	}
}

func TestSharedChunksBindsNamedValues(t *testing.T) {
	named := mustRun(t, `shared_chunks({Name: "b", Content: $b}; {Name: "a", Content: $a}) | .Coverage`)
	bare := mustRun(t, `shared_chunks($b; $a) | .Coverage`)
	if named[0] != bare[0] {
		t.Errorf("named coverage %v, bare coverage %v", named[0], bare[0])
	}
}

func TestSharedChunksMinMatch(t *testing.T) {
	loose := mustRun(t, `shared_chunks($c; $a; {MinMatch: 2}) | .MatchedBytes`)
	strict := mustRun(t, `shared_chunks($c; $a; {MinMatch: 64}) | .MatchedBytes`)
	l, _ := loose[0].(int)
	s, _ := strict[0].(int)
	if l <= s {
		t.Errorf("a 2-byte minimum matched %d bytes, no more than a 64-byte minimum's %d", l, s)
	}
	if def := mustRun(t, `shared_chunks($c; $a) | .MinMatch`); def[0] != 16 {
		t.Errorf("default MinMatch = %v, want 16", def[0])
	}
}

func TestSharedChunksIdenticalValuesCoverCompletely(t *testing.T) {
	results := mustRun(t, `shared_chunks($a; $a) | [.Coverage, .Spans]`)
	got := results[0].([]any)
	if got[0] != 1.0 {
		t.Errorf("a value against itself covered %v, want 1", got[0])
	}
	if got[1] != 1 {
		t.Errorf("a value against itself took %v spans, want 1", got[1])
	}
}

func TestSharedChunksRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"bad MinMatch":       `shared_chunks($a; $a; {MinMatch: 0})`,
		"unknown option":     `shared_chunks($a; $a; {MinMtach: 4})`,
		"target not bytes":   `shared_chunks(42; $a)`,
		"target null":        `shared_chunks(null; $a)`,
		"reference not byte": `shared_chunks($a; [1, 2, 3])`,
		"options not object": `shared_chunks($a; $a; 4)`,
	}
	for name, query := range cases {
		if _, err := run(t, query); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}
