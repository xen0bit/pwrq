// Package location provides PowerShell-style location management cmdlets.
// This file implements Push-Location functionality.
package location

import (
	"fmt"
	"os"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

const defaultStackName = "default"

// PushLocationOptions holds options for the push_location function
type PushLocationOptions struct {
	Path      string // Path to push (defaults to current location)
	StackName string // Location stack name
	PassThru  bool   // Return the PathInfo object after pushing
}

// RegisterPushLocation registers the push_location function with gojq
// Supports PowerShell-style parameters: -Path, -StackName, -PassThru
// Aliases: pushd
// Usage:
//   - push_location() - push current location onto stack
//   - push_location("/path") - push current location and change to path
//   - push_location("/path"; {"StackName": "myStack"}) - use named stack
func RegisterPushLocation() gojq.CompilerOption {
	return gojq.WithFunction("push_location", 0, 2, func(v any, args []any) any {
		var opts PushLocationOptions

		// Parse arguments
		if len(args) > 0 {
			firstArg := common.BindValue(args[0])
			
			if pathStr, isString := firstArg.(string); isString {
				opts.Path = pathStr
				
				// Second argument could be options
				if len(args) > 1 {
					if optsMap, ok := args[1].(map[string]any); ok {
						parsePushLocationOptions(&opts, optsMap)
					}
				}
			} else if optsMap, ok := firstArg.(map[string]any); ok {
				// First arg is options map
				parsePushLocationOptions(&opts, optsMap)
			}
		}

		// Get session state
		ss := common.GetSessionState()
		if ss == nil {
			return common.MakeUDFErrorResult(fmt.Errorf("push_location: session state not available"), nil)
		}

		// Get current working directory before changing
		cwd, err := os.Getwd()
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("push_location: failed to get current directory: %v", err), nil)
		}

		// Determine stack name
		stackName := defaultStackName
		if opts.StackName != "" {
			stackName = opts.StackName
		}

		// Push current location onto stack
		ss.PushLocationStack(stackName, cwd)

		// If path was provided, change to it
		targetPath := cwd
		if opts.Path != "" {
			err := os.Chdir(opts.Path)
			if err != nil {
				// Pop the location we just pushed since we failed
				ss.PopLocationStack(stackName)
				return common.MakeUDFErrorResult(fmt.Errorf("push_location: failed to change to path %q: %v", opts.Path, err), nil)
			}
			
			// Get the new current directory
			targetPath, err = os.Getwd()
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("push_location: failed to get new directory: %v", err), nil)
			}
		}

		// Update $? automatic variable
		ss.SetVariable("?", true, 0)

		// Build result object
		result := map[string]any{
			"Path":     targetPath,
			"Provider": "FileSystem",
			"StackName": stackName,
		}
		psobj := psobject.NewPSObjectWithTypeName(result, "System.Management.Automation.PathInfo")

		if opts.PassThru {
			return common.MakeUDFSuccessResult(psobj.ToMap(), map[string]any{
				"operation": "push_location",
				"path":      targetPath,
				"stack":     stackName,
			})
		}

		return common.MakeUDFSuccessResult(nil, map[string]any{
			"operation": "push_location",
			"path":      targetPath,
			"stack":     stackName,
		})
	})
}

// parsePushLocationOptions parses options from a map
func parsePushLocationOptions(opts *PushLocationOptions, optsMap map[string]any) {
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

// pushLocation is the internal implementation for testing
func pushLocation(path string, opts PushLocationOptions, sessionState *sessionstate.SessionState) (string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("failed to get current directory: %v", err)
	}

	stackName := defaultStackName
	if opts.StackName != "" {
		stackName = opts.StackName
	}

	// Push current location onto stack
	if sessionState == nil {
		return "", "", fmt.Errorf("session state not available for stack operation")
	}
	sessionState.PushLocationStack(stackName, cwd)

	targetPath := cwd
	if path != "" {
		err := os.Chdir(path)
		if err != nil {
			// Rollback - pop the location we just pushed
			sessionState.PopLocationStack(stackName)
			return "", "", fmt.Errorf("failed to change directory: %v", err)
		}
		targetPath, err = os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("failed to get new directory: %v", err)
		}
	}

	return cwd, targetPath, nil
}

// GetLocationStack returns a copy of the location stack from session state
func GetLocationStack(ss *sessionstate.SessionState, stackName string) []string {
	if ss == nil {
		return []string{}
	}
	if stackName == "" {
		stackName = defaultStackName
	}
	return ss.GetLocationStack(stackName)
}
