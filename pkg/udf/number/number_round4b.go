package number

import (
	"fmt"
	"strconv"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterToFixed registers to_fixed, a number rendered with a fixed number of
// decimal places as a string: to_fixed(n; places).
func RegisterToFixed() gojq.CompilerOption {
	return gojq.WithFunction("to_fixed", 1, 1, func(v any, args []any) any {
		places, ok := common.ToInt(args[0])
		if !ok || places < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("to_fixed: places must be a non-negative integer, got %v", args[0]), nil)
		}
		f, ok := common.ToFloat64(common.BindValue(v))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("to_fixed: expected a number, got %T", v), nil)
		}
		return common.MakeUDFSuccessResult(strconv.FormatFloat(f, 'f', places, 64), nil)
	})
}

// RegisterIsPowerOfTwo registers is_power_of_two, whether a positive integer
// is a power of two.
func RegisterIsPowerOfTwo() gojq.CompilerOption {
	return gojq.WithFunction("is_power_of_two", 0, 1, func(v any, args []any) any {
		n, err := intIn(v, args, "is_power_of_two")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if n <= 0 {
			return common.MakeUDFSuccessResult(false, nil)
		}
		return common.MakeUDFSuccessResult(n&(n-1) == 0, nil)
	})
}
