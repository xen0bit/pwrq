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
	return gojq.WithFunction("sha1", 0, 1, func(v any, args []any) any {
		// Use argument if provided, otherwise use current value
		var inputVal any
		if len(args) > 0 {
			inputVal = args[0]
		} else {
			inputVal = v
		}

		// Automatically extract _val if input is a UDF result object
		// This is standard behavior for all UDFs
		inputVal = common.ExtractUDFValue(inputVal)

		// Convert input to string or bytes
		var inputBytes []byte
		switch val := inputVal.(type) {
		case string:
			inputBytes = []byte(val)
		case []byte:
			inputBytes = val
		case io.Reader:
			// If it's a reader, read all data
			readBytes, err := io.ReadAll(val)
			if err != nil {
				return fmt.Errorf("sha1: failed to read input: %v", err)
			}
			inputBytes = readBytes
		default:
			// Try to convert to string
			if str, ok := val.(fmt.Stringer); ok {
				inputBytes = []byte(str.String())
			} else {
				return fmt.Errorf("sha1: argument must be a string or bytes, got %T", val)
			}
		}

		// Compute SHA1 hash
		hash := sha1.Sum(inputBytes)
		hashHex := fmt.Sprintf("%x", hash)

		// Return object with _val and _meta
		return map[string]any{
			"_val": hashHex,
			"_meta": map[string]any{
				"algorithm":     "sha1",
				"input_length":  len(inputBytes),
				"hash_length":   len(hashHex),
			},
		}
	})
}

