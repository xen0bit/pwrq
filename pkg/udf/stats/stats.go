// Package stats provides the descriptive statistics jq lacks: mean, median,
// mode, variance, standard deviation, percentiles and a one-call summary.
// variance and stdev are sample statistics (n-1 denominator).
package stats

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

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

// RegisterMean registers mean, the arithmetic average.
func RegisterMean() gojq.CompilerOption {
	return gojq.WithFunction("mean", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("mean: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(mean(values), nil)
	})
}

func mean(values []float64) float64 {
	sum := 0.0
	for _, f := range values {
		sum += f
	}
	return sum / float64(len(values))
}

// RegisterMedian registers median, the middle value of a sorted array (the
// average of the two middles when the length is even).
func RegisterMedian() gojq.CompilerOption {
	return gojq.WithFunction("median", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("median: %v", err), nil)
		}
		sort.Float64s(values)
		return common.MakeUDFSuccessResult(median(values), nil)
	})
}

func median(values []float64) float64 {
	n := len(values)
	if n%2 == 1 {
		return values[n/2]
	}
	return (values[n/2-1] + values[n/2]) / 2
}

// RegisterMode registers mode, the most frequent value. It works on any
// values; on a tie it reports the value that appeared first.
func RegisterMode() gojq.CompilerOption {
	return gojq.WithFunction("mode", 0, 1, func(v any, args []any) any {
		arr := arrInput(v, args)
		if len(arr) == 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("mode: expected a non-empty array"), nil)
		}
		counts := make(map[string]int)
		first := make(map[string]any)
		order := []string{}
		for _, item := range arr {
			key := jsonKey(item)
			if _, seen := counts[key]; !seen {
				first[key] = item
				order = append(order, key)
			}
			counts[key]++
		}
		bestKey, bestCount := order[0], -1
		for _, key := range order {
			if counts[key] > bestCount {
				bestCount = counts[key]
				bestKey = key
			}
		}
		return common.MakeUDFSuccessResult(first[bestKey], nil)
	})
}

// jsonKey renders a value as a stable key for tallying.
func jsonKey(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%T:%v", v, v)
	}
	return string(b)
}

// RegisterVariance registers variance, the sample variance (n-1).
func RegisterVariance() gojq.CompilerOption {
	return gojq.WithFunction("variance", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("variance: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(variance(values), nil)
	})
}

func variance(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	m := mean(values)
	sum := 0.0
	for _, f := range values {
		d := f - m
		sum += d * d
	}
	return sum / float64(len(values)-1)
}

// RegisterStdev registers stdev, the square root of the sample variance.
func RegisterStdev() gojq.CompilerOption {
	return gojq.WithFunction("stdev", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("stdev: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(math.Sqrt(variance(values)), nil)
	})
}

// RegisterPercentile registers percentile, the value below which p percent of
// the data falls (linear interpolation between closest ranks, p in 0..100).
func RegisterPercentile() gojq.CompilerOption {
	return gojq.WithFunction("percentile", 1, 2, func(v any, args []any) any {
		p, ok := common.ToFloat64(common.BindValue(args[0]))
		if !ok || p < 0 || p > 100 {
			return common.MakeUDFErrorResult(fmt.Errorf("percentile: p must be a number from 0 to 100, got %v", args[0]), nil)
		}
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("percentile: %v", err), nil)
		}
		sort.Float64s(values)
		n := len(values)
		if n == 1 {
			return common.MakeUDFSuccessResult(values[0], nil)
		}
		rank := p / 100 * float64(n-1)
		lo := int(math.Floor(rank))
		hi := int(math.Ceil(rank))
		if lo == hi {
			return common.MakeUDFSuccessResult(values[lo], nil)
		}
		fraction := rank - float64(lo)
		value := values[lo] + fraction*(values[hi]-values[lo])
		return common.MakeUDFSuccessResult(value, nil)
	})
}

// RegisterSummary registers summary, the descriptive statistics of an array in
// one call.
func RegisterSummary() gojq.CompilerOption {
	return gojq.WithFunction("summary", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("summary: %v", err), nil)
		}
		sorted := append([]float64(nil), values...)
		sort.Float64s(sorted)
		mn := mean(values)
		return common.MakeUDFSuccessResult(map[string]any{
			"count":  len(values),
			"min":    sorted[0],
			"max":    sorted[len(sorted)-1],
			"mean":   mn,
			"median": median(sorted),
			"stdev":  math.Sqrt(variance(values)),
		}, nil)
	})
}
