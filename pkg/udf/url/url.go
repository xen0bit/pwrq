package url

import (
	"fmt"
	"net/url"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterURLEncode registers the url_encode function with gojq
func RegisterURLEncode() gojq.CompilerOption {
	return gojq.WithFunction("url_encode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("url_encode: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("url_encode: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "url_encode",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("url_encode: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("url_encode: argument must be a string, got %T", val), nil)
				}
			}
		}

		encoded := url.QueryEscape(input)

		meta := map[string]any{
			"encoding":        "url",
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

// RegisterURLDecode registers the url_decode function with gojq
func RegisterURLDecode() gojq.CompilerOption {
	return gojq.WithFunction("url_decode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("url_decode: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("url_decode: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "url_decode",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("url_decode: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("url_decode: argument must be a string, got %T", val), nil)
				}
			}
		}

		decoded, err := url.QueryUnescape(input)
		if err != nil {
			meta := map[string]any{
				"encoding": "url",
			}
			if isFile {
				meta["file_path"] = filePath
				meta["file_size"] = int(fileSize)
			} else {
				meta["original_length"] = len(input)
			}
			return common.MakeUDFErrorResult(fmt.Errorf("url_decode: invalid URL-encoded string: %v", err), meta)
		}

		meta := map[string]any{
			"encoding":        "url",
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
