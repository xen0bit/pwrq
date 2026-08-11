package stats

import (
	"fmt"
	"testing"
)

func TestMovingAverage(t *testing.T) {
	got := run(t, `[1,2,3,4,5] | moving_average(3)`)
	arr := got.([]any)
	if len(arr) != 3 {
		t.Fatalf("moving_average = %v, want 3 values", arr)
	}
	if fmt.Sprint(arr[0]) != "2" || fmt.Sprint(arr[1]) != "3" || fmt.Sprint(arr[2]) != "4" {
		t.Errorf("moving_average = %v", arr)
	}
}

func TestGeomean(t *testing.T) {
	if got := fmt.Sprint(run(t, `[1,4,16] | geomean`)); got != "4" {
		t.Errorf("geomean = %s, want 4", got)
	}
}

func TestNormalize(t *testing.T) {
	got := run(t, `[2,4,6] | normalize`)
	arr := got.([]any)
	if fmt.Sprint(arr[0]) != "0" || fmt.Sprint(arr[1]) != "0.5" || fmt.Sprint(arr[2]) != "1" {
		t.Errorf("normalize = %v", arr)
	}
}
