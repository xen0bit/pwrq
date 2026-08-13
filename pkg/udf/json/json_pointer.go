package json

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterJSONPointer registers json_pointer, reading the value at an RFC 6901
// JSON pointer like "/a/b/0".
func RegisterJSONPointer() gojq.CompilerOption {
	return common.WithFunction("json_pointer", 1, 1, func(v any, args []any) any {
		pointer, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("json_pointer: pointer must be a string, got %T", args[0]), nil)
		}
		tokens, err := pointerTokens(pointer)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("json_pointer: %v", err), nil)
		}
		return pointerGet(common.BindValue(v), tokens)
	})
}

// RegisterJSONPointerSet registers json_pointer_set, returning the document
// with a value written at an RFC 6901 JSON pointer.
func RegisterJSONPointerSet() gojq.CompilerOption {
	return common.WithFunction("json_pointer_set", 2, 2, func(v any, args []any) any {
		pointer, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("json_pointer_set: pointer must be a string, got %T", args[0]), nil)
		}
		value := common.BindValue(args[1])
		tokens, err := pointerTokens(pointer)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("json_pointer_set: %v", err), nil)
		}
		out, err := pointerSet(common.BindValue(v), tokens, value)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("json_pointer_set: %v", err), nil)
		}
		return out
	})
}

// pointerTokens decodes an RFC 6901 pointer into its unescaped segments. The
// empty pointer means the whole document.
func pointerTokens(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("pointer %q must start with /", pointer)
	}
	parts := strings.Split(pointer[1:], "/")
	for i, p := range parts {
		p = strings.ReplaceAll(p, "~1", "/")
		p = strings.ReplaceAll(p, "~0", "~")
		parts[i] = p
	}
	return parts, nil
}

func pointerGet(v any, tokens []string) any {
	cur := v
	for _, t := range tokens {
		switch c := cur.(type) {
		case map[string]any:
			next, ok := c[t]
			if !ok {
				return nil
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(t)
			if err != nil || idx < 0 || idx >= len(c) {
				return nil
			}
			cur = c[idx]
		default:
			return nil
		}
	}
	return cur
}

// pointerSet writes value at the tokens, growing arrays and creating objects
// along the way. The input is a fresh value from the pipeline, so it is safe
// to mutate.
func pointerSet(v any, tokens []string, value any) (any, error) {
	slot := v
	if err := setSlot(&slot, tokens, value); err != nil {
		return nil, err
	}
	return slot, nil
}

func setSlot(slot *any, tokens []string, value any) error {
	if len(tokens) == 0 {
		*slot = value
		return nil
	}
	t := tokens[0]
	last := len(tokens) == 1
	switch c := (*slot).(type) {
	case map[string]any:
		if last {
			c[t] = value
			return nil
		}
		next, ok := c[t]
		if !ok {
			next = map[string]any{}
			c[t] = next
		}
		if err := setSlot(&next, tokens[1:], value); err != nil {
			return err
		}
		c[t] = next
		return nil
	case []any:
		idx, err := strconv.Atoi(t)
		if err != nil || idx < 0 {
			return fmt.Errorf("invalid array index %q", t)
		}
		for len(c) <= idx {
			c = append(c, nil)
		}
		*slot = c
		if last {
			c[idx] = value
			return nil
		}
		next := c[idx]
		if next == nil {
			next = map[string]any{}
			c[idx] = next
		}
		return setSlot(&c[idx], tokens[1:], value)
	default:
		return fmt.Errorf("cannot descend into %T at %q", *slot, t)
	}
}

// RegisterQueryStringParse registers query_string_parse, turning a URL query
// string like "a=1&b=two" into an object.
func RegisterQueryStringParse() gojq.CompilerOption {
	return common.WithFunction("query_string_parse", 0, 2, func(v any, args []any) any {
		inputVal, _, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("query_string_parse: %v", err), nil)
		}
		var input string
		switch val := common.BindValue(inputVal).(type) {
		case string:
			input = val
		case []byte:
			input = string(val)
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("query_string_parse: expected a string, got %T", inputVal), nil)
		}
		values, err := url.ParseQuery(input)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("query_string_parse: %v", err), nil)
		}
		out := make(map[string]any, len(values))
		for k, list := range values {
			if len(list) == 1 {
				out[k] = list[0]
			} else {
				arr := make([]any, len(list))
				for i, s := range list {
					arr[i] = s
				}
				out[k] = arr
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterQueryStringBuild registers query_string_build, the inverse of
// query_string_parse.
func RegisterQueryStringBuild() gojq.CompilerOption {
	return common.WithFunction("query_string_build", 0, 1, func(v any, args []any) any {
		input := common.BindValue(v)
		if len(args) > 0 {
			input = common.BindValue(args[0])
		}
		m, ok := input.(map[string]any)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("query_string_build: expected an object, got %T", input), nil)
		}
		values := url.Values{}
		for k, val := range m {
			switch item := val.(type) {
			case string:
				values.Add(k, item)
			case []any:
				for _, e := range item {
					if s, ok := e.(string); ok {
						values.Add(k, s)
					} else {
						values.Add(k, fmt.Sprint(e))
					}
				}
			default:
				values.Add(k, fmt.Sprint(item))
			}
		}
		return common.MakeUDFSuccessResult(values.Encode(), nil)
	})
}

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
	return common.WithFunction("set_path", 2, 3, func(v any, args []any) any {
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
	return common.WithFunction("has_path", 1, 2, func(v any, args []any) any {
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
	return common.WithFunction("del_path", 1, 2, func(v any, args []any) any {
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
