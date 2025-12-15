package rm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterRm registers the rm function with gojq
func RegisterRm() gojq.CompilerOption {
	return gojq.WithFunction("rm", 2, 2, func(v any, args []any) any {
		var targetPath string
		var targetType string

		// Parse required arguments: path and type
		if len(args) < 2 {
			return common.MakeUDFErrorResult(fmt.Errorf("rm: requires 2 arguments (path, type), got %d", len(args)), nil)
		}

		// Extract path from first argument
		if path, ok := args[0].(string); ok {
			targetPath = path
		} else {
			// Try to extract from UDF result
			pathVal := common.ExtractUDFValue(args[0])
			if pathStr, ok := pathVal.(string); ok {
				targetPath = pathStr
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("rm: first argument (path) must be a string, got %T", args[0]), nil)
			}
		}

		// Extract type from second argument
		if typeVal, ok := args[1].(string); ok {
			targetType = strings.ToLower(typeVal)
		} else {
			// Try to extract from UDF result
			typeVal := common.ExtractUDFValue(args[1])
			if typeStr, ok := typeVal.(string); ok {
				targetType = strings.ToLower(typeStr)
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("rm: second argument (type) must be a string, got %T", args[1]), nil)
			}
		}

		// Validate type argument
		if targetType != "file" && targetType != "folder" {
			return common.MakeUDFErrorResult(fmt.Errorf("rm: type must be 'file' or 'folder', got %q", targetType), nil)
		}

		// Expand ~ to home directory and get absolute path
		absPath, err := filepath.Abs(targetPath)
		if err != nil {
			// Try to expand ~ first
			if targetPath == "~" {
				home, homeErr := os.UserHomeDir()
				if homeErr != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("rm: cannot determine home directory: %v", homeErr), nil)
				}
				absPath = home
			} else if len(targetPath) > 0 && targetPath[0] == '~' && (len(targetPath) == 1 || targetPath[1] == '/') {
				home, homeErr := os.UserHomeDir()
				if homeErr != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("rm: cannot determine home directory: %v", homeErr), nil)
				}
				if len(targetPath) > 1 {
					absPath = filepath.Join(home, targetPath[2:])
				} else {
					absPath = home
				}
				absPath, err = filepath.Abs(absPath)
				if err != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("rm: cannot resolve path %q: %v", targetPath, err), nil)
				}
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("rm: cannot resolve path %q: %v", targetPath, err), nil)
			}
		}
		targetPath = absPath

		// Check if path exists
		info, err := os.Stat(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				meta := map[string]any{
					"operation": "rm",
					"path":       targetPath,
					"type":       targetType,
				}
				return common.MakeUDFErrorResult(fmt.Errorf("rm: path does not exist: %q", targetPath), meta)
			}
			if os.IsPermission(err) {
				meta := map[string]any{
					"operation": "rm",
					"path":       targetPath,
					"type":       targetType,
				}
				return common.MakeUDFErrorResult(fmt.Errorf("rm: permission denied accessing path: %q", targetPath), meta)
			}
			meta := map[string]any{
				"operation": "rm",
				"path":      targetPath,
				"type":      targetType,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("rm: failed to access path %q: %v", targetPath, err), meta)
		}

		// Verify type matches
		isDir := info.IsDir()
		if targetType == "file" && isDir {
			meta := map[string]any{
				"operation": "rm",
				"path":      targetPath,
				"type":      targetType,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("rm: path %q is a directory, but type 'file' was specified", targetPath), meta)
		}
		if targetType == "folder" && !isDir {
			meta := map[string]any{
				"operation": "rm",
				"path":      targetPath,
				"type":      targetType,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("rm: path %q is a file, but type 'folder' was specified", targetPath), meta)
		}

		// Remove the file or folder
		if targetType == "file" {
			err = os.Remove(targetPath)
			if err != nil {
				meta := map[string]any{
					"operation": "rm",
					"path":       targetPath,
					"type":       targetType,
				}
				if os.IsPermission(err) {
					return common.MakeUDFErrorResult(fmt.Errorf("rm: permission denied removing file: %q", targetPath), meta)
				}
				return common.MakeUDFErrorResult(fmt.Errorf("rm: failed to remove file %q: %v", targetPath, err), meta)
			}
		} else { // folder
			err = os.RemoveAll(targetPath)
			if err != nil {
				meta := map[string]any{
					"operation": "rm",
					"path":       targetPath,
					"type":       targetType,
				}
				if os.IsPermission(err) {
					return common.MakeUDFErrorResult(fmt.Errorf("rm: permission denied removing folder: %q", targetPath), meta)
				}
				return common.MakeUDFErrorResult(fmt.Errorf("rm: failed to remove folder %q: %v", targetPath, err), meta)
			}
		}

		// Verify it was removed
		_, err = os.Stat(targetPath)
		if err == nil {
			meta := map[string]any{
				"operation": "rm",
				"path":       targetPath,
				"type":       targetType,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("rm: path %q still exists after removal", targetPath), meta)
		}
		if !os.IsNotExist(err) {
			meta := map[string]any{
				"operation": "rm",
				"path":       targetPath,
				"type":       targetType,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("rm: unexpected error checking removal: %v", err), meta)
		}

		meta := map[string]any{
			"operation": "rm",
			"path":       targetPath,
			"type":       targetType,
			"removed":    true,
		}

		return common.MakeUDFSuccessResult(targetPath, meta)
	})
}

