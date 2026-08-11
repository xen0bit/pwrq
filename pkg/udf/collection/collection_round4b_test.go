package collection

import (
	"fmt"
	"testing"
)

func TestObjectHelpers(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`{"a":1,"b":2} | rename_keys({"a": "x"})`, "map[b:2 x:1]"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
