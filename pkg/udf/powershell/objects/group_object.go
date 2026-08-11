// Package objects provides PowerShell-style object manipulation cmdlets.
// This file implements Group-Object functionality for grouping pipeline objects
// by property values.
package objects

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// extractPropertyByPathLocal is a deprecated alias for common.ExtractPropertyByPath.
// Kept for backward compatibility within this file only.
func extractPropertyByPathLocal(value any, path string) (any, error) {
	return common.ExtractPropertyByPath(value, path)
}

// GroupObjectOptions holds options for the group_object function
type GroupObjectOptions struct {
	Property      string // Property to group by
	CaseSensitive bool   // Whether grouping is case-sensitive
	NoElement     bool   // Don't include the elements in output (-NoElement)
	NoGroup       bool   // Don't group, just output unique values (-NoGroup)
	AsHashTable   bool   // Return as hashtable instead of objects (-AsHashTable)
}

// GroupedObject represents a group of objects with a common property value
type GroupedObject struct {
	Name  string // The group key (property value)
	Count int    // Number of items in the group
	Group []any  // The objects in this group
}

// RegisterGroupObject registers the group_object function with gojq
// Supports PowerShell-style parameters:
//   - group_object(objects; {property: "Name"})
//   - group_object(objects; {property: "Name", casesensitive: true})
//   - group_object(objects; {property: "Name", noelement: true})
//
// Usage: group_object(objects) or group_object(objects; options)
func RegisterGroupObject() gojq.CompilerOption {
	return gojq.WithFunction("group_object", 1, 2, func(input any, args []any) any {
		// Parse arguments
		objects, opts, err := ParseGroupObjectArgs(args)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		// Group objects
		grouped, err := groupObject(objects, opts)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		return common.MakeUDFSuccessResult(grouped, map[string]any{
			"operation": "group_object",
			"count":     len(grouped),
		})
	})
}

// groupObject groups objects by the specified property
func groupObject(objects []any, opts GroupObjectOptions) ([]any, error) {
	if len(objects) == 0 {
		return []any{}, nil
	}

	// If no property specified, group by the whole object value
	if opts.Property == "" {
		return groupByValue(objects, opts)
	}

	// Group by property
	return groupByProperty(objects, opts)
}

// groupByProperty groups objects by a specific property value
func groupByProperty(objects []any, opts GroupObjectOptions) ([]any, error) {
	if len(objects) == 0 {
		return []any{}, nil
	}

	groupMap := make(map[string]*GroupedObject)
	groupOrder := make([]string, 0)

	for _, obj := range objects {
		// Extract property value - support wildcard patterns by trying to match
		propValue, err := extractPropertyByWildcard(obj, opts.Property)
		if err != nil {
			// Skip objects that don't have the property (PowerShell behavior)
			continue
		}

		// Normalize the key for grouping
		keyStr := normalizeGroupKey(propValue, opts.CaseSensitive)

		group, exists := groupMap[keyStr]
		if !exists {
			// Preserve the original property value as the group name (not normalized)
			nameStr := fmt.Sprintf("%v", propValue)
			group = &GroupedObject{
				Name:  nameStr,
				Count: 0,
				Group: make([]any, 0),
			}
			groupMap[keyStr] = group
			groupOrder = append(groupOrder, keyStr)
		}

		group.Group = append(group.Group, obj)
		group.Count++
	}

	if opts.AsHashTable {
		return formatGroupsAsHashTable(groupMap, groupOrder)
	}

	if opts.NoElement {
		return formatGroupsNoElement(groupMap, groupOrder, opts)
	}

	if opts.NoGroup {
		return formatGroupsNoGroup(groupMap, groupOrder)
	}

	return formatGroupsFull(groupMap, groupOrder)
}

// groupByValue groups objects by their entire value
func groupByValue(objects []any, opts GroupObjectOptions) ([]any, error) {
	groupMap := make(map[string]*GroupedObject)
	groupOrder := make([]string, 0)

	for _, obj := range objects {
		// Use the entire object as the key
		keyStr := fmt.Sprintf("%v", common.BindValue(obj))
		if !opts.CaseSensitive {
			keyStr = strings.ToLower(keyStr)
		}

		group, exists := groupMap[keyStr]
		if !exists {
			group = &GroupedObject{
				Name:  fmt.Sprintf("%v", common.BindValue(obj)),
				Count: 0,
				Group: make([]any, 0),
			}
			groupMap[keyStr] = group
			groupOrder = append(groupOrder, keyStr)
		}

		group.Group = append(group.Group, obj)
		group.Count++
	}

	if opts.NoElement {
		return formatGroupsNoElement(groupMap, groupOrder, opts)
	}

	if opts.NoGroup {
		return formatGroupsNoGroup(groupMap, groupOrder)
	}

	return formatGroupsFull(groupMap, groupOrder)
}

// normalizeGroupKey converts a value to a normalized string key for grouping
func normalizeGroupKey(key any, caseSensitive bool) string {
	keyStr := fmt.Sprintf("%v", key)
	if !caseSensitive {
		keyStr = strings.ToLower(keyStr)
	}
	return keyStr
}

// extractPropertyByWildcard extracts a property value from an object, supporting wildcard patterns.
// If the property contains wildcards (*, ?), it tries to match against available properties.
func extractPropertyByWildcard(obj any, pattern string) (any, error) {
	if pattern == "" {
		return obj, nil
	}

	// Check if pattern contains wildcards
	hasWildcard := strings.ContainsAny(pattern, "*?")

	// Extract the underlying value from PSObject if present
	value := common.BindValue(obj)

	if !hasWildcard {
		// Direct property access - use local implementation to avoid cross-file dependency
		return extractPropertyByPathLocal(value, pattern)
	}

	// Wildcard pattern - need to find matching property
	// Get all available property names from the object
	propNames := getPropertyNames(value)

	// Try to match the pattern against available properties
	for _, propName := range propNames {
		matched, _ := filepath.Match(pattern, propName)
		if matched {
			propValue, err := extractPropertyByPathLocal(value, propName)
			if err == nil {
				return propValue, nil
			}
		}
	}

	return nil, fmt.Errorf("property matching pattern %q not found", pattern)
}

// getPropertyNames returns all property names available on an object
func getPropertyNames(value any) []string {
	names := make([]string, 0)

	switch v := value.(type) {
	case map[string]any:
		for k := range v {
			names = append(names, k)
		}
	case *psobject.PSObject:
		// Get property names from PSObject members
		for name, member := range v.Members {
			if member.MemberType == psobject.MemberTypeNoteProperty {
				names = append(names, name)
			}
		}
		// Also get keys from the underlying value if it's a map
		if valMap, ok := v.Value.(map[string]any); ok {
			for k := range valMap {
				names = append(names, k)
			}
		}
	}

	return names
}

// formatGroupsFull formats groups as PSObjects with Name, Count, and Group properties
func formatGroupsFull(groupMap map[string]*GroupedObject, groupOrder []string) ([]any, error) {
	result := make([]any, 0, len(groupOrder))

	for _, key := range groupOrder {
		group := groupMap[key]
		groupObj := createGroupObject(group)
		result = append(result, groupObj)
	}

	return result, nil
}

// formatGroupsNoElement formats groups without the Group property (metadata only)
func formatGroupsNoElement(groupMap map[string]*GroupedObject, groupOrder []string, opts GroupObjectOptions) ([]any, error) {
	result := make([]any, 0, len(groupOrder))

	for _, key := range groupOrder {
		group := groupMap[key]
		valueMap := map[string]any{
			"Name":  group.Name,
			"Count": group.Count,
		}
		// Wrap in PSObject with proper type information
		psobj := psobject.NewPSObjectWithTypeName(valueMap, "Microsoft.PowerShell.Commands.GroupInfo")
		psobj.AddNoteProperty("Name", group.Name)
		psobj.AddNoteProperty("Count", group.Count)
		result = append(result, psobj.ToMap())
	}

	return result, nil
}

// formatGroupsAsHashTable formats groups as a hashtable (map[string]any)
func formatGroupsAsHashTable(groupMap map[string]*GroupedObject, groupOrder []string) ([]any, error) {
	hashTable := make(map[string]any)
	for _, key := range groupOrder {
		group := groupMap[key]
		hashTable[group.Name] = group.Group
	}
	// Return single-item slice containing the hashtable wrapped as PSObject
	psobj := psobject.NewPSObjectWithTypeName(hashTable, "System.Collections.Hashtable")
	return []any{psobj.ToMap()}, nil
}

// formatGroupsNoGroup returns unique values without grouping
func formatGroupsNoGroup(groupMap map[string]*GroupedObject, groupOrder []string) ([]any, error) {
	result := make([]any, 0, len(groupOrder))

	for _, key := range groupOrder {
		group := groupMap[key]
		if len(group.Group) > 0 {
			// Return the first object's property value
			if len(group.Group) > 0 {
				result = append(result, group.Group[0])
			}
		}
	}

	return result, nil
}

// createGroupObject creates a PSObject representing a group with TypeName="GroupInfo"
func createGroupObject(group *GroupedObject) map[string]any {
	result := map[string]any{
		"Name":  group.Name,
		"Count": group.Count,
		"Group": group.Group,
	}
	// Wrap in PSObject with proper type information
	psobj := psobject.NewPSObjectWithTypeName(result, "Microsoft.PowerShell.Commands.GroupInfo")
	psobj.AddNoteProperty("Name", group.Name)
	psobj.AddNoteProperty("Count", group.Count)
	psobj.AddNoteProperty("Group", group.Group)
	return psobj.ToMap()
}

// ParseGroupObjectArgs parses arguments for testing
func ParseGroupObjectArgs(args []any) ([]any, GroupObjectOptions, error) {
	opts := GroupObjectOptions{
		CaseSensitive: false,
		NoElement:     false,
		NoGroup:       false,
		AsHashTable:   false,
	}

	if len(args) == 0 {
		return []any{}, opts, fmt.Errorf("group_object: requires objects argument")
	}

	// Input validation - explicit nil check
	if args[0] == nil {
		return []any{}, opts, fmt.Errorf("group_object: objects argument is nil")
	}

	// First argument is objects
	var objects []any
	inputVal := common.BindValue(args[0])
	objects = common.NormalizeToSlice(inputVal)

	// Parse options if present
	if len(args) > 1 {
		if optsMap, ok := args[1].(map[string]any); ok {
			if propVal, exists := optsMap["property"]; exists {
				if propStr, ok := propVal.(string); ok {
					opts.Property = propStr
				}
			}
			if csVal, exists := optsMap["casesensitive"]; exists {
				if csBool, ok := csVal.(bool); ok {
					opts.CaseSensitive = csBool
				}
			}
			if noElemVal, exists := optsMap["noelement"]; exists {
				if noElemBool, ok := noElemVal.(bool); ok {
					opts.NoElement = noElemBool
				}
			}
			if noGroupVal, exists := optsMap["nogroup"]; exists {
				if noGroupBool, ok := noGroupVal.(bool); ok {
					opts.NoGroup = noGroupBool
				}
			}
			if hashVal, exists := optsMap["ashashtable"]; exists {
				if hashBool, ok := hashVal.(bool); ok {
					opts.AsHashTable = hashBool
				}
			}
		}
	}

	return objects, opts, nil
}

// GetGroupCount returns the count of groups for testing
func GetGroupCount(groups []any) int {
	return len(groups)
}

// GetGroupByName finds a group by its name for testing
func GetGroupByName(groups []any, name string) *GroupedObject {
	for _, g := range groups {
		groupMap, ok := g.(map[string]any)
		if !ok {
			continue
		}

		innerMap := groupMap

		if innerMap == nil {
			continue
		}

		if groupName, ok := innerMap["Name"].(string); ok && groupName == name {
			count := 0
			if countVal, ok := innerMap["Count"].(int); ok {
				count = countVal
			}
			group := []any{}
			if groupVal, ok := innerMap["Group"].([]any); ok {
				group = groupVal
			}
			return &GroupedObject{
				Name:  groupName,
				Count: count,
				Group: group,
			}
		}
	}
	return nil
}

// SortGroupsByCount sorts groups by their count
func SortGroupsByCount(groups []any, descending bool) []any {
	sorted := make([]any, len(groups))
	copy(sorted, groups)

	sort.Slice(sorted, func(i, j int) bool {
		countI := getGroupCount(sorted[i])
		countJ := getGroupCount(sorted[j])
		if descending {
			return countI > countJ
		}
		return countI < countJ
	})

	return sorted
}

// SortGroupsByName sorts groups alphabetically by name
func SortGroupsByName(groups []any, caseSensitive bool) []any {
	sorted := make([]any, len(groups))
	copy(sorted, groups)

	sort.Slice(sorted, func(i, j int) bool {
		nameI := getGroupName(sorted[i])
		nameJ := getGroupName(sorted[j])
		if !caseSensitive {
			nameI = strings.ToLower(nameI)
			nameJ = strings.ToLower(nameJ)
		}
		return nameI < nameJ
	})

	return sorted
}

func getGroupCount(group any) int {
	groupMap, ok := group.(map[string]any)
	if !ok {
		return 0
	}

	if count, ok := groupMap["Count"].(int); ok {
		return count
	}
	return 0
}

func getGroupName(group any) string {
	groupMap, ok := group.(map[string]any)
	if !ok {
		return ""
	}

	if name, ok := groupMap["Name"].(string); ok {
		return name
	}
	return ""
}
