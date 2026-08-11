package string

import (
	"fmt"
	"testing"
)

func TestTextTools(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"the quick brown fox" | reverse_words`, "fox brown quick the"},
		{`"the quick brown fox" | truncate_words(2)`, "the quick…"},
		{`"short" | truncate_words(5)`, "short"},
		{`"café" | remove_accents`, "cafe"},
		{`"hELLO World" | sentence_case`, "Hello world"},
		{`"a\nb\nc" | line_count`, "3"},
		{`"" | line_count`, "0"},
		{`"  a\n    b\n  c" | dedent`, "a\n  b\nc"},
		{`"HeLLo" | swap_case`, "hEllO"},
		{`"a\nb\nc" | reverse_lines`, "c\nb\na"},
		{`"b\na\nb" | unique_lines`, "b\na"},
		{`"b\na\nc" | sort_lines`, "a\nb\nc"},
		{`"\"quoted\"" | strip_quotes`, "quoted"},
		{`"hi" | pad_center(5)`, " hi  "},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query, nil, RegisterAll()...))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}

func TestCharFrequencies(t *testing.T) {
	got := run(t, `"aab" | char_frequencies`, nil, RegisterAll()...)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("char_frequencies = %T", got)
	}
	if fmt.Sprint(m["a"]) != "2" || fmt.Sprint(m["b"]) != "1" {
		t.Errorf("char_frequencies = %v", m)
	}
}
