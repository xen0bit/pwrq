package string

import (
	"fmt"
	"testing"
)

func TestTextExtras(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"user@example.com" | before_first("@")`, "user"},
		{`"user@example.com" | after_first("@")`, "example.com"},
		{`"no separator" | after_first("=")`, ""},
		{`"x" | surround("[ "; " ]")`, "[ x ]"},
		{`"Robert" | soundex`, "R163"},
		{`"Rupert" | soundex`, "R163"},
		{`"abcde" | is_isogram`, "true"},
		{`"letter" | is_isogram`, "false"},
		{`"hello" | count_vowels`, "2"},
		{`"hello" | count_consonants`, "3"},
		{`"hELLO" | capitalize_first`, "HELLO"},
		{`"héllo" | unicode_escape`, `h\u00e9llo`},
		{`"h\\u00e9llo" | unicode_unescape`, "héllo"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query, nil, RegisterAll()...))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}

func TestRegexGroups(t *testing.T) {
	got := run(t, `"a=1 b=22" | regex_groups("(\\w+)=(\\d+)")`, nil, RegisterAll()...)
	if fmt.Sprint(got) != "[[a 1] [b 22]]" {
		t.Errorf("regex_groups = %v", got)
	}
}

func TestDiffLines(t *testing.T) {
	got := run(t, `"a\nb\nc" | diff_lines("b\nc\nd")`, nil, RegisterAll()...)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("diff_lines = %T", got)
	}
	if fmt.Sprint(m["added"]) != "[d]" {
		t.Errorf("diff_lines added = %v", m["added"])
	}
	if fmt.Sprint(m["removed"]) != "[a]" {
		t.Errorf("diff_lines removed = %v", m["removed"])
	}
}
