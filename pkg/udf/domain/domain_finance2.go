package domain

import (
	"fmt"
	"math"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterNetPresentValue registers net_present_value, the present value of a
// series of cash flows at a rate per period: net_present_value(flows; rate).
// The first flow is typically the initial investment, negative.
func RegisterNetPresentValue() gojq.CompilerOption {
	return gojq.WithFunction("net_present_value", 2, 2, func(v any, args []any) any {
		flows, ok := common.BindValue(args[0]).([]any)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("net_present_value: flows must be an array, got %T", args[0]), nil)
		}
		rate, err := numArg(args, 1, "net_present_value")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		npv := 0.0
		for i, flow := range flows {
			f, ok := common.ToFloat64(common.BindValue(flow))
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("net_present_value: flow %d is not a number (%T)", i, flow), nil)
			}
			npv += f / math.Pow(1+rate, float64(i))
		}
		return common.MakeUDFSuccessResult(npv, nil)
	})
}
