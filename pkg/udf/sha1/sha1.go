package sha1

import (
	"crypto/sha1"
	"fmt"
	"io"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterSHA1 registers the sha1 function with gojq
func RegisterSHA1() gojq.CompilerOption {
	return gojq.WithFunction("sha1", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("sha1: %v", err)
		}

		// Automatically extract _val if input is a UDF result object
		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			// Input is a file path
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("sha1: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("sha1: %v", err)
			}

			inputBytes = fileData
			filePath = absPath
			fileSize = size
		} else {
			// Input is data to hash
			switch val := inputVal.(type) {
			case string:
				inputBytes = []byte(val)
			case []byte:
				inputBytes = val
			case io.Reader:
				readBytes, err := io.ReadAll(val)
				if err != nil {
					return fmt.Errorf("sha1: failed to read input: %v", err)
				}
				inputBytes = readBytes
			default:
				if str, ok := val.(fmt.Stringer); ok {
					inputBytes = []byte(str.String())
				} else {
					return fmt.Errorf("sha1: argument must be a string or bytes, got %T", val)
				}
			}
		}

		// Compute SHA1 hash
		hash := sha1.Sum(inputBytes)
		hashHex := fmt.Sprintf("%x", hash)

		// Build metadata
		meta := map[string]any{
			"algorithm":   "sha1",
			"hash_length": len(hashHex),
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		} else {
			meta["input_length"] = len(inputBytes)
		}

		return map[string]any{
			"_val":  hashHex,
			"_meta": meta,
		}
	})
}
