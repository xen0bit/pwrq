package sha256

import (
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterSHA256 registers the sha256 function with gojq
func RegisterSHA256() gojq.CompilerOption {
	return gojq.WithFunction("sha256", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("sha256: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("sha256: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "sha256",
				}
				return common.MakeUDFErrorResult(fmt.Errorf("sha256: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("sha256: failed to read input: %v", err), nil)
				}
				inputBytes = readBytes
			default:
				if str, ok := val.(fmt.Stringer); ok {
					inputBytes = []byte(str.String())
				} else {
					return common.MakeUDFErrorResult(fmt.Errorf("sha256: argument must be a string or bytes, got %T", val), nil)
				}
			}
		}

		hash := sha256.Sum256(inputBytes)
		hashHex := fmt.Sprintf("%x", hash)

		meta := map[string]any{
			"algorithm":   "sha256",
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
