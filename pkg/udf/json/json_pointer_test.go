package json

import (
	"fmt"
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

func TestJSONPointer(t *testing.T) {
	if got := fmt.Sprint(run(t, `{a: {b: [10, 20]}} | json_pointer("/a/b/1")`)); got != "20" {
		t.Errorf("json_pointer /a/b/1 = %s", got)
	}
	if got := fmt.Sprint(run(t, `{a: 1} | json_pointer("/a")`)); got != "1" {
		t.Errorf("json_pointer /a = %s", got)
	}
	if got := run(t, `{a: 1} | json_pointer("/missing")`); got != nil {
		t.Errorf("json_pointer missing = %v, want null", got)
	}
	// Escapes.
	if got := fmt.Sprint(run(t, `{"a/b": 1} | json_pointer("/a~1b")`)); got != "1" {
		t.Errorf("json_pointer escaped = %s", got)
	}
	// Empty pointer is the whole document.
	if got := fmt.Sprint(run(t, `{x: 1} | json_pointer("")`)); got != "map[x:1]" {
		t.Errorf("json_pointer empty = %s", got)
	}
}

func TestJSONPointerSet(t *testing.T) {
	if got := fmt.Sprint(run(t, `{a: 1} | json_pointer_set("/b"; 2)`)); got != "map[a:1 b:2]" {
		t.Errorf("json_pointer_set = %s", got)
	}
	if got := fmt.Sprint(run(t, `{a: {b: 1}} | json_pointer_set("/a/b"; 9)`)); got != "map[a:map[b:9]]" {
		t.Errorf("json_pointer_set nested = %s", got)
	}
	if got := fmt.Sprint(run(t, `[] | json_pointer_set("/1"; "x")`)); got != "[<nil> x]" {
		t.Errorf("json_pointer_set array = %s", got)
	}
}

func TestQueryString(t *testing.T) {
	parsed := run(t, `"a=1&b=two&b=three" | query_string_parse`)
	if fmt.Sprint(parsed) != "map[a:1 b:[two three]]" {
		t.Errorf("query_string_parse = %v", parsed)
	}
	built := run(t, `{a: "1", b: ["x", "y"]} | query_string_build`)
	if fmt.Sprint(built) != "a=1&b=x&b=y" {
		t.Errorf("query_string_build = %v", built)
	}
}
