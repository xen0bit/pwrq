package stats

import (
	"fmt"
	"math"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterMovingAverage registers moving_average, the rolling mean over a
// window of n values.
func RegisterMovingAverage() gojq.CompilerOption {
	return gojq.WithFunction("moving_average", 1, 2, func(v any, args []any) any {
		window, ok := common.ToInt(args[0])
		if !ok || window <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("moving_average: window must be a positive integer, got %v", args[0]), nil)
		}
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("moving_average: %v", err), nil)
		}
		if window > len(values) {
			return common.MakeUDFSuccessResult([]any{}, nil)
		}
		out := make([]any, 0, len(values)-window+1)
		sum := 0.0
		for i := 0; i < window; i++ {
			sum += values[i]
		}
		out = append(out, sum/float64(window))
		for i := window; i < len(values); i++ {
			sum += values[i] - values[i-window]
			out = append(out, sum/float64(window))
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterGeomean registers geomean, the geometric mean of an array of
// positive numbers.
func RegisterGeomean() gojq.CompilerOption {
	return gojq.WithFunction("geomean", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("geomean: %v", err), nil)
		}
		logSum := 0.0
		for _, f := range values {
			if f <= 0 {
				return common.MakeUDFErrorResult(fmt.Errorf("geomean: values must be positive, got %v", f), nil)
			}
			logSum += math.Log(f)
		}
		return common.MakeUDFSuccessResult(math.Exp(logSum/float64(len(values))), nil)
	})
}

// RegisterNormalize registers normalize, min-max scaling an array to [0,1].
func RegisterNormalize() gojq.CompilerOption {
	return gojq.WithFunction("normalize", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("normalize: %v", err), nil)
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
		out := make([]any, len(values))
		if hi == lo {
			for i := range out {
				out[i] = 0
			}
			return common.MakeUDFSuccessResult(out, nil)
		}
		span := hi - lo
		for i, f := range values {
			out[i] = (f - lo) / span
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}
