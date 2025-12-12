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
	return gojq.WithFunction("sha512_224", 0, 1, func(v any, args []any) any {
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
				return fmt.Errorf("sha512_224: failed to read input: %v", err)
			}
			inputBytes = readBytes
		default:
			// Try to convert to string
			if str, ok := val.(fmt.Stringer); ok {
				inputBytes = []byte(str.String())
			} else {
				return fmt.Errorf("sha512_224: argument must be a string or bytes, got %T", val)
			}
		}

		// Compute SHA512/224 hash
		hash := sha512.Sum512_224(inputBytes)
		hashHex := fmt.Sprintf("%x", hash)

		// Return object with _val and _meta
		return map[string]any{
			"_val": hashHex,
			"_meta": map[string]any{
				"algorithm":     "sha512_224",
				"input_length":  len(inputBytes),
				"hash_length":   len(hashHex),
			},
		}
	})
}

