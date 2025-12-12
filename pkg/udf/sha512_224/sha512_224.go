package sha512_224

import (
	"crypto/sha512"
	"fmt"
	"io"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterSHA512_224 registers the sha512_224 function with gojq
func RegisterSHA512_224() gojq.CompilerOption {
	return gojq.WithFunction("sha512_224", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("sha512_224: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("sha512_224: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "sha512_224",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("sha512_224: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("sha512_224: failed to read input: %v", err), nil)
				}
				inputBytes = readBytes
			default:
				if str, ok := val.(fmt.Stringer); ok {
					inputBytes = []byte(str.String())
				} else {
					return common.MakeUDFErrorResult(fmt.Errorf("sha512_224: argument must be a string or bytes, got %T", val), nil)
				}
			}
		}

		hash := sha512.Sum512_224(inputBytes)
		hashHex := fmt.Sprintf("%x", hash)

		meta := map[string]any{
			"algorithm":   "sha512_224",
			"hash_length": len(hashHex),
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		} else {
			meta["input_length"] = len(inputBytes)
		}

		return common.MakeUDFSuccessResult(hashHex, meta)
	})
}
