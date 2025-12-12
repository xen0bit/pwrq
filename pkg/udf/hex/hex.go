package hex

import (
	"encoding/hex"
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterHexEncode registers the hex_encode function with gojq
func RegisterHexEncode() gojq.CompilerOption {
	return gojq.WithFunction("hex_encode", 0, 1, func(v any, args []any) any {
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
		default:
			// Try to convert to string
			if str, ok := val.(fmt.Stringer); ok {
				inputBytes = []byte(str.String())
			} else {
				return fmt.Errorf("hex_encode: argument must be a string or bytes, got %T", val)
			}
		}

		// Encode to hex
		encoded := hex.EncodeToString(inputBytes)

		// Return object with _val and _meta
		return map[string]any{
			"_val": encoded,
			"_meta": map[string]any{
				"encoding":        "hex",
				"original_length": len(inputBytes),
				"encoded_length":  len(encoded),
			},
		}
	})
}

// RegisterHexDecode registers the hex_decode function with gojq
func RegisterHexDecode() gojq.CompilerOption {
	return gojq.WithFunction("hex_decode", 0, 1, func(v any, args []any) any {
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
				return fmt.Errorf("hex_decode: argument must be a string, got %T", val)
			}
		}

		// Decode from hex
		decoded, err := hex.DecodeString(input)
		if err != nil {
			return fmt.Errorf("hex_decode: invalid hex string: %v", err)
		}

		// Return object with _val and _meta
		return map[string]any{
			"_val": string(decoded),
			"_meta": map[string]any{
				"encoding":        "hex",
				"original_length": len(input),
				"decoded_length":  len(decoded),
			},
		}
	})
}

