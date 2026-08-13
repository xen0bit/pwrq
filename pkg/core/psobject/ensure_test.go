package psobject

import (
	"errors"
	"testing"
	"time"
)

func TestEnsureJSONLeavesCleanValuesAlone(t *testing.T) {
	// A value already in gojq's value space must come back as the same value,
	// not a copy: EnsureJSON sits on every cmdlet result, and deep-copying
	// each one would be the most expensive thing in the pipeline.
	inner := []any{1, "two", 3.0, true, nil}
	v := map[string]any{"list": inner}

	got, err := EnsureJSON(v)
	if err != nil {
		t.Fatalf("clean value rejected: %v", err)
	}
	gotMap, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want map[string]any", got)
	}
	if gotList, _ := gotMap["list"].([]any); &gotList[0] != &inner[0] {
		t.Error("clean value was rebuilt; it should be returned untouched")
	}
}

func TestEnsureJSONConvertsCmdletTypes(t *testing.T) {
	obj := NewPSObject("/tmp/x")
	obj.AddNoteProperty("Handles", int32(7))

	cases := []struct {
		name string
		give any
		want any
	}{
		{"int32", int32(5), 5},
		{"int64", int64(5), 5},
		{"uint32", uint32(5), 5},
		{"float32", float32(2.5), 2.5},
		{"duration", 3 * time.Second, "3s"},
		{"bytes", []byte("hi"), "hi"},
		{"error", errors.New("boom"), "boom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EnsureJSON(tc.give)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("EnsureJSON(%#v) = %#v, want %#v", tc.give, got, tc.want)
			}
		})
	}

	got, err := EnsureJSON(obj)
	if err != nil {
		t.Fatalf("PSObject rejected: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("PSObject became %T, want map[string]any", got)
	}
	if m["Handles"] != 7 {
		t.Errorf("Handles = %#v, want 7", m["Handles"])
	}
}

// nestSlice builds a value nested depth levels deep.
func nestSlice(depth int) any {
	var v any
	for range depth {
		v = []any{v}
	}
	return v
}

func TestEnsureJSONReportsTooDeep(t *testing.T) {
	if _, err := EnsureJSON(nestSlice(MaxJSONDepth - 1)); err != nil {
		t.Errorf("value inside the limit rejected: %v", err)
	}
	if _, err := EnsureJSON(nestSlice(MaxJSONDepth + 1)); !errors.Is(err, ErrTooDeep) {
		t.Errorf("value past the limit gave err=%v, want ErrTooDeep", err)
	}
}

// TestEnsureJSONDirtyAndDeep is the case that makes the depth check worth
// finishing rather than abandoning at the first value needing conversion.
//
// The map below is dirty at a key that may be visited first and catastrophically
// deep at another. Reporting "dirty, not too deep" would send it to
// NormalizeJSON, which recurses per level and dies with "fatal error: stack
// overflow" - unrecoverable, taking the process with it. The depth used here
// does overflow when the check gives up early, so this test fails as a crash if
// that regresses.
func TestEnsureJSONDirtyAndDeep(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates several million values")
	}
	v := map[string]any{
		"handles": int32(7),
		"deep":    nestSlice(5_000_000),
	}
	if _, err := EnsureJSON(v); !errors.Is(err, ErrTooDeep) {
		t.Errorf("err = %v, want ErrTooDeep", err)
	}
}

func TestEnsureJSONCountsDepthThroughPSObjects(t *testing.T) {
	// A note property is a level of nesting the same way a map entry is:
	// NormalizeJSON descends into it, so the depth check must too.
	var v any = "leaf"
	for range MaxJSONDepth + 10 {
		obj := NewPSObject(nil)
		obj.AddNoteProperty("next", v)
		v = obj
	}
	if _, err := EnsureJSON(v); !errors.Is(err, ErrTooDeep) {
		t.Errorf("err = %v, want ErrTooDeep: the walk must descend note properties", err)
	}
}
