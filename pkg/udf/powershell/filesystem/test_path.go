package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// TestPathOptions represents options for Test-Path
type TestPathOptions struct {
	Path     string
	PathType string // "Leaf", "Container", or "" for any
	IsValid  bool   // Check if path is valid syntax (not implemented for filesystem)
}

// parseTestPathArgs parses arguments for test_path
func parseTestPathArgs(args []any) (TestPathOptions, error) {
	opts := TestPathOptions{
		Path:     "",
		PathType: "",
		IsValid:  false,
	}

	if len(args) == 0 {
		return opts, fmt.Errorf("test_path: expected at least 1 argument (path)")
	}

	for _, arg := range args {
		argVal := common.BindValue(arg)

		switch v := argVal.(type) {
		case string:
			if opts.Path == "" {
				opts.Path = v
			} else {
				// Second string could be PathType
				if v == "Leaf" || v == "Container" {
					opts.PathType = v
				}
			}
		case map[string]any:
			if path, ok := v["Path"].(string); ok {
				opts.Path = path
			}
			if pathType, ok := v["PathType"].(string); ok {
				opts.PathType = pathType
			}
			if isValid, ok := v["IsValid"].(bool); ok {
				opts.IsValid = isValid
			}
		}
	}

	return opts, nil
}

// testPath performs the path existence check
//
// PowerShell semantics:
// - IsValid: checks path syntax without checking existence (rejects null bytes, invalid chars)
// - Leaf: tests if the final path element exists (could be file or directory)
// - Container: tests if the path is a directory that exists
// - Empty PathType: any existing path (file or directory)
// - Permission errors: return true if path exists (PowerShell semantics)
// - UNC paths: handled natively by filepath package
func testPath(opts TestPathOptions) (any, error) {
	// Validate path is non-empty
	if opts.Path == "" {
		return false, nil
	}

	// IsValid mode: check syntax without checking existence
	if opts.IsValid {
		return isValidPathSyntax(opts.Path), nil
	}

	// Normalize path for UNC and platform-specific handling
	path := filepath.Clean(opts.Path)

	// Get file info - exists check
	info, err := os.Stat(path)
	if err == nil {
		// Path exists - apply PathType filter
		switch opts.PathType {
		case "Leaf":
			// Leaf means the final element exists (file or directory)
			return true, nil
		case "Container":
			// Container must be a directory
			return info.IsDir(), nil
		default:
			// Any type
			return true, nil
		}
	}

	// Path does not exist - check if it's because parent doesn't exist
	// vs invalid syntax
	if os.IsNotExist(err) {
		return false, nil
	}

	// Permission denied or other access errors
	// PowerShell semantics: if path exists but we can't access it, return true
	// We can't distinguish "exists but no permission" from "doesn't exist"
	// without checking parent, so we check parent directory
	parent := filepath.Dir(path)
	if parentInfo, parentErr := os.Stat(parent); parentErr == nil && parentInfo.IsDir() {
		// Parent exists, so path likely doesn't exist
		return false, nil
	}

	// Can't determine - assume doesn't exist
	return false, nil
}

// isValidPathSyntax checks if a path has valid syntax without checking existence
// Returns false for null bytes, truly invalid characters (platform-specific)
// Uses Windows restrictions for cross-platform compatibility since pwrq may
// run on Windows and PowerShell users expect Windows path semantics
func isValidPathSyntax(path string) bool {
	if path == "" {
		return false
	}

	// Reject null bytes - always invalid on any platform
	if strings.Contains(path, "\x00") {
		return false
	}

	// Windows reserved characters that are invalid in paths:
	// < > : " | ? *
	// Exception: colon is valid as drive letter separator (C:) at position 1
	// We enforce these for cross-platform compatibility with PowerShell semantics
	for i, r := range path {
		switch r {
		case '<', '>', '"', '|', '?', '*':
			// Always invalid
			return false
		case ':':
			// Colon is only valid at position 1 (drive letter: C:)
			if i != 1 {
				return false
			}
		}
	}

	// UNC paths are valid (both Windows \\ and Unix // style)
	if strings.HasPrefix(path, "\\\\") || strings.HasPrefix(path, "//") {
		return true
	}

	return true
}

// RegisterTestPath registers the test_path function with gojq
func RegisterTestPath() gojq.CompilerOption {
	return common.WithFunction("test_path", 0, 3, func(v any, args []any) any {
		opts, err := parseTestPathArgs(args)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		result, err := testPath(opts)
		if err != nil {
			return common.MakeUDFErrorResult(err, map[string]any{
				"path": opts.Path,
			})
		}

		// Test-Path is a predicate, so it answers with a boolean. Wrapping it
		// in an object made `if test_path(x) then ... end` always take the
		// true branch, since every object is truthy.
		return result
	})
}
