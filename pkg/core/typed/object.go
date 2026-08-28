// Package typed carries pwrq's object model: a JSON value with a type name and
// named properties, on top of jq's value space.
//
// The model began as a transcription of PowerShell's PSObject and wore its
// vocabulary - PSTypeName on the wire, System.IO.FileInfo as a type name,
// System.String for a scalar. Those names claimed a lineage pwrq does not
// have. A pwrq object is not a .NET object; it has no methods, no script
// properties and no runtime behind it, and naming it after one told a caller
// to expect all three. So the names are pwrq's own: PwrqType and PwrqValue on
// the wire, Pwrq.* for what a cmdlet emits, and jq's own vocabulary - string,
// number, boolean, object, array, null - for a value that has no pwrq type at
// all. Only the ideas were borrowed, and they are kept.
package typed

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"time"
)

// MemberType enumerates the kinds of member an Object can have.
//
// There is exactly one, and the enum is kept because the wire form names it.
// The model once carried script properties, alias properties, methods and
// events, and never used any of them: a member only reaches a caller by way of
// ToMap, which resolves it to a plain JSON value, so anything that is not
// simply a name and a value has nowhere to go.
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

// Member is a named property of an Object.
type Member struct {
	Name        string
	MemberType  MemberType
	Value       any
	Description string
}

// Object is a value carrying a type name and named properties.
//
// The type name is a key into the shape catalogue, not a class: a caller reads
// Pwrq.FileSystem.File off a result and asks get_command what properties that
// stands for. A value the catalogue has nothing to say about reports its JSON
// type instead, which is the honest answer rather than a borrowed one.
type Object struct {
	Value    any                // The underlying value
	TypeName string             // Pwrq.* for a cmdlet's output, otherwise the JSON type
	Members  map[string]*Member // Named properties
}

// New creates an Object, naming its type from the value.
func New(value any) *Object {
	return &Object{
		Value:    value,
		TypeName: jsonTypeOf(value),
		Members:  make(map[string]*Member),
	}
}

// NewWithType creates an Object with an explicit type name.
func NewWithType(value any, typeName string) *Object {
	return &Object{
		Value:    value,
		TypeName: typeName,
		Members:  make(map[string]*Member),
	}
}

// jsonTypeOf names the JSON type a Go value will present as once it reaches the
// pipeline.
//
// It answers in jq's vocabulary - the same one shape.JSONType uses - because
// that is what the caller actually sees. A value here has no pwrq type: it was
// not built by a cmdlet, so there is no catalogue entry to point at, and
// inventing a name for it would put something in the type space that nothing
// can look up. Saying "string" says everything there is to say.
//
// The answer must match what NormalizeJSON will produce, which is why a
// time.Time is a string and a []byte is too.
func jsonTypeOf(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string, []byte, time.Time, time.Duration:
		return "string"
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, *big.Int, json.Number:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	}

	// Named types over JSON-compatible kinds, and typed slices and maps, reach
	// the pipeline as whatever NormalizeJSON's reflection fallback makes of
	// them. Asking the same question of the kind keeps the two answers aligned.
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map:
		return "object"
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return "null"
		}
		return jsonTypeOf(rv.Elem().Interface())
	default:
		// Structs and anything else are stringified rather than leaked.
		return "string"
	}
}

// AddMember adds a member to the Object.
func (p *Object) AddMember(name string, memberType MemberType, value any) {
	p.Members[name] = &Member{
		Name:       name,
		MemberType: memberType,
		Value:      value,
	}
}

// AddNoteProperty adds a simple property to the Object.
func (p *Object) AddNoteProperty(name string, value any) {
	p.AddMember(name, MemberTypeNoteProperty, value)
}

// GetMember retrieves a member by name.
func (p *Object) GetMember(name string) (*Member, bool) {
	member, ok := p.Members[name]
	return member, ok
}

// GetPropertyValue retrieves a property value by name.
func (p *Object) GetPropertyValue(name string) (any, error) {
	member, ok := p.Members[name]
	if !ok {
		return nil, fmt.Errorf("member %q not found", name)
	}
	return member.Value, nil
}

// TypeKey is the property under which an Object's type name travels on the
// wire, and the foreign key a caller resolves against the shape catalogue.
//
// It is a bare identifier so that .PwrqType works in a jq path expression
// without quoting, and prefixed so that it cannot collide with a key from the
// caller's own data - which a plain Type or TypeName would, get_command's own
// output being the first casualty.
const TypeKey = "PwrqType"

// ValueKey is the property under which an Object's underlying scalar travels
// when the object also carries named properties. It is what the next cmdlet in
// a pipeline binds to: a file item's path, a table object's database.
const ValueKey = "PwrqValue"

// ToJSON converts the Object to its wire representation: ordinary JSON.
//
// An object with named properties becomes a flat map whose keys are the
// property names, plus PwrqType. An object with no properties is just its
// underlying value - wrapping a scalar in an envelope would make every
// downstream jq expression pay for metadata it did not ask for.
//
// The result contains only JSON types (nil, bool, int, float64, *big.Int,
// string, []any, map[string]any), so it can be queried by jq and printed by the
// encoder without special cases.
func (p *Object) ToJSON() any {
	if len(p.Members) == 0 {
		return NormalizeJSON(p.Value)
	}
	return p.ToMap()
}

// ToMap converts the Object to its flat JSON object form.
func (p *Object) ToMap() map[string]any {
	result := make(map[string]any, len(p.Members)+2)

	// An Object wrapping a map exposes that map's entries as properties.
	// Without this, wrapping an object to attach one computed property would
	// discard the object.
	if underlying, ok := p.Value.(map[string]any); ok {
		for k, v := range underlying {
			result[k] = NormalizeJSON(v)
		}
	}

	// Members win over the underlying map: they are what was added on purpose.
	for name, member := range p.Members {
		result[name] = NormalizeJSON(member.Value)
	}

	// Preserve the underlying scalar so by-value pipeline binding still works
	// for objects whose value is meaningful (a path, a name).
	if _, taken := result[ValueKey]; !taken {
		if s, ok := p.Value.(string); ok && s != "" {
			result[ValueKey] = s
		}
	}

	if p.TypeName != "" {
		result[TypeKey] = p.TypeName
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
	case *Object:
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

// FromMap creates an Object from its flat JSON wire form. Every key other than
// PwrqType becomes a NoteProperty; PwrqType supplies the type. Any JSON object
// is therefore a valid Object, which is what lets cmdlets accept hand-written
// JSON as readily as cmdlet output.
func FromMap(m map[string]any) (*Object, error) {
	if m == nil {
		return nil, fmt.Errorf("cannot create an object from a nil map")
	}

	typeName, _ := m[TypeKey].(string)
	if typeName == "" {
		typeName = "object"
	}

	// The underlying value is the object's path when it carries one; otherwise
	// the map itself stands in as the value.
	var value any = m
	if s, ok := m[ValueKey].(string); ok {
		value = s
	}

	obj := NewWithType(value, typeName)
	for name, v := range m {
		if name == TypeKey {
			continue
		}
		obj.AddNoteProperty(name, v)
	}
	return obj, nil
}

// ConvertValue converts an Object's value to another type.
//
// The target names are the conversions on offer, and the resulting type name is
// what the value then *is*: int, long and double are three ways of reaching a
// number and all three produce one, because that is the only numeric type the
// pipeline has. Naming the result after the conversion would put int in the
// type space, where nothing could look it up.
func ConvertValue(obj *Object, targetType string) (*Object, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot convert a nil object")
	}

	var newValue any
	var typeName string
	var err error

	switch targetType {
	case "string":
		newValue, typeName = fmt.Sprintf("%v", obj.Value), "string"
	case "int", "integer":
		newValue, err = convertToInt(obj.Value)
		typeName = "number"
	case "long":
		newValue, err = convertToInt64(obj.Value)
		typeName = "number"
	case "double", "float", "number":
		newValue, err = convertToFloat64(obj.Value)
		typeName = "number"
	case "bool", "boolean":
		newValue, err = convertToBool(obj.Value)
		typeName = "boolean"
	case "datetime":
		newValue, err = convertToDateTime(obj.Value)
		typeName = "Pwrq.DateTime"
	default:
		return nil, fmt.Errorf("unsupported target type: %s", targetType)
	}

	if err != nil {
		return nil, err
	}

	return NewWithType(newValue, typeName), nil
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

// Is reports whether a value is an Object or carries a PwrqType.
//
// Note that a plain JSON object is not one by this test even though FromMap
// accepts one; the distinction is "was this produced by a cmdlet", which only
// PwrqType can answer.
func Is(v any) bool {
	if _, ok := v.(*Object); ok {
		return true
	}
	if m, ok := v.(map[string]any); ok {
		_, ok := m[TypeKey].(string)
		return ok
	}
	return false
}

// ValueOf extracts the underlying value from an Object or its wire form.
func ValueOf(v any) any {
	switch val := v.(type) {
	case *Object:
		return val.Value
	case map[string]any:
		if s, ok := val[ValueKey].(string); ok {
			return s
		}
	}
	return v
}

// TypeOf extracts the type name from an Object or its wire form, falling back
// to the JSON type of a value that has none.
func TypeOf(v any) string {
	switch val := v.(type) {
	case *Object:
		return val.TypeName
	case map[string]any:
		if t, ok := val[TypeKey].(string); ok {
			return t
		}
	}
	return jsonTypeOf(v)
}
