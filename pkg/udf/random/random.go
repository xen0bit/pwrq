// Package random provides cryptographically random integers, floats, strings,
// and array sampling and shuffling. crypto/rand works under GOOS=js, so these
// run in the browser too.
package random

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every random cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterRandomInt(),
		RegisterRandomFloat(),
		RegisterRandomString(),
		RegisterRandomChoice(),
		RegisterShuffle(),
		RegisterSample(),
	}
}

const defaultAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomIntBelow returns a uniform value in [0, n).
func randomIntBelow(n int64) (int64, error) {
	if n <= 0 {
		return 0, fmt.Errorf("range must be positive")
	}
	v, err := rand.Int(rand.Reader, big.NewInt(n))
	if err != nil {
		return 0, err
	}
	return v.Int64(), nil
}

// RegisterRandomInt registers random_int, a uniform integer in [min, max]
// (inclusive). With one argument it is 0..max; with none, a non-negative int.
func RegisterRandomInt() gojq.CompilerOption {
	return common.WithFunction("random_int", 0, 2, func(v any, args []any) any {
		min, max := int64(0), int64(math.MaxInt32)
		switch len(args) {
		case 1:
			if m, ok := common.ToInt(args[0]); ok {
				max = int64(m)
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("random_int: expected an integer, got %v", args[0]), nil)
			}
		case 2:
			if a, ok := common.ToInt(args[0]); ok {
				min = int64(a)
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("random_int: expected an integer, got %v", args[0]), nil)
			}
			if b, ok := common.ToInt(args[1]); ok {
				max = int64(b)
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("random_int: expected an integer, got %v", args[1]), nil)
			}
		}
		if min > max {
			min, max = max, min
		}
		lo, hi := big.NewInt(min), big.NewInt(max)
		span := new(big.Int).Add(new(big.Int).Sub(hi, lo), big.NewInt(1))
		drawn, err := rand.Int(rand.Reader, span)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(new(big.Int).Add(lo, drawn).Int64(), nil)
	})
}

// RegisterRandomFloat registers random_float, a uniform float. No arguments:
// [0,1). One: [0,max). Two: [min,max).
func RegisterRandomFloat() gojq.CompilerOption {
	return common.WithFunction("random_float", 0, 2, func(v any, args []any) any {
		min, max := 0.0, 1.0
		if len(args) == 1 {
			if m, ok := common.ToFloat64(common.BindValue(args[0])); ok {
				max = m
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("random_float: expected a number, got %v", args[0]), nil)
			}
		}
		if len(args) == 2 {
			a, aOK := common.ToFloat64(common.BindValue(args[0]))
			b, bOK := common.ToFloat64(common.BindValue(args[1]))
			if !aOK || !bOK {
				return common.MakeUDFErrorResult(fmt.Errorf("random_float: expected numbers"), nil)
			}
			min, max = a, b
		}
		if min > max {
			min, max = max, min
		}
		span := max - min
		// 53 random bits give a uniform double in [0,1).
		var b [7]byte
		if _, err := rand.Read(b[:]); err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		bits := binary.LittleEndian.Uint64(append(b[:], 0)) & ((uint64(1) << 53) - 1)
		unit := float64(bits) / float64(uint64(1)<<53)
		return common.MakeUDFSuccessResult(min+unit*span, nil)
	})
}

// RegisterRandomString registers random_string, n characters drawn from an
// alphabet (alphanumeric by default).
func RegisterRandomString() gojq.CompilerOption {
	return common.WithFunction("random_string", 1, 2, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok || n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("random_string: length must be a non-negative integer, got %v", args[0]), nil)
		}
		alphabet := defaultAlphabet
		if len(args) > 1 {
			if s, ok := common.BindValue(args[1]).(string); ok && s != "" {
				alphabet = s
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("random_string: alphabet must be a non-empty string"), nil)
			}
		}
		runes := []rune(alphabet)
		var out []rune
		for i := 0; i < n; i++ {
			idx, err := randomIntBelow(int64(len(runes)))
			if err != nil {
				return common.MakeUDFErrorResult(err, nil)
			}
			out = append(out, runes[idx])
		}
		return common.MakeUDFSuccessResult(string(out), nil)
	})
}

// arrInput resolves the array from the first argument or the pipeline.
func arrInput(v any, args []any) ([]any, error) {
	var arr []any
	if len(args) > 0 {
		if a, ok := common.BindValue(args[0]).([]any); ok {
			arr = a
		} else {
			return nil, fmt.Errorf("expected an array, got %T", args[0])
		}
	} else {
		arr = common.NormalizeToSlice(common.BindValue(v))
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("expected a non-empty array")
	}
	return arr, nil
}

// RegisterRandomChoice registers random_choice, a uniformly chosen element.
func RegisterRandomChoice() gojq.CompilerOption {
	return common.WithFunction("random_choice", 0, 1, func(v any, args []any) any {
		arr, err := arrInput(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("random_choice: %v", err), nil)
		}
		idx, err := randomIntBelow(int64(len(arr)))
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(arr[idx], nil)
	})
}

// RegisterShuffle registers shuffle, a random permutation of an array.
func RegisterShuffle() gojq.CompilerOption {
	return common.WithFunction("shuffle", 0, 1, func(v any, args []any) any {
		arr, err := arrInput(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("shuffle: %v", err), nil)
		}
		out := append([]any(nil), arr...)
		for i := len(out) - 1; i > 0; i-- {
			j, err := randomIntBelow(int64(i + 1))
			if err != nil {
				return common.MakeUDFErrorResult(err, nil)
			}
			out[i], out[j] = out[j], out[i]
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterSample registers sample, n distinct elements chosen at random (all
// of them when n is at least the array's length).
func RegisterSample() gojq.CompilerOption {
	return common.WithFunction("sample", 1, 2, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok || n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("sample: n must be a non-negative integer, got %v", args[0]), nil)
		}
		arr, err := arrInput(v, args[1:])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("sample: %v", err), nil)
		}
		if n >= len(arr) {
			return common.MakeUDFSuccessResult(append([]any(nil), arr...), nil)
		}
		pool := append([]any(nil), arr...)
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			idx, err := randomIntBelow(int64(len(pool)))
			if err != nil {
				return common.MakeUDFErrorResult(err, nil)
			}
			out = append(out, pool[idx])
			pool[idx] = pool[len(pool)-1]
			pool = pool[:len(pool)-1]
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}
