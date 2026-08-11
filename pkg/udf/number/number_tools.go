package number

import (
	"fmt"
	"math/bits"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterRescale registers rescale, mapping a value from one range to
// another: rescale(v; fromLo; fromHi; toLo; toHi).
func RegisterRescale() gojq.CompilerOption {
	return gojq.WithFunction("rescale", 4, 4, func(v any, args []any) any {
		vals := make([]float64, 4)
		for i := 0; i < 4; i++ {
			f, ok := common.ToFloat64(common.BindValue(args[i]))
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("rescale: expected numbers, got %v", args[i]), nil)
			}
			vals[i] = f
		}
		fromLo, fromHi, toLo, toHi := vals[0], vals[1], vals[2], vals[3]
		value, ok := common.ToFloat64(common.BindValue(v))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("rescale: expected a number, got %T", v), nil)
		}
		if fromLo == fromHi {
			return common.MakeUDFErrorResult(fmt.Errorf("rescale: the from-range is empty"), nil)
		}
		return common.MakeUDFSuccessResult((value-fromLo)/(fromHi-fromLo)*(toHi-toLo)+toLo, nil)
	})
}

// RegisterPctChange registers pct_change, the percentage change from one value
// to another.
func RegisterPctChange() gojq.CompilerOption {
	return gojq.WithFunction("pct_change", 1, 1, func(v any, args []any) any {
		a, aOK := common.ToFloat64(common.BindValue(v))
		b, bOK := common.ToFloat64(common.BindValue(args[0]))
		if !aOK || !bOK {
			return common.MakeUDFErrorResult(fmt.Errorf("pct_change: expected numbers"), nil)
		}
		if a == 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("pct_change: cannot divide by zero"), nil)
		}
		return common.MakeUDFSuccessResult((b-a)/a*100, nil)
	})
}

// RegisterDigitSum registers digit_sum, the sum of an integer's digits.
func RegisterDigitSum() gojq.CompilerOption {
	return gojq.WithFunction("digit_sum", 0, 0, func(v any, args []any) any {
		n, err := intInput(v, "digit_sum")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if n < 0 {
			n = -n
		}
		sum := int64(0)
		for n > 0 {
			sum += n % 10
			n /= 10
		}
		return common.MakeUDFSuccessResult(sum, nil)
	})
}

// RegisterHammingWeight registers hamming_weight, the number of set bits in an
// integer.
func RegisterHammingWeight() gojq.CompilerOption {
	return gojq.WithFunction("hamming_weight", 0, 0, func(v any, args []any) any {
		n, err := intInput(v, "hamming_weight")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(bits.OnesCount64(uint64(n)), nil)
	})
}
