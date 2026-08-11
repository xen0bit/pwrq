// Package number provides numeric utilities jq leaves to the caller: radix
// conversion, clamping, integer maths, rounding to a precision, byte-size
// humanizing and percentages.
package number

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every number cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterToBase(),
		RegisterFromBase(),
		RegisterToHexNumber(),
		RegisterFromHexNumber(),
		RegisterClamp(),
		RegisterGcd(),
		RegisterLcm(),
		RegisterRoundTo(),
		RegisterHumanBytes(),
		RegisterPercentage(),
		RegisterFactorial(),
		RegisterIsPrime(),
		RegisterFibonacci(),
		RegisterCombinationsCount(),
		RegisterPermutationsCount(),
		RegisterOrdinal(),
		RegisterLerp(),
		RegisterHumanNumber(),
		RegisterIsEven(),
		RegisterIsOdd(),
		RegisterRescale(),
		RegisterPctChange(),
		RegisterDigitSum(),
		RegisterHammingWeight(),
		// Number theory
		RegisterSign(),
		RegisterIsPerfectSquare(),
		RegisterIsCoprime(),
		RegisterNextPrime(),
		RegisterPrimeFactors(),
		RegisterProperDivisors(),
		RegisterIsPerfectNumber(),
		RegisterEulerTotient(),
		RegisterToFixed(),
		RegisterIsPowerOfTwo(),
		RegisterToWords(),
		RegisterRomanNumeral(),
		RegisterGroupDigits(),
		RegisterFormatCurrency(),
		RegisterCollatzSteps(),
	}
}

// num coerces the pipeline value to a float, reporting a usable error.
func num(v any, name string) (float64, error) {
	f, ok := common.ToFloat64(common.BindValue(v))
	if !ok {
		return 0, fmt.Errorf("%s: expected a number, got %T", name, v)
	}
	return f, nil
}

// RegisterToBase registers to_base, rendering a number in any base from 2 to 36.
func RegisterToBase() gojq.CompilerOption {
	return gojq.WithFunction("to_base", 1, 1, func(v any, args []any) any {
		base, ok := common.ToInt(args[0])
		if !ok || base < 2 || base > 36 {
			return common.MakeUDFErrorResult(fmt.Errorf("to_base: base must be 2-36, got %v", args[0]), nil)
		}
		n, err := num(v, "to_base")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(strconv.FormatInt(int64(n), base), nil)
	})
}

// RegisterFromBase registers from_base, parsing a number written in any base
// from 2 to 36.
func RegisterFromBase() gojq.CompilerOption {
	return gojq.WithFunction("from_base", 1, 1, func(v any, args []any) any {
		base, ok := common.ToInt(args[0])
		if !ok || base < 2 || base > 36 {
			return common.MakeUDFErrorResult(fmt.Errorf("from_base: base must be 2-36, got %v", args[0]), nil)
		}
		s, ok := common.BindValue(v).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("from_base: expected a string, got %T", v), nil)
		}
		n, err := strconv.ParseInt(strings.TrimSpace(s), base, 64)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("from_base: cannot parse %q in base %d: %v", s, base, err), nil)
		}
		return common.MakeUDFSuccessResult(n, nil)
	})
}

// RegisterToHexNumber registers to_hex_number, a number to a hex string (the
// byte-oriented hex_encode is the sibling for text).
func RegisterToHexNumber() gojq.CompilerOption {
	return gojq.WithFunction("to_hex_number", 0, 0, func(v any, args []any) any {
		n, err := num(v, "to_hex_number")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(strconv.FormatInt(int64(n), 16), nil)
	})
}

// RegisterFromHexNumber registers from_hex_number, a hex string to a number.
func RegisterFromHexNumber() gojq.CompilerOption {
	return gojq.WithFunction("from_hex_number", 0, 0, func(v any, args []any) any {
		s, ok := common.BindValue(v).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("from_hex_number: expected a string, got %T", v), nil)
		}
		n, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(s), "0x"), 16, 64)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("from_hex_number: cannot parse %q as hex: %v", s, err), nil)
		}
		return common.MakeUDFSuccessResult(n, nil)
	})
}

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

// RegisterGcd registers gcd, the greatest common divisor of two integers.
func RegisterGcd() gojq.CompilerOption {
	return gojq.WithFunction("gcd", 1, 1, func(v any, args []any) any {
		a, err := num(v, "gcd")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		b, ok := common.ToFloat64(common.BindValue(args[0]))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("gcd: expected a number, got %T", args[0]), nil)
		}
		x, y := int64(math.Abs(a)), int64(math.Abs(b))
		for y != 0 {
			x, y = y, x%y
		}
		return common.MakeUDFSuccessResult(x, nil)
	})
}

// RegisterLcm registers lcm, the least common multiple of two integers.
func RegisterLcm() gojq.CompilerOption {
	return gojq.WithFunction("lcm", 1, 1, func(v any, args []any) any {
		a, err := num(v, "lcm")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		b, ok := common.ToFloat64(common.BindValue(args[0]))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("lcm: expected a number, got %T", args[0]), nil)
		}
		x, y := int64(math.Abs(a)), int64(math.Abs(b))
		origX, origY := x, y
		for y != 0 {
			x, y = y, x%y
		}
		if x == 0 {
			return common.MakeUDFSuccessResult(int64(0), nil)
		}
		return common.MakeUDFSuccessResult((origX/x)*origY, nil)
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

// RegisterHumanBytes registers human_bytes, rendering a byte count in binary
// units (KiB, MiB, GiB, ...).
func RegisterHumanBytes() gojq.CompilerOption {
	return gojq.WithFunction("human_bytes", 0, 0, func(v any, args []any) any {
		n, err := num(v, "human_bytes")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(humanBytes(n), nil)
	})
}

func humanBytes(n float64) string {
	if n < 0 {
		return "-" + humanBytes(-n)
	}
	const units = "KMGTPE"
	if n < 1024 {
		return fmt.Sprintf("%d B", int64(n))
	}
	val := n / 1024
	i := 0
	for val >= 1024 && i < len(units)-1 {
		val /= 1024
		i++
	}
	s := fmt.Sprintf("%.1f", val)
	return s + " " + string(units[i]) + "iB"
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
