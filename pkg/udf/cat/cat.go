package cat

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterCat registers the cat function with gojq
func RegisterCat() gojq.CompilerOption {
	return gojq.WithFunction("cat", 0, 1, func(v any, args []any) any {
		var filePath string

		// Parse arguments: file path can come from pipe or as argument
		if len(args) > 0 {
			// File path provided as argument
			if path, ok := args[0].(string); ok {
				filePath = path
			} else {
				// Try to extract from UDF result
				pathVal := common.ExtractUDFValue(args[0])
				if pathStr, ok := pathVal.(string); ok {
					filePath = pathStr
				} else {
					return common.MakeUDFErrorResult(fmt.Errorf("cat: argument must be a string file path, got %T", args[0]), nil)
				}
			}
		} else {
			// File path from pipe
			inputVal := common.ExtractUDFValue(v)
			if pathStr, ok := inputVal.(string); ok {
				filePath = pathStr
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("cat: input must be a string file path, got %T", inputVal), nil)
			}
		}

		// Expand ~ to home directory and get absolute path
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			// Try to expand ~ first
			if filePath == "~" {
				home, homeErr := os.UserHomeDir()
				if homeErr != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot determine home directory: %v", homeErr), nil)
				}
				absPath = home
			} else if len(filePath) > 0 && filePath[0] == '~' && (len(filePath) == 1 || filePath[1] == '/') {
				home, homeErr := os.UserHomeDir()
				if homeErr != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot determine home directory: %v", homeErr), nil)
				}
				if len(filePath) > 1 {
					absPath = filepath.Join(home, filePath[2:])
				} else {
					absPath = home
				}
				absPath, err = filepath.Abs(absPath)
				if err != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot resolve path %q: %v", filePath, err), nil)
				}
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot resolve path %q: %v", filePath, err), nil)
			}
		}
		filePath = absPath

		// Read file contents
		fileData, err := os.ReadFile(filePath)
		if err != nil {
			meta := map[string]any{
				"operation": "cat",
				"file_path": filePath,
			}
			if os.IsNotExist(err) {
				return common.MakeUDFErrorResult(fmt.Errorf("cat: file does not exist: %q", filePath), meta)
			}
			if os.IsPermission(err) {
				return common.MakeUDFErrorResult(fmt.Errorf("cat: permission denied reading file: %q", filePath), meta)
			}
			return common.MakeUDFErrorResult(fmt.Errorf("cat: failed to read file %q: %v", filePath, err), meta)
		}

		// Get file info for metadata
		fileInfo, err := os.Stat(filePath)
		var fileSize int64
		var isDir bool
		if err == nil {
			fileSize = fileInfo.Size()
			isDir = fileInfo.IsDir()
		}

		// Return error if it's a directory
		if isDir {
			meta := map[string]any{
				"operation": "cat",
				"file_path": filePath,
				"file_size": int(fileSize),
			}
			return common.MakeUDFErrorResult(fmt.Errorf("cat: %q is a directory, not a file", filePath), meta)
		}

		// Return file contents as string
		content := string(fileData)

		meta := map[string]any{
			"operation": "cat",
			"file_path": filePath,
			"file_size": int(fileSize),
		}

		return common.MakeUDFSuccessResult(content, meta)
	})
}
