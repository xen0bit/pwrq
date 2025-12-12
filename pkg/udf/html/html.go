package html

import (
	"fmt"
	"html"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterHTMLEncode registers the html_encode function with gojq
func RegisterHTMLEncode() gojq.CompilerOption {
	return gojq.WithFunction("html_encode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("html_encode: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("html_encode: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "html_encode",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("html_encode: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("html_encode: argument must be a string, got %T", val), nil)
				}
			}
		}

		encoded := html.EscapeString(input)

		meta := map[string]any{
			"encoding":        "html",
			"original_length": len(input),
			"encoded_length":  len(encoded),
		}
		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			delete(meta, "original_length")
		}

		return common.MakeUDFSuccessResult(encoded, meta)
	})
}

// RegisterHTMLDecode registers the html_decode function with gojq
func RegisterHTMLDecode() gojq.CompilerOption {
	return gojq.WithFunction("html_decode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("html_decode: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("html_decode: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "html_decode",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("html_decode: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("html_decode: argument must be a string, got %T", val), nil)
				}
			}
		}

		decoded := html.UnescapeString(input)

		meta := map[string]any{
			"encoding":        "html",
			"original_length": len(input),
			"decoded_length":  len(decoded),
		}
		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			delete(meta, "original_length")
		}

		return common.MakeUDFSuccessResult(decoded, meta)
	})
}
