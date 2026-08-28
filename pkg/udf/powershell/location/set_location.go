// Package location provides PowerShell-style location management cmdlets.
// This file implements Set-Location functionality.
package location

import (
	"fmt"
	"os"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// SetLocationOptions holds options for the set_location function
type SetLocationOptions struct {
	Path      string // Path to set as current location
	StackName string // Location stack name (for push/pop)
	PassThru  bool   // Return the PathInfo object after setting
}

// RegisterSetLocation registers the set_location function with gojq
// Supports PowerShell-style parameters: -Path, -StackName, -PassThru
// Aliases: cd, sl
// Usage:
//   - set_location("/path/to/dir") - change directory
//   - set_location("/path"; {"PassThru": true}) - change and return PathInfo
func RegisterSetLocation() gojq.CompilerOption {
	return common.WithFunction("set_location", 0, 2, func(v any, args []any) any {
		var path string
		opts := SetLocationOptions{}

		// Parse arguments
		if len(args) == 0 {
			// No args - return current location (like `cd` without args in some shells)
			cwd, err := os.Getwd()
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("set_location: failed to get current directory: %v", err), nil)
			}
			result := map[string]any{
				"Path":     cwd,
				"Provider": "FileSystem",
			}
			psobj := psobject.NewPSObject(result)
			return common.MakeUDFSuccessResult(CurrentLocationShape.Build(psobj.ToMap()), map[string]any{
				"operation": "get_location",
			})
		}

		// First argument is path or options
		firstArg := common.BindValue(args[0])

		if pathStr, isString := firstArg.(string); isString {
			path = pathStr

			// Second argument could be options
			if len(args) > 1 {
				if optsMap, ok := args[1].(map[string]any); ok {
					parseSetLocationOptions(&opts, optsMap)
				}
			}
		} else if optsMap, ok := firstArg.(map[string]any); ok {
			// First arg is options map
			parseSetLocationOptions(&opts, optsMap)
			path = opts.Path

			// If path not in options, check second arg
			if path == "" && len(args) > 1 {
				if pStr, ok := args[1].(string); ok {
					path = pStr
				}
			}
		}

		// Validate path
		if path == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("set_location: path is required"), nil)
		}

		// Get session state
		ss := common.GetSessionState()

		// If StackName is specified, push current location onto that stack first
		if opts.StackName != "" && ss != nil {
			cwdBefore, err := os.Getwd()
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("set_location: failed to get current directory: %v", err), nil)
			}
			ss.PushLocationStack(opts.StackName, cwdBefore)
		}

		// Change directory
		err := os.Chdir(path)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("set_location: failed to change directory: %v", err), nil)
		}

		// Update $? automatic variable (last command success)
		if ss != nil {
			_ = ss.SetVariable("?", true, 0)
			_ = ss.SetVariable("PWD", path, 0)
		}

		// Get the new current directory
		cwd, err := os.Getwd()
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("set_location: failed to get new directory: %v", err), nil)
		}

		// Build result object
		result := map[string]any{
			"Path":     cwd,
			"Provider": "FileSystem",
		}
		psobj := psobject.NewPSObject(result)

		if opts.PassThru {
			return common.MakeUDFSuccessResult(CurrentLocationShape.Build(psobj.ToMap()), map[string]any{
				"operation": "set_location",
				"path":      cwd,
			})
		}

		return common.MakeUDFSuccessResult(nil, map[string]any{
			"operation": "set_location",
			"path":      cwd,
		})
	})
}

// parseSetLocationOptions parses options from a map
func parseSetLocationOptions(opts *SetLocationOptions, optsMap map[string]any) {
	if pathVal, exists := optsMap["Path"]; exists {
		if pathStr, ok := pathVal.(string); ok {
			opts.Path = pathStr
		}
	}
	if stackVal, exists := optsMap["StackName"]; exists {
		if stackStr, ok := stackVal.(string); ok {
			opts.StackName = stackStr
		}
	}
	if passThruVal, exists := optsMap["PassThru"]; exists {
		if b, ok := passThruVal.(bool); ok {
			opts.PassThru = b
		}
	}
}

// setLocation is the internal implementation for testing
func setLocation(path string, opts SetLocationOptions, sessionState *sessionstate.SessionState) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	// If StackName is specified, push current location onto that stack first
	if opts.StackName != "" {
		if sessionState == nil {
			return "", fmt.Errorf("session state not available for stack operation")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %v", err)
		}
		sessionState.PushLocationStack(opts.StackName, cwd)
	}

	err := os.Chdir(path)
	if err != nil {
		return "", fmt.Errorf("failed to change directory: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get new directory: %v", err)
	}

	// Update session state pwd variable if available
	if sessionState != nil {
		_ = sessionState.SetVariable("PWD", cwd, 0)
	}

	return cwd, nil
}
