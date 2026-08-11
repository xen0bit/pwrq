package stats

import (
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterCumsum registers cumsum, the running total of an array.
func RegisterCumsum() gojq.CompilerOption {
	return gojq.WithFunction("cumsum", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cumsum: %v", err), nil)
		}
		out := make([]any, len(values))
		sum := 0.0
		for i, f := range values {
			sum += f
			out[i] = sum
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterCumulativeMax registers cumulative_max, the largest value seen so
// far at each position.
func RegisterCumulativeMax() gojq.CompilerOption {
	return gojq.WithFunction("cumulative_max", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cumulative_max: %v", err), nil)
		}
		out := make([]any, len(values))
		best := values[0]
		for i, f := range values {
			if f > best {
				best = f
			}
			out[i] = best
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterCumulativeMin registers cumulative_min, the smallest value seen so
// far at each position.
func RegisterCumulativeMin() gojq.CompilerOption {
	return gojq.WithFunction("cumulative_min", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cumulative_min: %v", err), nil)
		}
		out := make([]any, len(values))
		best := values[0]
		for i, f := range values {
			if f < best {
				best = f
			}
			out[i] = best
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterDeltas registers deltas, the first differences between consecutive
// values, one shorter than the input.
func RegisterDeltas() gojq.CompilerOption {
	return gojq.WithFunction("deltas", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("deltas: %v", err), nil)
		}
		if len(values) < 2 {
			return common.MakeUDFSuccessResult([]any{}, nil)
		}
		out := make([]any, 0, len(values)-1)
		for i := 1; i < len(values); i++ {
			out = append(out, values[i]-values[i-1])
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterLag registers lag, shifting an array right by n positions and
// filling the gap with null. A negative n shifts left.
func RegisterLag() gojq.CompilerOption {
	return gojq.WithFunction("lag", 1, 2, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("lag: shift must be an integer, got %v", args[0]), nil)
		}
		values := arrInput(v, args)
		out := make([]any, len(values))
		for i := range out {
			out[i] = nil
		}
		for i, item := range values {
			target := i + n
			if target >= 0 && target < len(values) {
				out[target] = item
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterFillForward registers fill_forward, carrying the last non-null value
// forward over nulls. Leading nulls stay null.
func RegisterFillForward() gojq.CompilerOption {
	return gojq.WithFunction("fill_forward", 0, 1, func(v any, args []any) any {
		values := arrInput(v, args)
		out := make([]any, len(values))
		var carry any = nil
		for i, item := range values {
			bound := common.BindValue(item)
			if bound != nil {
				carry = bound
			}
			out[i] = carry
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterEma registers ema, the exponential moving average with smoothing
// factor alpha (0 < alpha <= 1), seeded with the first value.
func RegisterEma() gojq.CompilerOption {
	return gojq.WithFunction("ema", 1, 2, func(v any, args []any) any {
		alpha, ok := common.ToFloat64(common.BindValue(args[0]))
		if !ok || alpha <= 0 || alpha > 1 {
			return common.MakeUDFErrorResult(fmt.Errorf("ema: alpha must be in (0, 1], got %v", args[0]), nil)
		}
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("ema: %v", err), nil)
		}
		out := make([]any, len(values))
		prev := values[0]
		out[0] = prev
		for i := 1; i < len(values); i++ {
			prev = alpha*values[i] + (1-alpha)*prev
			out[i] = prev
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// rollingMinMax registers a rolling window extrema cmdlet.
func rollingExtrema(name string, wantMax bool) gojq.CompilerOption {
	return gojq.WithFunction(name, 1, 2, func(v any, args []any) any {
		window, ok := common.ToInt(args[0])
		if !ok || window <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: window must be a positive integer, got %v", name, args[0]), nil)
		}
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		if window > len(values) {
			return common.MakeUDFSuccessResult([]any{}, nil)
		}
		out := make([]any, 0, len(values)-window+1)
		for start := 0; start+window <= len(values); start++ {
			best := values[start]
			for i := start; i < start+window; i++ {
				if wantMax && values[i] > best {
					best = values[i]
				}
				if !wantMax && values[i] < best {
					best = values[i]
				}
			}
			out = append(out, best)
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterMovingMax registers moving_max, the largest value in each window of
// n.
func RegisterMovingMax() gojq.CompilerOption {
	return rollingExtrema("moving_max", true)
}

// RegisterMovingMin registers moving_min, the smallest value in each window of
// n.
func RegisterMovingMin() gojq.CompilerOption {
	return rollingExtrema("moving_min", false)
}
