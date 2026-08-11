// Package location provides PowerShell-style location management cmdlets.
// This file implements Pop-Location functionality.
package location

import (
	"fmt"
	"os"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// PopLocationOptions holds options for the pop_location function
type PopLocationOptions struct {
	StackName string // Location stack name
	PassThru  bool   // Return the PathInfo object after popping
}

// RegisterPopLocation registers the pop_location function with gojq
// Supports PowerShell-style parameters: -StackName, -PassThru
// Aliases: popd
// Usage:
//   - pop_location() - pop from default stack and change to that location
//   - pop_location({"StackName": "myStack"}) - pop from named stack
func RegisterPopLocation() gojq.CompilerOption {
	return gojq.WithFunction("pop_location", 0, 1, func(v any, args []any) any {
		var opts PopLocationOptions

		// Parse options if provided
		if len(args) > 0 {
			if optsMap, ok := args[0].(map[string]any); ok {
				parsePopLocationOptions(&opts, optsMap)
			}
		}

		// Determine stack name
		stackName := defaultStackName
		if opts.StackName != "" {
			stackName = opts.StackName
		}

		// Get session state
		ss := common.GetSessionState()
		if ss == nil {
			return common.MakeUDFErrorResult(fmt.Errorf("pop_location: session state not available"), nil)
		}

		// Pop location from stack
		poppedLocation, err := ss.PopLocationStack(stackName)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("pop_location: %v", err), nil)
		}

		// Check if the popped directory still exists
		if _, statErr := os.Stat(poppedLocation); os.IsNotExist(statErr) {
			return common.MakeUDFErrorResult(fmt.Errorf("pop_location: directory %q no longer exists", poppedLocation), nil)
		}

		// Change to the popped location
		err = os.Chdir(poppedLocation)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("pop_location: failed to change to directory %q: %v", poppedLocation, err), nil)
		}

		// Get the new current directory
		cwd, err := os.Getwd()
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("pop_location: failed to get new directory: %v", err), nil)
		}

		// Update $? automatic variable
		_ = ss.SetVariable("?", true, 0)

		// Build result object
		result := map[string]any{
			"Path":      cwd,
			"Provider":  "FileSystem",
			"StackName": stackName,
		}
		psobj := psobject.NewPSObjectWithTypeName(result, "System.Management.Automation.PathInfo")

		if opts.PassThru {
			return common.MakeUDFSuccessResult(psobj.ToMap(), map[string]any{
				"operation": "pop_location",
				"path":      cwd,
				"stack":     stackName,
			})
		}

		return common.MakeUDFSuccessResult(nil, map[string]any{
			"operation": "pop_location",
			"path":      cwd,
			"stack":     stackName,
		})
	})
}

// parsePopLocationOptions parses options from a map
func parsePopLocationOptions(opts *PopLocationOptions, optsMap map[string]any) {
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

// popLocation is the internal implementation for testing
func popLocation(opts PopLocationOptions, sessionState *sessionstate.SessionState) (string, error) {
	stackName := defaultStackName
	if opts.StackName != "" {
		stackName = opts.StackName
	}

	if sessionState == nil {
		return "", fmt.Errorf("session state not available for stack operation")
	}

	// Pop from session-based stack
	poppedLocation, err := sessionState.PopLocationStack(stackName)
	if err != nil {
		return "", err
	}

	// Check if the popped directory still exists
	if _, err := os.Stat(poppedLocation); os.IsNotExist(err) {
		return "", fmt.Errorf("popped directory %q no longer exists", poppedLocation)
	}

	err = os.Chdir(poppedLocation)
	if err != nil {
		return "", fmt.Errorf("failed to change directory: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get new directory: %v", err)
	}

	return cwd, nil
}
