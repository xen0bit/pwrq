package random

import (
	"fmt"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query string) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	iter := code.Run(nil)
	v, ok := iter.Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

func TestRandomIntBounds(t *testing.T) {
	for i := 0; i < 50; i++ {
		got := run(t, `random_int(5; 10)`)
		n, ok := toFloat(got)
		if !ok || n < 5 || n > 10 {
			t.Fatalf("random_int(5;10) = %v, want in [5,10]", got)
		}
	}
}

func TestRandomIntSingle(t *testing.T) {
	for i := 0; i < 20; i++ {
		n, ok := toFloat(run(t, `random_int(3)`))
		if !ok || n < 0 || n > 3 {
			t.Fatalf("random_int(3) = %v, want in [0,3]", n)
		}
	}
}

func TestRandomFloatRange(t *testing.T) {
	for i := 0; i < 50; i++ {
		f, ok := toFloat(run(t, `random_float`))
		if !ok || f < 0 || f >= 1 {
			t.Fatalf("random_float = %v, want in [0,1)", f)
		}
	}
	for i := 0; i < 20; i++ {
		f, ok := toFloat(run(t, `random_float(10; 20)`))
		if !ok || f < 10 || f >= 20 {
			t.Fatalf("random_float(10;20) = %v, want in [10,20)", f)
		}
	}
}

func TestRandomString(t *testing.T) {
	got := fmt.Sprint(run(t, `random_string(16)`))
	if len(got) != 16 {
		t.Errorf("random_string(16) length = %d, want 16", len(got))
	}
	for _, c := range got {
		if !strings.ContainsRune(defaultAlphabet, c) {
			t.Errorf("random_string(16) produced %q, outside the default alphabet", c)
		}
	}
	got = fmt.Sprint(run(t, `random_string(8; "01")`))
	for _, c := range got {
		if c != '0' && c != '1' {
			t.Errorf("random_string over \"01\" produced %q", c)
		}
	}
}

func TestRandomChoice(t *testing.T) {
	for i := 0; i < 30; i++ {
		got := fmt.Sprint(run(t, `[10,20,30] | random_choice`))
		if got != "10" && got != "20" && got != "30" {
			t.Fatalf("random_choice = %s, not a member", got)
		}
	}
}

func TestShuffle(t *testing.T) {
	got := run(t, `[1,2,3,4,5] | shuffle`)
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("shuffle = %T, want an array", got)
	}
	if len(arr) != 5 {
		t.Fatalf("shuffle length = %d, want 5", len(arr))
	}
	seen := map[string]bool{}
	for _, item := range arr {
		key := fmt.Sprint(item)
		if seen[key] {
			t.Errorf("shuffle duplicated %s", key)
		}
		seen[key] = true
	}
}

func TestSample(t *testing.T) {
	got := run(t, `[1,2,3,4,5] | sample(3)`)
	arr, ok := got.([]any)
	if !ok || len(arr) != 3 {
		t.Fatalf("sample(3) = %T len %d, want an array of 3", got, len(arr))
	}
	for _, item := range arr {
		n, ok := toFloat(item)
		if !ok || n < 1 || n > 5 {
			t.Errorf("sample produced %v, not a member of [1..5]", item)
		}
	}
	// n >= length returns everything.
	got = run(t, `[1,2] | sample(5)`)
	if arr, ok := got.([]any); !ok || len(arr) != 2 {
		t.Errorf("sample(5) over 2 elements = %v, want both", got)
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	default:
		return 0, false
	}
}
