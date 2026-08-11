package collection

import (
	"fmt"
	"testing"
)

func TestWindowsAndSets(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`[1,2,3,4] | windows(3)`, "[[1 2 3] [2 3 4]]"},
		{`[1,2,3] | pairs`, "[[1 2] [2 3]]"},
		{`[1,2] | is_subset([0,1,2,3])`, "true"},
		{`[1,4] | is_subset([1,2,3])`, "false"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
