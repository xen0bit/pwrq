package objects

import (
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

func TestSortObjectBasic(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Charlie", "Age": 30},
		map[string]any{"Name": "Alice", "Age": 25},
		map[string]any{"Name": "Bob", "Age": 35},
	}

	opts := SortObjectOptions{
		Properties: []SortProperty{{Name: "Name", Direction: SortDirectionAscending}},
	}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	// Should be sorted by Name ascending: Alice, Bob, Charlie
	if getName(result[0]) != "Alice" {
		t.Errorf("expected first to be Alice, got %s", getName(result[0]))
	}
	if getName(result[1]) != "Bob" {
		t.Errorf("expected second to be Bob, got %s", getName(result[1]))
	}
	if getName(result[2]) != "Charlie" {
		t.Errorf("expected third to be Charlie, got %s", getName(result[2]))
	}
}

func TestSortObjectDescending(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 25},
		map[string]any{"Name": "Bob", "Age": 35},
		map[string]any{"Name": "Charlie", "Age": 30},
	}

	opts := SortObjectOptions{
		Properties: []SortProperty{{Name: "Age", Direction: SortDirectionDescending}},
	}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	// Should be sorted by Age descending: 35, 30, 25
	if getAge(result[0]) != 35 {
		t.Errorf("expected first age to be 35, got %d", getAge(result[0]))
	}
	if getAge(result[1]) != 30 {
		t.Errorf("expected second age to be 30, got %d", getAge(result[1]))
	}
	if getAge(result[2]) != 25 {
		t.Errorf("expected third age to be 25, got %d", getAge(result[2]))
	}
}

func TestSortObjectMultipleProperties(t *testing.T) {
	objects := []any{
		map[string]any{"Department": "Sales", "Name": "Charlie"},
		map[string]any{"Department": "Engineering", "Name": "Alice"},
		map[string]any{"Department": "Sales", "Name": "Alice"},
		map[string]any{"Department": "Engineering", "Name": "Bob"},
	}

	opts := SortObjectOptions{
		Properties: []SortProperty{
			{Name: "Department", Direction: SortDirectionAscending},
			{Name: "Name", Direction: SortDirectionAscending},
		},
	}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	// Should be sorted by Department first, then Name:
	// Engineering/Alice, Engineering/Bob, Sales/Alice, Sales/Charlie
	expected := []struct {
		Dept string
		Name string
	}{
		{"Engineering", "Alice"},
		{"Engineering", "Bob"},
		{"Sales", "Alice"},
		{"Sales", "Charlie"},
	}

	for i, exp := range expected {
		if i >= len(result) {
			t.Fatalf("expected %d results, got %d", len(expected), len(result))
		}
		if getDept(result[i]) != exp.Dept {
			t.Errorf("result[%d]: expected dept %s, got %s", i, exp.Dept, getDept(result[i]))
		}
		if getName(result[i]) != exp.Name {
			t.Errorf("result[%d]: expected name %s, got %s", i, exp.Name, getName(result[i]))
		}
	}
}

func TestSortObjectUnique(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 25},
		map[string]any{"Name": "Bob", "Age": 30},
		map[string]any{"Name": "Alice", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
		map[string]any{"Name": "Bob", "Age": 30},
	}

	opts := SortObjectOptions{
		Properties: []SortProperty{{Name: "Name", Direction: SortDirectionAscending}},
		Unique:     true,
	}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	// Should have 3 unique entries
	if len(result) != 3 {
		t.Errorf("expected 3 unique results, got %d", len(result))
	}

	// Verify uniqueness
	names := make(map[string]bool)
	for _, obj := range result {
		name := getName(obj)
		if names[name] {
			t.Errorf("duplicate name found: %s", name)
		}
		names[name] = true
	}
}

func TestSortObjectPSObject(t *testing.T) {
	// Create PSObject-wrapped objects
	psobj1 := psobject.NewPSObject(map[string]any{"Name": "Charlie", "Value": 30})
	psobj2 := psobject.NewPSObject(map[string]any{"Name": "Alice", "Value": 10})
	psobj3 := psobject.NewPSObject(map[string]any{"Name": "Bob", "Value": 20})

	objects := []any{
		psobj1.ToMap(),
		psobj2.ToMap(),
		psobj3.ToMap(),
	}

	opts := SortObjectOptions{
		Properties: []SortProperty{{Name: "Value", Direction: SortDirectionAscending}},
	}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	// Should be sorted by Value: 10, 20, 30
	if getValue(result[0]) != 10 {
		t.Errorf("expected first value to be 10, got %v", getValue(result[0]))
	}
	if getValue(result[1]) != 20 {
		t.Errorf("expected second value to be 20, got %v", getValue(result[1]))
	}
	if getValue(result[2]) != 30 {
		t.Errorf("expected third value to be 30, got %v", getValue(result[2]))
	}
}

func TestSortObjectByStringValue(t *testing.T) {
	objects := []any{"zebra", "apple", "banana"}

	opts := SortObjectOptions{}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	// Should be sorted alphabetically
	expected := []string{"apple", "banana", "zebra"}
	for i, exp := range expected {
		if result[i] != exp {
			t.Errorf("result[%d]: expected %s, got %v", i, exp, result[i])
		}
	}
}

func TestSortObjectByNumericValue(t *testing.T) {
	objects := []any{5, 1, 3, 2, 4}

	opts := SortObjectOptions{}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	// Should be sorted numerically
	for i := 0; i < len(result); i++ {
		if result[i] != i+1 {
			t.Errorf("result[%d]: expected %d, got %v", i, i+1, result[i])
		}
	}
}

func TestSortObjectCaseSensitive(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "alice"},
		map[string]any{"Name": "Bob"},
		map[string]any{"Name": "CHARLIE"},
	}

	// Case-insensitive sort (default)
	opts := SortObjectOptions{
		Properties:    []SortProperty{{Name: "Name", Direction: SortDirectionAscending}},
		CaseSensitive: false,
	}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	// Case-insensitive: alice, Bob, CHARLIE (alphabetical regardless of case)
	if getName(result[0]) != "alice" {
		t.Errorf("expected first to be alice, got %s", getName(result[0]))
	}
}

func TestSortObjectEmpty(t *testing.T) {
	objects := []any{}

	opts := SortObjectOptions{
		Properties: []SortProperty{{Name: "Name", Direction: SortDirectionAscending}},
	}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestSortObjectNilProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 25},
		map[string]any{"Name": "Bob"}, // Missing Age
		map[string]any{"Name": "Charlie", "Age": 30},
	}

	opts := SortObjectOptions{
		Properties: []SortProperty{{Name: "Age", Direction: SortDirectionAscending}},
	}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	// Bob (missing Age) should sort first
	if getName(result[0]) != "Bob" {
		t.Errorf("expected first to be Bob (missing value), got %s", getName(result[0]))
	}
}

func TestParseSortPropertyString(t *testing.T) {
	tests := []struct {
		input     string
		wantName  string
		wantDesc  bool
		wantError bool
	}{
		{"Name", "Name", false, false},
		{"Name desc", "Name", true, false},
		{"Name DESC", "Name", true, false},
		{"Name descending", "Name", true, false},
		{"Name asc", "Name", false, false},
		{"Name ascending", "Name", false, false},
		{"-Name", "Name", true, false},
		{"Value desc", "Value", true, false},
	}

	for _, tt := range tests {
		prop, err := ParseSortPropertyString(tt.input)
		if (err != nil) != tt.wantError {
			t.Errorf("ParseSortPropertyString(%q): error = %v, wantError %v", tt.input, err, tt.wantError)
			continue
		}
		if !tt.wantError {
			if prop.Name != tt.wantName {
				t.Errorf("ParseSortPropertyString(%q): name = %q, want %q", tt.input, prop.Name, tt.wantName)
			}
			isDesc := prop.Direction == SortDirectionDescending
			if isDesc != tt.wantDesc {
				t.Errorf("ParseSortPropertyString(%q): descending = %v, want %v", tt.input, isDesc, tt.wantDesc)
			}
		}
	}
}

func TestSortObjectPropertyParsing(t *testing.T) {
	// Test string property with direction
	props, err := parseSortProperty("Name desc")
	if err != nil {
		t.Fatalf("parseSortProperty failed: %v", err)
	}
	if len(props) != 1 {
		t.Fatalf("expected 1 property, got %d", len(props))
	}
	if props[0].Name != "Name" {
		t.Errorf("expected name 'Name', got %s", props[0].Name)
	}
	if props[0].Direction != SortDirectionDescending {
		t.Error("expected descending direction")
	}

	// Test array of property names
	props, err = parseSortProperty([]any{"Name", "Age desc"})
	if err != nil {
		t.Fatalf("parseSortProperty failed: %v", err)
	}
	if len(props) != 2 {
		t.Fatalf("expected 2 properties, got %d", len(props))
	}
	if props[0].Name != "Name" || props[0].Direction != SortDirectionAscending {
		t.Error("first property should be Name ascending")
	}
	if props[1].Name != "Age" || props[1].Direction != SortDirectionDescending {
		t.Error("second property should be Age descending")
	}

	// Test property object
	props, err = parseSortProperty(map[string]any{
		"name":       "Value",
		"descending": true,
	})
	if err != nil {
		t.Fatalf("parseSortProperty failed: %v", err)
	}
	if len(props) != 1 {
		t.Fatalf("expected 1 property, got %d", len(props))
	}
	if props[0].Name != "Value" {
		t.Errorf("expected name 'Value', got %s", props[0].Name)
	}
	if props[0].Direction != SortDirectionDescending {
		t.Error("expected descending direction")
	}
}

func TestSortObjectArgumentParsing(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Bob"},
		map[string]any{"Name": "Alice"},
	}

	// Test with property option
	args := []any{
		objects,
		map[string]any{
			"property": "Name",
		},
	}

	parsedObjects, opts, err := ParseSortObjectArgs(args)
	if err != nil {
		t.Fatalf("ParseSortObjectArgs failed: %v", err)
	}

	if len(parsedObjects) != 2 {
		t.Errorf("expected 2 objects, got %d", len(parsedObjects))
	}

	if len(opts.Properties) != 1 {
		t.Fatalf("expected 1 property, got %d", len(opts.Properties))
	}
	if opts.Properties[0].Name != "Name" {
		t.Errorf("expected property name 'Name', got %s", opts.Properties[0].Name)
	}
}

func TestSortObjectUniqueByProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Category": "A", "Name": "First"},
		map[string]any{"Category": "B", "Name": "Second"},
		map[string]any{"Category": "A", "Name": "Third"},
		map[string]any{"Category": "B", "Name": "Fourth"},
	}

	opts := SortObjectOptions{
		Properties: []SortProperty{{Name: "Category", Direction: SortDirectionAscending}},
		Unique:     true,
	}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	// Should have 2 unique categories
	if len(result) != 2 {
		t.Errorf("expected 2 unique results, got %d", len(result))
	}

	// Verify we have one of each category
	categories := make(map[string]bool)
	for _, obj := range result {
		cat := getCategory(obj)
		categories[cat] = true
	}

	if !categories["A"] || !categories["B"] {
		t.Error("expected both categories A and B in results")
	}
}

func TestSortObjectStableSort(t *testing.T) {
	// Stable sort should preserve original order for equal elements
	objects := []any{
		map[string]any{"Age": 30, "Name": "First"},
		map[string]any{"Age": 30, "Name": "Second"},
		map[string]any{"Age": 30, "Name": "Third"},
	}

	opts := SortObjectOptions{
		Properties: []SortProperty{{Name: "Age", Direction: SortDirectionAscending}},
	}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	// With stable sort, order should be preserved for equal elements
	if getName(result[0]) != "First" {
		t.Errorf("expected stable sort to preserve order, got %s first", getName(result[0]))
	}
	if getName(result[1]) != "Second" {
		t.Errorf("expected stable sort to preserve order, got %s second", getName(result[1]))
	}
	if getName(result[2]) != "Third" {
		t.Errorf("expected stable sort to preserve order, got %s third", getName(result[2]))
	}
}

func TestSortObjectMixedTypes(t *testing.T) {
	objects := []any{
		map[string]any{"Value": 10},
		map[string]any{"Value": "zebra"},
		map[string]any{"Value": 5},
		map[string]any{"Value": "apple"},
	}

	opts := SortObjectOptions{
		Properties: []SortProperty{{Name: "Value", Direction: SortDirectionAscending}},
	}

	// This should not panic - it should handle mixed types gracefully
	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	if len(result) != 4 {
		t.Errorf("expected 4 results, got %d", len(result))
	}
}

func TestSortObjectNestedProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Person": map[string]any{"Name": "Charlie"}},
		map[string]any{"Person": map[string]any{"Name": "Alice"}},
		map[string]any{"Person": map[string]any{"Name": "Bob"}},
	}

	opts := SortObjectOptions{
		Properties: []SortProperty{{Name: "Person.Name", Direction: SortDirectionAscending}},
	}

	result, err := sortObject(objects, opts)
	if err != nil {
		t.Fatalf("sortObject failed: %v", err)
	}

	// Should be sorted by nested Name property
	if getNestedName(result[0]) != "Alice" {
		t.Errorf("expected first to be Alice, got %s", getNestedName(result[0]))
	}
	if getNestedName(result[1]) != "Bob" {
		t.Errorf("expected second to be Bob, got %s", getNestedName(result[1]))
	}
	if getNestedName(result[2]) != "Charlie" {
		t.Errorf("expected third to be Charlie, got %s", getNestedName(result[2]))
	}
}

func TestSortDirectionString(t *testing.T) {
	if SortDirectionAscending.String() != "Ascending" {
		t.Errorf("expected 'Ascending', got %s", SortDirectionAscending.String())
	}
	if SortDirectionDescending.String() != "Descending" {
		t.Errorf("expected 'Descending', got %s", SortDirectionDescending.String())
	}
}

func TestNewSortProperty(t *testing.T) {
	prop := NewSortProperty("Name", false)
	if prop.Name != "Name" {
		t.Errorf("expected name 'Name', got %s", prop.Name)
	}
	if prop.Direction != SortDirectionAscending {
		t.Error("expected ascending direction")
	}

	prop = NewSortProperty("Age", true)
	if prop.Name != "Age" {
		t.Errorf("expected name 'Age', got %s", prop.Name)
	}
	if prop.Direction != SortDirectionDescending {
		t.Error("expected descending direction")
	}
}

func TestSortByProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Charlie"},
		map[string]any{"Name": "Alice"},
		map[string]any{"Name": "Bob"},
	}

	result, err := SortByProperty(objects, "Name", false, false, false)
	if err != nil {
		t.Fatalf("SortByProperty failed: %v", err)
	}

	if getName(result[0]) != "Alice" {
		t.Errorf("expected first to be Alice, got %s", getName(result[0]))
	}
}

func TestSortByProperties(t *testing.T) {
	objects := []any{
		map[string]any{"Dept": "Sales", "Name": "Charlie"},
		map[string]any{"Dept": "Engineering", "Name": "Alice"},
	}

	result, err := SortByProperties(objects, []string{"Dept", "Name"}, []bool{false, false}, false, false)
	if err != nil {
		t.Fatalf("SortByProperties failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
}

func TestParseSortOptionsFromMap(t *testing.T) {
	m := map[string]any{
		"property":      "Name",
		"casesensitive": true,
		"unique":        true,
	}

	opts, err := ParseSortOptionsFromMap(m)
	if err != nil {
		t.Fatalf("ParseSortOptionsFromMap failed: %v", err)
	}

	if len(opts.Properties) != 1 {
		t.Fatalf("expected 1 property, got %d", len(opts.Properties))
	}
	if opts.Properties[0].Name != "Name" {
		t.Errorf("expected property name 'Name', got %s", opts.Properties[0].Name)
	}
	if !opts.CaseSensitive {
		t.Error("expected case sensitive to be true")
	}
	if !opts.Unique {
		t.Error("expected unique to be true")
	}
}

// Helper functions for extracting values from test objects

func getName(obj any) string {
	if m, ok := obj.(map[string]any); ok {
		if name, ok := m["Name"].(string); ok {
			return name
		}
	}
	return ""
}

func getAge(obj any) int {
	if m, ok := obj.(map[string]any); ok {
		if age, ok := m["Age"].(float64); ok {
			return int(age)
		}
		if age, ok := m["Age"].(int); ok {
			return age
		}
	}
	return 0
}

func getDept(obj any) string {
	if m, ok := obj.(map[string]any); ok {
		if dept, ok := m["Department"].(string); ok {
			return dept
		}
	}
	return ""
}

func getCategory(obj any) string {
	if m, ok := obj.(map[string]any); ok {
		if cat, ok := m["Category"].(string); ok {
			return cat
		}
	}
	return ""
}

func getValue(obj any) any {
	if m, ok := obj.(map[string]any); ok {
		// Check for PSObject format (_val/_meta)
		if val, ok := m["_val"]; ok {
			if valMap, ok := val.(map[string]any); ok {
				return valMap["Value"]
			}
		}
		// Direct property access
		return m["Value"]
	}
	return nil
}

func getNestedName(obj any) string {
	if m, ok := obj.(map[string]any); ok {
		if person, ok := m["Person"].(map[string]any); ok {
			if name, ok := person["Name"].(string); ok {
				return name
			}
		}
	}
	return ""
}
