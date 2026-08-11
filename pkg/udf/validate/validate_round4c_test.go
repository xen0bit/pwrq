package validate

import (
	"fmt"
	"testing"
)

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"123" | is_numeric`, "true"},
		{`"-1.5e3" | is_numeric`, "true"},
		{`"12.5" | is_numeric`, "true"},
		{`"abc" | is_numeric`, "false"},
		{`"12abc" | is_numeric`, "false"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
