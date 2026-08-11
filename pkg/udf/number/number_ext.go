package number

import (
	"fmt"
	"math"
	"math/big"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// intInput coerces the pipeline value to an int64.
func intInput(v any, name string) (int64, error) {
	f, ok := common.ToFloat64(common.BindValue(v))
	if !ok || f != math.Trunc(f) {
		return 0, fmt.Errorf("%s: expected an integer, got %T", name, v)
	}
	return int64(f), nil
}

// RegisterFactorial registers factorial, n! for an integer n.
func RegisterFactorial() gojq.CompilerOption {
	return gojq.WithFunction("factorial", 0, 0, func(v any, args []any) any {
		n, err := intInput(v, "factorial")
		if err != nil || n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("factorial: expected a non-negative integer"), nil)
		}
		result := big.NewInt(1)
		for i := int64(2); i <= n; i++ {
			result.Mul(result, big.NewInt(i))
		}
		return common.MakeUDFSuccessResult(result, nil)
	})
}

// RegisterIsPrime registers is_prime, whether an integer is prime.
func RegisterIsPrime() gojq.CompilerOption {
	return gojq.WithFunction("is_prime", 0, 0, func(v any, args []any) any {
		n, err := intInput(v, "is_prime")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(isPrime(n), nil)
	})
}

func isPrime(n int64) bool {
	if n < 2 {
		return false
	}
	if n < 4 {
		return true
	}
	if n%2 == 0 || n%3 == 0 {
		return false
	}
	// Trial division is exact and fast for the sizes that matter here.
	for i := int64(5); i*i <= n; i += 6 {
		if n%i == 0 || n%(i+2) == 0 {
			return false
		}
	}
	return true
}

// RegisterFibonacci registers fibonacci, the nth Fibonacci number (0-indexed).
func RegisterFibonacci() gojq.CompilerOption {
	return gojq.WithFunction("fibonacci", 0, 0, func(v any, args []any) any {
		n, err := intInput(v, "fibonacci")
		if err != nil || n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("fibonacci: expected a non-negative integer"), nil)
		}
		if n == 0 {
			return common.MakeUDFSuccessResult(big.NewInt(0), nil)
		}
		a, b := big.NewInt(0), big.NewInt(1)
		for i := int64(1); i < n; i++ {
			a, b = b, new(big.Int).Add(a, b)
		}
		return common.MakeUDFSuccessResult(b, nil)
	})
}

// RegisterCombinationsCount registers combinations_count, how many ways to
// choose k items from n.
func RegisterCombinationsCount() gojq.CompilerOption {
	return gojq.WithFunction("combinations_count", 1, 1, func(v any, args []any) any {
		n, err := intInput(v, "combinations_count")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		k, ok := common.ToInt(args[0])
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("combinations_count: expected an integer, got %v", args[0]), nil)
		}
		return common.MakeUDFSuccessResult(binomial(n, int64(k)), nil)
	})
}

func binomial(n, k int64) *big.Int {
	if k < 0 || k > n {
		return big.NewInt(0)
	}
	if k > n-k {
		k = n - k
	}
	result := big.NewInt(1)
	for i := int64(0); i < k; i++ {
		result.Mul(result, big.NewInt(n-i))
		result.Div(result, big.NewInt(i+1))
	}
	return result
}

// RegisterPermutationsCount registers permutations_count, how many ways to
// order k items chosen from n.
func RegisterPermutationsCount() gojq.CompilerOption {
	return gojq.WithFunction("permutations_count", 1, 1, func(v any, args []any) any {
		n, err := intInput(v, "permutations_count")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		k, ok := common.ToInt(args[0])
		if !ok || int64(k) > n {
			return common.MakeUDFErrorResult(fmt.Errorf("permutations_count: expected k <= n"), nil)
		}
		result := big.NewInt(1)
		for i := int64(0); i < int64(k); i++ {
			result.Mul(result, big.NewInt(n-i))
		}
		return common.MakeUDFSuccessResult(result, nil)
	})
}

// RegisterOrdinal registers ordinal, an integer as "1st", "2nd", "3rd", ...
func RegisterOrdinal() gojq.CompilerOption {
	return gojq.WithFunction("ordinal", 0, 0, func(v any, args []any) any {
		n, err := intInput(v, "ordinal")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(ordinal(n), nil)
	})
}

func ordinal(n int64) string {
	abs := n
	if abs < 0 {
		abs = -abs
	}
	suffix := "th"
	switch abs % 100 {
	case 11, 12, 13:
		suffix = "th"
	default:
		switch abs % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
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

// RegisterHumanNumber registers human_number, a count rendered compactly with
// k, M, B and T suffixes.
func RegisterHumanNumber() gojq.CompilerOption {
	return gojq.WithFunction("human_number", 0, 0, func(v any, args []any) any {
		n, ok := common.ToFloat64(common.BindValue(v))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("human_number: expected a number, got %T", v), nil)
		}
		return common.MakeUDFSuccessResult(humanNumber(n), nil)
	})
}

func humanNumber(n float64) string {
	if n < 0 {
		return "-" + humanNumber(-n)
	}
	abs := math.Abs(n)
	switch {
	case abs < 1000:
		return fmt.Sprintf("%.0f", n)
	case abs < 1e6:
		return trim1(n/1e3) + "k"
	case abs < 1e9:
		return trim1(n/1e6) + "M"
	case abs < 1e12:
		return trim1(n/1e9) + "B"
	default:
		return trim1(n/1e12) + "T"
	}
}

func trim1(f float64) string {
	s := fmt.Sprintf("%.1f", f)
	for len(s) > 1 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
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
