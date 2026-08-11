package udf_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// eval runs a query against the full registry and returns its single result.
func eval(t *testing.T, query string) (any, error) {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, udf.DefaultRegistry().Options()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	iter := code.Run(nil)
	got, ok := iter.Next()
	if !ok {
		t.Fatalf("%q produced no output", query)
	}
	if err, isErr := got.(error); isErr {
		return nil, err
	}
	return got, nil
}

// TestCallingFormsAgree pins the binding rule down: a cmdlet called with its
// input piped in and the same cmdlet called with the input as its leading
// argument must produce the same value.
//
// This is the test that was missing when zip_arrays and interleave shipped
// reversing their operands in the explicit form — the piped form was correct
// and the only form anyone had tried.
func TestCallingFormsAgree(t *testing.T) {
	cases := []struct{ piped, explicit string }{
		{`[1,2,3] | zip_arrays(["a","b","c"])`, `zip_arrays([1,2,3]; ["a","b","c"])`},
		{`[1,2] | interleave(["a","b"])`, `interleave([1,2]; ["a","b"])`},
		{`[1,2,3,4] | chunks(2)`, `chunks([1,2,3,4]; 2)`},
		{`[1,2,3] | rotate(1)`, `rotate([1,2,3]; 1)`},
		{`[3,1,2] | top_n(2)`, `top_n([3,1,2]; 2)`},
		{`[1,2,3] | windows(2)`, `windows([1,2,3]; 2)`},
		{`[1,1,2] | dedupe`, `dedupe([1,1,2])`},
		{`["b","a"] | natural_sort`, `natural_sort(["b","a"])`},
		{`[1,1] | all_equal`, `all_equal([1,1])`},
		{`[1,1] | contains_duplicates`, `contains_duplicates([1,1])`},
		{`[{"n":"ada"}] | lookup("n"; "ada")`, `lookup([{"n":"ada"}]; "n"; "ada")`},
		{`{"a":1} | deep_merge({"b":2})`, `deep_merge({"a":1}; {"b":2})`},
		{`{"a":1} | json_merge_patch({"b":2})`, `json_merge_patch({"a":1}; {"b":2})`},
		{`[{"d":"x"},{"d":"x"}] | count_by("d")`, `count_by([{"d":"x"},{"d":"x"}]; "d")`},
		{`[{"d":"x"}] | group_by_key("d")`, `group_by_key([{"d":"x"}]; "d")`},
		{`[{"d":"x"}] | index_by("d")`, `index_by([{"d":"x"}]; "d")`},
		{`[1,1,2] | value_counts`, `value_counts([1,1,2])`},
		{`[{"d":"x","p":1}] | sum_by("d"; "p")`, `sum_by([{"d":"x","p":1}]; "d"; "p")`},
		{`[{"d":"x","p":1}] | avg_by("d"; "p")`, `avg_by([{"d":"x","p":1}]; "d"; "p")`},
		{`[{"d":"x","p":1}] | summarize_by("d"; "p")`, `summarize_by([{"d":"x","p":1}]; "d"; "p")`},
		{`[{"s":1},{"s":9}] | top_by("s"; 1)`, `top_by([{"s":1},{"s":9}]; "s"; 1)`},
		{`[{"s":1},{"s":9}] | bottom_by("s"; 1)`, `bottom_by([{"s":1},{"s":9}]; "s"; 1)`},
		{`[{"d":"x","y":1,"a":2}] | pivot({rows:"d",cols:"y",values:"a"})`,
			`pivot([{"d":"x","y":1,"a":2}]; {rows:"d",cols:"y",values:"a"})`},
	}
	for _, c := range cases {
		piped, err := eval(t, c.piped)
		if err != nil {
			t.Errorf("%s: %v", c.piped, err)
			continue
		}
		explicit, err := eval(t, c.explicit)
		if err != nil {
			t.Errorf("%s: %v", c.explicit, err)
			continue
		}
		if fmt.Sprint(piped) != fmt.Sprint(explicit) {
			t.Errorf("calling forms disagree:\n  %s\n    = %v\n  %s\n    = %v",
				c.piped, piped, c.explicit, explicit)
		}
	}
}

// TestPairOrderFollowsInput is the specific regression: the input array
// supplies the left element of every pair, whichever form is used.
func TestPairOrderFollowsInput(t *testing.T) {
	for _, q := range []string{
		`[1,2] | zip_arrays(["a","b"])`,
		`zip_arrays([1,2]; ["a","b"])`,
	} {
		got, err := eval(t, q)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if want := "[[1 a] [2 b]]"; fmt.Sprint(got) != want {
			t.Errorf("%s = %v, want %v", q, got, want)
		}
	}
	for _, q := range []string{
		`[1,2] | interleave(["a","b"])`,
		`interleave([1,2]; ["a","b"])`,
	} {
		got, err := eval(t, q)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if want := "[1 a 2 b]"; fmt.Sprint(got) != want {
			t.Errorf("%s = %v, want %v", q, got, want)
		}
	}
}

// TestOperandOrderIsEnforced covers the other half of the old bug: the helper
// used to return whichever argument happened to be an array, so passing the
// operands the wrong way round succeeded quietly. It must now fail.
func TestOperandOrderIsEnforced(t *testing.T) {
	for _, q := range []string{
		`count_by("d"; [{"d":"x"}])`,
		`chunks(2; [1,2,3,4])`,
		`lookup("n"; "ada"; [{"n":"ada"}])`,
	} {
		got, err := eval(t, q)
		if err == nil {
			t.Errorf("%s should be an error, got %v", q, got)
			continue
		}
		if !strings.Contains(err.Error(), "expected an array") &&
			!strings.Contains(err.Error(), "must be a string") &&
			!strings.Contains(err.Error(), "key must be") {
			t.Errorf("%s: unexpected error %v", q, err)
		}
	}
}

// TestMinimumArityDoesNotPanic guards the crash deep_merge and
// json_merge_patch both had: reading an operand the caller never supplied.
func TestMinimumArityDoesNotPanic(t *testing.T) {
	for _, q := range []string{
		`{"a":1} | deep_merge({"b":2})`,
		`{"a":1} | json_merge_patch({"b":2})`,
	} {
		if _, err := eval(t, q); err != nil {
			t.Errorf("%s: %v", q, err)
		}
	}
}
