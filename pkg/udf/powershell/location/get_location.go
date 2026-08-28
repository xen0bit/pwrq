// Package location provides PowerShell-style location management cmdlets.
// This file implements Get-Location functionality.
package location

import (
	"fmt"
	"os"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// GetLocationOptions holds options for the get_location function
type GetLocationOptions struct {
	Drive     string // Get location from a specific drive
	StackName string // Get location from a specific location stack
}

// RegisterGetLocation registers the get_location function with gojq
// Options: Drive, StackName
// Usage:
//   - get_location() - get current location
//   - get_location({}) - get current location with options
//   - get_location({"Drive": "FileSystem"}) - get location from a specific drive
//   - get_location({"StackName": "myStack"}) - get top of named stack
func RegisterGetLocation() gojq.CompilerOption {
	return common.WithFunctionOf("get_location", 0, 1, CurrentLocationShape, func(v any, args []any) any {
		var opts GetLocationOptions

		// Parse options if provided
		if len(args) > 0 {
			if optsMap, ok := args[0].(map[string]any); ok {
				parseGetLocationOptions(&opts, optsMap)
			}
		}

		// Get session state
		ss := common.GetSessionState()

		// Get location (from stack or current directory)
		result, err := getLocation(opts, ss)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("get_location: %v", err), nil)
		}

		obj := typed.New(result)

		return common.MakeUDFSuccessResult(CurrentLocationShape.Build(obj.ToMap()), map[string]any{
			"operation": "get_location",
		})
	})
}

// parseGetLocationOptions parses options from a map
func parseGetLocationOptions(opts *GetLocationOptions, optsMap map[string]any) {
	if driveVal, exists := optsMap["Drive"]; exists {
		if driveStr, ok := driveVal.(string); ok {
			opts.Drive = driveStr
		}
	}
	if stackVal, exists := optsMap["StackName"]; exists {
		if stackStr, ok := stackVal.(string); ok {
			opts.StackName = stackStr
		}
	}
}

// getLocation is the internal implementation for testing
func getLocation(opts GetLocationOptions, sessionState *sessionstate.SessionState) (map[string]any, error) {
	var cwd string
	var err error

	// If StackName is specified, get the top of that stack without popping
	if opts.StackName != "" {
		if sessionState == nil {
			return nil, fmt.Errorf("session state not available for stack lookup")
		}
		stack := sessionState.GetLocationStack(opts.StackName)
		if len(stack) == 0 {
			return nil, fmt.Errorf("location stack %q is empty or does not exist", opts.StackName)
		}
		// Return the top of the stack (last item)
		cwd = stack[len(stack)-1]
	} else {
		// Get current working directory
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %v", err)
		}
	}

	result := map[string]any{
		"Path":     cwd,
		"Provider": "FileSystem",
	}

	if opts.Drive != "" {
		result["Drive"] = opts.Drive
	}

	if opts.StackName != "" {
		result["StackName"] = opts.StackName
	}

	return result, nil
}
