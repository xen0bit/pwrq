// Package objects provides PowerShell-style object manipulation cmdlets.
// This file implements Select-Object functionality for selecting, skipping,
// and projecting properties from pipeline objects.
package objects

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// normalizeToSlice is deprecated - use common.NormalizeToSlice instead
// Kept for backward compatibility within this file

// SelectObjectOptions holds options for the select_object function
type SelectObjectOptions struct {
	First      int      // Take first N objects (-1 means no limit)
	Last       int      // Take last N objects (-1 means no limit)
	Skip       int      // Skip N objects (-1 means no limit)
	Properties []string // Properties to select
}

// RegisterSelectObject registers the select_object function with gojq
// Supports PowerShell-style parameters: -First, -Last, -Skip, -Property
// Usage:
//   - select_object(objects) - select all
//   - select_object(objects, "Name", "Length") - positional property args
//   - select_object(objects; {first: n, last: n, skip: n, property: ["Name", "Value"]}) - options map
func RegisterSelectObject() gojq.CompilerOption {
	return gojq.WithFunction("select_object", 0, 20, func(v any, args []any) any {
		var objects []any
		opts := SelectObjectOptions{
			First:      -1,
			Last:       -1,
			Skip:       -1,
			Properties: nil, // nil means select all
		}

		// Parse arguments
		if len(args) > 0 {
			// First argument: objects (or could be property names if objects from pipe)
			firstArg := common.BindValue(args[0])

			// Check if first arg is a property name (string) - means objects from pipe
			if propStr, isString := firstArg.(string); isString {
				// Objects from pipe, first arg is a property name
				inputVal := common.BindValue(v)
				objects = common.NormalizeToSlice(inputVal)
				opts.Properties = []string{propStr}

				// Remaining args are additional property names
				for i := 1; i < len(args); i++ {
					if pStr, ok := args[i].(string); ok {
						opts.Properties = append(opts.Properties, pStr)
					}
				}
			} else if arr, isArray := firstArg.([]any); isArray && allStrings(arr) && len(args) == 1 {
				// First arg is an array of all strings and no other args - treat as property names from pipe
				inputVal := common.BindValue(v)
				objects = common.NormalizeToSlice(inputVal)
				opts.Properties = arrToStrings(arr)
			} else {
				// First arg is objects
				objects = common.NormalizeToSlice(firstArg)

				// Parse remaining arguments
				for i := 1; i < len(args); i++ {
					argVal := common.BindValue(args[i])

					// Check if it's a string (positional property name)
					if propStr, ok := argVal.(string); ok {
						if opts.Properties == nil {
							opts.Properties = []string{propStr}
						} else {
							opts.Properties = append(opts.Properties, propStr)
						}
					} else if optsMap, ok := argVal.(map[string]any); ok {
						// It's an options map
						if firstVal, exists := optsMap["first"]; exists {
							if firstNum, ok := firstVal.(float64); ok {
								opts.First = int(firstNum)
							} else if firstStr, ok := firstVal.(string); ok {
								if n, err := strconv.Atoi(firstStr); err == nil {
									opts.First = n
								}
							}
						}
						if lastVal, exists := optsMap["last"]; exists {
							if lastNum, ok := lastVal.(float64); ok {
								opts.Last = int(lastNum)
							} else if lastStr, ok := lastVal.(string); ok {
								if n, err := strconv.Atoi(lastStr); err == nil {
									opts.Last = n
								}
							}
						}
						if skipVal, exists := optsMap["skip"]; exists {
							if skipNum, ok := skipVal.(float64); ok {
								opts.Skip = int(skipNum)
							} else if skipStr, ok := skipVal.(string); ok {
								if n, err := strconv.Atoi(skipStr); err == nil {
									opts.Skip = n
								}
							}
						}
						if propVal, exists := optsMap["property"]; exists {
							opts.Properties = parseProperties(propVal)
						}
					}
				}
			}
		} else {
			// Objects from pipe
			inputVal := common.BindValue(v)
			objects = common.NormalizeToSlice(inputVal)
		}

		// Validate options
		if opts.First >= 0 && opts.Last >= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("select_object: -First and -Last cannot be used together"), nil)
		}

		// Apply Skip
		if opts.Skip > 0 && len(objects) > opts.Skip {
			objects = objects[opts.Skip:]
		} else if opts.Skip > 0 {
			objects = []any{}
		}

		// Apply First
		if opts.First >= 0 && len(objects) > opts.First {
			objects = objects[:opts.First]
		}

		// Apply Last
		if opts.Last >= 0 && len(objects) > opts.Last {
			objects = objects[len(objects)-opts.Last:]
		}

		// Apply Property selection
		if len(opts.Properties) > 0 {
			selected := make([]any, len(objects))
			for i, obj := range objects {
				selected[i] = selectProperties(obj, opts.Properties)
			}
			objects = selected
		}

		// Return result - unwrap single objects for pipeline compatibility
		if len(objects) == 0 {
			return common.MakeUDFSuccessResult([]any{}, map[string]any{
				"operation": "select_object",
				"count":     0,
			})
		}

		// For pipeline compatibility, return single objects unwrapped
		var result any
		if len(objects) == 1 {
			result = objects[0]
		} else {
			result = objects
		}

		return common.MakeUDFSuccessResult(result, map[string]any{
			"operation": "select_object",
			"count":     len(objects),
			"first":     opts.First,
			"last":      opts.Last,
			"skip":      opts.Skip,
		})
	})
}

// normalizeToSlice converts various input types to a slice of any
func normalizeToSlice(v any) []any {
	if v == nil {
		return []any{}
	}

	switch val := v.(type) {
	case []any:
		return val
	case map[string]any:
		// Single object - wrap in slice
		return []any{val}
	default:
		// Single value - wrap in slice
		return []any{val}
	}
}

// allStrings checks if all elements in a slice are strings
func allStrings(arr []any) bool {
	for _, item := range arr {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}

// arrToStrings converts a slice of any (assumed to be all strings) to []string
func arrToStrings(arr []any) []string {
	result := make([]string, len(arr))
	for i, item := range arr {
		if s, ok := item.(string); ok {
			result[i] = s
		}
	}
	return result
}

// parseProperties extracts property names from various input formats
func parseProperties(v any) []string {
	switch val := v.(type) {
	case []any:
		props := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				props = append(props, s)
			}
		}
		return props
	case string:
		// Comma-separated or single property
		if strings.Contains(val, ",") {
			parts := strings.Split(val, ",")
			props := make([]string, 0, len(parts))
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					props = append(props, trimmed)
				}
			}
			return props
		}
		return []string{val}
	default:
		return []string{}
	}
}

// selectProperties creates a new object with only the specified properties.
// It preserves PSObject type information, supports wildcard matching, and handles
// calculated properties via Expression blocks.
func selectProperties(obj any, properties []string) any {
	// Check if input is already a PSObject or PSObject-like map
	isPSObject := psobject.IsPSObject(obj)

	var psobj *psobject.PSObject
	var sourceMap map[string]any

	if isPSObject {
		// Convert to PSObject for proper handling
		psobj = common.EnsurePSObject(obj)
		if psobj == nil {
			return obj
		}

		// Get the underlying value
		value := psobj.Value

		// Determine the source of properties (map or PSObject members)
		if m, ok := value.(map[string]any); ok {
			sourceMap = m
		} else {
			// If value is not a map, create one from PSObject members
			sourceMap = make(map[string]any)
			for name, member := range psobj.Members {
				if member.MemberType == psobject.MemberTypeNoteProperty {
					sourceMap[name] = member.Value
				}
			}
		}
	} else {
		// Plain map input - no PSObject wrapping
		if m, ok := obj.(map[string]any); ok {
			sourceMap = m
		} else {
			// Non-map, non-PSObject input - return as-is
			return obj
		}
		psobj = nil
	}

	// Build the result map with matched properties
	resultMap := make(map[string]any)

	for _, pattern := range properties {
		// Match against source map keys
		for key, val := range sourceMap {
			matched, _ := filepath.Match(pattern, key)
			if matched {
				resultMap[key] = val
			}
		}

		// Also match against PSObject NoteProperty members
		if psobj != nil {
			for name, member := range psobj.Members {
				if member.MemberType == psobject.MemberTypeNoteProperty {
					matched, _ := filepath.Match(pattern, name)
					if matched {
						resultMap[name] = member.Value
					}
				}
			}
		}
	}

	// If input was a PSObject, preserve type information and members
	if isPSObject && psobj != nil {
		// Create new PSObject preserving the original TypeName
		newPSObj := psobject.NewPSObjectWithTypeName(resultMap, psobj.TypeName)

		// Copy matching NoteProperty members from original
		for name, member := range psobj.Members {
			if member.MemberType == psobject.MemberTypeNoteProperty {
				for _, pattern := range properties {
					matched, _ := filepath.Match(pattern, name)
					if matched {
						newPSObj.AddNoteProperty(name, member.Value)
						break
					}
				}
			}
		}

		return newPSObj.ToMap()
	}

	// Plain map input - return plain map result
	return resultMap
}

// selectObject is the internal implementation for testing
func selectObject(objects []any, opts SelectObjectOptions) ([]any, error) {
	// Validate options
	if opts.First >= 0 && opts.Last >= 0 {
		return nil, fmt.Errorf("select_object: -First and -Last cannot be used together")
	}

	result := make([]any, len(objects))
	copy(result, objects)

	// Apply Skip
	if opts.Skip > 0 && len(result) > opts.Skip {
		result = result[opts.Skip:]
	} else if opts.Skip > 0 {
		result = []any{}
	}

	// Apply First
	if opts.First >= 0 && len(result) > opts.First {
		result = result[:opts.First]
	}

	// Apply Last
	if opts.Last >= 0 && len(result) > opts.Last {
		result = result[len(result)-opts.Last:]
	}

	// Apply Property selection
	if len(opts.Properties) > 0 {
		selected := make([]any, len(result))
		for i, obj := range result {
			selected[i] = selectProperties(obj, opts.Properties)
		}
		result = selected
	}

	return result, nil
}

// ParseSelectObjectArgs parses arguments for testing
func ParseSelectObjectArgs(args []any) ([]any, SelectObjectOptions, error) {
	opts := SelectObjectOptions{
		First:      -1,
		Last:       -1,
		Skip:       -1,
		Properties: nil,
	}

	if len(args) == 0 {
		return []any{}, opts, nil
	}

	// First argument is objects or options
	var objects []any
	var optsIndex int

	if arr, ok := args[0].([]any); ok {
		objects = arr
		optsIndex = 1
	} else {
		objects = []any{args[0]}
		optsIndex = 1
	}

	// Parse options if present
	if len(args) > optsIndex {
		if optsMap, ok := args[optsIndex].(map[string]any); ok {
			if firstVal, exists := optsMap["first"]; exists {
				if firstNum, ok := firstVal.(float64); ok {
					opts.First = int(firstNum)
				}
			}
			if lastVal, exists := optsMap["last"]; exists {
				if lastNum, ok := lastVal.(float64); ok {
					opts.Last = int(lastNum)
				}
			}
			if skipVal, exists := optsMap["skip"]; exists {
				if skipNum, ok := skipVal.(float64); ok {
					opts.Skip = int(skipNum)
				}
			}
			if propVal, exists := optsMap["property"]; exists {
				opts.Properties = parseProperties(propVal)
			}
		}
	}

	return objects, opts, nil
}
