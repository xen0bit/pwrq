package objects

import (
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

func TestSelectObject_Basic(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := SelectObjectOptions{First: -1, Last: -1, Skip: -1}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("Expected 3 objects, got %d", len(result))
	}
}

func TestSelectObject_First(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := SelectObjectOptions{First: 2, Last: -1, Skip: -1}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}

	// Verify first object is Alice
	if m, ok := result[0].(map[string]any); !ok || m["Name"] != "Alice" {
		t.Errorf("Expected first object to be Alice, got %v", result[0])
	}
}

func TestSelectObject_Last(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := SelectObjectOptions{First: -1, Last: 2, Skip: -1}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}

	// Verify last object is Charlie
	if m, ok := result[1].(map[string]any); !ok || m["Name"] != "Charlie" {
		t.Errorf("Expected last object to be Charlie, got %v", result[1])
	}
}

func TestSelectObject_Skip(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := SelectObjectOptions{First: -1, Last: -1, Skip: 1}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}

	// Verify first object after skip is Bob
	if m, ok := result[0].(map[string]any); !ok || m["Name"] != "Bob" {
		t.Errorf("Expected first object after skip to be Bob, got %v", result[0])
	}
}

func TestSelectObject_Property(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30, "City": "NYC"},
		map[string]any{"Name": "Bob", "Age": 25, "City": "LA"},
	}

	opts := SelectObjectOptions{
		First:      -1,
		Last:       -1,
		Skip:       -1,
		Properties: []string{"Name", "Age"},
	}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}

	// Verify only selected properties are present
	for i, obj := range result {
		m, ok := obj.(map[string]any)
		if !ok {
			t.Errorf("Object %d is not a map", i)
			continue
		}
		if _, hasName := m["Name"]; !hasName {
			t.Errorf("Object %d missing Name property", i)
		}
		if _, hasAge := m["Age"]; !hasAge {
			t.Errorf("Object %d missing Age property", i)
		}
		if _, hasCity := m["City"]; hasCity {
			t.Errorf("Object %d should not have City property", i)
		}
	}
}

func TestSelectObject_FirstAndLastConflict(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice"},
	}

	opts := SelectObjectOptions{First: 1, Last: 1, Skip: -1}
	_, err := selectObject(objects, opts)
	if err == nil {
		t.Error("Expected error when using both -First and -Last")
	}
}

func TestSelectObject_EmptyInput(t *testing.T) {
	objects := []any{}

	opts := SelectObjectOptions{First: -1, Last: -1, Skip: -1}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected 0 objects, got %d", len(result))
	}
}

func TestSelectObject_SkipAll(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice"},
		map[string]any{"Name": "Bob"},
	}

	opts := SelectObjectOptions{First: -1, Last: -1, Skip: 10}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected 0 objects after skipping more than available, got %d", len(result))
	}
}

func TestSelectObject_PropertiesFromPSObject(t *testing.T) {
	// Create PSObjects with members
	psobj1 := psobject.NewPSObject(map[string]any{"Name": "Alice", "Age": 30})
	psobj1.AddNoteProperty("Name", "Alice")
	psobj1.AddNoteProperty("Age", 30)

	psobj2 := psobject.NewPSObject(map[string]any{"Name": "Bob", "Age": 25})
	psobj2.AddNoteProperty("Name", "Bob")
	psobj2.AddNoteProperty("Age", 25)

	objects := []any{psobj1, psobj2}

	opts := SelectObjectOptions{
		First:      -1,
		Last:       -1,
		Skip:       -1,
		Properties: []string{"Name"},
	}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}

	// Verify only Name property is selected
	for i, obj := range result {
		// Result is a PSObject-like map with _val containing the actual data
		m, ok := obj.(map[string]any)
		if !ok {
			t.Errorf("Object %d is not a map", i)
			continue
		}
		// Extract the actual value from PSObject wrapper
		val := m["_val"]
		valMap, ok := val.(map[string]any)
		if !ok {
			t.Errorf("Object %d _val is not a map", i)
			continue
		}
		if _, hasName := valMap["Name"]; !hasName {
			t.Errorf("Object %d missing Name property in _val", i)
		}
		if _, hasAge := valMap["Age"]; hasAge {
			t.Errorf("Object %d should not have Age property in _val", i)
		}
	}
}

func TestNormalizeToSlice(t *testing.T) {
	// Test nil input
	result := normalizeToSlice(nil)
	if len(result) != 0 {
		t.Errorf("Expected empty slice for nil input, got %d items", len(result))
	}

	// Test slice input
	input := []any{1, 2, 3}
	result = normalizeToSlice(input)
	if len(result) != 3 {
		t.Errorf("Expected 3 items, got %d", len(result))
	}

	// Test single map input
	singleMap := map[string]any{"key": "value"}
	result = normalizeToSlice(singleMap)
	if len(result) != 1 {
		t.Errorf("Expected 1 item for single map, got %d", len(result))
	}

	// Test single value input
	result = normalizeToSlice("hello")
	if len(result) != 1 {
		t.Errorf("Expected 1 item for single value, got %d", len(result))
	}
	if result[0] != "hello" {
		t.Errorf("Expected 'hello', got %v", result[0])
	}
}

func TestParseProperties(t *testing.T) {
	// Test string array
	input := []any{"Name", "Age", "City"}
	result := parseProperties(input)
	if len(result) != 3 {
		t.Errorf("Expected 3 properties, got %d", len(result))
	}

	// Test comma-separated string
	input2 := "Name, Age, City"
	result2 := parseProperties(input2)
	if len(result2) != 3 {
		t.Errorf("Expected 3 properties, got %d", len(result2))
	}

	// Test single string
	input3 := "Name"
	result3 := parseProperties(input3)
	if len(result3) != 1 {
		t.Errorf("Expected 1 property, got %d", len(result3))
	}
}

func TestParseSelectObjectArgs(t *testing.T) {
	args := []any{
		[]any{
			map[string]any{"Name": "Alice"},
			map[string]any{"Name": "Bob"},
		},
		map[string]any{
			"first":    float64(1),
			"property": []any{"Name"},
		},
	}

	objects, opts, err := ParseSelectObjectArgs(args)
	if err != nil {
		t.Fatalf("ParseSelectObjectArgs failed: %v", err)
	}

	if len(objects) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(objects))
	}

	if opts.First != 1 {
		t.Errorf("Expected First=1, got %d", opts.First)
	}

	if len(opts.Properties) != 1 || opts.Properties[0] != "Name" {
		t.Errorf("Expected Properties=[Name], got %v", opts.Properties)
	}
}

func TestSelectObject_WildcardProperties(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Namespace": "Default", "Age": 30, "City": "NYC"},
		map[string]any{"Name": "Bob", "Namespace": "Admin", "Age": 25, "City": "LA"},
	}

	opts := SelectObjectOptions{
		First:      -1,
		Last:       -1,
		Skip:       -1,
		Properties: []string{"Na*"}, // Wildcard: should match Name and Namespace
	}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}

	// Verify wildcard matched both Name and Namespace
	for i, obj := range result {
		m, ok := obj.(map[string]any)
		if !ok {
			t.Errorf("Object %d is not a map", i)
			continue
		}
		if _, hasName := m["Name"]; !hasName {
			t.Errorf("Object %d missing Name property (wildcard should match)", i)
		}
		if _, hasNamespace := m["Namespace"]; !hasNamespace {
			t.Errorf("Object %d missing Namespace property (wildcard should match)", i)
		}
		if _, hasAge := m["Age"]; hasAge {
			t.Errorf("Object %d should not have Age property (wildcard should not match)", i)
		}
		if _, hasCity := m["City"]; hasCity {
			t.Errorf("Object %d should not have City property (wildcard should not match)", i)
		}
	}
}

func TestSelectObject_PreservesTypeName(t *testing.T) {
	// Create PSObjects with explicit TypeName
	psobj1 := psobject.NewPSObjectWithTypeName(map[string]any{"Name": "Alice", "Age": 30}, "Custom.MyType")
	psobj1.AddNoteProperty("Name", "Alice")
	psobj1.AddNoteProperty("Age", 30)

	psobj2 := psobject.NewPSObjectWithTypeName(map[string]any{"Name": "Bob", "Age": 25}, "Custom.MyType")
	psobj2.AddNoteProperty("Name", "Bob")
	psobj2.AddNoteProperty("Age", 25)

	objects := []any{psobj1.ToMap(), psobj2.ToMap()}

	opts := SelectObjectOptions{
		First:      -1,
		Last:       -1,
		Skip:       -1,
		Properties: []string{"Name"},
	}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}

	// Verify TypeName is preserved
	for i, obj := range result {
		m, ok := obj.(map[string]any)
		if !ok {
			t.Errorf("Object %d is not a map", i)
			continue
		}
		meta, ok := m["_meta"].(map[string]any)
		if !ok {
			t.Errorf("Object %d missing _meta", i)
			continue
		}
		typeName, ok := meta["type"].(string)
		if !ok {
			t.Errorf("Object %d missing type in _meta", i)
			continue
		}
		if typeName != "Custom.MyType" {
			t.Errorf("Object %d: expected TypeName 'Custom.MyType', got '%s'", i, typeName)
		}
	}
}

func TestSelectObject_WildcardWithPSObject(t *testing.T) {
	// Create PSObject with multiple NoteProperties
	psobj := psobject.NewPSObjectWithTypeName(map[string]any{"Name": "Test", "Count": 42, "Enabled": true}, "Test.Type")
	psobj.AddNoteProperty("Name", "Test")
	psobj.AddNoteProperty("Count", 42)
	psobj.AddNoteProperty("Enabled", true)
	psobj.AddNoteProperty("Description", "A test object")

	objects := []any{psobj.ToMap()}

	opts := SelectObjectOptions{
		First:      -1,
		Last:       -1,
		Skip:       -1,
		Properties: []string{"*e*"}, // Wildcard: matches Name, Enabled, Description
	}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 object, got %d", len(result))
	}

	m, ok := result[0].(map[string]any)
	if !ok {
		t.Fatal("Result is not a map")
	}

	// Check _val for selected properties
	val, ok := m["_val"].(map[string]any)
	if !ok {
		t.Fatal("_val is not a map")
	}

	// Should have Name, Enabled, Description (all contain 'e')
	if _, hasName := val["Name"]; !hasName {
		t.Error("Missing Name (contains 'e')")
	}
	if _, hasEnabled := val["Enabled"]; !hasEnabled {
		t.Error("Missing Enabled (contains 'e')")
	}
	if _, hasDesc := val["Description"]; !hasDesc {
		t.Error("Missing Description (contains 'e')")
	}
	// Should NOT have Count (no 'e')
	if _, hasCount := val["Count"]; hasCount {
		t.Error("Should not have Count (no 'e')")
	}
}

func TestSelectObject_MultipleWildcards(t *testing.T) {
	objects := []any{
		map[string]any{"FirstName": "Alice", "LastName": "Smith", "Age": 30, "City": "NYC"},
	}

	opts := SelectObjectOptions{
		First:      -1,
		Last:       -1,
		Skip:       -1,
		Properties: []string{"First*", "Last*"}, // Multiple wildcards
	}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 object, got %d", len(result))
	}

	m, ok := result[0].(map[string]any)
	if !ok {
		t.Fatal("Result is not a map")
	}

	if _, hasFirst := m["FirstName"]; !hasFirst {
		t.Error("Missing FirstName")
	}
	if _, hasLast := m["LastName"]; !hasLast {
		t.Error("Missing LastName")
	}
	if _, hasAge := m["Age"]; hasAge {
		t.Error("Should not have Age")
	}
	if _, hasCity := m["City"]; hasCity {
		t.Error("Should not have City")
	}
}

func TestSelectObject_PositionalProperties(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30, "City": "NYC"},
		map[string]any{"Name": "Bob", "Age": 25, "City": "LA"},
	}

	opts := SelectObjectOptions{
		First:      -1,
		Last:       -1,
		Skip:       -1,
		Properties: []string{"Name", "Age"},
	}
	result, err := selectObject(objects, opts)
	if err != nil {
		t.Fatalf("selectObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}

	// Verify only selected properties are present
	for i, obj := range result {
		m, ok := obj.(map[string]any)
		if !ok {
			t.Errorf("Object %d is not a map", i)
			continue
		}
		if _, hasName := m["Name"]; !hasName {
			t.Errorf("Object %d missing Name property", i)
		}
		if _, hasAge := m["Age"]; !hasAge {
			t.Errorf("Object %d missing Age property", i)
		}
		if _, hasCity := m["City"]; hasCity {
			t.Errorf("Object %d should not have City property", i)
		}
	}
}
