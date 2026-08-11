package string

import (
	"fmt"
	"testing"
)

func TestPredicates(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{`"" | is_blank`, true},
		{`"   " | is_blank`, true},
		{`"abc" | is_blank`, false},
		{`"abc123" | is_alphanumeric`, true},
		{`"abc_123" | is_alphanumeric`, false},
		{`"abc" | is_alphabetic`, true},
		{`"abc1" | is_alphabetic`, false},
		{`"12345" | is_numeric_string`, true},
		{`"12a" | is_numeric_string`, false},
		{`"HELLO" | is_uppercase`, true},
		{`"Hello" | is_uppercase`, false},
		{`"hello" | is_lowercase`, true},
		{`"Hello" | is_lowercase`, false},
		{`"plain ascii" | is_ascii`, true},
		{`"héllo" | is_ascii`, false},
	}
	for _, tt := range tests {
		if got := run(t, tt.query, nil, RegisterAll()...); got != tt.want {
			t.Errorf("%s = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestWordCountAndNormalize(t *testing.T) {
	if got := fmt.Sprint(run(t, `"the quick brown fox" | word_count`, nil, RegisterAll()...)); got != "4" {
		t.Errorf("word_count = %s", got)
	}
	if got := fmt.Sprint(run(t, `"  a   b  c " | normalize_whitespace`, nil, RegisterAll()...)); got != "a b c" {
		t.Errorf("normalize_whitespace = %q", got)
	}
	if got := fmt.Sprint(run(t, `"International Business Machines" | acronym`, nil, RegisterAll()...)); got != "IBM" {
		t.Errorf("acronym = %q", got)
	}
}

func TestPatterns(t *testing.T) {
	if got := fmt.Sprint(run(t, `"a.b" | escape_regex`, nil, RegisterAll()...)); got != `a\.b` {
		t.Errorf("escape_regex = %q", got)
	}
	if got := fmt.Sprint(run(t, `"*.txt" | glob_to_regex`, nil, RegisterAll()...)); got != `^.*\.txt$` {
		t.Errorf("glob_to_regex = %q", got)
	}
	if got := run(t, `"notes.txt" | match_glob("*.txt")`, nil, RegisterAll()...); got != true {
		t.Errorf("match_glob positive = %v", got)
	}
	if got := run(t, `"notes.md" | match_glob("*.txt")`, nil, RegisterAll()...); got != false {
		t.Errorf("match_glob negative = %v", got)
	}
	if got := run(t, `"^[a-z]+$" | is_regex_valid`, nil, RegisterAll()...); got != true {
		t.Errorf("is_regex_valid = %v", got)
	}
	if got := run(t, `"[unclosed" | is_regex_valid`, nil, RegisterAll()...); got != false {
		t.Errorf("is_regex_valid invalid = %v", got)
	}
}
