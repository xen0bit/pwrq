package string

import (
	"fmt"
	"testing"
)

func TestRegex(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"a1 b22 c333" | regex_find_all("[0-9]+")`, "[1 22 333]"},
		{`"a1 b22 c333" | regex_find_all("[0-9]+"; 2)`, "[1 22]"},
		{`"abc123" | regex_extract_first("[0-9]+")`, "123"},
		{`"user=42" | regex_extract_first("user=(\\d+)"; 1)`, "42"},
		{`"no digits" | regex_extract_first("[0-9]+")`, "<nil>"},
		{`"foo-1 foo-2" | regex_replace_first("foo-([0-9]+)"; "bar-$1")`, "bar-1 foo-2"},
		{`"a,b;c" | regex_split("[,;]")`, "[a b c]"},
		{`"aa bb aa" | regex_count("aa")`, "2"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query, nil, RegisterAll()...))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}

func TestTextTools(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"A man a plan a canal panama" | is_palindrome`, "true"},
		{`"hello" | is_palindrome`, "false"},
		{`"the quick brown fox" | reverse_words`, "fox brown quick the"},
		{`"the quick brown fox" | truncate_words(2)`, "the quick…"},
		{`"short" | truncate_words(5)`, "short"},
		{`"café" | remove_accents`, "cafe"},
		{`"hELLO World" | sentence_case`, "Hello world"},
		{`"a\nb\nc" | line_count`, "3"},
		{`"" | line_count`, "0"},
		{`"  a\n    b\n  c" | dedent`, "a\n  b\nc"},
		{`"HeLLo" | swap_case`, "hEllO"},
		{`"hello world" | first_line`, "hello world"},
		{`"a\nb" | last_line`, "b"},
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

func TestCharFrequenciesAndAnagram(t *testing.T) {
	got := run(t, `"aab" | char_frequencies`, nil, RegisterAll()...)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("char_frequencies = %T", got)
	}
	if fmt.Sprint(m["a"]) != "2" || fmt.Sprint(m["b"]) != "1" {
		t.Errorf("char_frequencies = %v", m)
	}
	if fmt.Sprint(run(t, `"listen" | anagram("silent")`, nil, RegisterAll()...)) != "true" {
		t.Error("anagram listen/silent should be true")
	}
	if fmt.Sprint(run(t, `"listen" | anagram("list")`, nil, RegisterAll()...)) != "false" {
		t.Error("anagram listen/list should be false")
	}
}
