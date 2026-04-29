// Package variables provides PowerShell-style variable management cmdlets.
// This file implements Remove-Variable functionality.
package variables

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RemoveVariableOptions holds options for the remove_variable function
type RemoveVariableOptions struct {
	Name       string // Variable name (supports wildcards)
	Scope      string // Scope to search: global, script, local
	Force      bool   // Force removal (only bypasses "not found" errors)
	Exclude    string // Exclude pattern
}

// RegisterRemoveVariable registers the remove_variable function with gojq
// Supports PowerShell-style parameters: -Name, -Scope, -Force, -Exclude
// Usage:
//   - remove_variable("name") - remove a variable
//   - remove_variable("name"; {"Scope": "global"; "Force": true})
func RegisterRemoveVariable() gojq.CompilerOption {
	return gojq.WithFunction("remove_variable", 0, 2, func(v any, args []any) any {
		var name string
		opts := RemoveVariableOptions{
			Force: false,
			Scope: "",
		}

		// Parse arguments
		if len(args) == 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("remove_variable: requires a name argument"), nil)
		}

		// First argument is name or options
		firstArg := common.ExtractUDFValue(args[0])
		
		if nameStr, isString := firstArg.(string); isString {
			name = nameStr
		} else if optsMap, ok := firstArg.(map[string]any); ok {
			// First arg is options map
			parseRemoveVariableOptions(&opts, optsMap)
			if opts.Name != "" {
				name = opts.Name
			}
		}
		
		// Second argument could be options
		if len(args) > 1 {
			if optsMap, ok := args[1].(map[string]any); ok {
				parseRemoveVariableOptions(&opts, optsMap)
			}
		}

		// Validate name
		if name == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("remove_variable: variable name is required"), nil)
		}

		// Get session state
		ss := common.GetSessionState()
		if ss == nil {
			return common.MakeUDFErrorResult(fmt.Errorf("remove_variable: session state not initialized"), nil)
		}

		// Handle wildcard or specific variable
		var removedCount int
		var firstErr error

		if containsWildcard(name) {
			// Wildcard pattern - remove all matching variables
			removedCount, firstErr = removeVariablesByPattern(ss, name, opts)
		} else {
			// Single variable removal
			var varName string
			if opts.Scope != "" {
				scopePrefix := normalizeScope(opts.Scope)
				if scopePrefix != "" {
					varName = scopePrefix + ":" + name
				} else {
					varName = name
				}
			} else {
				varName = name
			}

			err := ss.RemoveVariable(varName)
			if err != nil {
				// Force mode only bypasses "not found" errors, not protection violations
				if opts.Force && strings.Contains(err.Error(), "not found") {
					// Ignore not found error in force mode
					err = nil
				}
				if err != nil {
					return common.MakeUDFErrorResult(err, nil)
				}
			}
			if err == nil {
				removedCount = 1
			}
		}

		if firstErr != nil {
			return common.MakeUDFErrorResult(firstErr, nil)
		}

		// Update $? automatic variable
		ss.SetVariable("?", true, sessionstate.None)

		return common.MakeUDFSuccessResult(nil, map[string]any{
			"operation": "remove_variable",
			"name":      name,
			"removed":   removedCount,
		})
	})
}

// removeVariablesByPattern removes all variables matching the pattern
func removeVariablesByPattern(ss *sessionstate.SessionState, pattern string, opts RemoveVariableOptions) (int, error) {
	// Use ss.GetVariables() which properly traverses the scope chain
	allVars := ss.GetVariables()
	
	var removedCount int
	var firstErr error
	
	for name := range allVars {
		// Apply Exclude filter (if specified)
		if opts.Exclude != "" {
			matched, err := filepath.Match(opts.Exclude, name)
			if err != nil || matched {
				continue // Skip if matches exclude pattern
			}
		}
		
		// Check if name matches the wildcard pattern
		matched, err := filepath.Match(pattern, name)
		if err != nil || !matched {
			continue
		}
		
		// If Scope is specified, filter by scope
		if opts.Scope != "" {
			entry := allVars[name]
			scopePrefix := normalizeScope(opts.Scope)
			if scopePrefix != "" && scopeTypeToString(entry.Scope) != strings.ToUpper(scopePrefix[:1])+scopePrefix[1:] {
				continue
			}
		}
		
		// Attempt to remove the variable
		err = ss.RemoveVariable(name)
		if err != nil {
			// Force mode only bypasses "not found" errors
			// ReadOnly and Constant protections are NOT bypassed by Force
			if opts.Force && strings.Contains(err.Error(), "not found") {
				continue // Skip not found errors in force mode
			}
			// Record first error but continue trying to remove other variables
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to remove %q: %w", name, err)
			}
			continue
		}
		removedCount++
	}
	
	return removedCount, firstErr
}

// parseRemoveVariableOptions parses options from a map
func parseRemoveVariableOptions(opts *RemoveVariableOptions, optsMap map[string]any) {
	if nameVal, exists := optsMap["Name"]; exists {
		if nStr, ok := nameVal.(string); ok {
			opts.Name = nStr
		}
	}
	if scopeVal, exists := optsMap["Scope"]; exists {
		if scopeStr, ok := scopeVal.(string); ok {
			opts.Scope = scopeStr
		}
	}
	if forceVal, exists := optsMap["Force"]; exists {
		if b, ok := forceVal.(bool); ok {
			opts.Force = b
		}
	}
	if excludeVal, exists := optsMap["Exclude"]; exists {
		if eStr, ok := excludeVal.(string); ok {
			opts.Exclude = eStr
		}
	}
}

// removeVariable is the internal implementation for testing
func removeVariable(ss *sessionstate.SessionState, name string, opts RemoveVariableOptions) error {
	if ss == nil {
		return fmt.Errorf("session state not initialized")
	}
	if name == "" {
		return fmt.Errorf("variable name is required")
	}

	var varName string
	if opts.Scope != "" {
		scopePrefix := normalizeScope(opts.Scope)
		if scopePrefix != "" {
			varName = scopePrefix + ":" + name
		} else {
			varName = name
		}
	} else {
		varName = name
	}

	err := ss.RemoveVariable(varName)
	if err != nil {
		// Force mode only bypasses "not found" errors
		if opts.Force && strings.Contains(err.Error(), "not found") {
			return nil
		}
	}
	return err
}
