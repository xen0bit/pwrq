package aggregate

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query string, input any, options ...gojq.CompilerOption) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, options...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	iter := code.Run(input)
	v, ok := iter.Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

const rows = `[{"dept":"eng","salary":90,"year":2020},
 {"dept":"eng","salary":110,"year":2021},
 {"dept":"ops","salary":80,"year":2020}]`

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected an object, got %T: %v", v, v)
	}
	return m
}

func TestGroupByKey(t *testing.T) {
	got := asMap(t, run(t, `group_by_key(.; "dept")`, mustArray(t, rows), RegisterAll()...))
	if len(got) != 2 {
		t.Fatalf("group_by_key has %d groups, want 2", len(got))
	}
	if eng, ok := got["eng"].([]any); !ok || len(eng) != 2 {
		t.Errorf("eng group = %v, want 2 rows", got["eng"])
	}
}

func TestCountBy(t *testing.T) {
	got := asMap(t, run(t, `count_by(.; "dept")`, mustArray(t, rows), RegisterAll()...))
	if got["eng"] != float64(2) && got["eng"] != 2 {
		t.Errorf("count_by eng = %v, want 2", got["eng"])
	}
	if got["ops"] != float64(1) && got["ops"] != 1 {
		t.Errorf("count_by ops = %v, want 1", got["ops"])
	}
}

func TestSumBy(t *testing.T) {
	got := asMap(t, run(t, `sum_by(.; "dept"; "salary")`, mustArray(t, rows), RegisterAll()...))
	if fmt.Sprint(got["eng"]) != "200" {
		t.Errorf("sum_by eng = %v, want 200", got["eng"])
	}
	if fmt.Sprint(got["ops"]) != "80" {
		t.Errorf("sum_by ops = %v, want 80", got["ops"])
	}
}

func TestAvgBy(t *testing.T) {
	got := asMap(t, run(t, `avg_by(.; "dept"; "salary")`, mustArray(t, rows), RegisterAll()...))
	if fmt.Sprint(got["eng"]) != "100" {
		t.Errorf("avg_by eng = %v, want 100", got["eng"])
	}
}

func TestIndexBy(t *testing.T) {
	got := asMap(t, run(t, `index_by(.; "dept")`, mustArray(t, rows), RegisterAll()...))
	eng, ok := got["eng"].(map[string]any)
	if !ok {
		t.Fatalf("index_by eng = %T", got["eng"])
	}
	if fmt.Sprint(eng["salary"]) != "90" {
		t.Errorf("index_by eng salary = %v, want 90 (the first row)", eng["salary"])
	}
}

func TestValueCounts(t *testing.T) {
	got := asMap(t, run(t, `value_counts`, []any{"a", "b", "a", "c", "a"}, RegisterAll()...))
	if fmt.Sprint(got["a"]) != "3" {
		t.Errorf("value_counts a = %v, want 3", got["a"])
	}
	if fmt.Sprint(got["b"]) != "1" {
		t.Errorf("value_counts b = %v, want 1", got["b"])
	}
}

func TestSummarizeBy(t *testing.T) {
	got := run(t, `summarize_by(.; "dept"; "salary") | map(select(.key == "eng")) | .[0]`, mustArray(t, rows), RegisterAll()...)
	m := asMap(t, got)
	if fmt.Sprint(m["sum"]) != "200" {
		t.Errorf("summarize_by sum = %v, want 200", m["sum"])
	}
	if fmt.Sprint(m["count"]) != "2" {
		t.Errorf("summarize_by count = %v, want 2", m["count"])
	}
	if fmt.Sprint(m["min"]) != "90" {
		t.Errorf("summarize_by min = %v, want 90", m["min"])
	}
	if fmt.Sprint(m["max"]) != "110" {
		t.Errorf("summarize_by max = %v, want 110", m["max"])
	}
}

const wideRows = `[{"dept":"eng","y2020":90,"y2021":110},
 {"dept":"ops","y2020":80,"y2021":null}]`

func TestPivotUnpivot(t *testing.T) {
	got := asMap(t, run(t, `pivot(.; {rows: "dept", cols: "year", values: "salary"})`, mustArray(t, rows), RegisterAll()...))
	eng, ok := got["eng"].(map[string]any)
	if !ok {
		t.Fatalf("pivot eng = %T", got["eng"])
	}
	if fmt.Sprint(eng["2020"]) != "90" {
		t.Errorf("pivot eng 2020 = %v, want 90", eng["2020"])
	}

	unpivoted := run(t, `unpivot(.; {cols: ["y2020", "y2021"], id: "dept"}) | map(select(.dept == "eng" and .key == "y2021")) | .[0]`,
		mustArray(t, wideRows), RegisterAll()...)
	um := asMap(t, unpivoted)
	if fmt.Sprint(um["value"]) != "110" {
		t.Errorf("unpivot eng/y2021 = %v, want 110", um["value"])
	}
}

func TestTopByBottomBy(t *testing.T) {
	top := run(t, `top_by(.; "salary"; 2) | map(.dept)`, mustArray(t, rows), RegisterAll()...)
	if !reflect.DeepEqual(top, []any{"eng", "eng"}) {
		t.Errorf("top_by = %v, want [eng eng]", top)
	}
	bottom := run(t, `bottom_by(.; "salary"; 1) | map(.dept)`, mustArray(t, rows), RegisterAll()...)
	if !reflect.DeepEqual(bottom, []any{"ops"}) {
		t.Errorf("bottom_by = %v, want [ops]", bottom)
	}
}

func TestDistinctCount(t *testing.T) {
	got := run(t, `distinct_count`, []any{1, 2, 1, 3, 3, 3}, RegisterAll()...)
	if fmt.Sprint(got) != "3" {
		t.Errorf("distinct_count = %v, want 3", got)
	}
}

func mustArray(t *testing.T, json string) []any {
	t.Helper()
	arr := run(t, json, nil, RegisterAll()...)
	a, ok := arr.([]any)
	if !ok {
		t.Fatalf("fixture is not an array: %T", arr)
	}
	return a
}
