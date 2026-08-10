package similarity

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

func TestLevenshtein(t *testing.T) {
	if got := fmt.Sprint(run(t, `levenshtein("kitten"; "sitting")`)); got != "3" {
		t.Errorf("levenshtein(kitten;sitting) = %s, want 3", got)
	}
	if got := fmt.Sprint(run(t, `levenshtein("hello"; "hello")`)); got != "0" {
		t.Errorf("levenshtein identity = %s, want 0", got)
	}
}

func TestHammingDistance(t *testing.T) {
	if got := fmt.Sprint(run(t, `hamming_distance("karolin"; "kathrin")`)); got != "3" {
		t.Errorf("hamming = %s, want 3", got)
	}
	if got := fmt.Sprint(run(t, `hamming_distance("abc"; "abc")`)); got != "0" {
		t.Errorf("hamming identity = %s, want 0", got)
	}
}

func TestJaccard(t *testing.T) {
	if got := fmt.Sprint(run(t, `jaccard([1,2,3]; [2,3,4])`)); got != "0.5" {
		t.Errorf("jaccard arrays = %s, want 0.5", got)
	}
	if got := fmt.Sprint(run(t, `jaccard("abc"; "abc")`)); got != "1" {
		t.Errorf("jaccard identical = %s, want 1", got)
	}
}

func TestDeepDiff(t *testing.T) {
	got := run(t, `deep_diff({a: 1, b: 2}; {a: 1, b: 3, c: 4})`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("deep_diff = %T, want an object", got)
	}
	added := asList(m["added"])
	if len(added) != 1 || fmt.Sprint(added[0].(map[string]any)["path"]) != "c" {
		t.Errorf("added = %v", m["added"])
	}
	removed := asList(m["removed"])
	if len(removed) != 0 {
		t.Errorf("removed = %v", m["removed"])
	}
	changed := asList(m["changed"])
	if len(changed) != 1 {
		t.Fatalf("changed = %v", m["changed"])
	}
	entry := changed[0].(map[string]any)
	if fmt.Sprint(entry["path"]) != "b" || fmt.Sprint(entry["before"]) != "2" || fmt.Sprint(entry["after"]) != "3" {
		t.Errorf("changed entry = %v", entry)
	}
}

func asList(v any) []any {
	if arr, ok := v.([]any); ok {
		return arr
	}
	return nil
}
