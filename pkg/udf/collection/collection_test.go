package collection

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

// runErr is run for queries expected to fail: it returns the error rather than
// ending the test with it.
func runErr(t *testing.T, query string) error {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		return err
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		return err
	}
	v, ok := code.Run(nil).Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		return e
	}
	return nil
}

func TestChunks(t *testing.T) {
	got := run(t, `[1,2,3,4,5,6,7] | chunks(3)`)
	arr := got.([]any)
	if len(arr) != 3 {
		t.Fatalf("chunks = %d chunks, want 3", len(arr))
	}
	if fmt.Sprint(arr[0]) != "[1 2 3]" || fmt.Sprint(arr[1]) != "[4 5 6]" || fmt.Sprint(arr[2]) != "[7]" {
		t.Errorf("chunks = %v", arr)
	}
}

func TestDedupe(t *testing.T) {
	got := run(t, `[3,1,2,1,3,4] | dedupe`)
	arr := got.([]any)
	if fmt.Sprint(arr) != "[3 1 2 4]" {
		t.Errorf("dedupe = %v", arr)
	}
}

func TestDeepMerge(t *testing.T) {
	got := run(t, `deep_merge({a: {x: 1}, b: 2}; {a: {y: 3}, c: 4})`)
	if fmt.Sprint(got) != "map[a:map[x:1 y:3] b:2 c:4]" {
		t.Errorf("deep_merge = %v", got)
	}
}

func TestPrune(t *testing.T) {
	got := run(t, `{a: 1, b: null, c: {d: "", e: 0}, f: [null, 2]} | prune`)
	if fmt.Sprint(got) != "map[a:1 c:map[e:0] f:[2]]" {
		t.Errorf("prune = %v", got)
	}
}

func TestFlattenUnflatten(t *testing.T) {
	got := run(t, `{a: {b: 1}, c: [2, {d: 3}]} | flatten_keys`)
	if fmt.Sprint(got) != "map[a.b:1 c[0]:2 c[1].d:3]" {
		t.Errorf("flatten_keys = %v", got)
	}
	back := run(t, `{"a.b": 1, "c[0]": 2, "c[1].d": 3} | unflatten_keys`)
	if fmt.Sprint(back) != "map[a:map[b:1] c:[2 map[d:3]]]" {
		t.Errorf("unflatten_keys = %v", back)
	}
}

func TestZipArrays(t *testing.T) {
	got := run(t, `[1,2,3] | zip_arrays(["a","b"])`)
	if fmt.Sprint(got) != "[[1 a] [2 b]]" {
		t.Errorf("zip_arrays = %v", got)
	}
}
