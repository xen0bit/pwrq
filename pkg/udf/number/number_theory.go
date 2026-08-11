// Number theory: primes, factorials, combinatorics and divisors.
package number

import (
	"fmt"
	"math"
	"math/big"
	"math/bits"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

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

// isPrime64 is the trial-division primality test the prime cmdlets share.
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

func gcd64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
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
