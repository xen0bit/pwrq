package number

import (
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// intIn resolves a non-negative integer from the pipeline or first argument.
func intIn(v any, args []any, name string) (int64, error) {
	var input any = v
	if len(args) > 0 {
		input = args[0]
	}
	f, ok := common.ToFloat64(common.BindValue(input))
	if !ok || f < 0 || f != float64(int64(f)) {
		return 0, fmt.Errorf("%s: expected a non-negative integer, got %v", name, input)
	}
	return int64(f), nil
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

// RegisterIsPerfectSquare registers is_perfect_square, whether an integer is a
// perfect square.
func RegisterIsPerfectSquare() gojq.CompilerOption {
	return gojq.WithFunction("is_perfect_square", 0, 1, func(v any, args []any) any {
		n, err := intIn(v, args, "is_perfect_square")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		root := integerSqrt(n)
		return common.MakeUDFSuccessResult(root*root == n, nil)
	})
}

// integerSqrt returns the floor of the square root, via Newton's method.
func integerSqrt(n int64) int64 {
	if n < 2 {
		return n
	}
	x := n
	y := (x + 1) / 2
	for y < x {
		x = y
		y = (x + n/x) / 2
	}
	return x
}

// RegisterIsCoprime registers is_coprime, whether two integers share no prime
// factor.
func RegisterIsCoprime() gojq.CompilerOption {
	return gojq.WithFunction("is_coprime", 1, 1, func(v any, args []any) any {
		a, err := intIn(v, nil, "is_coprime")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		b, err := intIn(args[0], nil, "is_coprime")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(gcd64(a, b) == 1, nil)
	})
}

func gcd64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// RegisterNextPrime registers next_prime, the smallest prime at least n.
func RegisterNextPrime() gojq.CompilerOption {
	return gojq.WithFunction("next_prime", 0, 1, func(v any, args []any) any {
		n, err := intIn(v, args, "next_prime")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if n < 2 {
			n = 2
		}
		for !isPrime64(n) {
			n++
		}
		return common.MakeUDFSuccessResult(n, nil)
	})
}

func isPrime64(n int64) bool {
	if n < 2 {
		return false
	}
	if n%2 == 0 {
		return n == 2
	}
	for d := int64(3); d*d <= n; d += 2 {
		if n%d == 0 {
			return false
		}
	}
	return true
}

// RegisterPrimeFactors registers prime_factors, the prime factors of an
// integer with multiplicity: 60 -> [2, 2, 3, 5].
func RegisterPrimeFactors() gojq.CompilerOption {
	return gojq.WithFunction("prime_factors", 0, 1, func(v any, args []any) any {
		n, err := intIn(v, args, "prime_factors")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if n < 2 {
			return common.MakeUDFSuccessResult([]any{}, nil)
		}
		out := []any{}
		for d := int64(2); d*d <= n; d++ {
			for n%d == 0 {
				out = append(out, d)
				n /= d
			}
		}
		if n > 1 {
			out = append(out, n)
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterProperDivisors registers proper_divisors, the positive divisors of an
// integer below itself: 12 -> [1, 2, 3, 4, 6].
func RegisterProperDivisors() gojq.CompilerOption {
	return gojq.WithFunction("proper_divisors", 0, 1, func(v any, args []any) any {
		n, err := intIn(v, args, "proper_divisors")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if n < 2 {
			return common.MakeUDFSuccessResult([]any{}, nil)
		}
		out := []any{int64(1)}
		for d := int64(2); d*d <= n; d++ {
			if n%d == 0 {
				out = append(out, d)
				if d != n/d {
					out = append(out, n/d)
				}
			}
		}
		sortInts(out)
		return common.MakeUDFSuccessResult(out, nil)
	})
}

func sortInts(arr []any) {
	for i := 1; i < len(arr); i++ {
		for j := i; j > 0; j-- {
			aj, okJ := arr[j].(int64)
			ap, okP := arr[j-1].(int64)
			if !okJ || !okP || aj >= ap {
				break
			}
			arr[j], arr[j-1] = arr[j-1], arr[j]
		}
	}
}

// RegisterIsPerfectNumber registers is_perfect_number, whether an integer
// equals the sum of its proper divisors (6 = 1+2+3).
func RegisterIsPerfectNumber() gojq.CompilerOption {
	return gojq.WithFunction("is_perfect_number", 0, 1, func(v any, args []any) any {
		n, err := intIn(v, args, "is_perfect_number")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if n < 2 {
			return common.MakeUDFSuccessResult(false, nil)
		}
		sum := int64(1)
		for d := int64(2); d*d <= n; d++ {
			if n%d == 0 {
				sum += d
				if d != n/d {
					sum += n / d
				}
			}
		}
		return common.MakeUDFSuccessResult(sum == n, nil)
	})
}

// RegisterEulerTotient registers euler_totient, how many integers below n are
// coprime to n.
func RegisterEulerTotient() gojq.CompilerOption {
	return gojq.WithFunction("euler_totient", 0, 1, func(v any, args []any) any {
		n, err := intIn(v, args, "euler_totient")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		result := n
		remaining := n
		for p := int64(2); p*p <= remaining; p++ {
			if remaining%p == 0 {
				for remaining%p == 0 {
					remaining /= p
				}
				result -= result / p
			}
		}
		if remaining > 1 {
			result -= result / remaining
		}
		return common.MakeUDFSuccessResult(result, nil)
	})
}
