package stats

import (
	"fmt"
	"math"
	"sort"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAutocorrelation registers autocorrelation, the correlation of an
// array with itself lag steps back: autocorrelation(arr; [lag]), lag
// defaulting to 1.
func RegisterAutocorrelation() gojq.CompilerOption {
	return gojq.WithFunction("autocorrelation", 0, 2, func(v any, args []any) any {
		lag := 1
		for _, a := range args {
			if n, ok := common.ToInt(a); ok {
				lag = n
			}
		}
		if lag < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("autocorrelation: lag must be non-negative, got %d", lag), nil)
		}
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("autocorrelation: %v", err), nil)
		}
		if lag >= len(values) {
			return common.MakeUDFErrorResult(fmt.Errorf("autocorrelation: lag %d needs at least %d values", lag, lag+1), nil)
		}
		m := mean(values)
		var num, den float64
		for i := 0; i+lag < len(values); i++ {
			num += (values[i] - m) * (values[i+lag] - m)
		}
		for _, f := range values {
			d := f - m
			den += d * d
		}
		if den == 0 {
			return common.MakeUDFSuccessResult(0, nil)
		}
		return common.MakeUDFSuccessResult(num/den, nil)
	})
}

// RegisterIQR registers iqr, the interquartile range (q3 - q1) of an array.
func RegisterIQR() gojq.CompilerOption {
	return gojq.WithFunction("iqr", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("iqr: %v", err), nil)
		}
		sort.Float64s(values)
		return common.MakeUDFSuccessResult(percentileAt(values, 75)-percentileAt(values, 25), nil)
	})
}

// RegisterMAD registers mad, the median absolute deviation: the median of the
// distances from each value to the median.
func RegisterMAD() gojq.CompilerOption {
	return gojq.WithFunction("mad", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("mad: %v", err), nil)
		}
		sorted := append([]float64(nil), values...)
		sort.Float64s(sorted)
		m := median(sorted)
		devs := make([]float64, len(values))
		for i, f := range values {
			d := f - m
			if d < 0 {
				d = -d
			}
			devs[i] = d
		}
		sort.Float64s(devs)
		return common.MakeUDFSuccessResult(median(devs), nil)
	})
}

// RegisterSpread registers spread, the difference between the largest and
// smallest values (jq's range is a generator, so this uses its own name).
func RegisterSpread() gojq.CompilerOption {
	return gojq.WithFunction("spread", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("spread: %v", err), nil)
		}
		lo, hi := values[0], values[0]
		for _, f := range values {
			if f < lo {
				lo = f
			}
			if f > hi {
				hi = f
			}
		}
		return common.MakeUDFSuccessResult(hi-lo, nil)
	})
}

// RegisterMovingStdev registers moving_stdev, the rolling sample standard
// deviation over a window of n values.
func RegisterMovingStdev() gojq.CompilerOption {
	return gojq.WithFunction("moving_stdev", 1, 2, func(v any, args []any) any {
		window, ok := common.ToInt(args[0])
		if !ok || window <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("moving_stdev: window must be a positive integer, got %v", args[0]), nil)
		}
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("moving_stdev: %v", err), nil)
		}
		if window < 2 || window > len(values) {
			return common.MakeUDFSuccessResult([]any{}, nil)
		}
		out := make([]any, 0, len(values)-window+1)
		for start := 0; start+window <= len(values); start++ {
			out = append(out, math.Sqrt(variance(values[start:start+window])))
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}
