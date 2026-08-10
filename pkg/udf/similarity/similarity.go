// Package similarity provides string and set similarity measures, plus a
// structural JSON diff that reports what was added, removed and changed.
package similarity

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every similarity cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterLevenshtein(),
		RegisterHammingDistance(),
		RegisterJaccard(),
		RegisterDeepDiff(),
	}
}

// RegisterLevenshtein registers levenshtein, the edit distance between two
// strings: the minimum insertions, deletions and substitutions to turn one
// into the other.
func RegisterLevenshtein() gojq.CompilerOption {
	return gojq.WithFunction("levenshtein", 2, 2, func(v any, args []any) any {
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
	return gojq.WithFunction("hamming_distance", 2, 2, func(v any, args []any) any {
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
	return gojq.WithFunction("jaccard", 2, 2, func(v any, args []any) any {
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
	return gojq.WithFunction("deep_diff", 2, 2, func(v any, args []any) any {
		a := common.BindValue(v)
		if len(args) > 0 {
			a = common.BindValue(args[0])
		}
		b := common.BindValue(args[1])
		res := &diffResult{}
		diff(a, b, "", res)
		return common.MakeUDFSuccessResult(map[string]any{
			"added":   res.added,
			"removed": res.removed,
			"changed": res.changed,
		}, nil)
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
