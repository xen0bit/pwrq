package binary

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterBinaryEncode registers the binary_encode function with gojq
func RegisterBinaryEncode() gojq.CompilerOption {
	return gojq.WithFunction("binary_encode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("binary_encode: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("binary_encode: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("binary_encode: %v", err)
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
					return fmt.Errorf("binary_encode: argument must be a string or bytes, got %T", val)
				}
			}
		}

		// Encode to binary (space-separated bytes)
		var parts []string
		for _, b := range inputBytes {
			parts = append(parts, fmt.Sprintf("%08b", b))
		}
		encoded := strings.Join(parts, " ")

		meta := map[string]any{
			"encoding": "binary",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			meta["encoded_length"] = len(encoded)
		} else {
			meta["original_length"] = len(inputBytes)
			meta["encoded_length"] = len(encoded)
		}

		return map[string]any{
			"_val":  encoded,
			"_meta": meta,
		}
	})
}

// RegisterBinaryDecode registers the binary_decode function with gojq
func RegisterBinaryDecode() gojq.CompilerOption {
	return gojq.WithFunction("binary_decode", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("binary_decode: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("binary_decode: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("binary_decode: %v", err)
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
					return fmt.Errorf("binary_decode: argument must be a string, got %T", val)
				}
			}
		}

		// Decode from binary (space-separated bytes or continuous string)
		// First try space-separated, then try continuous
		parts := strings.Fields(input)
		var decoded []byte
		
		if len(parts) > 1 {
			// Space-separated format
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if len(part) != 8 {
					return fmt.Errorf("binary_decode: each binary byte must be 8 bits, got %d bits in %q", len(part), part)
				}
				val, err := strconv.ParseUint(part, 2, 8)
				if err != nil {
					return fmt.Errorf("binary_decode: invalid binary string %q: %v", part, err)
				}
				decoded = append(decoded, byte(val))
			}
		} else {
			// Continuous format - split into 8-bit chunks
			binaryStr := strings.ReplaceAll(input, " ", "")
			if len(binaryStr)%8 != 0 {
				return fmt.Errorf("binary_decode: binary string length must be multiple of 8, got %d", len(binaryStr))
			}
			for i := 0; i < len(binaryStr); i += 8 {
				part := binaryStr[i : i+8]
				val, err := strconv.ParseUint(part, 2, 8)
				if err != nil {
					return fmt.Errorf("binary_decode: invalid binary string %q: %v", part, err)
				}
				decoded = append(decoded, byte(val))
			}
		}

		meta := map[string]any{
			"encoding": "binary",
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

