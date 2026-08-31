package shape

import (
	"strconv"
	"testing"
)

// The shapes cmdlets build through are on the hot path of every search: one
// Build per match, and a large search has a match per line. These measure that
// call rather than a query, because what it costs is decided here.

var benchShape = Fixed("Bench.Match",
	Prop("Path", String, "file"),
	Prop("Language", String, "language"),
	Prop("Pattern", String, "pattern"),
	Prop("LineNumber", Number, "line"),
	Prop("Column", Number, "column"),
	Prop("EndLineNumber", Number, "end line"),
	Prop("EndColumn", Number, "end column"),
	Prop("Offset", Number, "offset"),
	Prop("EndOffset", Number, "end offset"),
	Prop("Text", String, "text"),
	Prop("Captures", Object, "captures"),
	Prop("PwrqValue", String, "value"),
)

func benchMatch() map[string]any {
	return map[string]any{
		"Path": "pkg/udf/astsearch/select_ast.go", "Language": "go",
		"Pattern": "md5.New()", "LineNumber": 41, "Column": 2,
		"EndLineNumber": 41, "EndColumn": 12, "Offset": 913, "EndOffset": 923,
		"Text": "md5.New()", "Captures": map[string]any{"X": "value"},
		"PwrqValue": "pkg/udf/astsearch/select_ast.go",
	}
}

func BenchmarkBuild(b *testing.B) {
	values := make([]map[string]any, b.N)
	for i := range values {
		values[i] = benchMatch()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchShape.Build(values[i])
	}
}

// A shape whose declaration has fallen behind pays the recording cost as well,
// which is the case a hot loop must not fall into unnoticed.
func BenchmarkBuildUndeclared(b *testing.B) {
	values := make([]map[string]any, b.N)
	for i := range values {
		m := benchMatch()
		m["Extra"+strconv.Itoa(i%4)] = true
		values[i] = m
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchShape.Build(values[i])
	}
}
