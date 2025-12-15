package tempdir

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterTempDir registers the tempdir function with gojq
func RegisterTempDir() gojq.CompilerOption {
	return gojq.WithFunction("tempdir", 0, 2, func(v any, args []any) any {
		var prefix string
		var dir string

		// Parse arguments: optional prefix and optional dir
		if len(args) > 0 {
			// First argument: prefix
			if prefixVal, ok := args[0].(string); ok {
				prefix = prefixVal
			} else {
				prefixVal := common.ExtractUDFValue(args[0])
				if prefixStr, ok := prefixVal.(string); ok {
					prefix = prefixStr
				} else {
					return common.MakeUDFErrorResult(fmt.Errorf("tempdir: first argument (prefix) must be a string, got %T", args[0]), nil)
				}
			}
		}

		if len(args) > 1 {
			// Second argument: dir
			if dirVal, ok := args[1].(string); ok {
				dir = dirVal
			} else {
				dirVal := common.ExtractUDFValue(args[1])
				if dirStr, ok := dirVal.(string); ok {
					dir = dirStr
				} else {
					return common.MakeUDFErrorResult(fmt.Errorf("tempdir: second argument (dir) must be a string, got %T", args[1]), nil)
				}
			}

			// Expand ~ to home directory and get absolute path for dir
			absDir, err := filepath.Abs(dir)
			if err != nil {
				// Try to expand ~ first
				if dir == "~" {
					home, homeErr := os.UserHomeDir()
					if homeErr != nil {
						return common.MakeUDFErrorResult(fmt.Errorf("tempdir: cannot determine home directory: %v", homeErr), nil)
					}
					absDir = home
				} else if len(dir) > 0 && dir[0] == '~' && (len(dir) == 1 || dir[1] == '/') {
					home, homeErr := os.UserHomeDir()
					if homeErr != nil {
						return common.MakeUDFErrorResult(fmt.Errorf("tempdir: cannot determine home directory: %v", homeErr), nil)
					}
					if len(dir) > 1 {
						absDir = filepath.Join(home, dir[2:])
					} else {
						absDir = home
					}
					absDir, err = filepath.Abs(absDir)
					if err != nil {
						return common.MakeUDFErrorResult(fmt.Errorf("tempdir: cannot resolve directory path %q: %v", dir, err), nil)
					}
				} else {
					return common.MakeUDFErrorResult(fmt.Errorf("tempdir: cannot resolve directory path %q: %v", dir, err), nil)
				}
			}
			dir = absDir

			// Verify the directory exists
			dirInfo, err := os.Stat(dir)
			if err != nil {
				if os.IsNotExist(err) {
					return common.MakeUDFErrorResult(fmt.Errorf("tempdir: directory does not exist: %q", dir), nil)
				}
				if os.IsPermission(err) {
					return common.MakeUDFErrorResult(fmt.Errorf("tempdir: permission denied accessing directory: %q", dir), nil)
				}
				return common.MakeUDFErrorResult(fmt.Errorf("tempdir: failed to access directory %q: %v", dir, err), nil)
			}
			if !dirInfo.IsDir() {
				return common.MakeUDFErrorResult(fmt.Errorf("tempdir: %q is not a directory", dir), nil)
			}
		}

		// Create temporary directory
		tempDir, err := os.MkdirTemp(dir, prefix)
		if err != nil {
			meta := map[string]any{
				"operation": "tempdir",
			}
			if dir != "" {
				meta["dir"] = dir
			}
			if prefix != "" {
				meta["prefix"] = prefix
			}
			return common.MakeUDFErrorResult(fmt.Errorf("tempdir: failed to create temporary directory: %v", err), meta)
		}

		// Get absolute path
		absTempDir, err := filepath.Abs(tempDir)
		if err != nil {
			// If we can't get absolute path, use the path returned by MkdirTemp
			absTempDir = tempDir
		}

		meta := map[string]any{
			"operation": "tempdir",
			"path":      absTempDir,
		}
		if prefix != "" {
			meta["prefix"] = prefix
		}
		if dir != "" {
			meta["dir"] = dir
		}

		return common.MakeUDFSuccessResult(absTempDir, meta)
	})
}

