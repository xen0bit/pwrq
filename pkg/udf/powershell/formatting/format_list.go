// Package formatting provides PowerShell-style formatting cmdlets.
// This file implements Format-List functionality for displaying objects
// as a list of property=value pairs.
package formatting

import (
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// FormatListOptions holds options for the format_list function
type FormatListOptions struct {
	Property      []string // Properties to display (empty means all)
	CaseSensitive bool     // Whether property matching is case-sensitive
	Depth         int      // Maximum depth for nested object formatting (0 = unlimited)
}

// FormattedList represents a formatted list output
type FormattedList struct {
	Properties []PropertyDisplay // Properties to display
}

// PropertyDisplay represents a single property in the formatted output
type PropertyDisplay struct {
	Name  string // Property name
	Value any    // Property value
}

// RegisterFormatList registers the format_list function with gojq
// Supports PowerShell-style parameters:
//   - format_list(objects) - format all properties of all objects
//   - format_list(objects; {property: "Name"}) - format specific property
//   - format_list(objects; {property: ["Name", "Length"]}) - format multiple properties
//   - format_list(objects; {property: "*"}) - format all properties (explicit)
//   - format_list(objects; {property: "N*"}) - format properties matching wildcard
//
// Usage: format_list(objects) or format_list(objects; options)
func RegisterFormatList() gojq.CompilerOption {
	return gojq.WithFunction("format_list", 1, 2, func(input any, args []any) any {
		// Parse arguments
		objects, opts, err := ParseFormatListArgs(args)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		// Format objects as list
		formatted, err := formatList(objects, opts)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		return common.MakeUDFSuccessResult(formatted, map[string]any{
			"operation": "format_list",
			"count":     len(formatted),
		})
	})
}

// formatList formats objects as a list of property=value pairs
func formatList(objects []any, opts FormatListOptions) ([]any, error) {
	if len(objects) == 0 {
		return []any{}, nil
	}

	result := make([]any, 0, len(objects))

	for _, obj := range objects {
		formatted := formatSingleObject(obj, opts)
		result = append(result, formatted)
	}

	return result, nil
}

// formatSingleObject formats a single object as a list of properties
func formatSingleObject(obj any, opts FormatListOptions) map[string]any {
	// Input validation
	if obj == nil {
		return map[string]any{
			"properties": []PropertyDisplay{},
		}
	}

	// Get available properties from the object
	allProps := getAvailableProperties(obj)

	// Determine which properties to display
	var propsToShow []string
	if len(opts.Property) == 0 {
		// No property specified - show all
		propsToShow = allProps
	} else {
		// Match patterns against available properties
		propsToShow = matchProperties(allProps, opts.Property, opts.CaseSensitive)
	}

	// Build property display list
	properties := make([]PropertyDisplay, 0, len(propsToShow))
	for _, propName := range propsToShow {
		value := getPropertyValue(obj, propName)
		formattedValue := formatValue(value, opts.Depth, 0)
		properties = append(properties, PropertyDisplay{
			Name:  propName,
			Value: formattedValue,
		})
	}

	return map[string]any{
		"properties": properties,
	}
}



// ParseFormatListArgs parses arguments for the format_list function
func ParseFormatListArgs(args []any) ([]any, FormatListOptions, error) {
	opts := FormatListOptions{
		CaseSensitive: false,
		Depth:         0, // Unlimited by default (matches PowerShell's $FormatEnumerationLimit behavior)
	}

	if len(args) == 0 {
		return []any{}, opts, fmt.Errorf("format_list: requires objects argument")
	}

	// First argument is objects
	var objects []any
	inputVal := common.ExtractUDFValue(args[0])
	objects = common.NormalizeToSlice(inputVal)

	// Validate input - objects should not be nil after extraction
	if len(objects) == 0 && inputVal != nil {
		// Single object that wasn't a slice
		objects = []any{inputVal}
	}

	// Parse options if present
	if len(args) > 1 {
		if optsMap, ok := args[1].(map[string]any); ok {
			if propVal, exists := optsMap["property"]; exists {
				// Handle both string and array of strings
				switch p := propVal.(type) {
				case string:
					opts.Property = []string{p}
				case []any:
					opts.Property = make([]string, 0, len(p))
					for _, item := range p {
						if str, ok := item.(string); ok {
							opts.Property = append(opts.Property, str)
						}
					}
				}
			}
			if csVal, exists := optsMap["casesensitive"]; exists {
				if csBool, ok := csVal.(bool); ok {
					opts.CaseSensitive = csBool
				}
			}
			if depthVal, exists := optsMap["depth"]; exists {
				if depthInt, ok := depthVal.(float64); ok {
					opts.Depth = int(depthInt)
				} else if depthInt, ok := depthVal.(int); ok {
					opts.Depth = depthInt
				}
			}
		}
	}

	return objects, opts, nil
}

// FormatListToString converts a formatted list to a PowerShell-style string representation
// This is useful for display purposes
func FormatListToString(formattedList any) string {
	return FormatListToStringWithDepth(formattedList, 0)
}

// FormatListToStringWithDepth converts a formatted list to string with depth limiting
func FormatListToStringWithDepth(formattedList any, maxDepth int) string {
	if m, ok := formattedList.(map[string]any); ok {
		props := GetFormattedProperties(m)

		if len(props) == 0 {
			return ""
		}

		// Build formatted string
		var sb strings.Builder
		for i, prop := range props {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("%s : %s", prop.Name, formatValue(prop.Value, maxDepth, 0)))
		}
		return sb.String()
	}
	return ""
}



// GetFormattedProperties extracts the properties from a formatted list result
func GetFormattedProperties(formattedList any) []PropertyDisplay {
	if m, ok := formattedList.(map[string]any); ok {
		// Check for PSObject-wrapped format: {_val: {properties: [...]}, _meta: {...}}
		if val, exists := m["_val"]; exists {
			if inner, ok := val.(map[string]any); ok {
				if propsVal, exists := inner["properties"]; exists {
					if propsSlice, ok := propsVal.([]any); ok {
						result := make([]PropertyDisplay, 0, len(propsSlice))
						for _, p := range propsSlice {
							if propMap, ok := p.(map[string]any); ok {
								name, _ := propMap["Name"].(string)
								value := propMap["Value"]
								result = append(result, PropertyDisplay{Name: name, Value: value})
							}
						}
						return result
					}
				}
			}
		}
		// Also check direct format (for testing convenience)
		if propsVal, exists := m["properties"]; exists {
			if propsSlice, ok := propsVal.([]PropertyDisplay); ok {
				return propsSlice
			}
		}
	}
	return nil
}
