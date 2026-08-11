// Package string provides the string cmdlets: substrings, case, lines, text
// measurement, escaping and predicates.
package string

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterReplace registers the replace function with gojq
func RegisterReplace() gojq.CompilerOption {
	return gojq.WithFunction("replace", 2, 4, func(v any, args []any) any {
		// Parse arguments: old, new, optional input, optional file flag
		if len(args) < 2 {
			return common.MakeUDFErrorResult(fmt.Errorf("replace: expected at least 2 arguments (old, new)"), nil)
		}

		oldStr, ok := args[0].(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("replace: first argument (old) must be a string, got %T", args[0]), nil)
		}

		newStr, ok := args[1].(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("replace: second argument (new) must be a string, got %T", args[1]), nil)
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

		inputVal = common.BindValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("replace: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("replace: %v", err), nil)
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
					return common.MakeUDFErrorResult(fmt.Errorf("replace: argument must be a string, got %T", val), nil)
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

		return common.MakeUDFSuccessResult(result, meta)
	})
}

// RegisterTrim registers the trim function with gojq
func RegisterTrim() gojq.CompilerOption {
	return gojq.WithFunction("trim", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("trim: %v", err), nil)
		}

		inputVal = common.BindValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("trim: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("trim: %v", err), nil)
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
					return common.MakeUDFErrorResult(fmt.Errorf("trim: argument must be a string, got %T", val), nil)
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

		return common.MakeUDFSuccessResult(result, meta)
	})
}

// RegisterSplit registers the split function with gojq
func RegisterSplit() gojq.CompilerOption {
	return gojq.WithFunction("split", 1, 3, func(v any, args []any) any {
		if len(args) < 1 {
			return common.MakeUDFErrorResult(fmt.Errorf("split: expected at least 1 argument (separator)"), nil)
		}

		separator, ok := args[0].(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("split: first argument (separator) must be a string, got %T", args[0]), nil)
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

		inputVal = common.BindValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("split: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("split: %v", err), nil)
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
					return common.MakeUDFErrorResult(fmt.Errorf("split: argument must be a string, got %T", val), nil)
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
			return common.MakeUDFErrorResult(fmt.Errorf("join_string: expected at least 1 argument (separator)"), nil)
		}

		separator, ok := args[0].(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("join_string: first argument (separator) must be a string, got %T", args[0]), nil)
		}

		// Extract _val if it's a UDF result
		inputVal := common.BindValue(v)

		// Input should be an array
		var arr []any
		switch val := inputVal.(type) {
		case []any:
			arr = val
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("join_string: input must be an array, got %T", val), nil)
		}

		// Convert array elements to strings
		var parts []string
		for _, item := range arr {
			itemVal := common.BindValue(item)
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

		return common.MakeUDFSuccessResult(result, meta)
	})
}

// RegisterSurround registers surround, a string wrapped in a prefix and suffix:
// surround("x"; "[ "; " ]") -> "[ x ]".
func RegisterSurround() gojq.CompilerOption {
	return gojq.WithFunction("surround", 2, 2, func(v any, args []any) any {
		prefix, okP := common.BindValue(args[0]).(string)
		suffix, okS := common.BindValue(args[1]).(string)
		if !okP || !okS {
			return common.MakeUDFErrorResult(fmt.Errorf("surround: prefix and suffix must be strings"), nil)
		}
		input, err := strFromPipeline(v, "surround")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(prefix+input+suffix, nil)
	})
}

// RegisterBeforeFirst registers before_first, the part of a string before the
// first occurrence of a separator (the whole string when there is none).
func RegisterBeforeFirst() gojq.CompilerOption {
	return gojq.WithFunction("before_first", 1, 1, func(v any, args []any) any {
		sep, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("before_first: separator must be a string, got %T", args[0]), nil)
		}
		input, err := strFromPipeline(v, "before_first")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if i := strings.Index(input, sep); i >= 0 {
			return common.MakeUDFSuccessResult(input[:i], nil)
		}
		return common.MakeUDFSuccessResult(input, nil)
	})
}

// RegisterAfterFirst registers after_first, the part of a string after the
// first occurrence of a separator (the empty string when there is none).
func RegisterAfterFirst() gojq.CompilerOption {
	return gojq.WithFunction("after_first", 1, 1, func(v any, args []any) any {
		sep, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("after_first: separator must be a string, got %T", args[0]), nil)
		}
		input, err := strFromPipeline(v, "after_first")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if i := strings.Index(input, sep); i >= 0 {
			return common.MakeUDFSuccessResult(input[i+len(sep):], nil)
		}
		return common.MakeUDFSuccessResult("", nil)
	})
}

// RegisterStripQuotes registers strip_quotes, one layer of matching quotes
// ('…', "…", or `…`) removed from each end.
func RegisterStripQuotes() gojq.CompilerOption {
	return registerTextFn("strip_quotes", func(s string) any {
		s = strings.TrimSpace(s)
		if utf8.RuneCountInString(s) < 2 {
			return s
		}
		runes := []rune(s)
		first, last := runes[0], runes[len(runes)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') || (first == '`' && last == '`') {
			return string(runes[1 : len(runes)-1])
		}
		return s
	})
}

// ANSI escapes, matched so strip_ansi can clear terminal colour from logs.
var (
	ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	ansiOSC = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
)

// RegisterStripANSI registers strip_ansi, removing ANSI terminal escape
// sequences from a string.
func RegisterStripANSI() gojq.CompilerOption {
	return registerCaseConverter("strip_ansi", func(s string) string {
		s = ansiOSC.ReplaceAllString(s, "")
		s = ansiCSI.ReplaceAllString(s, "")
		return s
	})
}
