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
		arr, rest, err := arrInput(v, args, 1, "rotate")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		n, ok := common.ToInt(rest[0])
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("rotate: shift must be an integer, got %v", rest[0]), nil)
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
		arr, rest, err := arrInput(v, args, 1, "top_n")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		n, ok := common.ToInt(rest[0])
		if !ok || n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("top_n: count must be a non-negative integer, got %v", rest[0]), nil)
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

// RegisterInterleave registers interleave, alternating the elements of the
// input array with those of the argument array.
//
// The input supplies the first element of every pair in both calling forms:
// `[1,2] | interleave(["a","b"])` and `interleave([1,2]; ["a","b"])` agree.
func RegisterInterleave() gojq.CompilerOption {
	return gojq.WithFunction("interleave", 1, 2, func(v any, args []any) any {
		a, rest, err := arrInput(v, args, 1, "interleave")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		b, ok := common.BindValue(rest[0]).([]any)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("interleave: argument must be an array, got %T", rest[0]), nil)
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
