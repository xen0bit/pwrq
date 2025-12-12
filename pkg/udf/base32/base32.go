package base32

import (
	"encoding/base32"
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterBase32Encode registers the base32_encode function with gojq
func RegisterBase32Encode() gojq.CompilerOption {
	return gojq.WithFunction("base32_encode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("base32_encode: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("base32_encode: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "base32_encode",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("base32_encode: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("base32_encode: argument must be a string or bytes, got %T", val), nil)
				}
			}
		}

		encoded := base32.StdEncoding.EncodeToString(inputBytes)

		meta := map[string]any{
			"encoding":        "base32",
			"original_length": len(inputBytes),
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

// RegisterBase32Decode registers the base32_decode function with gojq
func RegisterBase32Decode() gojq.CompilerOption {
	return gojq.WithFunction("base32_decode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("base32_decode: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("base32_decode: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "base32_decode",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("base32_decode: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("base32_decode: argument must be a string, got %T", val), nil)
				}
			}
		}

		decoded, err := base32.StdEncoding.DecodeString(input)
		if err != nil {
			meta := map[string]any{
				"encoding": "base32",
			}
			if isFile {
				meta["file_path"] = filePath
				meta["file_size"] = int(fileSize)
			} else {
				meta["original_length"] = len(input)
			}
			return common.MakeUDFErrorResult(fmt.Errorf("base32_decode: invalid base32 string: %v", err), meta)
		}

		meta := map[string]any{
			"encoding":        "base32",
			"original_length": len(input),
			"decoded_length":  len(decoded),
		}
		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			delete(meta, "original_length")
		}

		return common.MakeUDFSuccessResult(string(decoded), meta)
	})
}
