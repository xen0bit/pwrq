package pwrgrep

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/astsearch"
)

// What combining match sets costs. A rule is a pipeline of these over whatever
// scan_ast found, so the input here is a few thousand matches spread over a
// few hundred files - the shape a corpus rule run over a repository has, and
// the shape that tells an operator scanning the other list per match apart
// from one that indexes it.

// benchMatches builds a match set the way scan_ast would.
func benchMatches(files, perFile int, pattern string) []any {
	out := make([]any, 0, files*perFile)
	for f := 0; f < files; f++ {
		path := "pkg/thing" + strconv.Itoa(f) + "/file.go"
		for i := 0; i < perFile; i++ {
			offset := i * 120
			out = append(out, astsearch.AstMatch.Build(map[string]any{
				"Path": path, "Language": "go", "Pattern": pattern,
				"LineNumber": i + 1, "Column": 2, "EndLineNumber": i + 1, "EndColumn": 12,
				"Offset": offset, "EndOffset": offset + 40,
				"Text":     pattern,
				"Captures": map[string]any{"X": "value" + strconv.Itoa(i)},
				"CaptureSpans": map[string]any{"X": map[string]any{
					"LineNumber": i + 1, "Column": 4, "EndLineNumber": i + 1,
					"EndColumn": 9, "Offset": offset + 4, "EndOffset": offset + 9,
				}},
				"PwrqValue": path,
			}))
		}
	}
	return out
}

// benchEnclosing is a match set whose spans cover the calls, which is what
// within and outside are asked about.
func benchEnclosing(files, perFile int) []any {
	out := make([]any, 0, files*perFile)
	for f := 0; f < files; f++ {
		path := "pkg/thing" + strconv.Itoa(f) + "/file.go"
		for i := 0; i < perFile; i++ {
			out = append(out, astsearch.AstMatch.Build(map[string]any{
				"Path": path, "Language": "go", "Pattern": "func $F($$$_) { $$$B }",
				"LineNumber": i*4 + 1, "Column": 1, "EndLineNumber": i*4 + 4, "EndColumn": 1,
				"Offset": i * 400, "EndOffset": i*400 + 400,
				"Text": "func f() {}", "Captures": map[string]any{},
				"CaptureSpans": map[string]any{}, "PwrqValue": path,
			}))
		}
	}
	return out
}

var (
	benchCalls   = benchMatches(200, 10, "md5.New()")
	benchImports = benchMatches(80, 1, `"crypto/md5"`)
	benchFuncs   = benchEnclosing(200, 3)
)

func benchNarrow(b *testing.B, narrow func(list, other []any) []any, list, other []any, want bool) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if kept := narrow(list, other); (len(kept) > 0) != want {
			b.Fatalf("kept %d of %d, which is not the case this measures", len(kept), len(list))
		}
	}
}

func BenchmarkInFilesWith(b *testing.B) {
	benchNarrow(b, func(list, other []any) []any {
		paths := pathsOf(other)
		return keepIf(list, func(m map[string]any) bool { return paths[text(m, "Path")] })
	}, benchCalls, benchImports, true)
}

func BenchmarkWithin(b *testing.B) {
	benchNarrow(b, func(list, other []any) []any {
		return keepIf(list, enclosuresOf(other).covers)
	}, benchCalls, benchFuncs, true)
}

func BenchmarkAtSamePlace(b *testing.B) {
	benchNarrow(b, func(list, other []any) []any {
		return keepIf(list, placesOf(other).holds)
	}, benchCalls, benchCalls, true)
}

func BenchmarkEnclosuresOf(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		enclosuresOf(benchFuncs)
	}
}

// report orders and de-duplicates what a rule's alternatives produced, so it
// runs over every finding a rule has.
func BenchmarkSortKey(b *testing.B) {
	findings := make([]map[string]any, 0, len(benchCalls))
	for i, v := range benchCalls {
		m := object(v)
		findings = append(findings, map[string]any{
			"RuleId": "go-weak-hash", "Path": m["Path"], "LineNumber": m["LineNumber"],
			"Column": m["Column"], "Message": fmt.Sprintf("finding %d", i),
		})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sortKey(findings[i%len(findings)])
	}
}

func BenchmarkByFile(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		byFile(benchCalls)
	}
}

// What a hole constrained by a pattern costs, with and without the short
// circuit for a pattern that constrains nothing.
//
// The distance between the two is the whole reason MatchesAnything exists. A
// rule that searches a bare `$X` gets one match per node in the tree - tens of
// thousands over a small repository - and a `where_capture_ast` against
// another bare hole then parses the caught text once per distinct one. The
// answer is true every time, so the parsing buys nothing.
//
// Python, because the corpus rules that do this are Python rules and because a
// caught fragment there is a whole construct on its own, which is the case
// that reaches the parser.
func benchPythonCaptures(n int) []any {
	out := make([]any, 0, n)
	for i := 0; i < n; i++ {
		path := "pkg/thing" + strconv.Itoa(i%40) + "/file.py"
		out = append(out, astsearch.AstMatch.Build(map[string]any{
			"Path": path, "Language": "python", "Pattern": "$X",
			"LineNumber": i + 1, "Column": 2, "EndLineNumber": i + 1, "EndColumn": 12,
			"Offset": i * 120, "EndOffset": i*120 + 40,
			"Text":         "value" + strconv.Itoa(i),
			"Captures":     map[string]any{"X": "value" + strconv.Itoa(i)},
			"CaptureSpans": map[string]any{},
			"PwrqValue":    path,
		}))
	}
	return out
}

func BenchmarkWhereCaptureAstUnconstrained(b *testing.B) {
	list := benchPythonCaptures(2000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		free := astsearch.MatchesAnything("$ARGS", "python")
		kept := keepIf(list, func(m map[string]any) bool {
			if capture(m, "X") == "" {
				return false
			}
			if free {
				return true
			}
			ok, err := astsearch.MatchesText("$ARGS", "python", capture(m, "X"))
			return err == nil && ok
		})
		if len(kept) != len(list) {
			b.Fatalf("kept %d of %d: an unconstrained pattern keeps everything", len(kept), len(list))
		}
	}
}

func BenchmarkWhereCaptureAstTheLongWay(b *testing.B) {
	list := benchPythonCaptures(2000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decided := map[string]bool{}
		kept := keepIf(list, func(m map[string]any) bool {
			caught := capture(m, "X")
			if caught == "" {
				return false
			}
			if answer, asked := decided[caught]; asked {
				return answer
			}
			ok, err := astsearch.MatchesText("$ARGS", "python", caught)
			decided[caught] = err == nil && ok
			return decided[caught]
		})
		if len(kept) != len(list) {
			b.Fatalf("kept %d of %d", len(kept), len(list))
		}
	}
}
