// Package stats provides descriptive statistics and series transforms over
// arrays of numbers.
package stats

import (
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every statistics cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterMean(),
		RegisterMedian(),
		RegisterMode(),
		RegisterVariance(),
		RegisterStdev(),
		RegisterPercentile(),
		RegisterSummary(),
		RegisterMovingAverage(),
		RegisterGeomean(),
		RegisterNormalize(),
		// Time series
		RegisterCumsum(),
		RegisterCumulativeMax(),
		RegisterCumulativeMin(),
		RegisterDeltas(),
		RegisterLag(),
		RegisterFillForward(),
		RegisterEma(),
		RegisterMovingMax(),
		RegisterMovingMin(),
		// Relations and shape
		RegisterCorrelation(),
		RegisterCovariance(),
		RegisterSkewness(),
		RegisterKurtosis(),
		RegisterWeightedMean(),
		RegisterHarmonicMean(),
		RegisterQuartiles(),
		RegisterTrimmedMean(),
		RegisterStandardize(),
		RegisterRMS(),
		RegisterProduct(),
		RegisterAutocorrelation(),
		RegisterIQR(),
		RegisterMAD(),
		RegisterMovingStdev(),
		RegisterPercentileRank(),
	}
}

// arrInput resolves the array from the first argument or the pipeline.
func arrInput(v any, args []any) []any {
	if len(args) > 0 {
		if arr, ok := common.BindValue(args[0]).([]any); ok {
			return arr
		}
	}
	return common.NormalizeToSlice(common.BindValue(v))
}

// nums converts an array to its numeric elements, failing on the first
// non-number or on an empty array.
func nums(arr []any) ([]float64, error) {
	if len(arr) == 0 {
		return nil, fmt.Errorf("expected an array of numbers, got an empty array")
	}
	out := make([]float64, 0, len(arr))
	for _, item := range arr {
		f, ok := common.ToFloat64(common.BindValue(item))
		if !ok {
			return nil, fmt.Errorf("expected a number, got %T", item)
		}
		out = append(out, f)
	}
	return out, nil
}

// twoArrays reads two arrays as floats: the pipeline and one argument, or two
// arguments. This lets both `[1,2,3] | correlation([2,4,6])` and
// `correlation([1,2,3]; [2,4,6])` work.
func twoArrays(v any, args []any, name string) ([]float64, []float64, error) {
	var a, b []any
	pipeArr, pipeIsArr := common.BindValue(v).([]any)
	if pipeIsArr {
		a = pipeArr
		other := firstArr(args)
		if other == nil {
			return nil, nil, fmt.Errorf("%s: a second array is required", name)
		}
		b = other
	} else {
		if len(args) < 2 {
			return nil, nil, fmt.Errorf("%s: two arrays are required", name)
		}
		var okA, okB bool
		a, okA = common.BindValue(args[0]).([]any)
		b, okB = common.BindValue(args[1]).([]any)
		if !okA || !okB {
			return nil, nil, fmt.Errorf("%s: expected two arrays, got %T and %T", name, args[0], args[1])
		}
	}
	if len(a) != len(b) {
		return nil, nil, fmt.Errorf("%s: arrays must be the same length (%d vs %d)", name, len(a), len(b))
	}
	af := make([]float64, len(a))
	bf := make([]float64, len(b))
	for i := range a {
		fa, okA := common.ToFloat64(common.BindValue(a[i]))
		fb, okB := common.ToFloat64(common.BindValue(b[i]))
		if !okA || !okB {
			return nil, nil, fmt.Errorf("%s: elements must be numbers", name)
		}
		af[i], bf[i] = fa, fb
	}
	return af, bf, nil
}

func firstArr(args []any) []any {
	for _, a := range args {
		if arr, ok := common.BindValue(a).([]any); ok {
			return arr
		}
	}
	return nil
}

// jsonKey renders a value as a stable key for tallying.
func jsonKey(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%T:%v", v, v)
	}
	return string(b)
}
