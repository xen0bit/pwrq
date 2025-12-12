package ssdeep

import (
	"fmt"

	"github.com/glaslos/ssdeep"
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterSSDeep registers the ssdeep function with gojq
func RegisterSSDeep() gojq.CompilerOption {
	return gojq.WithFunction("ssdeep", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("ssdeep: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("ssdeep: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				meta := map[string]any{
					"operation": "ssdeep",
					"file_path": absPath,
				}
				return common.MakeUDFErrorResult(fmt.Errorf("ssdeep: %v", err), meta)
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
					return common.MakeUDFErrorResult(fmt.Errorf("ssdeep: argument must be a string or bytes, got %T", val), nil)
				}
			}
		}

		// Calculate ssdeep hash
		// Note: ssdeep requires at least 4096 bytes for meaningful results
		hash, err := ssdeep.FuzzyBytes(inputBytes)
		if err != nil {
			meta := map[string]any{
				"algorithm": "ssdeep",
			}
			if isFile {
				meta["file_path"] = filePath
				meta["file_size"] = int(fileSize)
			} else {
				meta["input_length"] = len(inputBytes)
			}
			
			// Check if it's the "file too small" error
			if err.Error() == "did not process files large enough to produce meaningful results" {
				return common.MakeUDFErrorResult(fmt.Errorf("ssdeep: input too small (minimum 4096 bytes required, got %d bytes)", len(inputBytes)), meta)
			}
			return common.MakeUDFErrorResult(fmt.Errorf("ssdeep: failed to calculate hash: %v", err), meta)
		}

		meta := map[string]any{
			"algorithm": "ssdeep",
			"hash_length": len(hash),
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		} else {
			meta["input_length"] = len(inputBytes)
		}

		return common.MakeUDFSuccessResult(hash, meta)
	})
}

// RegisterSSDeepCompare registers the ssdeep_compare function with gojq
func RegisterSSDeepCompare() gojq.CompilerOption {
	return gojq.WithFunction("ssdeep_compare", 2, 2, func(v any, args []any) any {
		if len(args) < 2 {
			return common.MakeUDFErrorResult(fmt.Errorf("ssdeep_compare: expected 2 arguments (hash1, hash2)"), nil)
		}

		hash1Val := common.ExtractUDFValue(args[0])
		hash2Val := common.ExtractUDFValue(args[1])

		var hash1, hash2 string

		switch val := hash1Val.(type) {
		case string:
			hash1 = val
		case []byte:
			hash1 = string(val)
		default:
			if str, ok := val.(fmt.Stringer); ok {
				hash1 = str.String()
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("ssdeep_compare: first argument must be a string, got %T", val), nil)
			}
		}

		switch val := hash2Val.(type) {
		case string:
			hash2 = val
		case []byte:
			hash2 = string(val)
		default:
			if str, ok := val.(fmt.Stringer); ok {
				hash2 = str.String()
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("ssdeep_compare: second argument must be a string, got %T", val), nil)
			}
		}

		// Compare ssdeep hashes
		score, err := ssdeep.Distance(hash1, hash2)
		if err != nil {
			meta := map[string]any{
				"operation": "ssdeep_compare",
				"hash1":     hash1,
				"hash2":     hash2,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("ssdeep_compare: failed to compare hashes: %v", err), meta)
		}

		meta := map[string]any{
			"operation": "ssdeep_compare",
			"hash1":     hash1,
			"hash2":     hash2,
			"score":     score,
		}

		return common.MakeUDFSuccessResult(score, meta)
	})
}

