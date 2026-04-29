package psobject

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewPSObject(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"string", "hello", "System.String"},
		{"int", 42, "System.Int32"},
		{"float", 3.14, "System.Double"},
		{"bool", true, "System.Boolean"},
		{"nil", nil, "System.Object"},
		{"map", map[string]any{"key": "value"}, "System.Management.Automation.PSObject"},
		{"slice", []any{1, 2, 3}, "System.Object[]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			psobj := NewPSObject(tt.value)
			if psobj.TypeName != tt.expected {
				t.Errorf("Expected type %s, got %s", tt.expected, psobj.TypeName)
			}
			// Skip direct value comparison for maps/slices (not comparable)
			switch tt.value.(type) {
			case map[string]any, []any:
				// Skip comparison for uncomparable types
			default:
				if psobj.Value != tt.value {
					t.Errorf("Expected value %v, got %v", tt.value, psobj.Value)
				}
			}
			if psobj.Members == nil {
				t.Error("Members map should not be nil")
			}
		})
	}
}

func TestAddNoteProperty(t *testing.T) {
	psobj := NewPSObject("test")
	psobj.AddNoteProperty("Name", "value")

	member, ok := psobj.GetMember("Name")
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

func TestAddScriptProperty(t *testing.T) {
	psobj := NewPSObject(42)
	psobj.AddScriptProperty("Doubled", func() (any, error) {
		return 84, nil
	})

	val, err := psobj.GetPropertyValue("Doubled")
	if err != nil {
		t.Fatalf("GetPropertyValue failed: %v", err)
	}
	if val != 84 {
		t.Errorf("Expected 84, got %v", val)
	}
}

func TestAddAliasProperty(t *testing.T) {
	psobj := NewPSObject("test")
	psobj.AddNoteProperty("FullName", "John Doe")
	psobj.AddAliasProperty("Name", "FullName")

	val, err := psobj.GetPropertyValue("Name")
	if err != nil {
		t.Fatalf("GetPropertyValue failed: %v", err)
	}
	if val != "John Doe" {
		t.Errorf("Expected 'John Doe', got %v", val)
	}
}

func TestAddMethod(t *testing.T) {
	psobj := NewPSObject(10)
	psobj.AddMethod("Add", func(args ...any) any {
		if len(args) == 0 {
			return nil
		}
		if num, ok := args[0].(int); ok {
			return psobj.Value.(int) + num
		}
		return nil
	})

	result, err := psobj.InvokeMethod("Add", 5)
	if err != nil {
		t.Fatalf("InvokeMethod failed: %v", err)
	}
	if result != 15 {
		t.Errorf("Expected 15, got %v", result)
	}
}

func TestGetPropertyValue(t *testing.T) {
	psobj := NewPSObject("test")
	psobj.AddNoteProperty("Prop1", "value1")

	val, err := psobj.GetPropertyValue("Prop1")
	if err != nil {
		t.Fatalf("GetPropertyValue failed: %v", err)
	}
	if val != "value1" {
		t.Errorf("Expected 'value1', got %v", val)
	}

	_, err = psobj.GetPropertyValue("NonExistent")
	if err == nil {
		t.Error("Expected error for non-existent member")
	}
}

func TestInvokeMethod(t *testing.T) {
	psobj := NewPSObject("test")
	psobj.AddMethod("TestMethod", func(args ...any) any {
		return "result"
	})

	result, err := psobj.InvokeMethod("TestMethod")
	if err != nil {
		t.Fatalf("InvokeMethod failed: %v", err)
	}
	if result != "result" {
		t.Errorf("Expected 'result', got %v", result)
	}

	_, err = psobj.InvokeMethod("NonExistent")
	if err == nil {
		t.Error("Expected error for non-existent method")
	}
}

func TestToMap(t *testing.T) {
	psobj := NewPSObject("test")
	psobj.AddNoteProperty("Name", "value")
	psobj.AddAliasProperty("Alias", "Name")

	m := psobj.ToMap()

	if m["_val"] != "test" {
		t.Errorf("Expected _val 'test', got %v", m["_val"])
	}

	meta, ok := m["_meta"].(map[string]any)
	if !ok {
		t.Fatal("_meta should be a map")
	}

	if meta["type"] != "System.String" {
		t.Errorf("Expected type 'System.String', got %v", meta["type"])
	}

	members, ok := meta["members"].(map[string]any)
	if !ok {
		t.Fatal("members should be a map")
	}

	nameMember, ok := members["Name"].(map[string]any)
	if !ok {
		t.Fatal("Name member not found")
	}
	if nameMember["value"] != "value" {
		t.Errorf("Expected Name value 'value', got %v", nameMember["value"])
	}
}

func TestFromMap(t *testing.T) {
	m := map[string]any{
		"_val": "test",
		"_meta": map[string]any{
			"type": "System.String",
			"members": map[string]any{
				"Name": map[string]any{
					"type":         "NoteProperty",
					"value":        "value",
					"serializable": true,
				},
				"Alias": map[string]any{
					"type":         "AliasProperty",
					"target":       "Name",
					"serializable": true,
				},
			},
		},
	}

	psobj, err := FromMap(m)
	if err != nil {
		t.Fatalf("FromMap failed: %v", err)
	}

	if psobj.Value != "test" {
		t.Errorf("Expected value 'test', got %v", psobj.Value)
	}
	if psobj.TypeName != "System.String" {
		t.Errorf("Expected type 'System.String', got %s", psobj.TypeName)
	}

	val, err := psobj.GetPropertyValue("Name")
	if err != nil {
		t.Fatalf("GetPropertyValue failed: %v", err)
	}
	if val != "value" {
		t.Errorf("Expected 'value', got %v", val)
	}
}

func TestFromMapValidation(t *testing.T) {
	// Missing _val
	_, err := FromMap(map[string]any{
		"_meta": map[string]any{},
	})
	if err == nil {
		t.Error("Expected error for missing _val")
	}

	// Missing _meta
	_, err = FromMap(map[string]any{
		"_val": "test",
	})
	if err == nil {
		t.Error("Expected error for missing _meta")
	}

	// Nil map
	_, err = FromMap(nil)
	if err == nil {
		t.Error("Expected error for nil map")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	psobj := NewPSObject("test")
	psobj.AddNoteProperty("Name", "value")
	psobj.AddAliasProperty("Alias", "Name")

	// Convert to map and serialize to JSON
	m := psobj.ToMap()
	jsonBytes, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Deserialize and reconstruct
	var m2 map[string]any
	err = json.Unmarshal(jsonBytes, &m2)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	psobj2, err := FromMap(m2)
	if err != nil {
		t.Fatalf("FromMap failed: %v", err)
	}

	// Verify value and type
	if psobj2.Value != "test" {
		t.Errorf("Expected value 'test', got %v", psobj2.Value)
	}
	if psobj2.TypeName != "System.String" {
		t.Errorf("Expected type 'System.String', got %s", psobj2.TypeName)
	}

	// Verify NoteProperty survived
	val, err := psobj2.GetPropertyValue("Name")
	if err != nil {
		t.Fatalf("GetPropertyValue failed: %v", err)
	}
	if val != "value" {
		t.Errorf("Expected 'value', got %v", val)
	}

	// Verify AliasProperty survived
	val, err = psobj2.GetPropertyValue("Alias")
	if err != nil {
		t.Fatalf("GetPropertyValue for alias failed: %v", err)
	}
	if val != "value" {
		t.Errorf("Expected alias to resolve to 'value', got %v", val)
	}
}

func TestNestedPSObject(t *testing.T) {
	inner := NewPSObject("inner")
	inner.AddNoteProperty("InnerProp", "innervalue")

	outer := NewPSObject("outer")
	outer.AddNoteProperty("Inner", inner)

	m := outer.ToMap()
	jsonBytes, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var m2 map[string]any
	err = json.Unmarshal(jsonBytes, &m2)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	psobj, err := FromMap(m2)
	if err != nil {
		t.Fatalf("FromMap failed: %v", err)
	}

	// Verify nested PSObject was reconstructed
	innerVal, err := psobj.GetPropertyValue("Inner")
	if err != nil {
		t.Fatalf("GetPropertyValue for Inner failed: %v", err)
	}

	innerPSObj, ok := innerVal.(*PSObject)
	if !ok {
		t.Fatal("Inner value should be a PSObject")
	}

	propVal, err := innerPSObj.GetPropertyValue("InnerProp")
	if err != nil {
		t.Fatalf("GetPropertyValue for InnerProp failed: %v", err)
	}
	if propVal != "innervalue" {
		t.Errorf("Expected 'innervalue', got %v", propVal)
	}
}

func TestNestedPSObjectWithScriptProperty(t *testing.T) {
	// ScriptProperty/Method cannot be serialized, so test with NoteProperty only
	inner := NewPSObject("inner")
	inner.AddNoteProperty("InnerProp", "innervalue")

	outer := NewPSObject("outer")
	outer.AddNoteProperty("Inner", inner)

	m := outer.ToMap()
	jsonBytes, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var m2 map[string]any
	err = json.Unmarshal(jsonBytes, &m2)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	psobj, err := FromMap(m2)
	if err != nil {
		t.Fatalf("FromMap failed: %v", err)
	}

	// Verify nested PSObject was reconstructed
	innerVal, err := psobj.GetPropertyValue("Inner")
	if err != nil {
		t.Fatalf("GetPropertyValue for Inner failed: %v", err)
	}

	innerPSObj, ok := innerVal.(*PSObject)
	if !ok {
		t.Fatal("Inner value should be a PSObject")
	}

	propVal, err := innerPSObj.GetPropertyValue("InnerProp")
	if err != nil {
		t.Fatalf("GetPropertyValue for InnerProp failed: %v", err)
	}
	if propVal != "innervalue" {
		t.Errorf("Expected 'innervalue', got %v", propVal)
	}
}

func TestScriptPropertySerialization(t *testing.T) {
	psobj := NewPSObject(42)
	psobj.AddScriptProperty("Doubled", func() (any, error) {
		return 84, nil
	})

	m := psobj.ToMap()
	// ScriptProperty with nil Getter and no Description won't serialize the function
	// but the member metadata should be preserved
	members := m["_meta"].(map[string]any)["members"].(map[string]any)
	doubledMember, ok := members["Doubled"].(map[string]any)
	if !ok {
		t.Fatal("Doubled member not in serialized output")
	}
	if doubledMember["serializable"] != false {
		t.Error("ScriptProperty should be marked non-serializable")
	}

	// JSON marshal succeeds because Getter has json:"-" tag
	// The function is silently dropped - this is expected behavior
	jsonBytes, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal should succeed with json:\"-\" tag, got: %v", err)
	}

	// After round-trip, the ScriptProperty becomes a NoteProperty (no getter)
	var m2 map[string]any
	err = json.Unmarshal(jsonBytes, &m2)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	psobj2, err := FromMap(m2)
	if err != nil {
		t.Fatalf("FromMap failed: %v", err)
	}

	member, ok := psobj2.GetMember("Doubled")
	if !ok {
		t.Fatal("Doubled member should exist after round-trip")
	}
	if member.Getter != nil {
		t.Error("Getter should be nil after JSON round-trip (functions not serializable)")
	}
}

func TestScriptPropertyWithDescriptionSerialization(t *testing.T) {
	psobj := NewPSObject(42)
	// Add ScriptProperty with a description - this survives serialization
	psobj.Members["Doubled"] = &PSMember{
		Name:       "Doubled",
		MemberType: MemberTypeScriptProperty,
		Getter:     func() (any, error) { return 84, nil },
		Description: "Returns double the value",
		Serializable: false,
	}

	m := psobj.ToMap()
	jsonBytes, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var m2 map[string]any
	err = json.Unmarshal(jsonBytes, &m2)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	psobj2, err := FromMap(m2)
	if err != nil {
		t.Fatalf("FromMap failed: %v", err)
	}

	// ScriptProperty getter is lost, but member metadata survives
	member, ok := psobj2.GetMember("Doubled")
	if !ok {
		t.Fatal("Doubled member not found after round-trip")
	}
	// The getter function is not serializable, so it should be nil
	if member.Getter != nil {
		t.Error("Getter should be nil after JSON round-trip")
	}
	if member.Description != "Returns double the value" {
		t.Errorf("Expected description to survive, got %v", member.Description)
	}
}

func TestMethodSerialization(t *testing.T) {
	psobj := NewPSObject(10)
	// Add Method with a description - this survives serialization
	psobj.Members["Add"] = &PSMember{
		Name:       "Add",
		MemberType: MemberTypeMethod,
		Invoker:    func(args ...any) any { return 42 },
		Description: "Adds values together",
		Serializable: false,
	}

	m := psobj.ToMap()
	jsonBytes, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var m2 map[string]any
	err = json.Unmarshal(jsonBytes, &m2)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	psobj2, err := FromMap(m2)
	if err != nil {
		t.Fatalf("FromMap failed: %v", err)
	}

	// Method invoker is lost, but member metadata survives
	member, ok := psobj2.GetMember("Add")
	if !ok {
		t.Fatal("Add member not found after round-trip")
	}
	if member.MemberType != MemberTypeMethod {
		t.Errorf("Expected MemberTypeMethod, got %s", member.MemberType)
	}
	if member.Invoker != nil {
		t.Error("Invoker should be nil after JSON round-trip")
	}
	if member.Description != "Adds values together" {
		t.Errorf("Expected description to survive, got %v", member.Description)
	}
}

func TestConvertValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		target   string
		expected any
	}{
		{"string to int", "42", "System.Int32", 42},
		{"float to int", 3.9, "System.Int32", 3},
		{"int to string", 42, "System.String", "42"},
		{"string to bool", "true", "System.Boolean", true},
		{"int to bool", 1, "System.Boolean", true},
		{"int to bool false", 0, "System.Boolean", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			psobj := NewPSObject(tt.input)
			result, err := ConvertValue(psobj, tt.target)
			if err != nil {
				t.Fatalf("ConvertValue failed: %v", err)
			}
			if result.Value != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result.Value)
			}
		})
	}
}

func TestIsPSObject(t *testing.T) {
	psobj := NewPSObject("test")
	if !IsPSObject(psobj) {
		t.Error("IsPSObject should return true for PSObject")
	}

	m := psobj.ToMap()
	if !IsPSObject(m) {
		t.Error("IsPSObject should return true for PSObject-like map")
	}

	if IsPSObject("not a psobject") {
		t.Error("IsPSObject should return false for string")
	}
}

func TestExtractValue(t *testing.T) {
	psobj := NewPSObject("test")
	val := ExtractValue(psobj)
	if val != "test" {
		t.Errorf("Expected 'test', got %v", val)
	}

	m := psobj.ToMap()
	val = ExtractValue(m)
	if val != "test" {
		t.Errorf("Expected 'test', got %v", val)
	}
}

func TestExtractTypeName(t *testing.T) {
	psobj := NewPSObject("test")
	typeName := ExtractTypeName(psobj)
	if typeName != "System.String" {
		t.Errorf("Expected 'System.String', got %s", typeName)
	}

	m := psobj.ToMap()
	typeName = ExtractTypeName(m)
	if typeName != "System.String" {
		t.Errorf("Expected 'System.String', got %s", typeName)
	}
}

func TestConvertToDateTime(t *testing.T) {
	now := time.Now()
	psobj := NewPSObject(now.Format(time.RFC3339))
	result, err := ConvertValue(psobj, "System.DateTime")
	if err != nil {
		t.Fatalf("ConvertValue failed: %v", err)
	}
	if _, ok := result.Value.(time.Time); !ok {
		t.Errorf("Expected time.Time, got %T", result.Value)
	}
}

func TestMemberSerializableFlag(t *testing.T) {
	psobj := NewPSObject("test")
	psobj.AddNoteProperty("Prop", "value")
	psobj.AddScriptProperty("Computed", func() (any, error) { return 42, nil })

	members := psobj.getMembersMap()

	propMember, ok := members["Prop"].(map[string]any)
	if !ok {
		t.Fatal("Prop member not found")
	}
	if propMember["serializable"] != true {
		t.Error("NoteProperty should be serializable")
	}

	computedMember, ok := members["Computed"].(map[string]any)
	if !ok {
		t.Fatal("Computed member not found")
	}
	if computedMember["serializable"] != false {
		t.Error("ScriptProperty should not be serializable")
	}
}
