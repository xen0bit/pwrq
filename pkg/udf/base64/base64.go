package base64

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterBase64Encode registers the base64_encode function with gojq
func RegisterBase64Encode() gojq.CompilerOption {
	return gojq.WithFunction("base64_encode", 0, 2, func(v any, args []any) any {
		// Parse arguments: first is optional input, second is optional file flag
		var inputVal any
		var isFile bool

		if len(args) > 0 {
			// Check if first arg is boolean (file flag) or value
			if fileFlag, ok := args[0].(bool); ok {
				isFile = fileFlag
				inputVal = v
			} else {
				inputVal = args[0]
				// Check for file flag as second arg
				if len(args) > 1 {
					if fileFlag, ok := args[1].(bool); ok {
						isFile = fileFlag
					}
				}
			}
		} else {
			inputVal = v
		}

		// Automatically extract _val if input is a UDF result object
		// This is standard behavior for all UDFs
		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			// Input is a file path
			switch val := inputVal.(type) {
			case string:
				filePath = val
			default:
				return fmt.Errorf("base64_encode: file argument requires string path, got %T", val)
			}

			// Expand ~ to home directory
			if filePath == "~" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("base64_encode: cannot determine home directory: %v", err)
				}
				filePath = home
			} else if len(filePath) > 0 && filePath[0] == '~' && (len(filePath) == 1 || filePath[1] == '/') {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("base64_encode: cannot determine home directory: %v", err)
				}
				if len(filePath) > 1 {
					filePath = filepath.Join(home, filePath[2:])
				} else {
					filePath = home
				}
			}

			// Convert to absolute path
			absPath, err := filepath.Abs(filePath)
			if err != nil {
				return fmt.Errorf("base64_encode: cannot resolve path %q: %v", filePath, err)
			}

			// Read file contents
			fileData, err := os.ReadFile(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("base64_encode: file does not exist: %q", absPath)
				}
				if os.IsPermission(err) {
					return fmt.Errorf("base64_encode: permission denied reading file: %q", absPath)
				}
				return fmt.Errorf("base64_encode: failed to read file %q: %v", absPath, err)
			}

			input = string(fileData)
			filePath = absPath

			// Get file info for metadata
			fileInfo, err := os.Stat(absPath)
			if err == nil {
				fileSize = fileInfo.Size()
			}
		} else {
			// Input is string to encode
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
		}

		// Encode to base64
		encoded := base64.StdEncoding.EncodeToString([]byte(input))

		// Build metadata
		meta := map[string]any{
			"encoding": "base64",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			meta["encoded_length"] = len(encoded)
		} else {
			meta["original_length"] = len(input)
			meta["encoded_length"] = len(encoded)
		}

		// Return object with _val and _meta
		return map[string]any{
			"_val":  encoded,
			"_meta": meta,
		}
	})
}

// RegisterBase64Decode registers the base64_decode function with gojq
func RegisterBase64Decode() gojq.CompilerOption {
	return gojq.WithFunction("base64_decode", 0, 2, func(v any, args []any) any {
		// Parse arguments: first is optional input, second is optional file flag
		var inputVal any
		var isFile bool

		if len(args) > 0 {
			// Check if first arg is boolean (file flag) or value
			if fileFlag, ok := args[0].(bool); ok {
				isFile = fileFlag
				inputVal = v
			} else {
				inputVal = args[0]
				// Check for file flag as second arg
				if len(args) > 1 {
					if fileFlag, ok := args[1].(bool); ok {
						isFile = fileFlag
					}
				}
			}
		} else {
			inputVal = v
		}

		// Automatically extract _val if input is a UDF result object
		// This is standard behavior for all UDFs
		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			// Input is a file path
			switch val := inputVal.(type) {
			case string:
				filePath = val
			default:
				return fmt.Errorf("base64_decode: file argument requires string path, got %T", val)
			}

			// Expand ~ to home directory
			if filePath == "~" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("base64_decode: cannot determine home directory: %v", err)
				}
				filePath = home
			} else if len(filePath) > 0 && filePath[0] == '~' && (len(filePath) == 1 || filePath[1] == '/') {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("base64_decode: cannot determine home directory: %v", err)
				}
				if len(filePath) > 1 {
					filePath = filepath.Join(home, filePath[2:])
				} else {
					filePath = home
				}
			}

			// Convert to absolute path
			absPath, err := filepath.Abs(filePath)
			if err != nil {
				return fmt.Errorf("base64_decode: cannot resolve path %q: %v", filePath, err)
			}

			// Read file contents
			fileData, err := os.ReadFile(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("base64_decode: file does not exist: %q", absPath)
				}
				if os.IsPermission(err) {
					return fmt.Errorf("base64_decode: permission denied reading file: %q", absPath)
				}
				return fmt.Errorf("base64_decode: failed to read file %q: %v", absPath, err)
			}

			input = string(fileData)
			filePath = absPath

			// Get file info for metadata
			fileInfo, err := os.Stat(absPath)
			if err == nil {
				fileSize = fileInfo.Size()
			}
		} else {
			// Input is base64 string to decode
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
		}

		// Decode from base64
		decoded, err := base64.StdEncoding.DecodeString(input)
		if err != nil {
			return fmt.Errorf("base64_decode: invalid base64 string: %v", err)
		}

		// Build metadata
		meta := map[string]any{
			"encoding": "base64",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			meta["decoded_length"] = len(decoded)
		} else {
			meta["original_length"] = len(input)
			meta["decoded_length"] = len(decoded)
		}

		// Return object with _val and _meta
		return map[string]any{
			"_val":  string(decoded),
			"_meta": meta,
		}
	})
}
