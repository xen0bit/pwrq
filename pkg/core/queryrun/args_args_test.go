package queryrun

import (
	"context"
	"testing"
)

// TestARGSAndInputFilenameBound pins the jq vocabulary that used to be
// CLI-only: $ARGS is always defined, and input_filename resolves (to null when
// the host has no file input) instead of failing to compile.
func TestARGSAndInputFilenameBound(t *testing.T) {
	r := &Runner{}
	res := r.Run(context.Background(), &Request{
		Query:     `[$ARGS, input_filename, $n]`,
		NullInput: true,
		Compact:   true,
		Args:      []Arg{{Name: "n", Value: "7"}},
	})
	if res.Error != "" {
		t.Fatalf("run failed: %s (%s)", res.Error, res.Kind)
	}
	if len(res.Values) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(res.Values), res.Values)
	}
	want := `[{"named":{"n":7},"positional":[]},null,7]`
	if res.Values[0] != want {
		t.Errorf("$ARGS/input_filename = %s, want %s", res.Values[0], want)
	}
}
