package parsing

import (
	"encoding/json"
	"testing"

	"github.com/xen0bit/pwrq/pkg/d1/internal/testdata"
)

// TestMatchesReference checks every reading against d1's parsing.py
// (internal/tools/d1fixtures/parsing_fixtures.py).
func TestMatchesReference(t *testing.T) {
	var fx struct {
		Numbers []struct {
			Text, Context string
			Unit          *string
			Value         *json.Number
		}
		Units []struct {
			Text string
			Unit *string
		}
		Dates []struct {
			Text, Context string
			Value         *string
			Ambiguous     bool
		}
	}
	testdata.Load(t, "parsing_fixtures.json", &fx)
	for _, c := range fx.Numbers {
		unit := ""
		if c.Unit != nil {
			unit = *c.Unit
		}
		got, ok := ParseNumber(c.Text, c.Context, unit)
		switch {
		case c.Value == nil && ok:
			t.Errorf("ParseNumber(%q, %q) = %v, want nothing", c.Text, c.Context, got)
		case c.Value != nil && !ok:
			t.Errorf("ParseNumber(%q, %q) = nothing, want %s", c.Text, c.Context, *c.Value)
		case c.Value != nil:
			want, _ := c.Value.Float64()
			var g float64
			switch v := got.(type) {
			case int64:
				g = float64(v)
				if _, err := c.Value.Int64(); err != nil {
					t.Errorf("ParseNumber(%q) = int %d, want float %s", c.Text, v, *c.Value)
				}
			case float64:
				g = v
			}
			if g != want {
				t.Errorf("ParseNumber(%q, %q) = %v, want %s", c.Text, c.Context, got, *c.Value)
			}
		}
	}
	for _, c := range fx.Units {
		got, ok := WrittenUnit(c.Text)
		if (c.Unit == nil) == ok || c.Unit != nil && got != *c.Unit {
			t.Errorf("WrittenUnit(%q) = %q, %v; want %v", c.Text, got, ok, c.Unit)
		}
	}
	for _, c := range fx.Dates {
		got, ambiguous := ParseDate(c.Text, c.Context)
		want := ""
		if c.Value != nil {
			want = *c.Value
		}
		if got != want || ambiguous != c.Ambiguous {
			t.Errorf("ParseDate(%q, %q) = %q, %v; want %q, %v", c.Text, c.Context, got, ambiguous, want, c.Ambiguous)
		}
	}
}
