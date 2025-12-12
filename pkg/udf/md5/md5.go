package md5

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterMD5 registers the md5 function with gojq
func RegisterMD5() gojq.CompilerOption {
	return gojq.WithFunction("md5", 0, 2, func(v any, args []any) any {
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

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			// Input is a file path
			switch val := inputVal.(type) {
			case string:
				filePath = val
			default:
				return fmt.Errorf("md5: file argument requires string path, got %T", val)
			}

			// Expand ~ to home directory
			if filePath == "~" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("md5: cannot determine home directory: %v", err)
				}
				filePath = home
			} else if len(filePath) > 0 && filePath[0] == '~' && (len(filePath) == 1 || filePath[1] == '/') {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("md5: cannot determine home directory: %v", err)
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
				return fmt.Errorf("md5: cannot resolve path %q: %v", filePath, err)
			}

			// Read file contents
			fileData, err := os.ReadFile(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("md5: file does not exist: %q", absPath)
				}
				if os.IsPermission(err) {
					return fmt.Errorf("md5: permission denied reading file: %q", absPath)
				}
				return fmt.Errorf("md5: failed to read file %q: %v", absPath, err)
			}

			inputBytes = fileData
			filePath = absPath

			// Get file info for metadata
			fileInfo, err := os.Stat(absPath)
			if err == nil {
				fileSize = fileInfo.Size()
			}
		} else {
			// Input is data to hash
			switch val := inputVal.(type) {
			case string:
				inputBytes = []byte(val)
			case []byte:
				inputBytes = val
			case io.Reader:
				// If it's a reader, read all data
				readBytes, err := io.ReadAll(val)
				if err != nil {
					return fmt.Errorf("md5: failed to read input: %v", err)
				}
				inputBytes = readBytes
			default:
				// Try to convert to string
				if str, ok := val.(fmt.Stringer); ok {
					inputBytes = []byte(str.String())
				} else {
					return fmt.Errorf("md5: argument must be a string or bytes, got %T", val)
				}
			}
		}

		// Compute MD5 hash
		hash := md5.Sum(inputBytes)
		hashHex := fmt.Sprintf("%x", hash)

		// Build metadata
		meta := map[string]any{
			"algorithm":    "md5",
			"hash_length":  len(hashHex),
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		} else {
			meta["input_length"] = len(inputBytes)
		}

		// Return object with _val and _meta
		return map[string]any{
			"_val":  hashHex,
			"_meta": meta,
		}
	})
}
