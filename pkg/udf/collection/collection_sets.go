// Arrays as sets, and looking a row up inside one.
package collection

import (
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// twoArrs resolves two arrays: when the pipeline is an array it is the first
// and the argument the second; otherwise two array arguments are used. This is
// what lets both `[1,2] | union([2,3])` and `union([1,2]; [2,3])` work.
func twoArrs(v any, args []any) ([]any, []any, error) {
	pipeArr, pipeIsArr := common.BindValue(v).([]any)
	if pipeIsArr {
		other := firstArrArg(args)
		if other == nil {
			return nil, nil, fmt.Errorf("a second array is required")
		}
		return pipeArr, other, nil
	}
	if len(args) < 2 {
		return nil, nil, fmt.Errorf("two arrays are required")
	}
	a, okA := common.BindValue(args[0]).([]any)
	b, okB := common.BindValue(args[1]).([]any)
	if !okA || !okB {
		return nil, nil, fmt.Errorf("expected two arrays, got %T and %T", args[0], args[1])
	}
	return a, b, nil
}

func firstArrArg(args []any) []any {
	for _, a := range args {
		if arr, ok := common.BindValue(a).([]any); ok {
			return arr
		}
	}
	return nil
}

// keyOf renders a value as its JSON identity for set membership.
func keyOf(item any) string {
	key, err := json.Marshal(common.BindValue(item))
	if err != nil {
		return fmt.Sprintf("%T:%v", item, item)
	}
	return string(key)
}

// RegisterIntersection registers intersection, the elements present in both
// arrays, in first-array order.
func RegisterIntersection() gojq.CompilerOption {
	return gojq.WithFunction("intersection", 1, 2, func(v any, args []any) any {
		a, b, err := twoArrs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("intersection: %v", err), nil)
		}
		inB := make(map[string]bool, len(b))
		for _, item := range b {
			inB[keyOf(item)] = true
		}
		out := []any{}
		for _, item := range a {
			if inB[keyOf(item)] {
				out = append(out, item)
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterUnion registers union, the unique elements of both arrays, in
// first-array order then second-array.
func RegisterUnion() gojq.CompilerOption {
	return gojq.WithFunction("union", 1, 2, func(v any, args []any) any {
		a, b, err := twoArrs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("union: %v", err), nil)
		}
		seen := make(map[string]bool, len(a)+len(b))
		out := []any{}
		for _, item := range append(append([]any{}, a...), b...) {
			key := keyOf(item)
			if !seen[key] {
				seen[key] = true
				out = append(out, item)
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterDifference registers difference, the elements of the first array
// that are not in the second.
func RegisterDifference() gojq.CompilerOption {
	return gojq.WithFunction("difference", 1, 2, func(v any, args []any) any {
		a, b, err := twoArrs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("difference: %v", err), nil)
		}
		inB := make(map[string]bool, len(b))
		for _, item := range b {
			inB[keyOf(item)] = true
		}
		out := []any{}
		for _, item := range a {
			if !inB[keyOf(item)] {
				out = append(out, item)
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterSymmetricDifference registers symmetric_difference, the elements in
// exactly one of the two arrays.
func RegisterSymmetricDifference() gojq.CompilerOption {
	return gojq.WithFunction("symmetric_difference", 1, 2, func(v any, args []any) any {
		a, b, err := twoArrs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("symmetric_difference: %v", err), nil)
		}
		inA := make(map[string]bool, len(a))
		inB := make(map[string]bool, len(b))
		for _, item := range a {
			inA[keyOf(item)] = true
		}
		for _, item := range b {
			inB[keyOf(item)] = true
		}
		out := []any{}
		for _, item := range a {
			if !inB[keyOf(item)] {
				out = append(out, item)
			}
		}
		for _, item := range b {
			if !inA[keyOf(item)] {
				out = append(out, item)
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterAllEqual registers all_equal, whether every element of an array is
// the same value. An empty array is all-equal.
func RegisterAllEqual() gojq.CompilerOption {
	return gojq.WithFunction("all_equal", 0, 1, func(v any, args []any) any {
		arr, _, err := arrInput(v, args, 0, "all_equal")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if len(arr) < 2 {
			return common.MakeUDFSuccessResult(true, nil)
		}
		first := keyOf(arr[0])
		for _, item := range arr[1:] {
			if keyOf(item) != first {
				return common.MakeUDFSuccessResult(false, nil)
			}
		}
		return common.MakeUDFSuccessResult(true, nil)
	})
}

// RegisterContainsDuplicates registers contains_duplicates, whether any value
// appears more than once.
func RegisterContainsDuplicates() gojq.CompilerOption {
	return gojq.WithFunction("contains_duplicates", 0, 1, func(v any, args []any) any {
		arr, _, err := arrInput(v, args, 0, "contains_duplicates")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		seen := make(map[string]bool, len(arr))
		for _, item := range arr {
			key := keyOf(item)
			if seen[key] {
				return common.MakeUDFSuccessResult(true, nil)
			}
			seen[key] = true
		}
		return common.MakeUDFSuccessResult(false, nil)
	})
}

// RegisterCartesian registers cartesian, the array of [a, b] pairs from two
// arrays.
func RegisterCartesian() gojq.CompilerOption {
	return gojq.WithFunction("cartesian", 1, 2, func(v any, args []any) any {
		a, b, err := twoArrs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cartesian: %v", err), nil)
		}
		out := make([]any, 0, len(a)*len(b))
		for _, x := range a {
			for _, y := range b {
				out = append(out, []any{x, y})
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterDedupe registers dedupe, removing duplicate values while keeping
// first-occurrence order.
func RegisterDedupe() gojq.CompilerOption {
	return gojq.WithFunction("dedupe", 0, 1, func(v any, args []any) any {
		arr, _, err := arrInput(v, args, 0, "dedupe")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		seen := make(map[string]bool, len(arr))
		out := make([]any, 0, len(arr))
		for _, item := range arr {
			key, _ := json.Marshal(item)
			if !seen[string(key)] {
				seen[string(key)] = true
				out = append(out, item)
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterLookup registers lookup, the first row of an array of objects whose
// property equals a value: lookup("name"; "ada").
func RegisterLookup() gojq.CompilerOption {
	return gojq.WithFunction("lookup", 2, 3, func(v any, args []any) any {
		arr, rest, err := arrInput(v, args, 2, "lookup")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		key, ok := common.BindValue(rest[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("lookup: key must be a string, got %T", rest[0]), nil)
		}
		want := common.BindValue(rest[1])
		wantKey := keyOf(want)
		for _, row := range arr {
			if m, ok := common.BindValue(row).(map[string]any); ok {
				if val, exists := m[key]; exists && keyOf(val) == wantKey {
					return common.MakeUDFSuccessResult(row, nil)
				}
			}
		}
		return common.MakeUDFSuccessResult(nil, nil)
	})
}
