package stats

import (
	"fmt"
	"math"
	"sort"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

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

// RegisterCorrelation registers correlation, the Pearson product-moment
// correlation of two equal-length arrays.
func RegisterCorrelation() gojq.CompilerOption {
	return gojq.WithFunction("correlation", 1, 2, func(v any, args []any) any {
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
	return gojq.WithFunction("covariance", 1, 2, func(v any, args []any) any {
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

// RegisterSkewness registers skewness, the sample skewness of an array
// (adjusted Fisher-Pearson). 0 for an array too small to measure.
func RegisterSkewness() gojq.CompilerOption {
	return gojq.WithFunction("skewness", 0, 1, func(v any, args []any) any {
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
	return gojq.WithFunction("kurtosis", 0, 1, func(v any, args []any) any {
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

// RegisterWeightedMean registers weighted_mean, the mean of values weighted by
// parallel weights.
func RegisterWeightedMean() gojq.CompilerOption {
	return gojq.WithFunction("weighted_mean", 1, 2, func(v any, args []any) any {
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

// RegisterHarmonicMean registers harmonic_mean, the harmonic mean of an array
// of positive numbers.
func RegisterHarmonicMean() gojq.CompilerOption {
	return gojq.WithFunction("harmonic_mean", 0, 1, func(v any, args []any) any {
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

// RegisterQuartiles registers quartiles, the five-number summary
// [minimum, q1, median, q3, maximum] of an array.
func RegisterQuartiles() gojq.CompilerOption {
	return gojq.WithFunction("quartiles", 0, 1, func(v any, args []any) any {
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

// RegisterTrimmedMean registers trimmed_mean, the mean of an array with the
// given fraction trimmed from each end (fraction in 0..0.5).
func RegisterTrimmedMean() gojq.CompilerOption {
	return gojq.WithFunction("trimmed_mean", 1, 2, func(v any, args []any) any {
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

// RegisterStandardize registers standardize, each value as its z-score: how
// many sample standard deviations it lies from the mean.
func RegisterStandardize() gojq.CompilerOption {
	return gojq.WithFunction("standardize", 0, 1, func(v any, args []any) any {
		values, err := nums(arrInput(v, args))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("standardize: %v", err), nil)
		}
		m := mean(values)
		sd := math.Sqrt(variance(values))
		if sd == 0 {
			return common.MakeUDFSuccessResult(make([]any, len(values)), nil)
		}
		out := make([]any, len(values))
		for i, f := range values {
			out[i] = (f - m) / sd
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterRMS registers rms, the root mean square of an array.
func RegisterRMS() gojq.CompilerOption {
	return gojq.WithFunction("rms", 0, 1, func(v any, args []any) any {
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
	return gojq.WithFunction("product", 0, 1, func(v any, args []any) any {
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
