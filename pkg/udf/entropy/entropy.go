package entropy

import (
	"fmt"
	"math"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterEntropy registers the entropy function with gojq
func RegisterEntropy() gojq.CompilerOption {
	return gojq.WithFunction("entropy", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("entropy: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("entropy: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("entropy: %v", err)
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
					return fmt.Errorf("entropy: argument must be a string or bytes, got %T", val)
				}
			}
		}

		// Calculate Shannon entropy
		if len(inputBytes) == 0 {
			return map[string]any{
				"_val":  0.0,
				"_meta": map[string]any{
					"operation": "entropy",
					"entropy":   0.0,
					"bits":      0.0,
				},
			}
		}

		// Count byte frequencies
		freq := make(map[byte]int)
		for _, b := range inputBytes {
			freq[b]++
		}

		// Calculate entropy
		entropy := 0.0
		dataLength := float64(len(inputBytes))
		for _, count := range freq {
			if count > 0 {
				probability := float64(count) / dataLength
				entropy -= probability * math.Log2(probability)
			}
		}

		// Calculate bits (entropy * length)
		bits := entropy * dataLength

		meta := map[string]any{
			"operation": "entropy",
			"entropy":   entropy,
			"bits":      bits,
			"length":    len(inputBytes),
			"unique_bytes": len(freq),
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		}

		return map[string]any{
			"_val":  entropy,
			"_meta": meta,
		}
	})
}

