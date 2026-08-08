// Package variables provides PowerShell-style variable management cmdlets.
// This file implements Get-Variable functionality.
package variables

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// GetVariableOptions holds options for the get_variable function
type GetVariableOptions struct {
	Name       string // Variable name (supports wildcards)
	Scope      string // Scope to search: global, script, local
	ValueOnly  bool   // Return only the value, not variable info
	Exclude    string // Exclude pattern
	Include    string // Include pattern
}

// RegisterGetVariable registers the get_variable function with gojq
// Supports PowerShell-style parameters: -Name, -Scope, -ValueOnly, -Exclude, -Include
// Usage:
//   - get_variable("name") - get variable info
//   - get_variable("*") - get all variables
//   - get_variable("name"; {"ValueOnly": true}) - get only the value
//   - get_variable("name"; {"Scope": "global"})
func RegisterGetVariable() gojq.CompilerOption {
	return gojq.WithFunction("get_variable", 0, 2, func(v any, args []any) any {
		var name string
		opts := GetVariableOptions{
			ValueOnly: false,
			Scope:     "",
		}

		// Parse arguments
		if len(args) == 0 {
			// No args - return all variables (like Get-Variable with no name)
			name = "*"
		} else {
			// First argument is name or options
			firstArg := common.BindValue(args[0])
			
			if nameStr, isString := firstArg.(string); isString {
				name = nameStr
			} else if optsMap, ok := firstArg.(map[string]any); ok {
				// First arg is options map
				parseGetVariableOptions(&opts, optsMap)
				if opts.Name != "" {
					name = opts.Name
				} else {
					name = "*"
				}
			}
			
			// Second argument could be options
			if len(args) > 1 {
				if optsMap, ok := args[1].(map[string]any); ok {
					parseGetVariableOptions(&opts, optsMap)
				}
			}
		}

		// Get session state
		ss := common.GetSessionState()
		if ss == nil {
			return common.MakeUDFErrorResult(fmt.Errorf("get_variable: session state not initialized"), nil)
		}

		// Handle wildcard or specific variable
		if containsWildcard(name) {
			// Wildcard pattern - return all matching variables
			return getVariablesByPattern(ss, name, opts)
		}

		// Single variable lookup
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

		// Use GetVariableEntry to get full metadata
		entry, err := ss.GetVariableEntry(varName)
		if err != nil {
			// Variable not found
			if name == "*" {
				// Return empty array for wildcard with no matches
				return common.MakeUDFSuccessResult([]any{}, map[string]any{
					"operation": "get_variable",
					"pattern":   name,
					"count":     0,
				})
			}
			return common.MakeUDFErrorResult(fmt.Errorf("get_variable: variable %q not found", name), nil)
		}

		// Update $? automatic variable
		ss.SetVariable("?", true, sessionstate.None)

		// Return result
		if opts.ValueOnly {
			return common.MakeUDFSuccessResult(entry.Value, map[string]any{
				"operation": "get_variable",
				"name":      name,
			})
		}

		// Return variable info with all metadata (PowerShell-compatible format)
		result := map[string]any{
			"Name":    entry.Name,
			"Value":   entry.Value,
			"Options": variableOptionsToString(entry.Options),
			"Scope":   scopeTypeToString(entry.Scope),
		}
		if entry.Description != "" {
			result["Description"] = entry.Description
		}
		return common.MakeUDFSuccessResult(result, map[string]any{
			"operation": "get_variable",
			"name":      name,
		})
	})
}

// getVariablesByPattern returns all variables matching the pattern
func getVariablesByPattern(ss *sessionstate.SessionState, pattern string, opts GetVariableOptions) any {
	// Use ss.GetVariables() which properly traverses the scope chain
	// from global down to current, with later scopes shadowing earlier ones
	allVars := ss.GetVariables()
	
	var results []any
	for name, entry := range allVars {
		// Apply Include filter first (if specified)
		if opts.Include != "" {
			matched, err := filepath.Match(opts.Include, name)
			if err != nil || !matched {
				continue
			}
		}
		
		// Apply Exclude filter (if specified)
		if opts.Exclude != "" {
			matched, err := filepath.Match(opts.Exclude, name)
			if err != nil || matched {
				continue // Skip if matches exclude pattern
			}
		}
		
		// Apply wildcard pattern matching
		matched, err := filepath.Match(pattern, name)
		if err != nil || !matched {
			continue
		}
		
		// If Scope is specified, filter by scope
		if opts.Scope != "" {
			scopePrefix := normalizeScope(opts.Scope)
			if scopePrefix != "" && scopeTypeToString(entry.Scope) != strings.ToUpper(scopePrefix[:1])+scopePrefix[1:] {
				continue
			}
		}
		
		// Build variable info
		result := map[string]any{
			"Name":    entry.Name,
			"Value":   entry.Value,
			"Options": variableOptionsToString(entry.Options),
			"Scope":   scopeTypeToString(entry.Scope),
		}
		if entry.Description != "" {
			result["Description"] = entry.Description
		}
		results = append(results, result)
	}
	
	if results == nil {
		results = []any{}
	}
	
	return common.MakeUDFSuccessResult(results, map[string]any{
		"operation": "get_variable",
		"pattern":   pattern,
		"count":     len(results),
	})
}

// parseGetVariableOptions parses options from a map
func parseGetVariableOptions(opts *GetVariableOptions, optsMap map[string]any) {
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
	if valueOnlyVal, exists := optsMap["ValueOnly"]; exists {
		if b, ok := valueOnlyVal.(bool); ok {
			opts.ValueOnly = b
		}
	}
	if excludeVal, exists := optsMap["Exclude"]; exists {
		if eStr, ok := excludeVal.(string); ok {
			opts.Exclude = eStr
		}
	}
	if includeVal, exists := optsMap["Include"]; exists {
		if iStr, ok := includeVal.(string); ok {
			opts.Include = iStr
		}
	}
}

// getVariable is the internal implementation for testing
func getVariable(ss *sessionstate.SessionState, name string, opts GetVariableOptions) (any, error) {
	if ss == nil {
		return nil, fmt.Errorf("session state not initialized")
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

	entry, err := ss.GetVariableEntry(varName)
	if err != nil {
		return nil, err
	}

	if opts.ValueOnly {
		return entry.Value, nil
	}

	return map[string]any{
		"Name":    entry.Name,
		"Value":   entry.Value,
		"Options": variableOptionsToString(entry.Options),
		"Scope":   scopeTypeToString(entry.Scope),
	}, nil
}
