package sha512_256

import (
	"crypto/sha512"
	"fmt"
	"io"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterSHA512_256 registers the sha512_256 function with gojq
func RegisterSHA512_256() gojq.CompilerOption {
	return gojq.WithFunction("sha512_256", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("sha512_256: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("sha512_256: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("sha512_256: %v", err)
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
			case io.Reader:
				readBytes, err := io.ReadAll(val)
				if err != nil {
					return fmt.Errorf("sha512_256: failed to read input: %v", err)
				}
				inputBytes = readBytes
			default:
				if str, ok := val.(fmt.Stringer); ok {
					inputBytes = []byte(str.String())
				} else {
					return fmt.Errorf("sha512_256: argument must be a string or bytes, got %T", val)
				}
			}
		}

		hash := sha512.Sum512_256(inputBytes)
		hashHex := fmt.Sprintf("%x", hash)

		meta := map[string]any{
			"algorithm":   "sha512_256",
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
