// Package objects provides PowerShell-style object manipulation cmdlets.
// This file implements Select-Object functionality for selecting, skipping,
// and projecting properties from pipeline objects.
package objects

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/typed"
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
	common.DeclareInput("select_object", common.InputPipeline)
	return common.WithFunctionOf("select_object", 0, 20, SelectedProperties, func(v any, args []any) any {
		objects, opts, err := ParseSelectObjectArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
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
// It preserves the input's type name, supports wildcard matching, and handles
// calculated properties via Expression blocks.
func selectProperties(obj any, properties []string) any {
	// Check whether the input is already a typed object, or its wire form.
	wasTyped := typed.Is(obj)

	var wrapped *typed.Object
	var sourceMap map[string]any

	if wasTyped {
		wrapped = common.EnsureObject(obj)
		if wrapped == nil {
			return obj
		}

		// Get the underlying value
		value := wrapped.Value

		// Determine the source of properties (map or object members)
		if m, ok := value.(map[string]any); ok {
			sourceMap = m
		} else {
			// If value is not a map, create one from the object's members
			sourceMap = make(map[string]any)
			for name, member := range wrapped.Members {
				if member.MemberType == typed.MemberTypeNoteProperty {
					sourceMap[name] = member.Value
				}
			}
		}
	} else {
		// Plain map input - nothing was wrapped
		if m, ok := obj.(map[string]any); ok {
			sourceMap = m
		} else {
			// Neither a map nor a typed object - return as-is
			return obj
		}
		wrapped = nil
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

		// Also match against the object's NoteProperty members
		if wrapped != nil {
			for name, member := range wrapped.Members {
				if member.MemberType == typed.MemberTypeNoteProperty {
					matched, _ := filepath.Match(pattern, name)
					if matched {
						resultMap[name] = member.Value
					}
				}
			}
		}
	}

	// If the input was typed, preserve its type name and members
	if wasTyped && wrapped != nil {
		projected := typed.NewWithType(resultMap, wrapped.TypeName)

		// Copy matching NoteProperty members from original
		for name, member := range wrapped.Members {
			if member.MemberType == typed.MemberTypeNoteProperty {
				for _, pattern := range properties {
					matched, _ := filepath.Match(pattern, name)
					if matched {
						projected.AddNoteProperty(name, member.Value)
						break
					}
				}
			}
		}

		return projected.ToMap()
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

// ParseSelectObjectArgs binds select_object's objects and options. The input is
// the pipeline value or the leading argument; see selectObjectInput.
func ParseSelectObjectArgs(v any, args []any) ([]any, SelectObjectOptions, error) {
	opts := SelectObjectOptions{
		First:      -1,
		Last:       -1,
		Skip:       -1,
		Properties: nil, // nil means select all
	}

	objects, rest := selectObjectInput(v, args)
	for _, a := range rest {
		argVal := common.BindValue(a)
		switch av := argVal.(type) {
		case string:
			opts.Properties = append(opts.Properties, av)
		case []any:
			if !allStrings(av) {
				return nil, opts, fmt.Errorf("select_object: a property list must hold strings")
			}
			opts.Properties = append(opts.Properties, arrToStrings(av)...)
		case map[string]any:
			// gojq represents an integral literal as int, not float64,
			// so {first: 2} was never read at all.
			if n, ok := common.ToInt(av["first"]); ok {
				opts.First = n
			}
			if n, ok := common.ToInt(av["last"]); ok {
				opts.Last = n
			}
			if n, ok := common.ToInt(av["skip"]); ok {
				opts.Skip = n
			}
			if propVal, exists := av["property"]; exists {
				opts.Properties = parseProperties(propVal)
			}
		default:
			// Silently dropping this is how `select_object(.; 5)` used to
			// return every property and look like it had worked.
			return nil, opts, fmt.Errorf("select_object: expected a property name, a list of them or an options object, got %T", argVal)
		}
	}

	if opts.First >= 0 && opts.Last >= 0 {
		return nil, opts, fmt.Errorf("select_object: -First and -Last cannot be used together")
	}

	return objects, opts, nil
}

// selectObjectInput splits the call into the objects to select from and the
// property names and options that follow.
//
// select_object is the one object cmdlet whose operands are variadic and
// untagged, so the leading argument's role is read from what it is rather than
// from where it sits: a property name, a list of them or an options object
// leaves the input on the pipeline, and anything else is the input itself.
// The roles do not overlap - nothing select_object can select from is a string
// or a list of them - and without the rule the piped form the synopsis
// documents, `select_object("Name"; "Age")`, bound "Name" as the input and
// returned it.
func selectObjectInput(v any, args []any) ([]any, []any) {
	piped := func() ([]any, []any) {
		return common.NormalizeToSlice(common.BindObjectInput(v)), args
	}
	if len(args) == 0 {
		return common.NormalizeToSlice(common.BindObjectInput(v)), nil
	}
	switch first := common.BindValue(args[0]).(type) {
	case string, map[string]any:
		return piped()
	case []any:
		// An empty array is data: there are no names in it to select by.
		if len(first) > 0 && allStrings(first) {
			return piped()
		}
	}
	return common.NormalizeToSlice(common.BindObjectInput(args[0])), args[1:]
}
