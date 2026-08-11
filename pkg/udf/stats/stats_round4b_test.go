package stats

import (
	"fmt"
	"testing"
)

func TestStatsExtras(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`[1,2,3,4,5] | autocorrelation`, "0.4"},
		{`[1,2,3,4,5] | autocorrelation(0)`, "1"},
		{`[1,2,3,4,5,6,7,8] | iqr`, "3.5"},
		{`[1,2,3,4,5] | mad`, "1"},
		{`[3,1,9] | spread`, "8"},
		{`[1,2,3,4,5] | moving_stdev(2)`, "[0.7071067811865476 0.7071067811865476 0.7071067811865476 0.7071067811865476]"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
