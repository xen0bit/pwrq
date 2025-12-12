package hex

import (
	"encoding/hex"
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterHexEncode registers the hex_encode function with gojq
func RegisterHexEncode() gojq.CompilerOption {
	return gojq.WithFunction("hex_encode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("hex_encode: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("hex_encode: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "hex_encode",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("hex_encode: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("hex_encode: argument must be a string or bytes, got %T", val), nil)
				}
			}
		}

		encoded := hex.EncodeToString(inputBytes)

		meta := map[string]any{
			"encoding":        "hex",
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

// RegisterHexDecode registers the hex_decode function with gojq
func RegisterHexDecode() gojq.CompilerOption {
	return gojq.WithFunction("hex_decode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("hex_decode: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("hex_decode: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "hex_decode",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("hex_decode: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("hex_decode: argument must be a string, got %T", val), nil)
				}
			}
		}

		decoded, err := hex.DecodeString(input)
		if err != nil {
			meta := map[string]any{
				"encoding": "hex",
			}
			if isFile {
				meta["file_path"] = filePath
				meta["file_size"] = int(fileSize)
			} else {
				meta["original_length"] = len(input)
			}
			return common.MakeUDFErrorResult(fmt.Errorf("hex_decode: invalid hex string: %v", err), meta)
		}

		meta := map[string]any{
			"encoding":        "hex",
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
