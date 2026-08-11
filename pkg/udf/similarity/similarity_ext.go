package similarity

import (
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterSimilarityPercent registers similarity_percent, 1 minus the
// normalized Levenshtein distance, as a value from 0 to 1.
func RegisterSimilarityPercent() gojq.CompilerOption {
	return gojq.WithFunction("similarity_percent", 2, 2, func(v any, args []any) any {
		a, err := stringArg(v, args, 0)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("similarity_percent: %v", err), nil)
		}
		b, err := stringArg(v, args, 1)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("similarity_percent: %v", err), nil)
		}
		distance := levenshtein(a, b)
		maxLen := len([]rune(a))
		if bl := len([]rune(b)); bl > maxLen {
			maxLen = bl
		}
		if maxLen == 0 {
			return common.MakeUDFSuccessResult(1, nil)
		}
		return common.MakeUDFSuccessResult(1 - float64(distance)/float64(maxLen), nil)
	})
}

// RegisterNGrams registers n_grams, the array of n-character substrings of a
// string.
func RegisterNGrams() gojq.CompilerOption {
	return gojq.WithFunction("n_grams", 1, 2, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok || n <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("n_grams: size must be a positive integer, got %v", args[0]), nil)
		}
		input := common.BindValue(v)
		var a string
		switch val := input.(type) {
		case string:
			a = val
		case []byte:
			a = string(val)
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("n_grams: expected a string, got %T", input), nil)
		}
		runes := []rune(a)
		if n > len(runes) {
			return common.MakeUDFSuccessResult([]any{}, nil)
		}
		out := make([]any, 0, len(runes)-n+1)
		for i := 0; i+n <= len(runes); i++ {
			out = append(out, string(runes[i:i+n]))
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterJaroWinkler registers jaro_winkler, the Jaro-Winkler similarity of
// two strings, from 0 to 1. It weights a shared prefix, which suits name
// matching.
func RegisterJaroWinkler() gojq.CompilerOption {
	return gojq.WithFunction("jaro_winkler", 2, 2, func(v any, args []any) any {
		a, err := stringArg(v, args, 0)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("jaro_winkler: %v", err), nil)
		}
		b, err := stringArg(v, args, 1)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("jaro_winkler: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(jaroWinkler(a, b), nil)
	})
}

func jaroWinkler(a, b string) float64 {
	ar, br := []rune(a), []rune(b)
	la, lb := len(ar), len(br)
	if la == 0 && lb == 0 {
		return 1
	}
	if la == 0 || lb == 0 {
		return 0
	}
	matchDist := max(la, lb)/2 - 1
	if matchDist < 0 {
		matchDist = 0
	}
	am, bm := make([]bool, la), make([]bool, lb)
	matches := 0
	for i, ca := range ar {
		lo, hi := i-matchDist, i+matchDist
		if lo < 0 {
			lo = 0
		}
		if hi >= lb {
			hi = lb - 1
		}
		for j := lo; j <= hi; j++ {
			if !bm[j] && br[j] == ca {
				am[i], bm[j] = true, true
				matches++
				break
			}
		}
	}
	if matches == 0 {
		return 0
	}
	transpositions := 0
	k := 0
	for i := 0; i < la; i++ {
		if am[i] {
			for !bm[k] {
				k++
			}
			if ar[i] != br[k] {
				transpositions++
			}
			k++
		}
	}
	jaro := (float64(matches)/float64(la) + float64(matches)/float64(lb) +
		(float64(matches)-float64(transpositions)/2)/float64(matches)) / 3
	prefix := 0
	for prefix < la && prefix < lb && prefix < 4 && ar[prefix] == br[prefix] {
		prefix++
	}
	return jaro + float64(prefix)*0.1*(1-jaro)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
