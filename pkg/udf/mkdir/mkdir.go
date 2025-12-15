package mkdir

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterMkdir registers the mkdir function with gojq
func RegisterMkdir() gojq.CompilerOption {
	return gojq.WithFunction("mkdir", 1, 1, func(v any, args []any) any {
		var dirPath string

		// Parse required argument: directory path
		if len(args) == 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("mkdir: path argument is required"), nil)
		}

		// Extract path from argument
		if path, ok := args[0].(string); ok {
			dirPath = path
		} else {
			// Try to extract from UDF result
			pathVal := common.ExtractUDFValue(args[0])
			if pathStr, ok := pathVal.(string); ok {
				dirPath = pathStr
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("mkdir: argument must be a string path, got %T", args[0]), nil)
			}
		}

		// Expand ~ to home directory and get absolute path
		absPath, err := filepath.Abs(dirPath)
		if err != nil {
			// Try to expand ~ first
			if dirPath == "~" {
				home, homeErr := os.UserHomeDir()
				if homeErr != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("mkdir: cannot determine home directory: %v", homeErr), nil)
				}
				absPath = home
			} else if len(dirPath) > 0 && dirPath[0] == '~' && (len(dirPath) == 1 || dirPath[1] == '/') {
				home, homeErr := os.UserHomeDir()
				if homeErr != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("mkdir: cannot determine home directory: %v", homeErr), nil)
				}
				if len(dirPath) > 1 {
					absPath = filepath.Join(home, dirPath[2:])
				} else {
					absPath = home
				}
				absPath, err = filepath.Abs(absPath)
				if err != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("mkdir: cannot resolve path %q: %v", dirPath, err), nil)
				}
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("mkdir: cannot resolve path %q: %v", dirPath, err), nil)
			}
		}
		dirPath = absPath

		// Check if path already exists
		existingInfo, err := os.Stat(dirPath)
		if err == nil {
			// Path exists
			if existingInfo.IsDir() {
				// Directory already exists - return success (like mkdir -p)
				meta := map[string]any{
					"operation": "mkdir",
					"path":      dirPath,
					"existed":   true,
				}
				return common.MakeUDFSuccessResult(dirPath, meta)
			} else {
				// Path exists but is not a directory
				meta := map[string]any{
					"operation": "mkdir",
					"path":      dirPath,
				}
				return common.MakeUDFErrorResult(fmt.Errorf("mkdir: path %q already exists and is not a directory", dirPath), meta)
			}
		}

		// Path doesn't exist - create it (with parent directories)
		err = os.MkdirAll(dirPath, 0755)
		if err != nil {
			meta := map[string]any{
				"operation": "mkdir",
				"path":      dirPath,
			}
			if os.IsPermission(err) {
				return common.MakeUDFErrorResult(fmt.Errorf("mkdir: permission denied creating directory: %q", dirPath), meta)
			}
			return common.MakeUDFErrorResult(fmt.Errorf("mkdir: failed to create directory %q: %v", dirPath, err), meta)
		}

		// Verify the directory was created
		info, err := os.Stat(dirPath)
		if err != nil {
			meta := map[string]any{
				"operation": "mkdir",
				"path":      dirPath,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("mkdir: directory was created but cannot be accessed: %v", err), meta)
		}
		if !info.IsDir() {
			meta := map[string]any{
				"operation": "mkdir",
				"path":      dirPath,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("mkdir: created path %q is not a directory", dirPath), meta)
		}

		meta := map[string]any{
			"operation": "mkdir",
			"path":      dirPath,
			"created":   true,
		}

		return common.MakeUDFSuccessResult(dirPath, meta)
	})
}

