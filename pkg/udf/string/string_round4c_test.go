package string

import (
	"fmt"
	"testing"
)

func TestTextExtrasPartThree(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"hello=world\nnext" | quoted_printable_encode`, "hello=3Dworld\nnext"},
		{`"hello=3Dworld" | quoted_printable_decode`, "hello=world"},
		{`"a\nb" | prefix_lines("> ")`, "> a\n> b"},
		{`"a\nb\nc\nd" | first_lines(2)`, "a\nb"},
		{`"a\nb\nc\nd" | last_lines(2)`, "c\nd"},
		{`"(a[b]{c})" | is_balanced`, "true"},
		{`"([)]" | is_balanced`, "false"},
		{`"a1 b2 c3" | regex_last_match("[0-9]+")`, "3"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query, nil, RegisterAll()...))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}

func TestQuotedPrintableRoundTrip(t *testing.T) {
	got := run(t, `"café ☕" | quoted_printable_encode as $e | {encoded: $e, back: ($e | quoted_printable_decode)}`, nil, RegisterAll()...)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("quoted_printable = %T", got)
	}
	if m["back"] != "café ☕" {
		t.Errorf("round-trip = %v", m["back"])
	}
	if m["encoded"] != "caf=C3=A9 =E2=98=95" {
		t.Errorf("encoded = %v", m["encoded"])
	}
}
