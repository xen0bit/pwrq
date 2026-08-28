package typed

import (
	"encoding/json"
	"math/big"
	"reflect"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"string", "hello", "string"},
		{"int", 42, "number"},
		{"float", 3.14, "number"},
		{"bool", true, "boolean"},
		{"nil", nil, "null"},
		{"map", map[string]any{"key": "value"}, "object"},
		{"slice", []any{1, 2, 3}, "array"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := New(tt.value)
			if obj.TypeName != tt.expected {
				t.Errorf("Expected type %s, got %s", tt.expected, obj.TypeName)
			}
			// Skip direct value comparison for maps/slices (not comparable)
			switch tt.value.(type) {
			case map[string]any, []any:
				// Skip comparison for uncomparable types
			default:
				if obj.Value != tt.value {
					t.Errorf("Expected value %v, got %v", tt.value, obj.Value)
				}
			}
			if obj.Members == nil {
				t.Error("Members map should not be nil")
			}
		})
	}
}

func TestAddNoteProperty(t *testing.T) {
	obj := New("test")
	obj.AddNoteProperty("Name", "value")

	member, ok := obj.GetMember("Name")
	if !ok {
		t.Fatal("Member 'Name' not found")
	}
	if member.MemberType != MemberTypeNoteProperty {
		t.Errorf("Expected MemberTypeNoteProperty, got %s", member.MemberType)
	}
	if member.Value != "value" {
		t.Errorf("Expected value 'value', got %v", member.Value)
	}
}

func TestGetPropertyValue(t *testing.T) {
	obj := New("test")
	obj.AddNoteProperty("Prop1", "value1")

	val, err := obj.GetPropertyValue("Prop1")
	if err != nil {
		t.Fatalf("GetPropertyValue failed: %v", err)
	}
	if val != "value1" {
		t.Errorf("Expected 'value1', got %v", val)
	}

	_, err = obj.GetPropertyValue("NonExistent")
	if err == nil {
		t.Error("Expected error for non-existent member")
	}
}

func TestConvertValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		target   string
		expected any
	}{
		{"string to int", "42", "int", 42},
		{"float to int", 3.9, "int", 3},
		{"string to number", "3.5", "number", 3.5},
		{"int to string", 42, "string", "42"},
		{"string to bool", "true", "bool", true},
		{"int to bool", 1, "bool", true},
		{"int to bool false", 0, "bool", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := New(tt.input)
			result, err := ConvertValue(obj, tt.target)
			if err != nil {
				t.Fatalf("ConvertValue failed: %v", err)
			}
			if result.Value != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result.Value)
			}
		})
	}
}

func TestIs(t *testing.T) {
	obj := New("test")
	if !Is(obj) {
		t.Error("Is should return true for a typed object")
	}

	m := obj.ToMap()
	if !Is(m) {
		t.Error("Is should return true for an object carrying PwrqType")
	}

	if Is("not a typed object") {
		t.Error("Is should return false for a string")
	}
}

func TestConvertToDateTime(t *testing.T) {
	now := time.Now()
	obj := New(now.Format(time.RFC3339))
	result, err := ConvertValue(obj, "datetime")
	if err != nil {
		t.Fatalf("ConvertValue failed: %v", err)
	}
	if _, ok := result.Value.(time.Time); !ok {
		t.Errorf("Expected time.Time, got %T", result.Value)
	}
}

// The tests below specify pwrq's object wire format: a typed object travels as
// ordinary JSON so that jq can query it and the encoder can print it without
// knowing anything about PowerShell.

func TestToJSONScalarHasNoEnvelope(t *testing.T) {
	// A object with no properties is just its value. Wrapping it would make
	// every downstream expression pay for metadata it did not ask for.
	for _, value := range []any{"hello", 42, true, nil} {
		if got := New(value).ToJSON(); !reflect.DeepEqual(got, value) {
			t.Errorf("ToJSON(%v) = %#v, want the bare value", value, got)
		}
	}
}

func TestToMapFlattensProperties(t *testing.T) {
	obj := NewWithType("/tmp/x.txt", "Pwrq.FileSystem.File")
	obj.AddNoteProperty("Name", "x.txt")
	obj.AddNoteProperty("Length", 128)

	got := obj.ToMap()

	if got["Name"] != "x.txt" || got["Length"] != 128 {
		t.Errorf("properties should be top-level keys, got %#v", got)
	}
	if got[TypeKey] != "Pwrq.FileSystem.File" {
		t.Errorf("%s = %v, want Pwrq.FileSystem.File", TypeKey, got[TypeKey])
	}
	if got[ValueKey] != "/tmp/x.txt" {
		t.Errorf("%s = %v, want the underlying value", ValueKey, got[ValueKey])
	}
	for _, retired := range []string{"_val", "_meta", "_err"} {
		if _, present := got[retired]; present {
			t.Errorf("wire format must not carry %q", retired)
		}
	}
}

func TestToMapIsJSONEncodable(t *testing.T) {
	// Whatever ToMap produces has to survive the encoder, which only knows
	// JSON types.
	obj := New("base")
	obj.AddNoteProperty("When", time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC))
	obj.AddNoteProperty("Size", int64(99))
	obj.AddNoteProperty("Raw", []byte("bytes"))

	got := obj.ToMap()
	if got["When"] != "2026-08-07T12:00:00Z" {
		t.Errorf("When = %#v, want an RFC3339 string", got["When"])
	}
	if got["Size"] != 99 {
		t.Errorf("Size = %#v (%T), want int 99", got["Size"], got["Size"])
	}
	if got["Raw"] != "bytes" {
		t.Errorf("Raw = %#v, want a string", got["Raw"])
	}
	if _, err := json.Marshal(got); err != nil {
		t.Errorf("ToMap output must be JSON-encodable: %v", err)
	}
}

func TestFromMapAcceptsPlainJSON(t *testing.T) {
	// Any JSON object is a valid object; that is what lets cmdlets accept
	// hand-written JSON as readily as cmdlet output.
	obj, err := FromMap(map[string]any{"Name": "x", "Length": 3})
	if err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	if v, err := obj.GetPropertyValue("Name"); err != nil || v != "x" {
		t.Errorf("Name = %#v (err %v), want x", v, err)
	}
	if obj.TypeName != "object" {
		t.Errorf("untyped JSON should report the object type, got %s", obj.TypeName)
	}
}

func TestRoundTrip(t *testing.T) {
	obj := NewWithType("/tmp/x.txt", "Pwrq.FileSystem.File")
	obj.AddNoteProperty("Name", "x.txt")
	obj.AddNoteProperty("Length", 128)

	back, err := FromMap(obj.ToMap())
	if err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	if back.TypeName != "Pwrq.FileSystem.File" {
		t.Errorf("TypeName lost in round-trip: %s", back.TypeName)
	}
	if v, _ := back.GetPropertyValue("Length"); v != 128 {
		t.Errorf("Length lost in round-trip: %#v", v)
	}
	if back.Value != "/tmp/x.txt" {
		t.Errorf("underlying value lost in round-trip: %#v", back.Value)
	}
}

func TestIsRequiresPwrqType(t *testing.T) {
	// A plain JSON object is not cmdlet output, even though FromMap accepts it.
	if Is(map[string]any{"Name": "x"}) {
		t.Error("plain JSON should not be reported as a typed object")
	}
	if !Is(map[string]any{TypeKey: "string"}) {
		t.Error("an object carrying PwrqType is a typed object")
	}
	if Is(map[string]any{"_val": 1, "_meta": map[string]any{}}) {
		t.Error("the retired envelope must no longer be recognised")
	}
}

func TestNormalizeJSONProducesJQValues(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"int64", int64(5), 5},
		{"uint8", uint8(5), 5},
		{"float32", float32(1.5), 1.5},
		{"bytes", []byte("hi"), "hi"},
		{"duration", 90 * time.Second, "1m30s"},
		{"bigint", big.NewInt(7), big.NewInt(7)},
		{"nil", nil, nil},
		{"nested", map[string]any{"a": []any{int64(1)}}, map[string]any{"a": []any{1}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeJSON(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("NormalizeJSON(%#v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}
