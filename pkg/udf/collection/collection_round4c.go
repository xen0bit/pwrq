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
