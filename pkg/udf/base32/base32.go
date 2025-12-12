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
			return fmt.Errorf("base32_encode: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("base32_encode: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("base32_encode: %v", err)
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
					return fmt.Errorf("base32_encode: argument must be a string, got %T", val)
				}
			}
		}

		// Encode to base32
		encoded := base32.StdEncoding.EncodeToString([]byte(input))

		meta := map[string]any{
			"encoding": "base32",
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

// RegisterBase32Decode registers the base32_decode function with gojq
func RegisterBase32Decode() gojq.CompilerOption {
	return gojq.WithFunction("base32_decode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("base32_decode: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("base32_decode: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("base32_decode: %v", err)
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
					return fmt.Errorf("base32_decode: argument must be a string, got %T", val)
				}
			}
		}

		// Decode from base32
		decoded, err := base32.StdEncoding.DecodeString(input)
		if err != nil {
			return fmt.Errorf("base32_decode: invalid base32 string: %v", err)
		}

		meta := map[string]any{
			"encoding": "base32",
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
			"_val":  string(decoded),
			"_meta": meta,
		}
	})
}

