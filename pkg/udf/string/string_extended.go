package string

import (
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterTrim registers the trim function with gojq
func RegisterTrim() gojq.CompilerOption {
	return gojq.WithFunction("trim", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("trim: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("trim: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("trim: %v", err)
			}

			input = string(fileData)
			filePath = absPath
			fileSize = size
		} else {
			switch val := inputVal.(type) {
			case string:
				input = val
			case []byte:
				input = string(val)
			default:
				if str, ok := val.(fmt.Stringer); ok {
					input = str.String()
				} else {
					return fmt.Errorf("trim: argument must be a string, got %T", val)
				}
			}
		}

		result := strings.TrimSpace(input)

		meta := map[string]any{
			"operation": "trim",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		} else {
			meta["original_length"] = len(input)
			meta["trimmed_length"] = len(result)
		}

		return map[string]any{
			"_val":  result,
			"_meta": meta,
		}
	})
}

// RegisterSplit registers the split function with gojq
func RegisterSplit() gojq.CompilerOption {
	return gojq.WithFunction("split", 1, 3, func(v any, args []any) any {
		if len(args) < 1 {
			return fmt.Errorf("split: expected at least 1 argument (separator)")
		}

		separator, ok := args[0].(string)
		if !ok {
			return fmt.Errorf("split: first argument (separator) must be a string, got %T", args[0])
		}

		var inputVal any
		var isFile bool

		if len(args) > 1 {
			if fileFlag, ok := args[1].(bool); ok {
				isFile = fileFlag
				inputVal = v
			} else {
				inputVal = args[1]
				if len(args) > 2 {
					if fileFlag, ok := args[2].(bool); ok {
						isFile = fileFlag
					}
				}
			}
		} else {
			inputVal = v
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("split: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("split: %v", err)
			}

			input = string(fileData)
			filePath = absPath
			fileSize = size
		} else {
			switch val := inputVal.(type) {
			case string:
				input = val
			case []byte:
				input = string(val)
			default:
				if str, ok := val.(fmt.Stringer); ok {
					input = str.String()
				} else {
					return fmt.Errorf("split: argument must be a string, got %T", val)
				}
			}
		}

		parts := strings.Split(input, separator)
		// Convert to array of any
		result := make([]any, len(parts))
		for i, part := range parts {
			result[i] = part
		}

		meta := map[string]any{
			"operation": "split",
			"separator": separator,
			"count":     len(parts),
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		}

		// For split, return the array directly (not wrapped in _val/_meta)
		// This allows it to be used with array operations
		return result
	})
}

// RegisterJoin registers the join_string function with gojq (renamed to avoid conflict with gojq's built-in join)
func RegisterJoin() gojq.CompilerOption {
	return gojq.WithFunction("join_string", 1, 1, func(v any, args []any) any {
		if len(args) < 1 {
			return fmt.Errorf("join_string: expected at least 1 argument (separator)")
		}

		separator, ok := args[0].(string)
		if !ok {
			return fmt.Errorf("join_string: first argument (separator) must be a string, got %T", args[0])
		}

		// Extract _val if it's a UDF result
		inputVal := common.ExtractUDFValue(v)

		// Input should be an array
		var arr []any
		switch val := inputVal.(type) {
		case []any:
			arr = val
		default:
			return fmt.Errorf("join_string: input must be an array, got %T", val)
		}

		// Convert array elements to strings
		var parts []string
		for _, item := range arr {
			itemVal := common.ExtractUDFValue(item)
			switch v := itemVal.(type) {
			case string:
				parts = append(parts, v)
			case []byte:
				parts = append(parts, string(v))
			default:
				parts = append(parts, fmt.Sprintf("%v", v))
			}
		}

		result := strings.Join(parts, separator)

		meta := map[string]any{
			"operation": "join_string",
			"separator": separator,
			"count":     len(parts),
		}

		return map[string]any{
			"_val":  result,
			"_meta": meta,
		}
	})
}

