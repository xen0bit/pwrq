package json

import (
	"fmt"
	"testing"
)

func TestJSONMergePatch(t *testing.T) {
	got := run(t, `json_merge_patch({a: 1, b: {x: 1}, c: 3}; {a: 9, b: {y: 2}, c: null})`)
	if fmt.Sprint(got) != "map[a:9 b:map[x:1 y:2]]" {
		t.Errorf("json_merge_patch = %v", got)
	}
}

func TestJSONLParse(t *testing.T) {
	got := run(t, `"{\"a\":1}\n{\"a\":2}\n\n{\"a\":3}" | jsonl_parse`)
	arr, ok := got.([]any)
	if !ok || len(arr) != 3 {
		t.Fatalf("jsonl_parse = %v", got)
	}
	if fmt.Sprint(arr[0]) != "map[a:1]" || fmt.Sprint(arr[2]) != "map[a:3]" {
		t.Errorf("jsonl_parse = %v", arr)
	}
}

func TestGetPath(t *testing.T) {
	if got := fmt.Sprint(run(t, `{a: {b: [10, 20]}} | get_path("a.b[1]")`)); got != "20" {
		t.Errorf("get_path = %s", got)
	}
}
