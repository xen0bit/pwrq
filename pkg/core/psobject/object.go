// Package psobject provides PowerShell-style object wrapping for the pwrq pipeline.
// It wraps JSON values with type information and member support, enabling
// PowerShell-like object manipulation on top of jq's JSON foundation.
package psobject

import (
	"fmt"
	"reflect"
	"strconv"
	"time"
)

// MemberType enumerates the types of members a PSObject can have.
type MemberType int

const (
	MemberTypeNoteProperty MemberType = iota
	MemberTypeScriptProperty
	MemberTypeAliasProperty
	MemberTypeMethod
	MemberTypeEvent
)

func (m MemberType) String() string {
	switch m {
	case MemberTypeNoteProperty:
		return "NoteProperty"
	case MemberTypeScriptProperty:
		return "ScriptProperty"
	case MemberTypeAliasProperty:
		return "AliasProperty"
	case MemberTypeMethod:
		return "Method"
	case MemberTypeEvent:
		return "Event"
	default:
		return "Unknown"
	}
}

// PSMember represents a member of a PSObject (property, method, etc.)
type PSMember struct {
	Name         string
	MemberType   MemberType
	Value        any
	Getter       func() (any, error)  `json:"-"` // For ScriptProperty
	Invoker      func(args ...any) any `json:"-"` // For Method
	Description  string
	Serializable bool // Indicates if this member survives JSON round-trip
}

// PSObject wraps a value with PowerShell-style type information and members.
// This enables the pwrq pipeline to support PowerShell's rich object model
// while maintaining compatibility with jq's JSON streaming.
type PSObject struct {
	Value    any                 // The underlying value
	TypeName string              // PowerShell type name (e.g., "System.String", "System.Int32")
	Members  map[string]*PSMember // Named members (properties, methods, etc.)
}

// NewPSObject creates a new PSObject with automatic type detection.
func NewPSObject(value any) *PSObject {
	return &PSObject{
		Value:    value,
		TypeName: inferTypeName(value),
		Members:  make(map[string]*PSMember),
	}
}

// NewPSObjectWithTypeName creates a new PSObject with an explicit type name.
func NewPSObjectWithTypeName(value any, typeName string) *PSObject {
	return &PSObject{
		Value:    value,
		TypeName: typeName,
		Members:  make(map[string]*PSMember),
	}
}

// inferTypeName determines the PowerShell-style type name for a Go value.
func inferTypeName(value any) string {
	if value == nil {
		return "System.Object"
	}

	switch value.(type) {
	case string:
		return "System.String"
	case int, int8, int16, int32, int64:
		return "System.Int32"
	case uint, uint8, uint16, uint32, uint64:
		return "System.UInt32"
	case float32, float64:
		return "System.Double"
	case bool:
		return "System.Boolean"
	case time.Time:
		return "System.DateTime"
	case []byte:
		return "System.Byte[]"
	case map[string]any:
		return "System.Management.Automation.PSObject"
	case []any:
		return "System.Object[]"
	default:
		t := reflect.TypeOf(value)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		return fmt.Sprintf("System.%s", t.Name())
	}
}

// AddMember adds a member to the PSObject.
func (p *PSObject) AddMember(name string, memberType MemberType, value any) {
	p.Members[name] = &PSMember{
		Name:       name,
		MemberType: memberType,
		Value:      value,
	}
}

// AddNoteProperty adds a simple property to the PSObject.
func (p *PSObject) AddNoteProperty(name string, value any) {
	p.AddMember(name, MemberTypeNoteProperty, value)
}

// AddScriptProperty adds a computed property with a getter function.
func (p *PSObject) AddScriptProperty(name string, getter func() (any, error)) {
	p.Members[name] = &PSMember{
		Name:       name,
		MemberType: MemberTypeScriptProperty,
		Getter:     getter,
	}
}

// AddAliasProperty adds an alias to another property.
func (p *PSObject) AddAliasProperty(name string, targetProperty string) {
	p.Members[name] = &PSMember{
		Name:       name,
		MemberType: MemberTypeAliasProperty,
		Value:      targetProperty,
	}
}

// AddMethod adds a method to the PSObject.
func (p *PSObject) AddMethod(name string, invoker func(args ...any) any) {
	p.Members[name] = &PSMember{
		Name:       name,
		MemberType: MemberTypeMethod,
		Invoker:    invoker,
	}
}

// GetMember retrieves a member by name.
func (p *PSObject) GetMember(name string) (*PSMember, bool) {
	member, ok := p.Members[name]
	return member, ok
}

// GetPropertyValue retrieves a property value, handling different member types.
func (p *PSObject) GetPropertyValue(name string) (any, error) {
	member, ok := p.Members[name]
	if !ok {
		return nil, fmt.Errorf("member %q not found", name)
	}

	switch member.MemberType {
	case MemberTypeNoteProperty:
		return member.Value, nil
	case MemberTypeScriptProperty:
		if member.Getter == nil {
			return nil, fmt.Errorf("script property %q has no getter", name)
		}
		return member.Getter()
	case MemberTypeAliasProperty:
		targetName, ok := member.Value.(string)
		if !ok {
			return nil, fmt.Errorf("alias property %q has invalid target", name)
		}
		return p.GetPropertyValue(targetName)
	default:
		return nil, fmt.Errorf("member %q is not a property (type: %s)", name, member.MemberType)
	}
}

// InvokeMethod calls a method on the PSObject.
func (p *PSObject) InvokeMethod(name string, args ...any) (any, error) {
	member, ok := p.Members[name]
	if !ok {
		return nil, fmt.Errorf("method %q not found", name)
	}
	if member.MemberType != MemberTypeMethod {
		return nil, fmt.Errorf("member %q is not a method (type: %s)", name, member.MemberType)
	}
	if member.Invoker == nil {
		return nil, fmt.Errorf("method %q has no invoker", name)
	}
	return member.Invoker(args...), nil
}

// ToMap converts the PSObject to a map representation for JSON serialization.
// This maintains compatibility with the existing {_val, _meta} pattern.
func (p *PSObject) ToMap() map[string]any {
	result := map[string]any{
		"_val":  p.serializeValue(p.Value),
		"_meta": map[string]any{
			"type":    p.TypeName,
			"members": p.getMembersMap(),
		},
	}
	return result
}

// serializeValue recursively converts PSObjects to maps for JSON serialization.
func (p *PSObject) serializeValue(value any) any {
	switch v := value.(type) {
	case *PSObject:
		return v.ToMap()
	case map[string]any:
		result := make(map[string]any)
		for k, val := range v {
			result[k] = p.serializeValue(val)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = p.serializeValue(item)
		}
		return result
	default:
		return value
	}
}

// getMembersMap returns members in a serializable format.
func (p *PSObject) getMembersMap() map[string]any {
	result := make(map[string]any)
	for name, member := range p.Members {
		memberData := map[string]any{
			"type":         member.MemberType.String(),
			"serializable": member.Serializable,
		}

		switch member.MemberType {
		case MemberTypeNoteProperty:
			// NoteProperty values are always serializable
			memberData["value"] = p.serializeValue(member.Value)
			memberData["serializable"] = true
		case MemberTypeScriptProperty:
			// ScriptProperty: serialize the description or a placeholder
			// The getter function cannot be serialized
			if member.Description != "" {
				memberData["description"] = member.Description
			}
			memberData["serializable"] = false
		case MemberTypeAliasProperty:
			// AliasProperty: store the target name
			if target, ok := member.Value.(string); ok {
				memberData["target"] = target
				memberData["serializable"] = true
			}
		case MemberTypeMethod:
			// Method: the invoker function cannot be serialized
			if member.Description != "" {
				memberData["description"] = member.Description
			}
			memberData["serializable"] = false
		case MemberTypeEvent:
			memberData["serializable"] = false
		}

		result[name] = memberData
	}
	return result
}

// FromMap creates a PSObject from a map representation.
// Returns an error if the map does not have a valid PSObject shape.
func FromMap(m map[string]any) (*PSObject, error) {
	if err := validatePSObjectShape(m); err != nil {
		return nil, err
	}

	val := m["_val"]
	typeName := "System.Object"
	var members map[string]any

	if meta, ok := m["_meta"].(map[string]any); ok {
		if t, ok := meta["type"].(string); ok {
			typeName = t
		}
		if mems, ok := meta["members"].(map[string]any); ok {
			members = mems
		}
	}

	psobj := NewPSObjectWithTypeName(val, typeName)

	// Restore members
	for name, memberData := range members {
		memberMap, ok := memberData.(map[string]any)
		if !ok {
			continue
		}

		typeStr, _ := memberMap["type"].(string)
		memberType := parseMemberType(typeStr)

		switch memberType {
		case MemberTypeNoteProperty:
			if value, ok := memberMap["value"]; ok {
				// Handle nested PSObjects recursively
				value = unwrapNestedPSObject(value)
				psobj.AddNoteProperty(name, value)
			}
		case MemberTypeScriptProperty:
			// ScriptProperty getters cannot be restored from JSON
			// Store as NoteProperty with description if available
			psobj.AddMember(name, MemberTypeScriptProperty, nil)
			if desc, ok := memberMap["description"].(string); ok {
				psobj.Members[name].Description = desc
			}
		case MemberTypeAliasProperty:
			if target, ok := memberMap["target"].(string); ok {
				psobj.AddAliasProperty(name, target)
			}
		case MemberTypeMethod:
			// Method invokers cannot be restored from JSON
			// Store metadata only
			psobj.AddMember(name, MemberTypeMethod, nil)
			if desc, ok := memberMap["description"].(string); ok {
				psobj.Members[name].Description = desc
			}
		case MemberTypeEvent:
			// Events cannot be restored from JSON
			psobj.AddMember(name, MemberTypeEvent, nil)
		}
	}

	return psobj, nil
}

// validatePSObjectShape checks if a map has the required PSObject structure.
func validatePSObjectShape(m map[string]any) error {
	if m == nil {
		return fmt.Errorf("cannot create PSObject from nil map")
	}
	if _, ok := m["_val"]; !ok {
		return fmt.Errorf("map missing required '_val' field")
	}
	if _, ok := m["_meta"]; !ok {
		return fmt.Errorf("map missing required '_meta' field")
	}
	return nil
}

// unwrapNestedPSObject recursively converts nested PSObject maps to PSObjects.
func unwrapNestedPSObject(value any) any {
	switch v := value.(type) {
	case map[string]any:
		if _, hasVal := v["_val"]; hasVal {
			if _, hasMeta := v["_meta"]; hasMeta {
				// This is a nested PSObject map
				psobj, err := FromMap(v)
				if err != nil {
					return v // Return as-is if conversion fails
				}
				return psobj
			}
		}
		// Recursively check nested maps and slices
		result := make(map[string]any)
		for k, val := range v {
			result[k] = unwrapNestedPSObject(val)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = unwrapNestedPSObject(item)
		}
		return result
	default:
		return value
	}
}

// parseMemberType converts a string to MemberType.
func parseMemberType(s string) MemberType {
	switch s {
	case "NoteProperty":
		return MemberTypeNoteProperty
	case "ScriptProperty":
		return MemberTypeScriptProperty
	case "AliasProperty":
		return MemberTypeAliasProperty
	case "Method":
		return MemberTypeMethod
	case "Event":
		return MemberTypeEvent
	default:
		return MemberTypeNoteProperty
	}
}

// ConvertValue performs type conversion on a PSObject's value.
// This mimics PowerShell's type conversion behavior.
func ConvertValue(psobj *PSObject, targetType string) (*PSObject, error) {
	if psobj == nil {
		return nil, fmt.Errorf("cannot convert nil PSObject")
	}

	var newValue any
	var err error

	switch targetType {
	case "System.String", "string":
		newValue = fmt.Sprintf("%v", psobj.Value)
	case "System.Int32", "int":
		newValue, err = convertToInt(psobj.Value)
	case "System.Int64", "long":
		newValue, err = convertToInt64(psobj.Value)
	case "System.Double", "double":
		newValue, err = convertToFloat64(psobj.Value)
	case "System.Boolean", "bool":
		newValue, err = convertToBool(psobj.Value)
	case "System.DateTime":
		newValue, err = convertToDateTime(psobj.Value)
	default:
		return nil, fmt.Errorf("unsupported target type: %s", targetType)
	}

	if err != nil {
		return nil, err
	}

	return NewPSObjectWithTypeName(newValue, targetType), nil
}

func convertToInt(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int", value)
	}
}

func convertToInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", value)
	}
}

func convertToFloat64(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}

func convertToBool(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case int:
		return v != 0, nil
	case int64:
		return v != 0, nil
	case float64:
		return v != 0, nil
	case string:
		switch v {
		case "true", "True", "TRUE", "1":
			return true, nil
		case "false", "False", "FALSE", "0":
			return false, nil
		default:
			return false, fmt.Errorf("cannot convert string %q to bool", v)
		}
	default:
		return false, fmt.Errorf("cannot convert %T to bool", value)
	}
}

func convertToDateTime(value any) (time.Time, error) {
	switch v := value.(type) {
	case time.Time:
		return v, nil
	case string:
		return time.Parse(time.RFC3339, v)
	case int64:
		return time.Unix(v, 0), nil
	case int:
		return time.Unix(int64(v), 0), nil
	default:
		return time.Time{}, fmt.Errorf("cannot convert %T to DateTime", value)
	}
}

// IsPSObject checks if a value is a PSObject or PSObject-like map.
func IsPSObject(v any) bool {
	if _, ok := v.(*PSObject); ok {
		return true
	}
	if m, ok := v.(map[string]any); ok {
		_, hasVal := m["_val"]
		_, hasMeta := m["_meta"]
		return hasVal && hasMeta
	}
	return false
}

// ExtractValue extracts the underlying value from a PSObject or PSObject-like map.
func ExtractValue(v any) any {
	switch val := v.(type) {
	case *PSObject:
		return val.Value
	case map[string]any:
		if v, ok := val["_val"]; ok {
			return v
		}
	}
	return v
}

// ExtractTypeName extracts the type name from a PSObject or PSObject-like map.
func ExtractTypeName(v any) string {
	switch val := v.(type) {
	case *PSObject:
		return val.TypeName
	case map[string]any:
		if meta, ok := val["_meta"].(map[string]any); ok {
			if t, ok := meta["type"].(string); ok {
				return t
			}
		}
	}
	return inferTypeName(v)
}
