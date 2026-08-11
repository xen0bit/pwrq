package stats

import (
	"fmt"
	"sort"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterPercentileRank registers percentile_rank, the percentage of an
// array's values at or below a given value: percentile_rank(arr; value).
func RegisterPercentileRank() gojq.CompilerOption {
	return gojq.WithFunction("percentile_rank", 1, 2, func(v any, args []any) any {
		want, ok := common.ToFloat64(common.BindValue(args[0]))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("percentile_rank: value must be a number, got %T", args[0]), nil)
		}
		values, err := nums(arrInput(v, args[1:]))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("percentile_rank: %v", err), nil)
		}
		sort.Float64s(values)
		atOrBelow := 0
		for _, f := range values {
			if f <= want {
				atOrBelow++
			}
		}
		return common.MakeUDFSuccessResult(float64(atOrBelow)/float64(len(values))*100, nil)
	})
}
