package stats

import (
	"fmt"
	"testing"
)

func TestPercentileRank(t *testing.T) {
	got := run(t, `[1,2,3,4,5] | percentile_rank(3)`)
	if fmt.Sprint(got) != "60" {
		t.Errorf("percentile_rank(3) = %v, want 60", got)
	}
	got = run(t, `[1,2,3,4,5] | percentile_rank(0)`)
	if fmt.Sprint(got) != "0" {
		t.Errorf("percentile_rank(0) = %v, want 0", got)
	}
	got = run(t, `[1,2,3,4,5] | percentile_rank(5)`)
	if fmt.Sprint(got) != "100" {
		t.Errorf("percentile_rank(5) = %v, want 100", got)
	}
}
