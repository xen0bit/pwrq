package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterJSONMergePatch registers json_merge_patch, applying an RFC 7386 merge
// patch: null in the patch deletes a key, objects merge recursively, and
// everything else replaces.
func RegisterJSONMergePatch() gojq.CompilerOption {
	return gojq.WithFunction("json_merge_patch", 1, 2, func(v any, args []any) any {
		target := common.BindValue(v)
		if len(args) > 0 {
			target = common.BindValue(args[0])
		}
		patch := common.BindValue(args[1])
		return common.MakeUDFSuccessResult(mergePatch(target, patch), nil)
	})
}

func mergePatch(target, patch any) any {
	patchMap, patchIsObject := patch.(map[string]any)
	targetMap, targetIsObject := target.(map[string]any)
	if !patchIsObject {
		if patch == nil {
			return nil
		}
		return patch
	}
	if !targetIsObject {
		// Replacing a non-object with an object patch.
		result := make(map[string]any, len(patchMap))
		for k, v := range patchMap {
			if v == nil {
				continue
			}
			result[k] = mergePatch(nil, v)
		}
		return result
	}
	result := make(map[string]any, len(targetMap)+len(patchMap))
	for k, v := range targetMap {
		result[k] = v
	}
	for k, v := range patchMap {
		if v == nil {
			delete(result, k)
			continue
		}
		result[k] = mergePatch(result[k], v)
	}
	return result
}

// RegisterJSONLParse registers jsonl_parse, parsing newline-delimited JSON into
// an array, skipping blank lines.
func RegisterJSONLParse() gojq.CompilerOption {
	return gojq.WithFunction("jsonl_parse", 0, 2, func(v any, args []any) any {
		inputVal, _, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("jsonl_parse: %v", err), nil)
		}
		var input string
		switch val := common.BindValue(inputVal).(type) {
		case string:
			input = val
		case []byte:
			input = string(val)
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("jsonl_parse: expected a string, got %T", inputVal), nil)
		}
		var out []any
		dec := json.NewDecoder(bytes.NewBufferString(input))
		for {
			var item any
			if err := dec.Decode(&item); err != nil {
				break
			}
			out = append(out, item)
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterGetPath registers get_path, reading the value at a dot-and-bracket
// path like "a.b[0]".
func RegisterGetPath() gojq.CompilerOption {
	return gojq.WithFunction("get_path", 1, 1, func(v any, args []any) any {
		path, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("get_path: path must be a string, got %T", args[0]), nil)
		}
		tokens, err := dotTokens(path)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("get_path: %v", err), nil)
		}
		return pointerGet(common.BindValue(v), tokens)
	})
}

// dotTokens splits a dot-and-bracket path like "a.b[0].c" into plain segments
// (array indices are rendered as bare strings, which pointerGet resolves).
func dotTokens(path string) ([]string, error) {
	var tokens []string
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
			if j >= len(path) {
				return nil, fmt.Errorf("unclosed bracket in path %q", path)
			}
			idx, err := strconv.Atoi(path[i+1 : j])
			if err != nil {
				return nil, fmt.Errorf("bad array index in path %q", path)
			}
			tokens = append(tokens, strconv.Itoa(idx))
			i = j + 1
		default:
			j := i
			for j < len(path) && path[j] != '.' && path[j] != '[' {
				j++
			}
			tokens = append(tokens, path[i:j])
			i = j
		}
	}
	return tokens, nil
}
