package collection

import (
	"fmt"
	"sort"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterRotate registers rotate, rotating an array left by n (negative
// rotates right).
func RegisterRotate() gojq.CompilerOption {
	return gojq.WithFunction("rotate", 1, 2, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("rotate: shift must be an integer, got %v", args[0]), nil)
		}
		arr, err := arrInput(v, args[1:])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("rotate: %v", err), nil)
		}
		if len(arr) == 0 {
			return common.MakeUDFSuccessResult(arr, nil)
		}
		shift := n % len(arr)
		if shift < 0 {
			shift += len(arr)
		}
		out := make([]any, len(arr))
		for i := range arr {
			out[i] = arr[(i+shift)%len(arr)]
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterTopN registers top_n, the n largest values of an array, sorted
// descending.
func RegisterTopN() gojq.CompilerOption {
	return gojq.WithFunction("top_n", 1, 2, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok || n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("top_n: count must be a non-negative integer, got %v", args[0]), nil)
		}
		arr, err := arrInput(v, args[1:])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("top_n: %v", err), nil)
		}
		type scored struct {
			value float64
			item  any
		}
		scoredList := make([]scored, 0, len(arr))
		for _, item := range arr {
			f, ok := common.ToFloat64(common.BindValue(item))
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("top_n: expected numbers, got %T", item), nil)
			}
			scoredList = append(scoredList, scored{value: f, item: item})
		}
		sort.Slice(scoredList, func(i, j int) bool { return scoredList[i].value > scoredList[j].value })
		if n > len(scoredList) {
			n = len(scoredList)
		}
		out := make([]any, n)
		for i := 0; i < n; i++ {
			out[i] = scoredList[i].item
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterInterleave registers interleave, alternating the elements of an
// array from the pipeline with one from the argument.
func RegisterInterleave() gojq.CompilerOption {
	return gojq.WithFunction("interleave", 1, 2, func(v any, args []any) any {
		b, ok := common.BindValue(args[0]).([]any)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("interleave: argument must be an array, got %T", args[0]), nil)
		}
		a, err := arrInput(v, args[1:])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("interleave: %v", err), nil)
		}
		n := len(a)
		if len(b) < n {
			n = len(b)
		}
		out := make([]any, 0, n*2)
		for i := 0; i < n; i++ {
			out = append(out, a[i], b[i])
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}
