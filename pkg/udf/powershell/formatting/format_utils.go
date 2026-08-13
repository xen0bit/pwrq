// Package formatting provides PowerShell-style formatting cmdlets.
// This file contains shared utilities used by format_list.go and format_table.go.
package formatting

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// getAvailableProperties returns all property names available on an object.
// PSTypeName is excluded: it identifies the object rather than describing it,
// and PowerShell's own formatters do not show it as a column.
func getAvailableProperties(value any) []string {
	names := make([]string, 0)

	switch v := value.(type) {
	case map[string]any:
		for k := range v {
			if k != psobject.PSTypeNameKey {
				names = append(names, k)
			}
		}
	case *psobject.PSObject:
		// Get property names from PSObject members
		for name := range v.Members {
			if !containsString(names, name) {
				names = append(names, name)
			}
		}
		// Also get keys from the underlying value if it's a map
		if valMap, ok := v.Value.(map[string]any); ok {
			for k := range valMap {
				if !containsString(names, k) {
					names = append(names, k)
				}
			}
		}
	}

	return names
}

// matchProperties matches property patterns against available properties
func matchProperties(allProps []string, patterns []string, caseSensitive bool) []string {
	result := make([]string, 0)

	// Check for wildcard "all" pattern
	for _, pattern := range patterns {
		if pattern == "*" {
			return allProps
		}
	}

	// Match each pattern against available properties
	for _, pattern := range patterns {
		for _, prop := range allProps {
			if matchProperty(prop, pattern, caseSensitive) {
				if !containsString(result, prop) {
					result = append(result, prop)
				}
			}
		}
	}

	return result
}

// matchProperty checks if a property name matches a pattern (supports wildcards)
func matchProperty(prop, pattern string, caseSensitive bool) bool {
	if !caseSensitive {
		prop = strings.ToLower(prop)
		pattern = strings.ToLower(pattern)
	}

	matched, err := filepath.Match(pattern, prop)
	return err == nil && matched
}

// getPropertyValue extracts a property value from an object.
//
// Pipeline values are always maps by the time they get here - a cmdlet's
// PSObject is flattened to its wire form on the way out - but a formatting
// helper may still be handed one directly by the cmdlet that built it, so both
// are accepted.
func getPropertyValue(value any, propertyName string) any {
	switch v := value.(type) {
	case map[string]any:
		if val, exists := v[propertyName]; exists {
			return val
		}
	case *psobject.PSObject:
		if member, ok := v.Members[propertyName]; ok {
			return member.Value
		}
		// Fall back to the underlying value's own keys.
		if valMap, ok := v.Value.(map[string]any); ok {
			if val, exists := valMap[propertyName]; exists {
				return val
			}
		}
	}
	return nil
}

// containsString checks if a string slice contains a specific string
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// formatValue formats a value for display with depth limiting
func formatValue(value any, maxDepth int, currentDepth int) string {
	if value == nil {
		return ""
	}

	// Check depth limit (0 = unlimited)
	if maxDepth > 0 && currentDepth >= maxDepth {
		return "..."
	}

	switch v := value.(type) {
	case *psobject.PSObject:
		wrapped := v.ToMap()
		return formatNestedObject(wrapped, maxDepth, currentDepth+1)
	case map[string]any:
		if psobject.IsPSObject(v) {
			return formatNestedObject(v, maxDepth, currentDepth+1)
		}
		return formatMap(v, maxDepth, currentDepth+1)
	case []any:
		return formatSlice(v, maxDepth, currentDepth+1)
	default:
		return fmt.Sprintf("%v", value)
	}
}

// formatNestedObject formats a nested PSObject as a string
func formatNestedObject(obj map[string]any, maxDepth int, currentDepth int) string {
	if maxDepth > 0 && currentDepth >= maxDepth {
		return "..."
	}

	tempOpts := FormatListOptions{
		Property:      []string{},
		CaseSensitive: false,
		Depth:         maxDepth,
	}

	formatted := formatSingleObject(obj, tempOpts)
	if propsVal, ok := formatted["properties"].([]PropertyDisplay); ok {
		if len(propsVal) == 0 {
			return "{}"
		}

		var sb strings.Builder
		sb.WriteString("{")
		for i, prop := range propsVal {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%s: %s", prop.Name, formatValue(prop.Value, maxDepth, currentDepth))
		}
		sb.WriteString("}")
		return sb.String()
	}
	return "{}"
}

// formatMap formats a regular map as a string
func formatMap(m map[string]any, maxDepth int, currentDepth int) string {
	if len(m) == 0 {
		return "{}"
	}

	if maxDepth > 0 && currentDepth >= maxDepth {
		return "{...}"
	}

	var sb strings.Builder
	sb.WriteString("{")
	first := true
	for k, v := range m {
		if !first {
			sb.WriteString(", ")
		}
		first = false
		fmt.Fprintf(&sb, "%s: %s", k, formatValue(v, maxDepth, currentDepth))
	}
	sb.WriteString("}")
	return sb.String()
}

// formatSlice formats a slice as a string
func formatSlice(s []any, maxDepth int, currentDepth int) string {
	if len(s) == 0 {
		return "[]"
	}

	if maxDepth > 0 && currentDepth >= maxDepth {
		return "[...]"
	}

	var sb strings.Builder
	sb.WriteString("[")
	for i, v := range s {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(formatValue(v, maxDepth, currentDepth))
	}
	sb.WriteString("]")
	return sb.String()
}

// padRight pads a string to the specified width with spaces
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
