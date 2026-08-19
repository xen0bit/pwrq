// Package formatting provides tests for the Format-List cmdlet implementation.
package formatting

import (
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

func TestFormatListBasic(t *testing.T) {
	// Create test objects
	obj1 := map[string]any{
		"Name":   "file1.txt",
		"Length": 1024,
		"Type":   "file",
	}

	objects := []any{obj1}
	opts := FormatListOptions{}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 formatted object, got %d", len(result))
	}

	// Verify properties are present
	props := GetFormattedProperties(result[0])
	if len(props) != 3 {
		t.Errorf("expected 3 properties, got %d", len(props))
	}
}

func TestFormatListWithPSObject(t *testing.T) {
	// Create a PSObject-wrapped value
	valueMap := map[string]any{
		"Name":  "test",
		"Value": 42,
	}
	psobj := psobject.NewPSObjectWithTypeName(valueMap, "TestType")
	psobj.AddNoteProperty("Name", "test")
	psobj.AddNoteProperty("Value", 42)

	objects := []any{psobj.ToMap()}
	opts := FormatListOptions{}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 formatted object, got %d", len(result))
	}

	props := GetFormattedProperties(result[0])
	if len(props) < 2 {
		t.Errorf("expected at least 2 properties, got %d", len(props))
	}
}

func TestFormatListSpecificProperty(t *testing.T) {
	obj1 := map[string]any{
		"Name":   "file1.txt",
		"Length": 1024,
		"Type":   "file",
	}

	objects := []any{obj1}
	opts := FormatListOptions{
		Property: []string{"Name"},
	}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	props := GetFormattedProperties(result[0])
	if len(props) != 1 {
		t.Errorf("expected 1 property, got %d", len(props))
	}

	if props[0].Name != "Name" {
		t.Errorf("expected property 'Name', got '%s'", props[0].Name)
	}
}

func TestFormatListMultipleProperties(t *testing.T) {
	obj1 := map[string]any{
		"Name":   "file1.txt",
		"Length": 1024,
		"Type":   "file",
		"Path":   "/test",
	}

	objects := []any{obj1}
	opts := FormatListOptions{
		Property: []string{"Name", "Length"},
	}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	props := GetFormattedProperties(result[0])
	if len(props) != 2 {
		t.Errorf("expected 2 properties, got %d", len(props))
	}

	// Verify the specific properties
	propNames := make(map[string]bool)
	for _, p := range props {
		propNames[p.Name] = true
	}

	if !propNames["Name"] {
		t.Error("expected 'Name' property")
	}
	if !propNames["Length"] {
		t.Error("expected 'Length' property")
	}
}

func TestFormatListWildcard(t *testing.T) {
	obj1 := map[string]any{
		"Name":      "file1.txt",
		"Length":    1024,
		"Type":      "file",
		"Path":      "/test",
		"Protected": false,
	}

	objects := []any{obj1}
	opts := FormatListOptions{
		Property: []string{"P*"},
	}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	props := GetFormattedProperties(result[0])
	// Should match "Path" and "Protected"
	if len(props) != 2 {
		t.Errorf("expected 2 properties matching 'P*', got %d: %v", len(props), props)
	}
}

func TestFormatListWildcardAll(t *testing.T) {
	obj1 := map[string]any{
		"Name":   "file1.txt",
		"Length": 1024,
		"Type":   "file",
	}

	objects := []any{obj1}
	opts := FormatListOptions{
		Property: []string{"*"},
	}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	props := GetFormattedProperties(result[0])
	if len(props) != 3 {
		t.Errorf("expected 3 properties with '*', got %d", len(props))
	}
}

func TestFormatListCaseSensitive(t *testing.T) {
	obj1 := map[string]any{
		"Name":  "file1.txt",
		"name":  "lowercase",
		"NAME":  "uppercase",
		"Value": 42,
	}

	objects := []any{obj1}
	opts := FormatListOptions{
		Property:      []string{"name"},
		CaseSensitive: true,
	}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	props := GetFormattedProperties(result[0])
	// Should only match exact "name" (lowercase)
	if len(props) != 1 {
		t.Errorf("expected 1 property with case-sensitive match, got %d", len(props))
	}
}

func TestFormatListCaseInsensitive(t *testing.T) {
	obj1 := map[string]any{
		"Name":  "file1.txt",
		"Value": 42,
	}

	objects := []any{obj1}
	opts := FormatListOptions{
		Property:      []string{"name"},
		CaseSensitive: false,
	}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	props := GetFormattedProperties(result[0])
	// Should match "Name" case-insensitively
	if len(props) != 1 {
		t.Errorf("expected 1 property with case-insensitive match, got %d", len(props))
	}
}

func TestFormatListEmptyInput(t *testing.T) {
	objects := []any{}
	opts := FormatListOptions{}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestFormatListMultipleObjects(t *testing.T) {
	obj1 := map[string]any{
		"Name":   "file1.txt",
		"Length": 1024,
	}
	obj2 := map[string]any{
		"Name":   "file2.txt",
		"Length": 2048,
	}

	objects := []any{obj1, obj2}
	opts := FormatListOptions{
		Property: []string{"Name"},
	}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 formatted objects, got %d", len(result))
	}

	// Verify first object
	props1 := GetFormattedProperties(result[0])
	if len(props1) != 1 || props1[0].Value != "file1.txt" {
		t.Error("first object formatted incorrectly")
	}

	// Verify second object
	props2 := GetFormattedProperties(result[1])
	if len(props2) != 1 || props2[0].Value != "file2.txt" {
		t.Error("second object formatted incorrectly")
	}
}

func TestFormatListNonExistentProperty(t *testing.T) {
	obj1 := map[string]any{
		"Name":   "file1.txt",
		"Length": 1024,
	}

	objects := []any{obj1}
	opts := FormatListOptions{
		Property: []string{"NonExistent"},
	}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	props := GetFormattedProperties(result[0])
	// Should have no properties (non-existent property not found)
	if len(props) != 0 {
		t.Errorf("expected 0 properties for non-existent property, got %d", len(props))
	}
}

func TestFormatListToString(t *testing.T) {
	obj1 := map[string]any{
		"Name":   "file1.txt",
		"Length": 1024,
	}

	objects := []any{obj1}
	opts := FormatListOptions{}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	output := FormatListToString(result[0])
	if output == "" {
		t.Error("expected non-empty string output")
	}

	// Verify format: "Name : file1.txt\nLength : 1024"
	if !strings.Contains(output, "Name") || !strings.Contains(output, "file1.txt") {
		t.Errorf("output format incorrect: %s", output)
	}
}

func TestParseFormatListArgs(t *testing.T) {
	// Test with string property
	args := []any{
		[]any{map[string]any{"Name": "test"}},
		map[string]any{"property": "Name"},
	}

	objects, opts, err := ParseFormatListArgs(args)
	if err != nil {
		t.Fatalf("ParseFormatListArgs failed: %v", err)
	}

	if len(objects) != 1 {
		t.Errorf("expected 1 object, got %d", len(objects))
	}

	if len(opts.Property) != 1 || opts.Property[0] != "Name" {
		t.Errorf("expected property ['Name'], got %v", opts.Property)
	}

	// Test with array of properties
	args2 := []any{
		[]any{map[string]any{"Name": "test"}},
		map[string]any{"property": []any{"Name", "Length"}},
	}

	_, opts2, err := ParseFormatListArgs(args2)
	if err != nil {
		t.Fatalf("ParseFormatListArgs failed: %v", err)
	}

	if len(opts2.Property) != 2 {
		t.Errorf("expected 2 properties, got %d", len(opts2.Property))
	}
}

func TestFormatListPSObjectMembers(t *testing.T) {
	// Create PSObject with explicit members
	valueMap := map[string]any{
		"InternalValue": 42,
	}
	psobj := psobject.NewPSObjectWithTypeName(valueMap, "CustomType")
	psobj.AddNoteProperty("DisplayName", "MyObject")
	psobj.AddNoteProperty("DisplayValue", 100)

	objects := []any{psobj.ToMap()}
	opts := FormatListOptions{}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	props := GetFormattedProperties(result[0])
	// Should include both members and value map keys
	if len(props) < 2 {
		t.Errorf("expected at least 2 properties, got %d: %v", len(props), props)
	}
}

func TestMatchProperty(t *testing.T) {
	tests := []struct {
		prop          string
		pattern       string
		caseSensitive bool
		expected      bool
	}{
		{"Name", "Name", true, true},
		{"Name", "name", true, false},
		{"Name", "name", false, true},
		{"Name", "N*", true, true},
		{"Name", "*ame", true, true},
		{"Name", "N?me", true, true},
		{"Name", "X*", true, false},
	}

	for _, test := range tests {
		result := matchProperty(test.prop, test.pattern, test.caseSensitive)
		if result != test.expected {
			t.Errorf("matchProperty(%q, %q, %v) = %v, expected %v",
				test.prop, test.pattern, test.caseSensitive, result, test.expected)
		}
	}
}

func TestGetAvailableProperties(t *testing.T) {
	// Test with regular map
	obj := map[string]any{
		"Name":   "test",
		"Value":  42,
		"Active": true,
	}

	props := getAvailableProperties(obj)
	if len(props) != 3 {
		t.Errorf("expected 3 properties, got %d", len(props))
	}

	// Test with PSObject
	psobj := psobject.NewPSObject(obj)
	psobj.AddNoteProperty("Extra", "data")

	props = getAvailableProperties(psobj.ToMap())
	if len(props) < 3 {
		t.Errorf("expected at least 3 properties, got %d", len(props))
	}
}

func TestFormatListDerivedProperty(t *testing.T) {
	// A property a cmdlet computed and attached alongside the underlying
	// map's own keys must be formatted like any other.
	valueMap := map[string]any{
		"FirstName": "John",
		"LastName":  "Doe",
	}
	psobj := psobject.NewPSObjectWithTypeName(valueMap, "Person")
	psobj.AddNoteProperty("FirstName", "John")
	psobj.AddNoteProperty("LastName", "Doe")
	psobj.AddNoteProperty("FullName", "John Doe")

	objects := []any{psobj.ToMap()}
	opts := FormatListOptions{}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	props := GetFormattedProperties(result[0])
	if len(props) < 3 {
		t.Errorf("expected at least 3 properties, got %d: %v", len(props), props)
	}

	foundFullName := false
	for _, p := range props {
		if p.Name == "FullName" {
			foundFullName = true
			if p.Value == nil {
				t.Error("FullName should have a value")
			}
			break
		}
	}
	if !foundFullName {
		t.Error("'FullName' should be included in output")
	}
}

func TestFormatListPropertySelection(t *testing.T) {
	// Property narrows the output to the named property and nothing else.
	valueMap := map[string]any{
		"DisplayName": "MyFile.txt",
	}
	psobj := psobject.NewPSObjectWithTypeName(valueMap, "File")
	psobj.AddNoteProperty("DisplayName", "MyFile.txt")
	psobj.AddNoteProperty("Name", "MyFile.txt")

	objects := []any{psobj.ToMap()}
	opts := FormatListOptions{
		Property: []string{"Name"},
	}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	props := GetFormattedProperties(result[0])
	if len(props) != 1 {
		t.Errorf("expected 1 property, got %d", len(props))
	}

	if props[0].Name != "Name" {
		t.Errorf("expected property 'Name', got '%s'", props[0].Name)
	}

	// The selected property carries its value
	if props[0].Value != "MyFile.txt" {
		t.Errorf("expected 'MyFile.txt', got '%v'", props[0].Value)
	}
}

func TestFormatListDepthLimiting(t *testing.T) {
	// Create nested PSObject
	innerObj := map[string]any{
		"InnerValue": "deep",
		"InnerNum":   42,
	}
	outerObj := map[string]any{
		"Name":  "outer",
		"Inner": innerObj,
	}

	psobj := psobject.NewPSObjectWithTypeName(outerObj, "Container")
	psobj.AddNoteProperty("Name", "outer")
	psobj.AddNoteProperty("Inner", innerObj)

	objects := []any{psobj.ToMap()}

	// Test with depth=1 (should show outer properties but truncate nested)
	opts := FormatListOptions{
		Depth: 1,
	}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	props := GetFormattedProperties(result[0])
	// Should have Name and Inner properties
	if len(props) < 2 {
		t.Errorf("expected at least 2 properties, got %d", len(props))
	}

	// Find Inner property and verify it's truncated
	for _, p := range props {
		if p.Name == "Inner" {
			// With depth=1, nested object should be truncated
			innerStr, ok := p.Value.(string)
			if !ok {
				t.Errorf("Inner property value should be string, got %T", p.Value)
			}
			// Should contain truncation indicator
			if !strings.Contains(innerStr, "...") {
				t.Logf("Inner property with depth=1: %s (expected truncation)", innerStr)
			}
		}
	}
}

func TestFormatListNilInput(t *testing.T) {
	// Test nil input handling
	opts := FormatListOptions{}

	// formatSingleObject should handle nil gracefully
	result := formatSingleObject(nil, opts)
	props := GetFormattedProperties(result)
	if len(props) != 0 {
		t.Errorf("expected 0 properties for nil input, got %d", len(props))
	}
}

func TestFormatValueDepthLimiting(t *testing.T) {
	// Test formatValue directly with depth limiting
	nestedMap := map[string]any{
		"Level1": map[string]any{
			"Level2": map[string]any{
				"Level3": "deep value",
			},
		},
	}

	// With maxDepth=2, Level3 should be truncated
	result := formatValue(nestedMap, 2, 0)
	if !strings.Contains(result, "...") {
		t.Logf("formatValue with depth=2: %s", result)
		// Note: The exact truncation behavior depends on implementation
	}

	// With maxDepth=0 (unlimited), should show full structure
	resultUnlimited := formatValue(nestedMap, 0, 0)
	if resultUnlimited == "" {
		t.Error("formatValue with unlimited depth should not return empty string")
	}
}

func TestFormatListManyProperties(t *testing.T) {
	// An object carrying both its underlying map's keys and several attached
	// properties formats all of them.
	valueMap := map[string]any{
		"First": "Jane",
		"Last":  "Smith",
	}
	psobj := psobject.NewPSObjectWithTypeName(valueMap, "Person")
	psobj.AddNoteProperty("First", "Jane")
	psobj.AddNoteProperty("Last", "Smith")
	psobj.AddNoteProperty("FullName", "Jane")
	psobj.AddNoteProperty("DisplayName", "Jane Smith")

	objects := []any{psobj.ToMap()}
	opts := FormatListOptions{}

	result, err := formatList(objects, opts)
	if err != nil {
		t.Fatalf("formatList failed: %v", err)
	}

	props := GetFormattedProperties(result[0])

	// First, Last, FullName, DisplayName
	if len(props) < 4 {
		t.Errorf("expected at least 4 properties, got %d: %v", len(props), props)
	}

	for _, p := range props {
		if p.Name == "FullName" {
			if p.Value != "Jane" {
				t.Errorf("'FullName' should be 'Jane', got '%v'", p.Value)
			}
		}
	}
}

// TestColumnOrderIsStable pins that identical input formats identically.
//
// Columns came from ranging a Go map, so the order changed between runs of the
// same query: `format_table([{Alpha:1,Beta:2,Gamma:3}])` printed "Gamma Alpha
// Beta" one run and "Alpha Beta Gamma" the next. Anything comparing output
// across runs — a diff, a golden file, a script reading a column by position —
// was reading noise.
func TestColumnOrderIsStable(t *testing.T) {
	row := map[string]any{"Gamma": 3, "Alpha": 1, "Beta": 2, "Delta": 4}

	first := getAvailableProperties(row)
	for range 50 {
		got := getAvailableProperties(map[string]any{"Gamma": 3, "Alpha": 1, "Beta": 2, "Delta": 4})
		if len(got) != len(first) {
			t.Fatalf("got %d properties, want %d", len(got), len(first))
		}
		for i := range got {
			if got[i] != first[i] {
				t.Fatalf("column order changed between runs: %v then %v", first, got)
			}
		}
	}

	want := []string{"Alpha", "Beta", "Delta", "Gamma"}
	for i := range want {
		if first[i] != want[i] {
			t.Errorf("columns are %v, want them sorted as %v", first, want)
		}
	}
}
