// Package collection provides data-structure utilities jq leaves to the
// caller: chunking, order-preserving dedupe, recursive merging, key sorting,
// compaction, pruning, and dot-path key flattening.
package collection

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every collection cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterChunks(),
		RegisterDedupe(),
		RegisterDeepMerge(),
		RegisterSortKeys(),
		RegisterCompact(),
		RegisterPrune(),
		RegisterFlattenKeys(),
		RegisterUnflattenKeys(),
		RegisterZipArrays(),
	}
}

// arrInput resolves an array from the pipeline or the first argument.
func arrInput(v any, args []any) ([]any, error) {
	if len(args) > 0 {
		if arr, ok := common.BindValue(args[0]).([]any); ok {
			return arr, nil
		}
	}
	switch val := common.BindValue(v).(type) {
	case []any:
		return val, nil
	case nil:
		return nil, fmt.Errorf("expected an array, got null")
	default:
		return nil, fmt.Errorf("expected an array, got %T", val)
	}
}

// RegisterChunks registers chunks, splitting an array into chunks of at most n
// elements.
func RegisterChunks() gojq.CompilerOption {
	return gojq.WithFunction("chunks", 1, 2, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok || n <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("chunks: size must be a positive integer, got %v", args[0]), nil)
		}
		arr, err := arrInput(v, args[1:])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("chunks: %v", err), nil)
		}
		out := make([]any, 0, (len(arr)+n-1)/n)
		for i := 0; i < len(arr); i += n {
			end := i + n
			if end > len(arr) {
				end = len(arr)
			}
			out = append(out, arr[i:end])
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterDedupe registers dedupe, removing duplicate values while keeping
// first-occurrence order.
func RegisterDedupe() gojq.CompilerOption {
	return gojq.WithFunction("dedupe", 0, 1, func(v any, args []any) any {
		arr, err := arrInput(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("dedupe: %v", err), nil)
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

// RegisterDeepMerge registers deep_merge, recursively merging two objects with
// the second winning.
func RegisterDeepMerge() gojq.CompilerOption {
	return gojq.WithFunction("deep_merge", 1, 2, func(v any, args []any) any {
		a := common.BindValue(v)
		if len(args) > 0 {
			a = common.BindValue(args[0])
		}
		b := common.BindValue(args[1])
		return common.MakeUDFSuccessResult(deepMerge(a, b), nil)
	})
}

func deepMerge(a, b any) any {
	am, aOK := a.(map[string]any)
	bm, bOK := b.(map[string]any)
	if aOK && bOK {
		out := make(map[string]any, len(am)+len(bm))
		for k, av := range am {
			out[k] = av
		}
		for k, bv := range bm {
			if existing, has := out[k]; has {
				out[k] = deepMerge(existing, bv)
			} else {
				out[k] = bv
			}
		}
		return out
	}
	return b
}

// RegisterSortKeys registers sort_keys, recursively sorting an object's keys.
func RegisterSortKeys() gojq.CompilerOption {
	return gojq.WithFunction("sort_keys", 0, 1, func(v any, args []any) any {
		input := common.BindValue(v)
		if len(args) > 0 {
			input = common.BindValue(args[0])
		}
		return common.MakeUDFSuccessResult(sortKeys(input), nil)
	})
}

func sortKeys(v any) any {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(val))
		for _, k := range keys {
			out[k] = sortKeys(val[k])
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = sortKeys(item)
		}
		return out
	default:
		return v
	}
}

// isEmpty reports whether a value is null, empty, false, or a blank string.
func isEmpty(v any) bool {
	switch val := v.(type) {
	case nil:
		return true
	case string:
		return val == ""
	case []any:
		return len(val) == 0
	case map[string]any:
		return len(val) == 0
	case bool:
		return !val
	}
	return false
}

// RegisterCompact registers compact, dropping null, empty and false values
// from an array (one level deep).
func RegisterCompact() gojq.CompilerOption {
	return gojq.WithFunction("compact", 0, 1, func(v any, args []any) any {
		arr, err := arrInput(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("compact: %v", err), nil)
		}
		out := make([]any, 0, len(arr))
		for _, item := range arr {
			if !isEmpty(common.BindValue(item)) {
				out = append(out, item)
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterPrune registers prune, recursively removing empty values from
// objects and arrays.
func RegisterPrune() gojq.CompilerOption {
	return gojq.WithFunction("prune", 0, 1, func(v any, args []any) any {
		input := common.BindValue(v)
		if len(args) > 0 {
			input = common.BindValue(args[0])
		}
		return common.MakeUDFSuccessResult(prune(input), nil)
	})
}

func prune(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			cleaned := prune(item)
			if !isEmpty(cleaned) {
				out[k] = cleaned
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(val))
		for _, item := range val {
			cleaned := prune(item)
			if !isEmpty(cleaned) {
				out = append(out, cleaned)
			}
		}
		return out
	default:
		return v
	}
}

// RegisterFlattenKeys registers flatten_keys, turning a nested object into a
// flat one with dot-and-bracket keys ("a.b[0]").
func RegisterFlattenKeys() gojq.CompilerOption {
	return gojq.WithFunction("flatten_keys", 0, 1, func(v any, args []any) any {
		input := common.BindValue(v)
		if len(args) > 0 {
			input = common.BindValue(args[0])
		}
		out := make(map[string]any)
		flatten(input, "", out)
		return common.MakeUDFSuccessResult(out, nil)
	})
}

func flatten(v any, prefix string, out map[string]any) {
	switch val := v.(type) {
	case map[string]any:
		for k, item := range val {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flatten(item, key, out)
		}
	case []any:
		for i, item := range val {
			flatten(item, prefix+"["+strconv.Itoa(i)+"]", out)
		}
	default:
		out[prefix] = v
	}
}

// RegisterUnflattenKeys registers unflatten_keys, the inverse of
// flatten_keys.
func RegisterUnflattenKeys() gojq.CompilerOption {
	return gojq.WithFunction("unflatten_keys", 0, 1, func(v any, args []any) any {
		input := common.BindValue(v)
		if len(args) > 0 {
			input = common.BindValue(args[0])
		}
		m, ok := input.(map[string]any)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("unflatten_keys: expected an object, got %T", input), nil)
		}
		root := any(make(map[string]any))
		for path, value := range m {
			if err := setPath(&root, parsePath(path), value); err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("unflatten_keys: %v", err), nil)
			}
		}
		return common.MakeUDFSuccessResult(root, nil)
	})
}

type pathSegment struct {
	name       string
	indexValue int
	isIndex    bool
}

// parsePath tokenizes a dot-and-bracket path like "a.b[0].c".
func parsePath(path string) []pathSegment {
	var segments []pathSegment
	i := 0
	for i < len(path) {
		switch {
		case path[i] == '.':
			i++
		case path[i] == '[':
			j := i + 1
			for j < len(path) && path[j] != ']' {
				j++
			}
			if idx, err := strconv.Atoi(path[i+1 : j]); err == nil {
				segments = append(segments, pathSegment{indexValue: idx, isIndex: true})
			}
			i = j + 1
		default:
			j := i
			for j < len(path) && path[j] != '.' && path[j] != '[' {
				j++
			}
			segments = append(segments, pathSegment{name: path[i:j]})
			i = j
		}
	}
	return segments
}

// setPath writes a value at the given path segments, creating intermediate
// objects and arrays as needed. slot points at the container to descend into.
func setPath(slot *any, segments []pathSegment, value any) error {
	if len(segments) == 0 {
		return fmt.Errorf("empty path")
	}
	seg := segments[0]
	last := len(segments) == 1
	if seg.isIndex {
		arr, ok := (*slot).([]any)
		if !ok {
			return fmt.Errorf("index %d into a non-array", seg.indexValue)
		}
		for len(arr) <= seg.indexValue {
			arr = append(arr, nil)
		}
		*slot = arr
		if last {
			arr[seg.indexValue] = value
			return nil
		}
		child := arr[seg.indexValue]
		if child == nil {
			child = newContainer(segments[1])
			arr[seg.indexValue] = child
		}
		return setPath(&arr[seg.indexValue], segments[1:], value)
	}
	m, ok := (*slot).(map[string]any)
	if !ok {
		return fmt.Errorf("key %q into a non-object", seg.name)
	}
	if last {
		m[seg.name] = value
		return nil
	}
	child, has := m[seg.name]
	if !has {
		child = newContainer(segments[1])
		m[seg.name] = child
	}
	if err := setPath(&child, segments[1:], value); err != nil {
		return err
	}
	m[seg.name] = child
	return nil
}

func newContainer(seg pathSegment) any {
	if seg.isIndex {
		return []any{}
	}
	return map[string]any{}
}

// RegisterZipArrays registers zip_arrays, pairing an array from the pipeline
// with one from the argument, element by element up to the shorter length.
func RegisterZipArrays() gojq.CompilerOption {
	return gojq.WithFunction("zip_arrays", 1, 2, func(v any, args []any) any {
		right, ok := common.BindValue(args[0]).([]any)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("zip_arrays: argument must be an array, got %T", args[0]), nil)
		}
		left, err := arrInput(v, args[1:])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("zip_arrays: %v", err), nil)
		}
		n := len(left)
		if len(right) < n {
			n = len(right)
		}
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, []any{left[i], right[i]})
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}
