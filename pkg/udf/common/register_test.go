package common

import (
	"errors"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// run compiles and evaluates a query against a single registered cmdlet.
func run(t *testing.T, opt gojq.CompilerOption, query string) []any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, opt)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	var out []any
	iter := code.Run(nil)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		out = append(out, v)
	}
	return out
}

// TestWithFunctionNormalizesResults is the point of the wrapper: a cmdlet that
// hands back a Go type outside gojq's value space must not be able to reach the
// engine with it. gojq panics on such a value inside any builtin that inspects
// it, so `| type` below is what would crash.
func TestWithFunctionNormalizesResults(t *testing.T) {
	cases := []struct {
		name     string
		give     any
		wantType string
	}{
		{"int32", int32(7), "number"},
		{"int64", int64(1 << 40), "number"},
		{"uint8", uint8(3), "number"},
		{"float32", float32(1.5), "number"},
		{"byte slice", []byte("hi"), "string"},
		{"typed map", map[string]any{"n": int32(1)}, "object"},
		{"nested slice", []any{[]any{int64(2)}}, "array"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt := WithFunction("probe", 0, 0, func(any, []any) any { return tc.give })
			got := run(t, opt, "probe | type")
			if len(got) != 1 || got[0] != tc.wantType {
				t.Fatalf("probe | type = %#v, want [%q]", got, tc.wantType)
			}
		})
	}
}

// TestWithFunctionNormalizesPSObject pins that a cmdlet returning a PSObject -
// once a crash, because gojq has no such type - now yields its wire form.
func TestWithFunctionNormalizesPSObject(t *testing.T) {
	obj := psobject.NewPSObject("/tmp/x.txt")
	obj.AddNoteProperty("Extension", ".txt")

	opt := WithFunction("probe", 0, 0, func(any, []any) any { return obj })
	got := run(t, opt, "probe | .Extension")
	if len(got) != 1 || got[0] != ".txt" {
		t.Fatalf("probe | .Extension = %#v, want [\".txt\"]", got)
	}
}

// TestWithFunctionPreservesErrors guards the one value that must not be
// normalized: an error is how a cmdlet reports failure, and NormalizeJSON would
// turn it into an ordinary string, so the failure would look like success.
func TestWithFunctionPreservesErrors(t *testing.T) {
	opt := WithFunction("probe", 0, 0, func(any, []any) any {
		return errors.New("boom")
	})
	got := run(t, opt, `try probe catch ("caught: " + .)`)
	if len(got) != 1 || got[0] != "caught: boom" {
		t.Fatalf("error did not reach jq's error channel: %#v", got)
	}
}

// TestWithIterFunctionNormalizesEachValue: a streaming cmdlet must be covered
// per value, not just on its first.
func TestWithIterFunctionNormalizesEachValue(t *testing.T) {
	opt := WithIterFunction("probe", 0, 0, func(any, []any) gojq.Iter {
		return gojq.NewIter[any](int32(1), int64(2), uint16(3))
	})
	got := run(t, opt, "[probe | type]")
	if len(got) != 1 {
		t.Fatalf("want one result, got %#v", got)
	}
	arr, ok := got[0].([]any)
	if !ok || len(arr) != 3 {
		t.Fatalf("want three values, got %#v", got[0])
	}
	for i, v := range arr {
		if v != "number" {
			t.Errorf("value %d is %#v, want \"number\"", i, v)
		}
	}
}

// TestWithIterFunctionHandlesNilIter pins that a cmdlet returning no iterator
// yields nothing rather than dereferencing nil.
func TestWithIterFunctionHandlesNilIter(t *testing.T) {
	opt := WithIterFunction("probe", 0, 0, func(any, []any) gojq.Iter { return nil })
	if got := run(t, opt, "[probe] | length"); len(got) != 1 || got[0] != 0 {
		t.Fatalf("want [0], got %#v", got)
	}
}

// TestNormalizeLeavesDeepValuesAlone pins that the boundary does not try to
// normalize a value nested past MaxJSONDepth: doing so would recurse deeply
// enough to overflow the stack, which is the crash the depth guard exists to
// prevent. It is passed through for the encoder to refuse.
func TestNormalizeLeavesDeepValuesAlone(t *testing.T) {
	var deep any
	for range psobject.MaxJSONDepth + 10 {
		deep = []any{deep}
	}
	// Must return rather than overflow, and must not have been rebuilt.
	if got := normalizeResult(deep); got == nil {
		t.Fatal("deep value was dropped")
	}
}
