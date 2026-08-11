package collection

import (
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterWindows registers windows, the rolling n-element windows of an array:
// [1,2,3,4] | windows(3) -> [[1,2,3],[2,3,4]].
func RegisterWindows() gojq.CompilerOption {
	return gojq.WithFunction("windows", 1, 2, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok || n <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("windows: size must be a positive integer, got %v", args[0]), nil)
		}
		arr, err := arrInput(v, args[1:])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("windows: %v", err), nil)
		}
		if n > len(arr) {
			return common.MakeUDFSuccessResult([]any{}, nil)
		}
		out := make([]any, 0, len(arr)-n+1)
		for start := 0; start+n <= len(arr); start++ {
			window := make([]any, n)
			copy(window, arr[start:start+n])
			out = append(out, window)
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterPairs registers pairs, the adjacent pairs of an array:
// [1,2,3] | pairs -> [[1,2],[2,3]].
func RegisterPairs() gojq.CompilerOption {
	return gojq.WithFunction("pairs", 0, 1, func(v any, args []any) any {
		arr, err := arrInput(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("pairs: %v", err), nil)
		}
		out := make([]any, 0, len(arr))
		for i := 0; i+1 < len(arr); i++ {
			out = append(out, []any{arr[i], arr[i+1]})
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterIsSubset registers is_subset, whether every element of the first
// array is present in the second.
func RegisterIsSubset() gojq.CompilerOption {
	return gojq.WithFunction("is_subset", 1, 2, func(v any, args []any) any {
		a, b, err := twoArrs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("is_subset: %v", err), nil)
		}
		inB := make(map[string]bool, len(b))
		for _, item := range b {
			inB[keyOf(item)] = true
		}
		for _, item := range a {
			if !inB[keyOf(item)] {
				return common.MakeUDFSuccessResult(false, nil)
			}
		}
		return common.MakeUDFSuccessResult(true, nil)
	})
}
