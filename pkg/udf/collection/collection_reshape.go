// Reshaping arrays: grouping them, reordering them, and pairing them up.
package collection

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterChunks registers chunks, splitting an array into chunks of at most n
// elements.
func RegisterChunks() gojq.CompilerOption {
	return common.WithFunction("chunks", 1, 2, func(v any, args []any) any {
		arr, rest, err := arrInput(v, args, 1, "chunks")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		n, ok := common.ToInt(rest[0])
		if !ok || n <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("chunks: size must be a positive integer, got %v", rest[0]), nil)
		}
		out := make([]any, 0, (len(arr)+n-1)/n)
		for i := 0; i < len(arr); i += n {
			end := i + n
			if end > len(arr) {
				end = len(arr)
			}
			out = append(out, arr[i:end])
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterWindows registers windows, the rolling n-element windows of an array:
// [1,2,3,4] | windows(3) -> [[1,2,3],[2,3,4]].
func RegisterWindows() gojq.CompilerOption {
	return common.WithFunction("windows", 1, 2, func(v any, args []any) any {
		arr, rest, err := arrInput(v, args, 1, "windows")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		n, ok := common.ToInt(rest[0])
		if !ok || n <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("windows: size must be a positive integer, got %v", rest[0]), nil)
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

// RegisterRotate registers rotate, rotating an array left by n (negative
// rotates right).
func RegisterRotate() gojq.CompilerOption {
	return common.WithFunction("rotate", 1, 2, func(v any, args []any) any {
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

// RegisterZipArrays registers zip_arrays, pairing the input array with the
// argument array element by element, up to the shorter length.
//
// The left element of every pair comes from the input and the right from the
// operand, in both calling forms: `[1,2] | zip_arrays(["a","b"])` and
// `zip_arrays([1,2]; ["a","b"])` agree.
func RegisterZipArrays() gojq.CompilerOption {
	return common.WithFunction("zip_arrays", 1, 2, func(v any, args []any) any {
		left, rest, err := arrInput(v, args, 1, "zip_arrays")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		right, ok := common.BindValue(rest[0]).([]any)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("zip_arrays: argument must be an array, got %T", rest[0]), nil)
		}
		n := len(left)
		if len(right) < n {
			n = len(right)
		}
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, []any{left[i], right[i]})
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
	return common.WithFunction("interleave", 1, 2, func(v any, args []any) any {
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

// RegisterColumn registers column, the nth element of every row of an array of
// arrays.
func RegisterColumn() gojq.CompilerOption {
	return common.WithFunction("column", 1, 2, func(v any, args []any) any {
		arr, rest, err := arrInput(v, args, 1, "column")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		n, ok := common.ToInt(rest[0])
		if !ok || n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("column: index must be a non-negative integer, got %v", rest[0]), nil)
		}
		out := make([]any, 0, len(arr))
		for _, row := range arr {
			rowArr, ok := common.BindValue(row).([]any)
			if !ok || n >= len(rowArr) {
				out = append(out, nil)
				continue
			}
			out = append(out, rowArr[n])
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterTopN registers top_n, the n largest values of an array, sorted
// descending.
func RegisterTopN() gojq.CompilerOption {
	return common.WithFunction("top_n", 1, 2, func(v any, args []any) any {
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

// RegisterNaturalSort registers natural_sort, an array of strings in human
// order, where "file2" sorts before "file10".
func RegisterNaturalSort() gojq.CompilerOption {
	return common.WithFunction("natural_sort", 0, 1, func(v any, args []any) any {
		arr, _, err := arrInput(v, args, 0, "natural_sort")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		out := append([]any{}, arr...)
		sort.SliceStable(out, func(i, j int) bool {
			return naturalLess(stringify(out[i]), stringify(out[j]))
		})
		return common.MakeUDFSuccessResult(out, nil)
	})
}

func stringify(v any) string {
	switch val := common.BindValue(v).(type) {
	case string:
		return val
	case nil:
		return ""
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprint(val)
		}
		return string(b)
	}
}

// naturalLess compares two strings digit-run by digit-run, so "a2" < "a10".
func naturalLess(a, b string) bool {
	for len(a) > 0 && len(b) > 0 {
		if isDigit(a[0]) && isDigit(b[0]) {
			i, j := 0, 0
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			na, nb := a[:i], b[:j]
			// Skip leading zeros for length comparison.
			ta, tb := na, nb
			for len(ta) > 1 && ta[0] == '0' {
				ta = ta[1:]
			}
			for len(tb) > 1 && tb[0] == '0' {
				tb = tb[1:]
			}
			if len(ta) != len(tb) {
				return len(ta) < len(tb)
			}
			if ta != tb {
				return ta < tb
			}
			a, b = a[i:], b[j:]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
