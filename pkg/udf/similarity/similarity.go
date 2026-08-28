// Package similarity provides string and set similarity measures, plus a
// structural JSON diff that reports what was added, removed and changed.
//
// The compression-distance measures live in the rncd subpackage rather than
// here, because they carry a zstd encoder and a match finder where everything
// in this file is string arithmetic. They are the same vocabulary to a caller
// — one Similarity category, registered together — so RegisterAll returns both.
package similarity

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"github.com/xen0bit/pwrq/pkg/udf/similarity/rncd"
)

// RegisterAll registers every similarity cmdlet.
func RegisterAll() []gojq.CompilerOption {
	opts := []gojq.CompilerOption{
		RegisterLevenshtein(),
		RegisterHammingDistance(),
		RegisterJaccard(),
		RegisterDeepDiff(),
		RegisterSimilarityPercent(),
		RegisterNGrams(),
		RegisterJaroWinkler(),
		RegisterCosineSimilarity(),
	}
	// rncd measures bytes, not paths, so nothing it does needs a filesystem:
	// it is registered wherever this package is, the browser included.
	return append(opts, rncd.RegisterAll()...)
}

// RegisterLevenshtein registers levenshtein, the edit distance between two
// strings: the minimum insertions, deletions and substitutions to turn one
// into the other.
func RegisterLevenshtein() gojq.CompilerOption {
	return common.WithFunction("levenshtein", 2, 2, func(v any, args []any) any {
		a, err := stringArg(v, args, 0)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("levenshtein: %v", err), nil)
		}
		b, err := stringArg(v, args, 1)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("levenshtein: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(levenshtein(a, b), nil)
	})
}

func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, ca := range ar {
		cur[0] = i + 1
		for j, cb := range br {
			cost := 0
			if ca != cb {
				cost = 1
			}
			cur[j+1] = min3(cur[j]+1, prev[j+1]+1, prev[j]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// RegisterHammingDistance registers hamming_distance, the number of positions
// at which two equal-length strings differ.
func RegisterHammingDistance() gojq.CompilerOption {
	return common.WithFunction("hamming_distance", 2, 2, func(v any, args []any) any {
		a, err := stringArg(v, args, 0)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("hamming_distance: %v", err), nil)
		}
		b, err := stringArg(v, args, 1)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("hamming_distance: %v", err), nil)
		}
		ar, br := []rune(a), []rune(b)
		if len(ar) != len(br) {
			return common.MakeUDFErrorResult(fmt.Errorf("hamming_distance: strings differ in length (%d vs %d)", len(ar), len(br)), nil)
		}
		diff := 0
		for i := range ar {
			if ar[i] != br[i] {
				diff++
			}
		}
		return common.MakeUDFSuccessResult(diff, nil)
	})
}

// RegisterJaccard registers jaccard, the Jaccard similarity between two
// strings (sets of characters) or two arrays (sets of elements), from 0 to 1.
func RegisterJaccard() gojq.CompilerOption {
	return common.WithFunction("jaccard", 2, 2, func(v any, args []any) any {
		a := common.BindValue(args[0])
		b := common.BindValue(args[1])
		if _, ok := a.(string); !ok {
			if _, isArr := a.([]any); !isArr {
				return common.MakeUDFErrorResult(fmt.Errorf("jaccard: expected strings or arrays, got %T", a), nil)
			}
		}
		as, bs := setOf(a), setOf(b)
		intersection := 0
		for key := range as {
			if bs[key] {
				intersection++
			}
		}
		union := len(as) + len(bs) - intersection
		if union == 0 {
			return common.MakeUDFSuccessResult(1, nil)
		}
		return common.MakeUDFSuccessResult(float64(intersection)/float64(union), nil)
	})
}

// RegisterCosineSimilarity registers cosine_similarity, the cosine of the
// angle between two numeric vectors.
//
// It is here rather than beside invoke_embedding because it is a pure
// transform over two arrays of numbers — no network, no credentials — so it
// belongs with the other similarity measures and, like them, works in the
// browser. What it is for is the comparison the rest of this package cannot
// make: levenshtein and jaccard compare spelling, and two sentences that mean
// the same thing in different words score near zero on both. Over embeddings,
// this scores them near one.
func RegisterCosineSimilarity() gojq.CompilerOption {
	const op = "cosine_similarity"
	return common.WithFunction(op, 2, 2, func(v any, args []any) any {
		a, err := vectorArg(op, args[0], "first")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		b, err := vectorArg(op, args[1], "second")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if len(a) != len(b) {
			return common.MakeUDFErrorResult(fmt.Errorf(
				"%s: vectors have different lengths (%d and %d); embeddings from two different models cannot be compared",
				op, len(a), len(b)), nil)
		}
		if len(a) == 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: vectors are empty", op), nil)
		}

		var dot, normA, normB float64
		for i := range a {
			dot += a[i] * b[i]
			normA += a[i] * a[i]
			normB += b[i] * b[i]
		}
		if normA == 0 || normB == 0 {
			// A zero vector has no direction, so there is no angle to report.
			// Returning 0 would claim the two are unrelated, which is a
			// different statement from "this cannot be measured".
			return common.MakeUDFErrorResult(fmt.Errorf("%s: a zero vector has no direction", op), nil)
		}
		return common.MakeUDFSuccessResult(dot/(math.Sqrt(normA)*math.Sqrt(normB)), nil)
	})
}

// vectorArg binds an argument that must be an array of numbers.
func vectorArg(op string, arg any, which string) ([]float64, error) {
	raw, ok := common.BindValue(arg).([]any)
	if !ok {
		return nil, fmt.Errorf("%s: the %s argument must be an array of numbers, got %T", op, which, common.BindValue(arg))
	}
	out := make([]float64, len(raw))
	for i, item := range raw {
		f, ok := common.ToFloat64(item)
		if !ok {
			return nil, fmt.Errorf("%s: the %s vector has a non-number at index %d", op, which, i)
		}
		out[i] = f
	}
	return out, nil
}

func setOf(v any) map[string]bool {
	out := make(map[string]bool)
	switch val := v.(type) {
	case string:
		for _, r := range val {
			out[string(r)] = true
		}
	case []any:
		for _, item := range val {
			b, err := json.Marshal(item)
			if err != nil {
				out[fmt.Sprintf("%T:%v", item, item)] = true
			} else {
				out[string(b)] = true
			}
		}
	}
	return out
}

// stringArg reads an argument, falling back to the pipeline for the first one.
func stringArg(v any, args []any, index int) (string, error) {
	var input any
	if index < len(args) {
		input = common.BindValue(args[index])
	} else if index == 0 {
		input = common.BindValue(v)
	}
	switch val := input.(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	default:
		return "", fmt.Errorf("expected a string, got %T", input)
	}
}

// ---------------------------------------------------------------------------
// deep_diff

type diffResult struct {
	added   []any
	removed []any
	changed []any
}

// RegisterDeepDiff registers deep_diff, a structural JSON diff summarized as
// {added, removed, changed}, each a list of {path, ...} entries.
func RegisterDeepDiff() gojq.CompilerOption {
	return common.WithFunctionOf("deep_diff", 2, 2, DeepDiffShape, func(v any, args []any) any {
		a := common.BindValue(v)
		if len(args) > 0 {
			a = common.BindValue(args[0])
		}
		b := common.BindValue(args[1])
		res := &diffResult{}
		diff(a, b, "", res)
		return common.MakeUDFSuccessResult(DeepDiffShape.Build(map[string]any{
			"added":   res.added,
			"removed": res.removed,
			"changed": res.changed,
		}), nil)
	})
}

func diff(a, b any, path string, out *diffResult) {
	if reflect.DeepEqual(a, b) {
		return
	}
	am, aIsMap := a.(map[string]any)
	bm, bIsMap := b.(map[string]any)
	if aIsMap && bIsMap {
		keys := make(map[string]bool, len(am)+len(bm))
		for k := range am {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			child := joinPath(path, k)
			av, aOK := am[k]
			bv, bOK := bm[k]
			switch {
			case !aOK:
				out.added = append(out.added, map[string]any{"path": child, "value": bv})
			case !bOK:
				out.removed = append(out.removed, map[string]any{"path": child, "value": av})
			default:
				diff(av, bv, child, out)
			}
		}
		return
	}
	aa, aIsArr := a.([]any)
	ba, bIsArr := b.([]any)
	if aIsArr && bIsArr {
		max := len(aa)
		if len(ba) > max {
			max = len(ba)
		}
		for i := 0; i < max; i++ {
			child := path + "[" + fmt.Sprintf("%d", i) + "]"
			switch {
			case i >= len(aa):
				out.added = append(out.added, map[string]any{"path": child, "value": ba[i]})
			case i >= len(ba):
				out.removed = append(out.removed, map[string]any{"path": child, "value": aa[i]})
			default:
				diff(aa[i], ba[i], child, out)
			}
		}
		return
	}
	out.changed = append(out.changed, map[string]any{"path": path, "before": a, "after": b})
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// RegisterSimilarityPercent registers similarity_percent, 1 minus the
// normalized Levenshtein distance, as a value from 0 to 1.
func RegisterSimilarityPercent() gojq.CompilerOption {
	return common.WithFunction("similarity_percent", 2, 2, func(v any, args []any) any {
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
		return common.MakeUDFSuccessResult(1-float64(distance)/float64(maxLen), nil)
	})
}

// RegisterNGrams registers n_grams, the array of n-character substrings of a
// string.
func RegisterNGrams() gojq.CompilerOption {
	return common.WithFunction("n_grams", 1, 2, func(v any, args []any) any {
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
	return common.WithFunction("jaro_winkler", 2, 2, func(v any, args []any) any {
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
