// Package objects provides PowerShell-style object manipulation cmdlets.
// This file implements Sort-Object functionality for sorting pipeline objects
// by property values in ascending or descending order.
package objects

import (
	"fmt"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// SortDirection enumerates the sort order
type SortDirection int

const (
	SortDirectionAscending SortDirection = iota
	SortDirectionDescending
)

// SortProperty represents a property to sort by with direction
type SortProperty struct {
	Name      string
	Direction SortDirection
}

// SortObjectOptions holds options for the sort_object function
type SortObjectOptions struct {
	Properties    []SortProperty // Properties to sort by
	Descending    bool           // -Descending, applied to every property
	CaseSensitive bool           // Whether sorting is case-sensitive
	Unique        bool           // Remove duplicates (-Unique)
}

// RegisterSortObject registers the sort_object function with gojq
// Supports PowerShell-style parameters:
//   - sort_object(objects; {property: "Name"})
//   - sort_object(objects; {property: ["Name", "Value"]})
//   - sort_object(objects; {property: [{name: "Name", descending: true}]})
//   - sort_object(objects; {unique: true})
//
// Usage: sort_object(objects) or sort_object(objects; options)
func RegisterSortObject() gojq.CompilerOption {
	return common.WithFunction("sort_object", 1, 2, func(input any, args []any) any {
		// Parse arguments
		objects, opts, err := ParseSortObjectArgs(args)
		if err != nil {
			return newSortErrorObject(err.Error())
		}

		// Sort objects
		sorted, err := sortObject(objects, opts)
		if err != nil {
			return newSortErrorObject(err.Error())
		}

		return common.MakeUDFSuccessResult(sorted, map[string]any{
			"operation": "sort_object",
			"count":     len(sorted),
		})
	})
}

// parseSortProperty parses a sort property from various input formats
func parseSortProperty(v any) ([]SortProperty, error) {
	properties := make([]SortProperty, 0)

	switch val := v.(type) {
	case string:
		// Single property name, possibly with direction suffix
		// Support formats: "Name", "Name desc", "Name descending", "-Name"
		name := strings.TrimSpace(val)
		direction := SortDirectionAscending

		// Check for descending indicators (check longer suffixes first)
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, " descending") {
			direction = SortDirectionDescending
			name = strings.TrimSpace(name[:len(name)-11])
		} else if strings.HasSuffix(lower, " desc") {
			direction = SortDirectionDescending
			name = strings.TrimSpace(name[:len(name)-5])
		} else if strings.HasSuffix(lower, " ascending") {
			name = strings.TrimSpace(name[:len(name)-10])
		} else if strings.HasSuffix(lower, " asc") {
			name = strings.TrimSpace(name[:len(name)-4])
		} else if strings.HasPrefix(name, "-") {
			// PowerShell style: -Property for descending
			direction = SortDirectionDescending
			name = strings.TrimPrefix(name, "-")
		}

		if name != "" {
			properties = append(properties, SortProperty{Name: name, Direction: direction})
		}

	case []any:
		// Array of property names or property objects
		for _, item := range val {
			switch itemVal := item.(type) {
			case string:
				name := strings.TrimSpace(itemVal)
				direction := SortDirectionAscending

				lower := strings.ToLower(name)
				if strings.HasSuffix(lower, " descending") {
					direction = SortDirectionDescending
					name = strings.TrimSpace(name[:len(name)-11])
				} else if strings.HasSuffix(lower, " desc") {
					direction = SortDirectionDescending
					name = strings.TrimSpace(name[:len(name)-5])
				} else if strings.HasPrefix(name, "-") {
					direction = SortDirectionDescending
					name = strings.TrimPrefix(name, "-")
				}

				if name != "" {
					properties = append(properties, SortProperty{Name: name, Direction: direction})
				}

			case map[string]any:
				// Property object with name and direction
				prop := SortProperty{Direction: SortDirectionAscending}

				if nameVal, ok := itemVal["name"]; ok {
					if nameStr, ok := nameVal.(string); ok {
						prop.Name = nameStr
					}
				}
				if descVal, ok := itemVal["descending"]; ok {
					if descBool, ok := descVal.(bool); ok && descBool {
						prop.Direction = SortDirectionDescending
					}
				}

				if prop.Name != "" {
					properties = append(properties, prop)
				}
			}
		}

	case map[string]any:
		// Single property object
		prop := SortProperty{Direction: SortDirectionAscending}

		if nameVal, ok := val["name"]; ok {
			if nameStr, ok := nameVal.(string); ok {
				prop.Name = nameStr
			}
		}
		if descVal, ok := val["descending"]; ok {
			if descBool, ok := descVal.(bool); ok && descBool {
				prop.Direction = SortDirectionDescending
			}
		}

		if prop.Name != "" {
			properties = append(properties, prop)
		}
	}

	return properties, nil
}

// ParseSortObjectArgs parses arguments for testing
func ParseSortObjectArgs(args []any) ([]any, SortObjectOptions, error) {
	opts := SortObjectOptions{
		CaseSensitive: false,
		Unique:        false,
	}

	if len(args) == 0 {
		return []any{}, opts, fmt.Errorf("sort_object: requires objects argument")
	}

	// First argument is objects
	var objects []any
	inputVal := common.BindValue(args[0])
	objects = common.NormalizeToSlice(inputVal)

	// Parse options if present
	if len(args) > 1 {
		if optsMap, ok := args[1].(map[string]any); ok {
			if propVal, exists := optsMap["property"]; exists {
				props, err := parseSortProperty(propVal)
				if err != nil {
					return nil, opts, fmt.Errorf("invalid property specification: %w", err)
				}
				opts.Properties = props
			}
			// PowerShell spells this `Sort-Object Age -Descending`, so a
			// top-level flag is the form users reach for first; without it the
			// only way to sort descending was the "Age desc" property suffix,
			// and {property: "Age", descending: true} silently sorted ascending.
			if descVal, exists := optsMap["descending"]; exists {
				if descBool, ok := descVal.(bool); ok && descBool {
					opts.Descending = true
					for i := range opts.Properties {
						opts.Properties[i].Direction = SortDirectionDescending
					}
				}
			}
			if csVal, exists := optsMap["casesensitive"]; exists {
				if csBool, ok := csVal.(bool); ok {
					opts.CaseSensitive = csBool
				}
			}
			if uniqueVal, exists := optsMap["unique"]; exists {
				if uniqueBool, ok := uniqueVal.(bool); ok {
					opts.Unique = uniqueBool
				}
			}
		}
	}

	return objects, opts, nil
}

// sortObject sorts objects by the specified properties
func sortObject(objects []any, opts SortObjectOptions) ([]any, error) {
	if len(objects) == 0 {
		return objects, nil
	}

	// Input validation - ensure we have a slice/array
	if objects == nil {
		return []any{}, nil
	}

	// If no properties specified, sort by the whole object value
	if len(opts.Properties) == 0 {
		return sortByValue(objects, opts.Descending, opts.CaseSensitive, opts.Unique)
	}

	// Create a sortable wrapper
	sorter := &objectSorter{
		objects:       objects,
		properties:    opts.Properties,
		caseSensitive: opts.CaseSensitive,
	}

	sort.Stable(sorter)

	result := sorter.objects

	// Apply unique if requested
	if opts.Unique {
		result = deduplicateObjects(result, opts.Properties)
	}

	return result, nil
}

// sortByValue sorts objects by their raw value
func sortByValue(objects []any, descending, caseSensitive, unique bool) ([]any, error) {
	sorter := &valueSorter{
		objects:       objects,
		descending:    descending,
		caseSensitive: caseSensitive,
	}

	sort.Stable(sorter)

	result := sorter.objects

	if unique {
		result = deduplicateByValue(result)
	}

	return result, nil
}

// objectSorter implements sort.Interface for sorting objects by properties
type objectSorter struct {
	objects       []any
	properties    []SortProperty
	caseSensitive bool
}

func (s *objectSorter) Len() int {
	return len(s.objects)
}

func (s *objectSorter) Swap(i, j int) {
	s.objects[i], s.objects[j] = s.objects[j], s.objects[i]
}

func (s *objectSorter) Less(i, j int) bool {
	objI := s.objects[i]
	objJ := s.objects[j]

	for _, prop := range s.properties {
		valI, errI := extractPropertyByWildcard(objI, prop.Name)
		valJ, errJ := extractPropertyByWildcard(objJ, prop.Name)

		// Handle missing properties
		if errI != nil && errJ != nil {
			continue // Both missing, continue to next property
		}
		if errI != nil {
			return true // Missing values sort first
		}
		if errJ != nil {
			return false
		}

		// Compare values
		cmp := compareValuesForSort(valI, valJ, s.caseSensitive)

		if cmp != 0 {
			if prop.Direction == SortDirectionDescending {
				return cmp > 0
			}
			return cmp < 0
		}
		// Equal on this property, continue to next
	}

	return false // All properties equal
}

// valueSorter implements sort.Interface for sorting by raw value
type valueSorter struct {
	objects       []any
	descending    bool
	caseSensitive bool
}

func (s *valueSorter) Len() int {
	return len(s.objects)
}

func (s *valueSorter) Swap(i, j int) {
	s.objects[i], s.objects[j] = s.objects[j], s.objects[i]
}

func (s *valueSorter) Less(i, j int) bool {
	cmp := compareValuesForSort(s.objects[i], s.objects[j], s.caseSensitive)
	if s.descending {
		return cmp > 0
	}
	return cmp < 0
}

// compareValuesForSort compares two values for sorting purposes
// Returns -1 if a < b, 0 if equal, 1 if a > b
func compareValuesForSort(a, b any, caseSensitive bool) int {
	// Handle nil
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Extract wrapped values if present
	a = common.BindValue(a)
	b = common.BindValue(b)

	// Try numeric comparison first
	aNum, aIsNum := toNumber(a)
	bNum, bIsNum := toNumber(b)

	if aIsNum && bIsNum {
		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
		return 0
	}

	// String comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)

	if !caseSensitive {
		aStr = strings.ToLower(aStr)
		bStr = strings.ToLower(bStr)
	}

	if aStr < bStr {
		return -1
	}
	if aStr > bStr {
		return 1
	}
	return 0
}

// deduplicateObjects removes duplicate objects based on sort properties
func deduplicateObjects(objects []any, properties []SortProperty) []any {
	if len(objects) == 0 {
		return objects
	}

	result := make([]any, 0, len(objects))
	seen := make(map[string]bool)

	for _, obj := range objects {
		key := buildDedupKey(obj, properties)
		if !seen[key] {
			seen[key] = true
			result = append(result, obj)
		}
	}

	return result
}

// deduplicateByValue removes duplicate objects by their raw value
func deduplicateByValue(objects []any) []any {
	if len(objects) == 0 {
		return objects
	}

	result := make([]any, 0, len(objects))
	seen := make(map[string]bool)

	for _, obj := range objects {
		key := fmt.Sprintf("%v", common.BindValue(obj))
		if !seen[key] {
			seen[key] = true
			result = append(result, obj)
		}
	}

	return result
}

// buildDedupKey creates a unique key for an object based on its sort properties
func buildDedupKey(obj any, properties []SortProperty) string {
	if len(properties) == 0 {
		return fmt.Sprintf("%v", common.BindValue(obj))
	}

	parts := make([]string, 0, len(properties))
	for _, prop := range properties {
		val, err := extractPropertyByWildcard(obj, prop.Name)
		if err != nil {
			parts = append(parts, "<nil>")
		} else {
			parts = append(parts, fmt.Sprintf("%v", common.BindValue(val)))
		}
	}

	return strings.Join(parts, "|")
}

// newSortErrorObject reports a sort failure as a jq error.
func newSortErrorObject(message string) any {
	return common.MakeUDFErrorResult(fmt.Errorf("sort_object: %s", message), nil)
}

// GetSortDirection is a helper for tests to create sort directions
func GetSortDirection(descending bool) SortDirection {
	if descending {
		return SortDirectionDescending
	}
	return SortDirectionAscending
}

// NewSortProperty is a helper for tests to create sort properties
func NewSortProperty(name string, descending bool) SortProperty {
	dir := SortDirectionAscending
	if descending {
		dir = SortDirectionDescending
	}
	return SortProperty{Name: name, Direction: dir}
}

// ParseSortPropertyString parses a property string like "Name desc" or "-Name"
func ParseSortPropertyString(s string) (SortProperty, error) {
	props, err := parseSortProperty(s)
	if err != nil {
		return SortProperty{}, err
	}
	if len(props) == 0 {
		return SortProperty{}, fmt.Errorf("empty property specification")
	}
	return props[0], nil
}

// String returns the string representation of SortDirection
func (d SortDirection) String() string {
	switch d {
	case SortDirectionAscending:
		return "Ascending"
	case SortDirectionDescending:
		return "Descending"
	default:
		return "Unknown"
	}
}

// SortPropertyOptions holds options for a single sort property
type SortPropertyOptions struct {
	Name       string
	Descending bool
}

// ParseSortOptionsFromMap parses sort options from a map (for testing)
func ParseSortOptionsFromMap(m map[string]any) (SortObjectOptions, error) {
	opts := SortObjectOptions{}

	if propVal, ok := m["property"]; ok {
		props, err := parseSortProperty(propVal)
		if err != nil {
			return opts, err
		}
		opts.Properties = props
	}

	if csVal, ok := m["casesensitive"]; ok {
		if csBool, ok := csVal.(bool); ok {
			opts.CaseSensitive = csBool
		}
	}

	if uniqueVal, ok := m["unique"]; ok {
		if uniqueBool, ok := uniqueVal.(bool); ok {
			opts.Unique = uniqueBool
		}
	}

	return opts, nil
}

// SortByProperty is a convenience function for sorting by a single property
func SortByProperty(objects []any, property string, descending, caseSensitive, unique bool) ([]any, error) {
	opts := SortObjectOptions{
		Properties:    []SortProperty{{Name: property, Direction: SortDirectionAscending}},
		CaseSensitive: caseSensitive,
		Unique:        unique,
	}

	if descending {
		opts.Properties[0].Direction = SortDirectionDescending
	}

	return sortObject(objects, opts)
}

// SortByProperties is a convenience function for sorting by multiple properties
func SortByProperties(objects []any, properties []string, descending []bool, caseSensitive, unique bool) ([]any, error) {
	sortProps := make([]SortProperty, len(properties))
	for i, name := range properties {
		dir := SortDirectionAscending
		if i < len(descending) && descending[i] {
			dir = SortDirectionDescending
		}
		sortProps[i] = SortProperty{Name: name, Direction: dir}
	}

	opts := SortObjectOptions{
		Properties:    sortProps,
		CaseSensitive: caseSensitive,
		Unique:        unique,
	}

	return sortObject(objects, opts)
}
