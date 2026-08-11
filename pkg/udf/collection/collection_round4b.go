package collection

import (
	"fmt"
	"sort"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterRenameKeys registers rename_keys, an object's keys renamed by a
// {old: new} mapping. Unmapped keys are kept as they are.
func RegisterRenameKeys() gojq.CompilerOption {
	return gojq.WithFunction("rename_keys", 1, 2, func(v any, args []any) any {
		mapping, ok := common.BindValue(args[len(args)-1]).(map[string]any)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("rename_keys: the mapping must be an object, got %T", args[len(args)-1]), nil)
		}
		obj := common.BindValue(v)
		if len(args) > 1 {
			obj = common.BindValue(args[0])
		}
		src, ok := obj.(map[string]any)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("rename_keys: expected an object, got %T", obj), nil)
		}
		out := make(map[string]any, len(src))
		for key, val := range src {
			newKey := key
			if renamed, ok := mapping[key].(string); ok {
				newKey = renamed
			}
			out[newKey] = val
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

func arrOrObj(v any) (map[string]any, error) {
	m, ok := common.BindValue(v).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected an object, got %T", v)
	}
	return m, nil
}

// RegisterInvertObject registers invert_object, an object with its keys and
// scalar values swapped. Later duplicate values win.
func RegisterInvertObject() gojq.CompilerOption {
	return gojq.WithFunction("invert_object", 0, 1, func(v any, args []any) any {
		obj, err := arrOrObj(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("invert_object: %v", err), nil)
		}
		out := make(map[string]any)
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			out[invertKey(obj[key])] = key
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// invertKey renders a scalar value as a plain object key.
func invertKey(v any) string {
	switch val := common.BindValue(v).(type) {
	case string:
		return val
	case nil:
		return "null"
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		if f, ok := common.ToFloat64(val); ok {
			return fmt.Sprintf("%g", f)
		}
		return keyOf(val)
	}
}

// RegisterPluck registers pluck, the value of a property from every row of an
// array of objects: pluck(arr; key) is map(.key) with a string key and
// dot-path support.
func RegisterPluck() gojq.CompilerOption {
	return gojq.WithFunction("pluck", 1, 2, func(v any, args []any) any {
		arr, rest, err := arrInput(v, args, 1, "pluck")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		key, ok := common.BindValue(rest[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("pluck: key must be a string, got %T", rest[0]), nil)
		}
		out := make([]any, 0, len(arr))
		for _, row := range arr {
			if got, err := common.ExtractPropertyByPath(row, key); err == nil {
				out = append(out, got)
			} else {
				out = append(out, nil)
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}
