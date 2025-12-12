package json

import (
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterJSONParse registers the json_parse function with gojq
func RegisterJSONParse() gojq.CompilerOption {
	return gojq.WithFunction("json_parse", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("json_parse: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

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
	return gojq.WithFunction("json_stringify", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("json_stringify: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		// Stringify the input value
		jsonBytes, err := json.Marshal(inputVal)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("json_stringify: failed to marshal: %v", err), nil)
		}

		result := string(jsonBytes)

		meta := map[string]any{
			"operation": "json_stringify",
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

