package stats

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

func TestMeanMedian(t *testing.T) {
	if got := fmt.Sprint(run(t, `[1,2,3,4] | mean`)); got != "2.5" {
		t.Errorf("mean = %s", got)
	}
	if got := fmt.Sprint(run(t, `[3,1,2] | median`)); got != "2" {
		t.Errorf("median = %s", got)
	}
	if got := fmt.Sprint(run(t, `[1,2,3,4] | median`)); got != "2.5" {
		t.Errorf("median even = %s", got)
	}
}

func TestMode(t *testing.T) {
	if got := fmt.Sprint(run(t, `["a","b","a","c","a"] | mode`)); got != "a" {
		t.Errorf("mode = %s", got)
	}
	if got := fmt.Sprint(run(t, `[1,2,2,3] | mode`)); got != "2" {
		t.Errorf("mode numeric = %s", got)
	}
}

func TestVarianceStdev(t *testing.T) {
	// sample variance of [2,4,4,4,5,5,7,9] is 32/7 = 4.571428...
	if got := fmt.Sprint(run(t, `[2,4,4,4,5,5,7,9] | variance`)); got != "4.571428571428571" {
		t.Errorf("variance = %s", got)
	}
	if got := fmt.Sprint(run(t, `[2,4,4,4,5,5,7,9] | stdev`)); got != "2.138089935299395" {
		t.Errorf("stdev = %s", got)
	}
	if got := fmt.Sprint(run(t, `[5] | variance`)); got != "0" {
		t.Errorf("variance single = %s", got)
	}
}

func TestPercentile(t *testing.T) {
	if got := fmt.Sprint(run(t, `[1,2,3,4] | percentile(50)`)); got != "2.5" {
		t.Errorf("percentile 50 = %s", got)
	}
	if got := fmt.Sprint(run(t, `[10,20,30] | percentile(0)`)); got != "10" {
		t.Errorf("percentile 0 = %s", got)
	}
	if got := fmt.Sprint(run(t, `[10,20,30] | percentile(100)`)); got != "30" {
		t.Errorf("percentile 100 = %s", got)
	}
}

func TestSummary(t *testing.T) {
	got := run(t, `[1,2,3,4] | summary`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("summary = %T, want an object", got)
	}
	if fmt.Sprint(m["count"]) != "4" || fmt.Sprint(m["min"]) != "1" || fmt.Sprint(m["max"]) != "4" ||
		fmt.Sprint(m["mean"]) != "2.5" || fmt.Sprint(m["median"]) != "2.5" {
		t.Errorf("summary = %v", m)
	}
}
