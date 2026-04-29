// Package variables provides PowerShell-style variable management cmdlets.
// This file contains shared utility functions for the variables package.
package variables

import (
	"strings"

	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
)

// containsWildcard checks if a pattern contains wildcard characters
func containsWildcard(s string) bool {
	for _, c := range s {
		if c == '*' || c == '?' {
			return true
		}
	}
	return false
}

// normalizeScope normalizes scope names to lowercase standard forms
func normalizeScope(scope string) string {
	switch strings.ToLower(scope) {
	case "global":
		return "global"
	case "script":
		return "script"
	case "local":
		return "local"
	case "private":
		return "private"
	default:
		return ""
	}
}

// variableOptionsToString converts VariableOptions to a string representation
// Handles bitmask combinations (e.g., ReadOnly|AllScope -> "ReadOnly, AllScope")
func variableOptionsToString(opts sessionstate.VariableOptions) string {
	if opts == sessionstate.None {
		return "None"
	}
	
	var parts []string
	if opts&sessionstate.ReadOnly != 0 {
		parts = append(parts, "ReadOnly")
	}
	if opts&sessionstate.Constant != 0 {
		parts = append(parts, "Constant")
	}
	if opts&sessionstate.Private != 0 {
		parts = append(parts, "Private")
	}
	if opts&sessionstate.AllScope != 0 {
		parts = append(parts, "AllScope")
	}
	
	if len(parts) == 0 {
		return "None"
	}
	return strings.Join(parts, ", ")
}

// scopeTypeToString converts ScopeType to a string representation
func scopeTypeToString(scope sessionstate.ScopeType) string {
	switch scope {
	case sessionstate.ScopeGlobal:
		return "Global"
	case sessionstate.ScopeScript:
		return "Script"
	case sessionstate.ScopeLocal:
		return "Local"
	case sessionstate.ScopePrivate:
		return "Private"
	default:
		return "Unknown"
	}
}

// parseVariableOption parses a string option into VariableOptions
func parseVariableOption(s string) sessionstate.VariableOptions {
	switch strings.ToLower(s) {
	case "readonly":
		return sessionstate.ReadOnly
	case "constant":
		return sessionstate.Constant
	case "private":
		return sessionstate.Private
	case "allscope":
		return sessionstate.AllScope
	default:
		return sessionstate.None
	}
}
