// Package variables provides PowerShell-style variable management cmdlets.
// This file implements Set-Variable functionality.
package variables

import (
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// SetVariableOptions holds options for the set_variable function
type SetVariableOptions struct {
	Name        string                  // Variable name
	Value       any                     // Value to set
	Description string                  // Variable description
	Option      sessionstate.VariableOptions // Variable options (ReadOnly, Constant, etc.)
	Scope       string                  // Target scope: global, script, local, private
	PassThru    bool                    // Return the variable after setting
}

// RegisterSetVariable registers the set_variable function with gojq
// Supports PowerShell-style parameters: -Name, -Value, -Description, -Option, -Scope, -PassThru
// Usage:
//   - set_variable("name"; "value") - basic usage
//   - set_variable("name"; "value"; {"Description": "my var"; "Scope": "global"; "PassThru": true})
func RegisterSetVariable() gojq.CompilerOption {
	return gojq.WithFunction("set_variable", 0, 3, func(v any, args []any) any {
		var name string
		var value any
		opts := SetVariableOptions{
			Option: sessionstate.None,
			Scope:  "",
		}

		// Parse arguments
		if len(args) == 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("set_variable: requires at least a name argument"), nil)
		}

		// First argument is name or value
		firstArg := common.BindValue(args[0])
		
		// Check if first arg is a string (variable name)
		if nameStr, isString := firstArg.(string); isString {
			name = nameStr
			// Check if second arg is value or options
			if len(args) > 1 {
				secondArg := common.BindValue(args[1])
				if _, ok := secondArg.(map[string]any); ok && len(args) == 2 {
					// Second arg is options map, value missing
					return common.MakeUDFErrorResult(fmt.Errorf("set_variable: value is required"), nil)
				}
				value = secondArg
			} else {
				// Only name provided, value from pipe
				value = common.BindValue(v)
			}
			
			// Third argument could be options
			if len(args) > 2 {
				if optsMap, ok := args[2].(map[string]any); ok {
					parseSetVariableOptions(&opts, optsMap)
				}
			}
		} else if optsMap, ok := firstArg.(map[string]any); ok {
			// First arg is options map
			parseSetVariableOptions(&opts, optsMap)
			
			// Name and value should be in the options map or additional args
			if opts.Name != "" {
				name = opts.Name
			}
			if opts.Value != nil {
				value = opts.Value
			}
			
			// If name/value not in map, check additional args
			if name == "" && len(args) > 1 {
				if nStr, ok := args[1].(string); ok {
					name = nStr
				}
			}
			if value == nil && len(args) > 2 {
				value = common.BindValue(args[2])
			}
		} else {
			// First arg is value, name from pipe (unusual but supported)
			value = firstArg
			if len(args) > 1 {
				if nStr, ok := args[1].(string); ok {
					name = nStr
				}
			}
			if len(args) > 2 {
				if optsMap, ok := args[2].(map[string]any); ok {
					parseSetVariableOptions(&opts, optsMap)
				}
			}
		}

		// Validate name
		if name == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("set_variable: variable name is required"), nil)
		}

		// Set the variable
		ss := common.GetSessionState()
		if ss == nil {
			return common.MakeUDFErrorResult(fmt.Errorf("set_variable: session state not initialized"), nil)
		}

		// Build the variable name with scope prefix if specified
		varName := name
		if opts.Scope != "" {
			// Normalize scope to lowercase for consistency
			scopePrefix := normalizeScope(opts.Scope)
			if scopePrefix != "" {
				varName = scopePrefix + ":" + name
			}
		}

		err := ss.SetVariable(varName, value, opts.Option)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		// Update $? automatic variable (last command success)
		ss.SetVariable("?", true, sessionstate.None)

		// Return result
		if opts.PassThru {
			// Return variable info with all fields
			result := map[string]any{
				"Name":  name,
				"Value": value,
			}
			if opts.Description != "" {
				result["Description"] = opts.Description
			}
			// Include Option field if set
			if opts.Option != sessionstate.None {
				result["Options"] = variableOptionsToString(opts.Option)
			}
			if opts.Scope != "" {
				result["Scope"] = normalizeScope(opts.Scope)
			}
			return common.MakeUDFSuccessResult(result, map[string]any{
				"operation": "set_variable",
				"name":      name,
			})
		}

		return common.MakeUDFSuccessResult(nil, map[string]any{
			"operation": "set_variable",
			"name":      name,
		})
	})
}

// parseSetVariableOptions parses options from a map
func parseSetVariableOptions(opts *SetVariableOptions, optsMap map[string]any) {
	if nameVal, exists := optsMap["Name"]; exists {
		if nStr, ok := nameVal.(string); ok {
			opts.Name = nStr
		}
	}
	if valueVal, exists := optsMap["Value"]; exists {
		opts.Value = valueVal
	}
	if descVal, exists := optsMap["Description"]; exists {
		if dStr, ok := descVal.(string); ok {
			opts.Description = dStr
		}
	}
	if scopeVal, exists := optsMap["Scope"]; exists {
		if scopeStr, ok := scopeVal.(string); ok {
			opts.Scope = scopeStr
		}
	}
	if passThruVal, exists := optsMap["PassThru"]; exists {
		if b, ok := passThruVal.(bool); ok {
			opts.PassThru = b
		}
	}
	if optionVal, exists := optsMap["Option"]; exists {
		// Option can be a string or array of strings
		switch opt := optionVal.(type) {
		case string:
			opts.Option = parseVariableOption(opt)
		case []any:
			// Multiple options
			for _, o := range opt {
				if oStr, ok := o.(string); ok {
					opts.Option |= parseVariableOption(oStr)
				}
			}
		}
	}
}

// setVariable is the internal implementation for testing
func setVariable(ss *sessionstate.SessionState, name string, value any, opts SetVariableOptions) error {
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

	return ss.SetVariable(varName, value, opts.Option)
}
