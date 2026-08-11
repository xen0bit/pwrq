package collection

import (
	"fmt"
	"testing"
)

// sides renders a compare_object result as "value:indicator" pairs, which is
// the whole of what the cmdlet promises and reads legibly in a failure.
func sides(t *testing.T, query string) []string {
	t.Helper()
	rows, ok := run(t, query).([]any)
	if !ok {
		t.Fatalf("%s did not return an array", query)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		m := r.(map[string]any)
		out = append(out, fmt.Sprintf("%v:%v", m["InputObject"], m["SideIndicator"]))
	}
	return out
}

func TestCompareObject(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`compare_object(["a","b"]; ["b","c"])`, "[a:<= c:=>]"},
		{`["a","b"] | compare_object(["b","c"])`, "[a:<= c:=>]"},
		{`compare_object(["a","b"]; ["b","c"]; {IncludeEqual: true})`, "[a:<= b:== c:=>]"},
		{`compare_object(["a"]; ["a"])`, "[]"},
		{`compare_object([]; ["x"])`, "[x:=>]"},
		{`compare_object(["x"]; [])`, "[x:<=]"},
		{`compare_object(["a","b"]; ["b","c"]; {ExcludeDifferent: true, IncludeEqual: true})`, "[b:==]"},
	}
	for _, tt := range tests {
		if got := fmt.Sprint(sides(t, tt.query)); got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}

// TestCompareObjectCountsOccurrences is the difference between this and the set
// cmdlets: two "a" on the left against one on the right leaves one unmatched,
// and reporting nothing would be a wrong answer.
func TestCompareObjectCountsOccurrences(t *testing.T) {
	if got := fmt.Sprint(sides(t, `compare_object(["a","a"]; ["a"])`)); got != "[a:<=]" {
		t.Errorf("surplus on the left = %s, want [a:<=]", got)
	}
	if got := fmt.Sprint(sides(t, `compare_object(["a"]; ["a","a"])`)); got != "[a:=>]" {
		t.Errorf("surplus on the right = %s, want [a:=>]", got)
	}
	if got := fmt.Sprint(sides(t, `compare_object(["a","a"]; ["a","a"])`)); got != "[]" {
		t.Errorf("equal multisets = %s, want []", got)
	}
}

func TestCompareObjectByProperty(t *testing.T) {
	// Matched on id, so the changed name is "equal" — which is the point of
	// comparing by a key rather than by whole-value identity.
	got := sides(t, `compare_object([{"id":1,"n":"a"}]; [{"id":1,"n":"CHANGED"}]; {Property: "id", IncludeEqual: true})`)
	if len(got) != 1 || got[0][len(got[0])-2:] != "==" {
		t.Errorf("compare by property = %v, want one == row", got)
	}
	// Without the key, the rows differ by value and both sides are reported.
	plain := sides(t, `compare_object([{"id":1,"n":"a"}]; [{"id":1,"n":"CHANGED"}])`)
	if len(plain) != 2 {
		t.Errorf("compare by value = %v, want two rows", plain)
	}
}

func TestCompareObjectRejectsBadOptions(t *testing.T) {
	for _, q := range []string{
		`compare_object(["a"]; ["b"]; {Nonsense: true})`,
		`compare_object(["a"]; ["b"]; {IncludeEqual: "yes"})`,
		`compare_object(["a"]; "not an array")`,
	} {
		if err := runErr(t, q); err == nil {
			t.Errorf("%s should be an error", q)
		}
	}
}
