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
			return fmt.Errorf("html_encode: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("html_encode: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("html_encode: %v", err)
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
					return fmt.Errorf("html_encode: argument must be a string, got %T", val)
				}
			}
		}

		// HTML encode
		encoded := html.EscapeString(input)

		meta := map[string]any{
			"encoding": "html",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			meta["encoded_length"] = len(encoded)
		} else {
			meta["original_length"] = len(input)
			meta["encoded_length"] = len(encoded)
		}

		return map[string]any{
			"_val":  encoded,
			"_meta": meta,
		}
	})
}

// RegisterHTMLDecode registers the html_decode function with gojq
func RegisterHTMLDecode() gojq.CompilerOption {
	return gojq.WithFunction("html_decode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("html_decode: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("html_decode: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("html_decode: %v", err)
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
					return fmt.Errorf("html_decode: argument must be a string, got %T", val)
				}
			}
		}

		// HTML decode
		decoded := html.UnescapeString(input)

		meta := map[string]any{
			"encoding": "html",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			meta["decoded_length"] = len(decoded)
		} else {
			meta["original_length"] = len(input)
			meta["decoded_length"] = len(decoded)
		}

		return map[string]any{
			"_val":  decoded,
			"_meta": meta,
		}
	})
}

