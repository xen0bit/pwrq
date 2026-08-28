package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterJSONParse registers the json_parse function with gojq
func RegisterJSONParse() gojq.CompilerOption {
	return common.WithFunction("json_parse", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("json_parse: %v", err), nil)
		}

		inputVal = common.BindValue(inputVal)

		var result any
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("json_parse: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("json_parse: %v", err), nil)
			}

			// Parse JSON from file
			if err := json.Unmarshal(fileData, &result); err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("json_parse: invalid JSON in file: %v", err), nil)
			}
			filePath = absPath
			fileSize = size
		} else {
			// Check if input is already a parsed object/array
			switch val := inputVal.(type) {
			case map[string]any, []any:
				// Already parsed, return as-is
				result = val
			case string:
				// Parse JSON string
				if err := json.Unmarshal([]byte(val), &result); err != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("json_parse: invalid JSON: %v", err), nil)
				}
			case []byte:
				// Parse JSON bytes
				if err := json.Unmarshal(val, &result); err != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("json_parse: invalid JSON: %v", err), nil)
				}
			default:
				// Try to convert to string and parse
				if str, ok := val.(fmt.Stringer); ok {
					if err := json.Unmarshal([]byte(str.String()), &result); err != nil {
						return common.MakeUDFErrorResult(fmt.Errorf("json_parse: invalid JSON: %v", err), nil)
					}
				} else {
					// If it's a simple type (number, bool, null), return as-is
					result = val
				}
			}
		}

		meta := map[string]any{
			"operation": "json_parse",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		}

		// For json_parse, return the parsed object directly (not wrapped in _val/_meta)
		// This allows it to be used with object operations
		return result
	})
}

// RegisterJSONStringify registers the json_stringify function with gojq
func RegisterJSONStringify() gojq.CompilerOption {
	return common.WithFunction("json_stringify", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("json_stringify: %v", err), nil)
		}

		inputVal = common.BindValue(inputVal)

		// Stringify the input value
		jsonBytes, err := json.Marshal(inputVal)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("json_stringify: failed to marshal: %v", err), nil)
		}

		result := string(jsonBytes)

		meta := map[string]any{
			"operation":     "json_stringify",
			"output_length": len(result),
		}

		if isFile {
			filePathStr, ok := inputVal.(string)
			if ok {
				_, absPath, size, err := common.ReadFileFromPath(filePathStr)
				if err == nil {
					meta["file_path"] = absPath
					meta["file_size"] = int(size)
				}
			}
		}

		return common.MakeUDFSuccessResult(result, meta)
	})
}

// RegisterJSONMergePatch registers json_merge_patch, applying an RFC 7386 merge
// patch: null in the patch deletes a key, objects merge recursively, and
// everything else replaces.
func RegisterJSONMergePatch() gojq.CompilerOption {
	common.DeclareInput("json_merge_patch", common.InputPipeline)
	return common.WithFunctionOf("json_merge_patch", 1, 2, EditedDocument, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		target, patch := common.BindValue(in), common.BindValue(rest[0])
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
	return common.WithFunction("jsonl_parse", 0, 2, func(v any, args []any) any {
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
	return common.WithFunction("get_path", 1, 1, func(v any, args []any) any {
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
		switch path[i] {
		case '.':
			i++
		case '[':
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
