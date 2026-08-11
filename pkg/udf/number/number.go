// Package number provides numeric cmdlets: number theory, arithmetic helpers
// and human-readable formatting.
package number

import (
	"fmt"
	"math"

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
		RegisterNextPrime(),
		RegisterPrimeFactors(),
		RegisterToFixed(),
		RegisterIsPowerOfTwo(),
		RegisterGroupDigits(),
		RegisterFormatCurrency(),
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

// intInput coerces the pipeline value to an int64.
func intInput(v any, name string) (int64, error) {
	f, ok := common.ToFloat64(common.BindValue(v))
	if !ok || f != math.Trunc(f) {
		return 0, fmt.Errorf("%s: expected an integer, got %T", name, v)
	}
	return int64(f), nil
}

// intIn resolves a non-negative integer from the pipeline or first argument.
func intIn(v any, args []any, name string) (int64, error) {
	var input = v
	if len(args) > 0 {
		input = args[0]
	}
	f, ok := common.ToFloat64(common.BindValue(input))
	if !ok || f < 0 || f != float64(int64(f)) {
		return 0, fmt.Errorf("%s: expected a non-negative integer, got %v", name, input)
	}
	return int64(f), nil
}
