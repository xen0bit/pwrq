package stats

import (
	"fmt"
	"testing"
)

func TestTimeSeries(t *testing.T) {
	if got := sprint(run(t, `[1,2,3] | cumsum`)); got != "[1 3 6]" {
		t.Errorf("cumsum = %v", got)
	}
	if got := sprint(run(t, `[3,1,4,2] | cumulative_max`)); got != "[3 3 4 4]" {
		t.Errorf("cumulative_max = %v", got)
	}
	if got := sprint(run(t, `[3,1,4,2] | cumulative_min`)); got != "[3 1 1 1]" {
		t.Errorf("cumulative_min = %v", got)
	}
	if got := sprint(run(t, `[1,4,9,2] | deltas`)); got != "[3 5 -7]" {
		t.Errorf("deltas = %v", got)
	}
	if got := sprint(run(t, `[1] | deltas`)); got != "[]" {
		t.Errorf("deltas single = %v", got)
	}
	if got := sprint(run(t, `[1,2,3] | lag(1)`)); got != "[<nil> 1 2]" {
		t.Errorf("lag = %v", got)
	}
	if got := sprint(run(t, `[1,2,3] | lag(-1)`)); got != "[2 3 <nil>]" {
		t.Errorf("lag negative = %v", got)
	}
	if got := sprint(run(t, `[1,null,2,null] | fill_forward`)); got != "[1 1 2 2]" {
		t.Errorf("fill_forward = %v", got)
	}
	if got := run(t, `[1,2,3] | ema(0.5)`); len(got.([]any)) != 3 || fmt.Sprint(got.([]any)[2]) != "2.25" {
		t.Errorf("ema = %v", got)
	}
	if got := sprint(run(t, `[3,1,4,1,5] | moving_max(3)`)); got != "[4 4 5]" {
		t.Errorf("moving_max = %v", got)
	}
	if got := sprint(run(t, `[3,1,4,1,5] | moving_min(3)`)); got != "[1 1 1]" {
		t.Errorf("moving_min = %v", got)
	}
}

// sprint renders a value the way fmt %v does, so float64(3) and int(3) compare
// equal.
func sprint(v any) string {
	arr, ok := v.([]any)
	if !ok {
		return fmt.Sprint(v)
	}
	out := "["
	for i, item := range arr {
		if i > 0 {
			out += " "
		}
		out += fmt.Sprint(item)
	}
	return out + "]"
}

func TestCorrelationCovariance(t *testing.T) {
	got := run(t, `correlation([1,2,3,4,5]; [2,4,6,8,10])`)
	if fmt.Sprint(got) != "1" {
		t.Errorf("correlation perfect = %s", got)
	}
	got = run(t, `correlation([1,2,3,4,5]; [10,8,6,4,2])`)
	if fmt.Sprint(got) != "-1" {
		t.Errorf("correlation inverse = %s", got)
	}
	got = run(t, `covariance([1,2,3]; [2,4,6])`)
	if fmt.Sprint(got) != "2" {
		t.Errorf("covariance = %s", got)
	}
}

func TestSkewnessKurtosis(t *testing.T) {
	// A right-skewed set is positive.
	got := run(t, `[1,1,1,1,10] | skewness`)
	if f, ok := got.(float64); !ok || f <= 0 {
		t.Errorf("skewness positive = %v", got)
	}
	got = run(t, `[1,2,3,4,5,6,7,8] | kurtosis`)
	if fmt.Sprint(got) != "-1.2" {
		t.Errorf("kurtosis uniform = %s, want -1.2", got)
	}
}

func TestMeans(t *testing.T) {
	if got := fmt.Sprint(run(t, `weighted_mean([1,2,3]; [1,1,1])`)); got != "2" {
		t.Errorf("weighted_mean = %s", got)
	}
	if got := fmt.Sprint(run(t, `weighted_mean([1,2,3]; [1,0,0])`)); got != "1" {
		t.Errorf("weighted_mean skewed = %s", got)
	}
	if got := fmt.Sprint(run(t, `[1,2,4] | harmonic_mean`)); got != "1.7142857142857142" {
		t.Errorf("harmonic_mean = %s", got)
	}
	if got := fmt.Sprint(run(t, `[1,2,3,4] | rms`)); got != "2.7386127875258306" {
		t.Errorf("rms = %s", got)
	}
	if got := fmt.Sprint(run(t, `[1,2,3] | product`)); got != "6" {
		t.Errorf("product = %s", got)
	}
}

func TestQuartilesAndTrimming(t *testing.T) {
	got := run(t, `[1,2,3,4,5,6,7,8] | quartiles`)
	arr := got.([]any)
	if len(arr) != 5 {
		t.Fatalf("quartiles = %v", arr)
	}
	want := []string{"1", "2.75", "4.5", "6.25", "8"}
	for i, w := range want {
		if fmt.Sprint(arr[i]) != w {
			t.Errorf("quartiles[%d] = %v, want %s", i, arr[i], w)
		}
	}
	if got := fmt.Sprint(run(t, `[1,2,3,100] | trimmed_mean(0.25)`)); got != "2.5" {
		t.Errorf("trimmed_mean = %s", got)
	}
}

func TestStandardize(t *testing.T) {
	got := run(t, `[1,2,3] | standardize`)
	arr := got.([]any)
	if fmt.Sprint(arr[0]) != "-1" || fmt.Sprint(arr[2]) != "1" {
		t.Errorf("standardize = %v", arr)
	}
}
