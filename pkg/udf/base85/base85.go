package base85

import (
	"encoding/ascii85"
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterBase85Encode registers the base85_encode function with gojq
func RegisterBase85Encode() gojq.CompilerOption {
	return gojq.WithFunction("base85_encode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("base85_encode: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("base85_encode: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "base85_encode",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("base85_encode: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("base85_encode: argument must be a string or bytes, got %T", val), nil)
				}
			}
		}

		// Encode to base85
		encoded := make([]byte, ascii85.MaxEncodedLen(len(inputBytes)))
		n := ascii85.Encode(encoded, inputBytes)
		encoded = encoded[:n]

		meta := map[string]any{
			"encoding": "base85",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			meta["encoded_length"] = len(encoded)
		} else {
			meta["original_length"] = len(inputBytes)
			meta["encoded_length"] = len(encoded)
		}

		return common.MakeUDFSuccessResult(string(encoded), meta)
	})
}

// RegisterBase85Decode registers the base85_decode function with gojq
func RegisterBase85Decode() gojq.CompilerOption {
	return gojq.WithFunction("base85_decode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("base85_decode: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("base85_decode: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "base85_decode",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("base85_decode: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("base85_decode: argument must be a string or bytes, got %T", val), nil)
				}
			}
		}

		// Decode from base85
		// Base85 encoding expands data, so we need a larger buffer
		decoded := make([]byte, ascii85.MaxEncodedLen(len(inputBytes)))
		n, _, err := ascii85.Decode(decoded, inputBytes, true)
		if err != nil {
			meta := map[string]any{
				"encoding": "base85",
			}
			if isFile {
				meta["file_path"] = filePath
				meta["file_size"] = int(fileSize)
			} else {
				meta["original_length"] = len(inputBytes)
			}
			return common.MakeUDFErrorResult(fmt.Errorf("base85_decode: invalid base85 string: %v", err), meta)
		}
		decoded = decoded[:n]

		meta := map[string]any{
			"encoding":        "base85",
			"original_length": len(inputBytes),
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

