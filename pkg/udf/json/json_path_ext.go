package json

import (
	"fmt"
	"strconv"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func atoi(s string) (int, error) {
	return strconv.Atoi(s)
}

// dotInput resolves the document from the pipeline or first argument, and the
// path tokens from the given argument.
func dotInput(v any, args []any, name string, pathArg int) (any, []string, error) {
	doc := common.BindValue(v)
	if len(args) > pathArg {
		doc = common.BindValue(args[pathArg])
	}
	path, ok := common.BindValue(args[0]).(string)
	if !ok {
		return nil, nil, fmt.Errorf("%s: path must be a string, got %T", name, args[0])
	}
	tokens, err := dotTokens(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %v", name, err)
	}
	return doc, tokens, nil
}

// RegisterSetPath registers set_path, the document with a value written at a
// dot-and-bracket path: set_path(obj; "a.b[0]"; value).
func RegisterSetPath() gojq.CompilerOption {
	return gojq.WithFunction("set_path", 2, 3, func(v any, args []any) any {
		doc, tokens, err := dotInput(v, args, "set_path", 2)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		value := common.BindValue(args[1])
		out, err := pointerSet(doc, tokens, value)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("set_path: %v", err), nil)
		}
		return out
	})
}

// RegisterHasPath registers has_path, whether a dot-and-bracket path exists in
// a document.
func RegisterHasPath() gojq.CompilerOption {
	return gojq.WithFunction("has_path", 1, 2, func(v any, args []any) any {
		doc, tokens, err := dotInput(v, args, "has_path", 1)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		cur := doc
		for _, t := range tokens {
			switch c := cur.(type) {
			case map[string]any:
				next, ok := c[t]
				if !ok {
					return common.MakeUDFSuccessResult(false, nil)
				}
				cur = next
			case []any:
				idx, err := atoi(t)
				if err != nil || idx < 0 || idx >= len(c) {
					return common.MakeUDFSuccessResult(false, nil)
				}
				cur = c[idx]
			default:
				return common.MakeUDFSuccessResult(false, nil)
			}
		}
		return common.MakeUDFSuccessResult(true, nil)
	})
}

// RegisterDelPath registers del_path, the document with the value at a
// dot-and-bracket path removed. Removing the last key of a nested object
// leaves an empty object, matching jq's del.
func RegisterDelPath() gojq.CompilerOption {
	return gojq.WithFunction("del_path", 1, 2, func(v any, args []any) any {
		doc, tokens, err := dotInput(v, args, "del_path", 1)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if len(tokens) == 0 {
			return common.MakeUDFSuccessResult(nil, nil)
		}
		out, err := deleteAt(doc, tokens)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("del_path: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

func deleteAt(v any, tokens []string) (any, error) {
	if len(tokens) == 0 {
		return nil, nil
	}
	switch c := v.(type) {
	case map[string]any:
		key := tokens[0]
		if len(tokens) == 1 {
			delete(c, key)
			return c, nil
		}
		child, ok := c[key]
		if !ok {
			return c, nil
		}
		updated, err := deleteAt(child, tokens[1:])
		if err != nil {
			return nil, err
		}
		c[key] = updated
		return c, nil
	case []any:
		idx, err := atoi(tokens[0])
		if err != nil || idx < 0 || idx >= len(c) {
			return v, nil
		}
		if len(tokens) == 1 {
			return append(c[:idx], c[idx+1:]...), nil
		}
		updated, err := deleteAt(c[idx], tokens[1:])
		if err != nil {
			return nil, err
		}
		c[idx] = updated
		return c, nil
	default:
		return v, nil
	}
}
