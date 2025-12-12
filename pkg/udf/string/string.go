package string

import (
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterUpper registers the upper function with gojq
func RegisterUpper() gojq.CompilerOption {
	return gojq.WithFunction("upper", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("upper: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("upper: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("upper: %v", err)
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
					return fmt.Errorf("upper: argument must be a string, got %T", val)
				}
			}
		}

		result := strings.ToUpper(input)

		meta := map[string]any{
			"operation": "upper",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		} else {
			meta["original_length"] = len(input)
		}

		return map[string]any{
			"_val":  result,
			"_meta": meta,
		}
	})
}

// RegisterLower registers the lower function with gojq
func RegisterLower() gojq.CompilerOption {
	return gojq.WithFunction("lower", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("lower: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("lower: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("lower: %v", err)
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
					return fmt.Errorf("lower: argument must be a string, got %T", val)
				}
			}
		}

		result := strings.ToLower(input)

		meta := map[string]any{
			"operation": "lower",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		} else {
			meta["original_length"] = len(input)
		}

		return map[string]any{
			"_val":  result,
			"_meta": meta,
		}
	})
}

// RegisterReverse registers the reverse_string function with gojq
func RegisterReverse() gojq.CompilerOption {
	return gojq.WithFunction("reverse_string", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("reverse: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("reverse: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("reverse: %v", err)
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
					return fmt.Errorf("reverse: argument must be a string, got %T", val)
				}
			}
		}

		// Reverse string
		runes := []rune(input)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		result := string(runes)

		meta := map[string]any{
			"operation": "reverse",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		} else {
			meta["original_length"] = len(input)
		}

		return map[string]any{
			"_val":  result,
			"_meta": meta,
		}
	})
}

// RegisterReplace registers the replace function with gojq
func RegisterReplace() gojq.CompilerOption {
	return gojq.WithFunction("replace", 2, 4, func(v any, args []any) any {
		// Parse arguments: old, new, optional input, optional file flag
		if len(args) < 2 {
			return fmt.Errorf("replace: expected at least 2 arguments (old, new)")
		}

		oldStr, ok := args[0].(string)
		if !ok {
			return fmt.Errorf("replace: first argument (old) must be a string, got %T", args[0])
		}

		newStr, ok := args[1].(string)
		if !ok {
			return fmt.Errorf("replace: second argument (new) must be a string, got %T", args[1])
		}

		var inputVal any
		var isFile bool

		if len(args) > 2 {
			// Check if third arg is boolean (file flag) or value
			if fileFlag, ok := args[2].(bool); ok {
				isFile = fileFlag
				inputVal = v
			} else {
				inputVal = args[2]
				// Check for file flag as fourth arg
				if len(args) > 3 {
					if fileFlag, ok := args[3].(bool); ok {
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
				return fmt.Errorf("replace: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("replace: %v", err)
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
					return fmt.Errorf("replace: argument must be a string, got %T", val)
				}
			}
		}

		result := strings.ReplaceAll(input, oldStr, newStr)

		meta := map[string]any{
			"operation": "replace",
			"old":       oldStr,
			"new":       newStr,
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		} else {
			meta["original_length"] = len(input)
		}

		return map[string]any{
			"_val":  result,
			"_meta": meta,
		}
	})
}

