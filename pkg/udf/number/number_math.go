// Arithmetic helpers: rounding, clamping, scaling and parity.
package number

import (
	"fmt"
	"math"
	"strconv"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterClamp registers clamp, bounding a number to a range.
func RegisterClamp() gojq.CompilerOption {
	return gojq.WithFunction("clamp", 2, 2, func(v any, args []any) any {
		n, err := num(v, "clamp")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		lo, loOK := common.ToFloat64(common.BindValue(args[0]))
		hi, hiOK := common.ToFloat64(common.BindValue(args[1]))
		if !loOK || !hiOK {
			return common.MakeUDFErrorResult(fmt.Errorf("clamp: bounds must be numbers, got %v and %v", args[0], args[1]), nil)
		}
		if lo > hi {
			lo, hi = hi, lo
		}
		return common.MakeUDFSuccessResult(math.Min(math.Max(n, lo), hi), nil)
	})
}

// RegisterRoundTo registers round_to, rounding a number to a number of decimal
// places (negative places round to tens, hundreds and so on).
func RegisterRoundTo() gojq.CompilerOption {
	return gojq.WithFunction("round_to", 1, 1, func(v any, args []any) any {
		places, ok := common.ToInt(args[0])
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("round_to: places must be an integer, got %v", args[0]), nil)
		}
		n, err := num(v, "round_to")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		factor := math.Pow10(places)
		return common.MakeUDFSuccessResult(math.Round(n*factor)/factor, nil)
	})
}

// RegisterToFixed registers to_fixed, a number rendered with a fixed number of
// decimal places as a string: to_fixed(n; places).
func RegisterToFixed() gojq.CompilerOption {
	return gojq.WithFunction("to_fixed", 1, 1, func(v any, args []any) any {
		places, ok := common.ToInt(args[0])
		if !ok || places < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("to_fixed: places must be a non-negative integer, got %v", args[0]), nil)
		}
		f, ok := common.ToFloat64(common.BindValue(v))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("to_fixed: expected a number, got %T", v), nil)
		}
		return common.MakeUDFSuccessResult(strconv.FormatFloat(f, 'f', places, 64), nil)
	})
}

// RegisterSign registers sign, -1, 0 or 1 for a number.
func RegisterSign() gojq.CompilerOption {
	return gojq.WithFunction("sign", 0, 1, func(v any, args []any) any {
		f, ok := common.ToFloat64(common.BindValue(v))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("sign: expected a number, got %T", v), nil)
		}
		switch {
		case f > 0:
			return common.MakeUDFSuccessResult(1, nil)
		case f < 0:
			return common.MakeUDFSuccessResult(-1, nil)
		default:
			return common.MakeUDFSuccessResult(0, nil)
		}
	})
}

// RegisterIsEven registers is_even, whether an integer is even.
func RegisterIsEven() gojq.CompilerOption {
	return gojq.WithFunction("is_even", 0, 0, func(v any, args []any) any {
		n, err := intInput(v, "is_even")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(n%2 == 0, nil)
	})
}

// RegisterIsOdd registers is_odd, whether an integer is odd.
func RegisterIsOdd() gojq.CompilerOption {
	return gojq.WithFunction("is_odd", 0, 0, func(v any, args []any) any {
		n, err := intInput(v, "is_odd")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(n%2 != 0, nil)
	})
}

// RegisterIsPowerOfTwo registers is_power_of_two, whether a positive integer
// is a power of two.
func RegisterIsPowerOfTwo() gojq.CompilerOption {
	return gojq.WithFunction("is_power_of_two", 0, 1, func(v any, args []any) any {
		n, err := intIn(v, args, "is_power_of_two")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if n <= 0 {
			return common.MakeUDFSuccessResult(false, nil)
		}
		return common.MakeUDFSuccessResult(n&(n-1) == 0, nil)
	})
}

// RegisterLerp registers lerp, linear interpolation between a and b at t in
// [0,1].
func RegisterLerp() gojq.CompilerOption {
	return gojq.WithFunction("lerp", 2, 2, func(v any, args []any) any {
		a, aOK := common.ToFloat64(common.BindValue(v))
		b, bOK := common.ToFloat64(common.BindValue(args[0]))
		t, tOK := common.ToFloat64(common.BindValue(args[1]))
		if !aOK || !bOK || !tOK {
			return common.MakeUDFErrorResult(fmt.Errorf("lerp: expected numbers for a, b and t"), nil)
		}
		return common.MakeUDFSuccessResult(a+(b-a)*t, nil)
	})
}

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

// RegisterPercentage registers percentage, part as a percentage of whole.
func RegisterPercentage() gojq.CompilerOption {
	return gojq.WithFunction("percentage", 1, 1, func(v any, args []any) any {
		part, err := num(v, "percentage")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		whole, ok := common.ToFloat64(common.BindValue(args[0]))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("percentage: expected a number, got %T", args[0]), nil)
		}
		if whole == 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("percentage: cannot divide by zero"), nil)
		}
		return common.MakeUDFSuccessResult(part/whole*100, nil)
	})
}
