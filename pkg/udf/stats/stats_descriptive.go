// Descriptive statistics: centre, spread, shape and correlation.
package stats

import (
	"fmt"
	"math"
	"sort"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterMean registers mean, the arithmetic average.
func RegisterMean() gojq.CompilerOption {
	return common.WithFunction("mean", 0, 1, func(v any, args []any) any {
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
	return common.WithFunction("median", 0, 1, func(v any, args []any) any {
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
	return common.WithFunction("mode", 0, 1, func(v any, args []any) any {
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

// RegisterVariance registers variance, the sample variance (n-1).
func RegisterVariance() gojq.CompilerOption {
	return common.WithFunction("variance", 0, 1, func(v any, args []any) any {
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
	return common.WithFunction("stdev", 0, 1, func(v any, args []any) any {
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
	return common.WithFunction("percentile", 1, 2, func(v any, args []any) any {
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

// percentileAt reuses the package's linear-interpolation percentile logic.
func percentileAt(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 1 {
		return sorted[0]
	}
	rank := p / 100 * float64(n-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	fraction := rank - float64(lo)
	return sorted[lo] + fraction*(sorted[hi]-sorted[lo])
}

// RegisterPercentileRank registers percentile_rank, the percentage of an
// array's values at or below a given value: percentile_rank(arr; value).
func RegisterPercentileRank() gojq.CompilerOption {
	return common.WithFunction("percentile_rank", 1, 2, func(v any, args []any) any {
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

// RegisterQuartiles registers quartiles, the five-number summary
// [minimum, q1, median, q3, maximum] of an array.
func RegisterQuartiles() gojq.CompilerOption {
	return common.WithFunction("quartiles", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("quartiles: %v", err), nil)
		}
		sort.Float64s(values)
		q1 := percentileAt(values, 25)
		q3 := percentileAt(values, 75)
		return common.MakeUDFSuccessResult([]any{
			values[0], q1, median(values), q3, values[len(values)-1],
		}, nil)
	})
}

// RegisterIQR registers iqr, the interquartile range (q3 - q1) of an array.
func RegisterIQR() gojq.CompilerOption {
	return common.WithFunction("iqr", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("iqr: %v", err), nil)
		}
		sort.Float64s(values)
		return common.MakeUDFSuccessResult(percentileAt(values, 75)-percentileAt(values, 25), nil)
	})
}

// RegisterMAD registers mad, the median absolute deviation: the median of the
// distances from each value to the median.
func RegisterMAD() gojq.CompilerOption {
	return common.WithFunction("mad", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("mad: %v", err), nil)
		}
		sorted := append([]float64(nil), values...)
		sort.Float64s(sorted)
		m := median(sorted)
		devs := make([]float64, len(values))
		for i, f := range values {
			d := f - m
			if d < 0 {
				d = -d
			}
			devs[i] = d
		}
		sort.Float64s(devs)
		return common.MakeUDFSuccessResult(median(devs), nil)
	})
}

// RegisterSummary registers summary, the descriptive statistics of an array in
// one call.
func RegisterSummary() gojq.CompilerOption {
	return common.WithFunction("summary", 0, 1, func(v any, args []any) any {
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

// RegisterGeomean registers geomean, the geometric mean of an array of
// positive numbers.
func RegisterGeomean() gojq.CompilerOption {
	return common.WithFunction("geomean", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("geomean: %v", err), nil)
		}
		logSum := 0.0
		for _, f := range values {
			if f <= 0 {
				return common.MakeUDFErrorResult(fmt.Errorf("geomean: values must be positive, got %v", f), nil)
			}
			logSum += math.Log(f)
		}
		return common.MakeUDFSuccessResult(math.Exp(logSum/float64(len(values))), nil)
	})
}

// RegisterHarmonicMean registers harmonic_mean, the harmonic mean of an array
// of positive numbers.
func RegisterHarmonicMean() gojq.CompilerOption {
	return common.WithFunction("harmonic_mean", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("harmonic_mean: %v", err), nil)
		}
		recip := 0.0
		for _, f := range values {
			if f <= 0 {
				return common.MakeUDFErrorResult(fmt.Errorf("harmonic_mean: values must be positive, got %v", f), nil)
			}
			recip += 1 / f
		}
		return common.MakeUDFSuccessResult(float64(len(values))/recip, nil)
	})
}

// RegisterWeightedMean registers weighted_mean, the mean of values weighted by
// parallel weights.
func RegisterWeightedMean() gojq.CompilerOption {
	return common.WithFunction("weighted_mean", 1, 2, func(v any, args []any) any {
		values, weights, err := twoArrays(v, args, "weighted_mean")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		var num, den float64
		for i := range values {
			num += values[i] * weights[i]
			den += weights[i]
		}
		if den == 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("weighted_mean: weights sum to zero"), nil)
		}
		return common.MakeUDFSuccessResult(num/den, nil)
	})
}

// RegisterTrimmedMean registers trimmed_mean, the mean of an array with the
// given fraction trimmed from each end (fraction in 0..0.5).
func RegisterTrimmedMean() gojq.CompilerOption {
	return common.WithFunction("trimmed_mean", 1, 2, func(v any, args []any) any {
		fraction, ok := common.ToFloat64(common.BindValue(args[0]))
		if !ok || fraction < 0 || fraction > 0.5 {
			return common.MakeUDFErrorResult(fmt.Errorf("trimmed_mean: fraction must be from 0 to 0.5, got %v", args[0]), nil)
		}
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("trimmed_mean: %v", err), nil)
		}
		sort.Float64s(values)
		trim := int(fraction * float64(len(values)))
		values = values[trim : len(values)-trim]
		if len(values) == 0 {
			return common.MakeUDFSuccessResult(0, nil)
		}
		return common.MakeUDFSuccessResult(mean(values), nil)
	})
}

// RegisterRMS registers rms, the root mean square of an array.
func RegisterRMS() gojq.CompilerOption {
	return common.WithFunction("rms", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("rms: %v", err), nil)
		}
		sum := 0.0
		for _, f := range values {
			sum += f * f
		}
		return common.MakeUDFSuccessResult(math.Sqrt(sum/float64(len(values))), nil)
	})
}

// RegisterProduct registers product, the product of an array's numbers (jq's
// add sums; this multiplies).
func RegisterProduct() gojq.CompilerOption {
	return common.WithFunction("product", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("product: %v", err), nil)
		}
		out := 1.0
		for _, f := range values {
			out *= f
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterSkewness registers skewness, the sample skewness of an array
// (adjusted Fisher-Pearson). 0 for an array too small to measure.
func RegisterSkewness() gojq.CompilerOption {
	return common.WithFunction("skewness", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("skewness: %v", err), nil)
		}
		if len(values) < 3 {
			return common.MakeUDFSuccessResult(0, nil)
		}
		m := mean(values)
		m2, m3 := 0.0, 0.0
		for _, f := range values {
			d := f - m
			m2 += d * d
			m3 += d * d * d
		}
		n := float64(len(values))
		m2, m3 = m2/n, m3/n
		if m2 == 0 {
			return common.MakeUDFSuccessResult(0, nil)
		}
		return common.MakeUDFSuccessResult(n*n/((n-1)*(n-2))*m3/math.Pow(m2, 1.5), nil)
	})
}

// RegisterKurtosis registers kurtosis, the sample excess kurtosis of an array
// (0 for a normal distribution). 0 for an array too small to measure.
func RegisterKurtosis() gojq.CompilerOption {
	return common.WithFunction("kurtosis", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("kurtosis: %v", err), nil)
		}
		if len(values) < 4 {
			return common.MakeUDFSuccessResult(0, nil)
		}
		m := mean(values)
		m2, m4 := 0.0, 0.0
		for _, f := range values {
			d := f - m
			m2 += d * d
			m4 += d * d * d * d
		}
		n := float64(len(values))
		m2, m4 = m2/n, m4/n
		if m2 == 0 {
			return common.MakeUDFSuccessResult(0, nil)
		}
		num := n - 1
		den := (n - 2) * (n - 3)
		return common.MakeUDFSuccessResult(num/den*((n+1)*(m4/(m2*m2))-3*(n-1)), nil)
	})
}

// RegisterCorrelation registers correlation, the Pearson product-moment
// correlation of two equal-length arrays.
func RegisterCorrelation() gojq.CompilerOption {
	return common.WithFunction("correlation", 1, 2, func(v any, args []any) any {
		a, b, err := twoArrays(v, args, "correlation")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		ma, mb := mean(a), mean(b)
		var num, denA, denB float64
		for i := range a {
			da, db := a[i]-ma, b[i]-mb
			num += da * db
			denA += da * da
			denB += db * db
		}
		if denA == 0 || denB == 0 {
			return common.MakeUDFSuccessResult(0, nil)
		}
		return common.MakeUDFSuccessResult(num/math.Sqrt(denA*denB), nil)
	})
}

// RegisterCovariance registers covariance, the sample covariance (n-1) of two
// equal-length arrays.
func RegisterCovariance() gojq.CompilerOption {
	return common.WithFunction("covariance", 1, 2, func(v any, args []any) any {
		a, b, err := twoArrays(v, args, "covariance")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		ma, mb := mean(a), mean(b)
		sum := 0.0
		for i := range a {
			sum += (a[i] - ma) * (b[i] - mb)
		}
		if len(a) < 2 {
			return common.MakeUDFSuccessResult(0, nil)
		}
		return common.MakeUDFSuccessResult(sum/float64(len(a)-1), nil)
	})
}
