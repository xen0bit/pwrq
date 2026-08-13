// Package psobject provides PowerShell-style object wrapping for the pwrq pipeline.
// It wraps JSON values with type information and member support, enabling
// PowerShell-like object manipulation on top of jq's JSON foundation.
package psobject

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"time"
)

// MemberType enumerates the types of members a PSObject can have.
//
// PowerShell also has script properties, alias properties, methods and events.
// pwrq carried all four for a while and never used any of them: a member only
// reaches a caller by way of ToMap, which resolves it to a plain JSON value, so
// anything that is not simply a name and a value has nowhere to go. They are
// gone rather than sitting unused, waiting to be a special case.
type MemberType int

const (
	MemberTypeNoteProperty MemberType = iota
)

func (m MemberType) String() string {
	switch m {
	case MemberTypeNoteProperty:
		return "NoteProperty"
	default:
		return "Unknown"
	}
}

// PSMember represents a named property of a PSObject.
type PSMember struct {
	Name        string
	MemberType  MemberType
	Value       any
	Description string
}

// PSObject wraps a value with PowerShell-style type information and members.
// This enables the pwrq pipeline to support PowerShell's rich object model
// while maintaining compatibility with jq's JSON streaming.
type PSObject struct {
	Value    any                  // The underlying value
	TypeName string               // PowerShell type name (e.g., "System.String", "System.Int32")
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
		if t.Kind() == reflect.Pointer {
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

// GetMember retrieves a member by name.
func (p *PSObject) GetMember(name string) (*PSMember, bool) {
	member, ok := p.Members[name]
	return member, ok
}

// GetPropertyValue retrieves a property value by name.
func (p *PSObject) GetPropertyValue(name string) (any, error) {
	member, ok := p.Members[name]
	if !ok {
		return nil, fmt.Errorf("member %q not found", name)
	}
	return member.Value, nil
}

// PSTypeNameKey is the property under which a PSObject's PowerShell type name
// travels on the wire. PowerShell's own ConvertTo-Json uses the same idea.
const PSTypeNameKey = "PSTypeName"

// PSPathKey is the property under which a PSObject's underlying scalar value
// travels when the object also carries named properties.
const PSPathKey = "PSPath"

// ToJSON converts the PSObject to its wire representation: ordinary JSON.
//
// An object with named properties becomes a flat map whose keys are the
// PowerShell property names, plus PSTypeName. An object with no properties is
// just its underlying value - wrapping a scalar in an envelope would make every
// downstream jq expression pay for metadata it did not ask for.
//
// The result contains only JSON types (nil, bool, int, float64, *big.Int,
// string, []any, map[string]any), so it can be queried by jq and printed by the
// encoder without special cases.
func (p *PSObject) ToJSON() any {
	if len(p.Members) == 0 {
		return NormalizeJSON(p.Value)
	}
	return p.ToMap()
}

// ToMap converts the PSObject to its flat JSON object form, evaluating
// ScriptProperty getters and resolving AliasProperties so that computed
// properties actually reach the output.
//
// Methods and events are omitted: they have no JSON representation, and
// emitting a placeholder for them only adds noise to every query.
func (p *PSObject) ToMap() map[string]any {
	result := make(map[string]any, len(p.Members)+2)

	// A PSObject wrapping a map exposes that map's entries as properties, the
	// way PowerShell surfaces a hashtable's keys. Without this, wrapping an
	// object to attach one computed property would discard the object.
	if underlying, ok := p.Value.(map[string]any); ok {
		for k, v := range underlying {
			result[k] = NormalizeJSON(v)
		}
	}

	// Members win over the underlying map: they are what was added on purpose.
	for name, member := range p.Members {
		result[name] = NormalizeJSON(member.Value)
	}

	// Preserve the underlying scalar so ByValue pipeline binding still works
	// for objects whose value is meaningful (a path, a name).
	if _, taken := result[PSPathKey]; !taken {
		if s, ok := p.Value.(string); ok && s != "" {
			result[PSPathKey] = s
		}
	}

	if p.TypeName != "" {
		result[PSTypeNameKey] = p.TypeName
	}
	return result
}

// NormalizeJSON converts a Go value into the value space gojq operates on:
// nil, bool, int, float64, *big.Int, string, []any, map[string]any.
//
// Without this, values like time.Time or os.FileMode flow into the pipeline
// where jq builtins cannot act on them, and then reach the encoder as a query
// error. Converting at the boundary keeps both usable.
func NormalizeJSON(value any) any {
	switch v := value.(type) {
	case nil, bool, string, int, float64, *big.Int:
		return v
	case json.Number:
		// The CLI decodes with UseNumber, so numbers arrive as json.Number.
		// It is also a fmt.Stringer, so it must be matched before that case or
		// every number in the pipeline turns into a string.
		return v
	case *PSObject:
		return v.ToJSON()
	case time.Time:
		return v.Format(time.RFC3339)
	case time.Duration:
		return v.String()
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	case error:
		return v.Error()
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float32:
		return float64(v)
	case map[string]any:
		result := make(map[string]any, len(v))
		for k, val := range v {
			result[k] = NormalizeJSON(val)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = NormalizeJSON(item)
		}
		return result
	}

	// Fall back to reflection for named types over JSON-compatible kinds
	// (os.FileMode, custom string enums) and for typed slices/maps.
	return normalizeByReflection(value)
}

func normalizeByReflection(value any) any {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	case reflect.String:
		return rv.String()
	case reflect.Slice, reflect.Array:
		result := make([]any, rv.Len())
		for i := range result {
			result[i] = NormalizeJSON(rv.Index(i).Interface())
		}
		return result
	case reflect.Map:
		result := make(map[string]any, rv.Len())
		for _, key := range rv.MapKeys() {
			result[fmt.Sprintf("%v", key.Interface())] = NormalizeJSON(rv.MapIndex(key).Interface())
		}
		return result
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return nil
		}
		return NormalizeJSON(rv.Elem().Interface())
	default:
		// Structs and anything else: stringify rather than leak a Go value.
		return fmt.Sprintf("%v", value)
	}
}

// FromMap creates a PSObject from its flat JSON wire form. Every key other than
// PSTypeName becomes a NoteProperty; PSTypeName supplies the type. Any JSON
// object is therefore a valid PSObject, which is what lets cmdlets accept
// hand-written JSON as readily as cmdlet output.
func FromMap(m map[string]any) (*PSObject, error) {
	if m == nil {
		return nil, fmt.Errorf("cannot create PSObject from nil map")
	}

	typeName, _ := m[PSTypeNameKey].(string)
	if typeName == "" {
		typeName = "System.Management.Automation.PSCustomObject"
	}

	// The underlying value is the object's path when it carries one; otherwise
	// the map itself stands in as the value.
	var value any = m
	if s, ok := m[PSPathKey].(string); ok {
		value = s
	}

	psobj := NewPSObjectWithTypeName(value, typeName)
	for name, v := range m {
		if name == PSTypeNameKey {
			continue
		}
		psobj.AddNoteProperty(name, v)
	}
	return psobj, nil
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

// IsPSObject reports whether a value is a PSObject or carries a PSTypeName.
//
// Note that a plain JSON object is not a PSObject by this test even though
// FromMap accepts one; the distinction is "was this produced by a cmdlet",
// which only PSTypeName can answer.
func IsPSObject(v any) bool {
	if _, ok := v.(*PSObject); ok {
		return true
	}
	if m, ok := v.(map[string]any); ok {
		_, ok := m[PSTypeNameKey].(string)
		return ok
	}
	return false
}

// ExtractValue extracts the underlying value from a PSObject or its wire form.
func ExtractValue(v any) any {
	switch val := v.(type) {
	case *PSObject:
		return val.Value
	case map[string]any:
		if s, ok := val[PSPathKey].(string); ok {
			return s
		}
	}
	return v
}

// ExtractTypeName extracts the type name from a PSObject or its wire form.
func ExtractTypeName(v any) string {
	switch val := v.(type) {
	case *PSObject:
		return val.TypeName
	case map[string]any:
		if t, ok := val[PSTypeNameKey].(string); ok {
			return t
		}
	}
	return inferTypeName(v)
}
