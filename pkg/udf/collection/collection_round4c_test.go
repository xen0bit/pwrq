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
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
