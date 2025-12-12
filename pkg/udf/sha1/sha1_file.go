package sha1

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterSHA1File registers the sha1_file function with gojq
func RegisterSHA1File() gojq.CompilerOption {
	return gojq.WithFunction("sha1_file", 0, 1, func(v any, args []any) any {
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

		// Convert input to file path string
		var filePath string
		switch val := inputVal.(type) {
		case string:
			filePath = val
		default:
			return fmt.Errorf("sha1_file: argument must be a string (file path), got %T", val)
		}

		// Expand ~ to home directory
		if filePath == "~" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("sha1_file: cannot determine home directory: %v", err)
			}
			filePath = home
		} else if len(filePath) > 0 && filePath[0] == '~' && (len(filePath) == 1 || filePath[1] == '/') {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("sha1_file: cannot determine home directory: %v", err)
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
			return fmt.Errorf("sha1_file: cannot resolve path %q: %v", filePath, err)
		}

		// Read file contents
		fileData, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("sha1_file: file does not exist: %q", absPath)
			}
			if os.IsPermission(err) {
				return fmt.Errorf("sha1_file: permission denied reading file: %q", absPath)
			}
			return fmt.Errorf("sha1_file: failed to read file %q: %v", absPath, err)
		}

		// Compute SHA1 hash
		hash := sha1.Sum(fileData)
		hashHex := fmt.Sprintf("%x", hash)

		// Get file info for metadata
		fileInfo, err := os.Stat(absPath)
		var fileSize int64
		if err == nil {
			fileSize = fileInfo.Size()
		}

		// Return object with _val and _meta
		return map[string]any{
			"_val": hashHex,
			"_meta": map[string]any{
				"algorithm":    "sha1",
				"file_path":    absPath,
				"file_size":    int(fileSize),
				"hash_length":  len(hashHex),
			},
		}
	})
}

