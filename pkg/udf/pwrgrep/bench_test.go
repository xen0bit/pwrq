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
