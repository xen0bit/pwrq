package base64

import (
	"encoding/base64"
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterBase64Encode registers the base64_encode function with gojq
func RegisterBase64Encode() gojq.CompilerOption {
	return gojq.WithFunction("base64_encode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("base64_encode: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("base64_encode: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "base64_encode",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("base64_encode: %v", err), meta)
			}

			inputBytes = fileData
			filePath = absPath
			fileSize = size
		} else {
			switch val := inputVal.(type) {
			case string:
				inputBytes = []byte(val)
			case []byte:
				inputBytes = val
			default:
				if str, ok := val.(fmt.Stringer); ok {
					inputBytes = []byte(str.String())
				} else {
					return common.MakeUDFErrorResult(fmt.Errorf("base64_encode: argument must be a string or bytes, got %T", val), nil)
				}
			}
		}

		encoded := base64.StdEncoding.EncodeToString(inputBytes)

		meta := map[string]any{
			"encoding":        "base64",
			"original_length": len(inputBytes),
			"encoded_length":  len(encoded),
		}
		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			delete(meta, "original_length") // Remove original_length if it's a file
		}

		return common.MakeUDFSuccessResult(encoded, meta)
	})
}

// RegisterBase64Decode registers the base64_decode function with gojq
func RegisterBase64Decode() gojq.CompilerOption {
	return gojq.WithFunction("base64_decode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("base64_decode: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("base64_decode: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "base64_decode",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("base64_decode: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("base64_decode: argument must be a string, got %T", val), nil)
				}
			}
		}

		// Decode from base64
		decoded, err := base64.StdEncoding.DecodeString(input)
		if err != nil {
			meta := map[string]any{
				"encoding": "base64",
			}
			if isFile {
				meta["file_path"] = filePath
				meta["file_size"] = int(fileSize)
			} else {
				meta["original_length"] = len(input)
			}
			return common.MakeUDFErrorResult(fmt.Errorf("base64_decode: invalid base64 string: %v", err), meta)
		}

		meta := map[string]any{
			"encoding":        "base64",
			"original_length": len(input),
			"decoded_length":  len(decoded),
		}
		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			delete(meta, "original_length") // Remove original_length if it's a file
		}

		return common.MakeUDFSuccessResult(string(decoded), meta)
	})
}
