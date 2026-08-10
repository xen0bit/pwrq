package collection

import (
	"fmt"
	"testing"
)

func TestRotate(t *testing.T) {
	if got := fmt.Sprint(run(t, `[1,2,3,4,5] | rotate(2)`)); got != "[3 4 5 1 2]" {
		t.Errorf("rotate(2) = %v", got)
	}
	if got := fmt.Sprint(run(t, `[1,2,3,4,5] | rotate(-1)`)); got != "[5 1 2 3 4]" {
		t.Errorf("rotate(-1) = %v", got)
	}
}

func TestTopN(t *testing.T) {
	got := run(t, `[1,9,3,7,5] | top_n(2)`)
	arr := got.([]any)
	if fmt.Sprint(arr) != "[9 7]" {
		t.Errorf("top_n(2) = %v", arr)
	}
}

func TestInterleave(t *testing.T) {
	if got := fmt.Sprint(run(t, `[1,2,3] | interleave(["a","b","c"])`)); got != "[1 a 2 b 3 c]" {
		t.Errorf("interleave = %v", got)
	}
}
