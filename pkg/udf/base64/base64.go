package base64

import (
	"encoding/base64"
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterBase64Encode registers the base64_encode function with gojq
func RegisterBase64Encode() gojq.CompilerOption {
	return gojq.WithFunction("base64_encode", 0, 1, func(v any, args []any) any {
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

		// Convert input to string
		var input string
		switch val := inputVal.(type) {
		case string:
			input = val
		case []byte:
			input = string(val)
		default:
			// Try to convert to string
			if str, ok := val.(fmt.Stringer); ok {
				input = str.String()
			} else {
				return fmt.Errorf("base64_encode: argument must be a string, got %T", val)
			}
		}

		// Encode to base64
		encoded := base64.StdEncoding.EncodeToString([]byte(input))

		// Return object with _val and _meta
		return map[string]any{
			"_val": encoded,
			"_meta": map[string]any{
				"encoding":        "base64",
				"original_length": len(input),
				"encoded_length":  len(encoded),
			},
		}
	})
}

// RegisterBase64Decode registers the base64_decode function with gojq
func RegisterBase64Decode() gojq.CompilerOption {
	return gojq.WithFunction("base64_decode", 0, 1, func(v any, args []any) any {
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

		// Convert input to string
		var input string
		switch val := inputVal.(type) {
		case string:
			input = val
		case []byte:
			input = string(val)
		default:
			// Try to convert to string
			if str, ok := val.(fmt.Stringer); ok {
				input = str.String()
			} else {
				return fmt.Errorf("base64_decode: argument must be a string, got %T", val)
			}
		}

		// Decode from base64
		decoded, err := base64.StdEncoding.DecodeString(input)
		if err != nil {
			return fmt.Errorf("base64_decode: invalid base64 string: %v", err)
		}

		// Return object with _val and _meta
		return map[string]any{
			"_val": string(decoded),
			"_meta": map[string]any{
				"encoding":        "base64",
				"original_length": len(input),
				"decoded_length":  len(decoded),
			},
		}
	})
}
