package objects

import (
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

func TestWhereObject_Basic(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		ScriptBlock: ".Age > 25",
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects (Alice and Charlie), got %d", len(result))
	}
}

func TestWhereObject_PropertyOperator(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		Property: "Age",
		Operator: FilterOperatorGt,
		Value:    28,
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects (Alice and Charlie), got %d", len(result))
	}
}

func TestWhereObject_Equals(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		Property: "Name",
		Operator: FilterOperatorEq,
		Value:    "Bob",
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 object, got %d", len(result))
	}

	if m, ok := result[0].(map[string]any); !ok || m["Name"] != "Bob" {
		t.Errorf("Expected Bob, got %v", result[0])
	}
}

func TestWhereObject_NotEquals(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		Property: "Name",
		Operator: FilterOperatorNe,
		Value:    "Bob",
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}
}

func TestWhereObject_GreaterThan(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		Property: "Age",
		Operator: FilterOperatorGt,
		Value:    28,
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects (Alice and Charlie), got %d", len(result))
	}
}

func TestWhereObject_LessThan(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		Property: "Age",
		Operator: FilterOperatorLt,
		Value:    30,
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 object (Bob), got %d", len(result))
	}
}

func TestWhereObject_Like(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		Property: "Name",
		Operator: FilterOperatorLike,
		Value:    "A*",
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 object (Alice), got %d", len(result))
	}
}

func TestWhereObject_NotLike(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		Property: "Name",
		Operator: FilterOperatorNotLike,
		Value:    "A*",
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}
}

func TestWhereObject_Match(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		Property: "Name",
		Operator: FilterOperatorMatch,
		Value:    "^[A-C]",
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("Expected 3 objects, got %d", len(result))
	}
}

func TestWhereObject_NotMatch(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		Property: "Name",
		Operator: FilterOperatorNotMatch,
		Value:    "^A",
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}
}

func TestWhereObject_Contains(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		Property: "Name",
		Operator: FilterOperatorContains,
		Value:    "li",
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	// Both Alice and Charlie contain "li"
	if len(result) != 2 {
		t.Errorf("Expected 2 objects (Alice and Charlie), got %d", len(result))
	}
}

func TestWhereObject_EmptyResult(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
	}

	opts := WhereObjectOptions{
		Property: "Age",
		Operator: FilterOperatorGt,
		Value:    100,
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected 0 objects, got %d", len(result))
	}
}

func TestWhereObject_NoProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
	}

	opts := WhereObjectOptions{}
	_, err := whereObject(objects, opts)
	if err == nil {
		t.Error("Expected error when no property or script is specified")
	}
}

func TestWhereObject_PSOjbect(t *testing.T) {
	// Create PSObjects with members
	psobj1 := psobject.NewPSObject(map[string]any{"Name": "Alice", "Age": 30})
	psobj1.AddNoteProperty("Name", "Alice")
	psobj1.AddNoteProperty("Age", 30)

	psobj2 := psobject.NewPSObject(map[string]any{"Name": "Bob", "Age": 25})
	psobj2.AddNoteProperty("Name", "Bob")
	psobj2.AddNoteProperty("Age", 25)

	objects := []any{psobj1.ToMap(), psobj2.ToMap()}

	opts := WhereObjectOptions{
		Property: "Age",
		Operator: FilterOperatorGt,
		Value:    28,
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 object, got %d", len(result))
	}
}

func TestWhereObject_NestedProperty(t *testing.T) {
	objects := []any{
		map[string]any{
			"Name": "Alice",
			"Address": map[string]any{
				"City": "NYC",
				"State": "NY",
			},
		},
		map[string]any{
			"Name": "Bob",
			"Address": map[string]any{
				"City": "LA",
				"State": "CA",
			},
		},
	}

	opts := WhereObjectOptions{
		Property: "Address.City",
		Operator: FilterOperatorEq,
		Value:    "NYC",
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 object, got %d", len(result))
	}

	if m, ok := result[0].(map[string]any); !ok || m["Name"] != "Alice" {
		t.Errorf("Expected Alice, got %v", result[0])
	}
}

func TestWhereObject_ScriptBlock(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		ScriptBlock: ".Age -gt 28",
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}
}

func TestWhereObject_ScriptBlockContains(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := WhereObjectOptions{
		ScriptBlock: `.Name | contains("li")`,
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	// Both Alice and Charlie contain "li"
	if len(result) != 2 {
		t.Errorf("Expected 2 objects (Alice and Charlie), got %d", len(result))
	}
}

func TestParseOperator(t *testing.T) {
	tests := []struct {
		input    string
		expected FilterOperator
	}{
		{"eq", FilterOperatorEq},
		{"-eq", FilterOperatorEq},
		{"equals", FilterOperatorEq},
		{"ne", FilterOperatorNe},
		{"-ne", FilterOperatorNe},
		{"gt", FilterOperatorGt},
		{"-gt", FilterOperatorGt},
		{"ge", FilterOperatorGe},
		{"-ge", FilterOperatorGe},
		{"lt", FilterOperatorLt},
		{"-lt", FilterOperatorLt},
		{"le", FilterOperatorLe},
		{"-le", FilterOperatorLe},
		{"like", FilterOperatorLike},
		{"notlike", FilterOperatorNotLike},
		{"match", FilterOperatorMatch},
		{"notmatch", FilterOperatorNotMatch},
		{"contains", FilterOperatorContains},
		{"notcontains", FilterOperatorNotContains},
	}

	for _, test := range tests {
		result := parseOperator(test.input)
		if result != test.expected {
			t.Errorf("parseOperator(%q) = %v, expected %v", test.input, result, test.expected)
		}
	}
}

func TestParseWhereObjectArgs(t *testing.T) {
	args := []any{
		[]any{
			map[string]any{"Name": "Alice", "Age": 30},
			map[string]any{"Name": "Bob", "Age": 25},
		},
		map[string]any{
			"property": "Age",
			"operator": "gt",
			"value":    float64(28),
		},
	}

	objects, opts, err := ParseWhereObjectArgs(args)
	if err != nil {
		t.Fatalf("ParseWhereObjectArgs failed: %v", err)
	}

	if len(objects) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(objects))
	}

	if opts.Property != "Age" {
		t.Errorf("Expected Property=Age, got %s", opts.Property)
	}

	if opts.Operator != FilterOperatorGt {
		t.Errorf("Expected Operator=FilterOperatorGt, got %v", opts.Operator)
	}

	if opts.Value != float64(28) {
		t.Errorf("Expected Value=28, got %v", opts.Value)
	}
}

func TestWhereObject_CaseInsensitive(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "bob", "Age": 25},
		map[string]any{"Name": "CHARLIE", "Age": 35},
	}

	opts := WhereObjectOptions{
		Property: "Name",
		Operator: FilterOperatorLike,
		Value:    "a*",
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	// Should match Alice, bob (no), CHARLIE (no) - only Alice starts with 'a' case-insensitive
	if len(result) != 1 {
		t.Errorf("Expected 1 object, got %d", len(result))
	}
}

func TestWhereObject_CaseSensitive(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "alice", "Age": 25},
	}

	opts := WhereObjectOptions{
		Property:      "Name",
		Operator:      FilterOperatorLike,
		Value:         "A*",
		CaseSensitive: true,
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	// Should only match Alice (capital A)
	if len(result) != 1 {
		t.Errorf("Expected 1 object, got %d", len(result))
	}

	if m, ok := result[0].(map[string]any); !ok || m["Name"] != "Alice" {
		t.Errorf("Expected Alice, got %v", result[0])
	}
}

func TestWhereObject_BooleanValue(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Active": true},
		map[string]any{"Name": "Bob", "Active": false},
		map[string]any{"Name": "Charlie", "Active": true},
	}

	opts := WhereObjectOptions{
		Property: "Active",
		Operator: FilterOperatorEq,
		Value:    true,
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}
}

func TestWhereObject_NullValue(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Email": "alice@example.com"},
		map[string]any{"Name": "Bob", "Email": nil},
		map[string]any{"Name": "Charlie", "Email": "charlie@example.com"},
	}

	opts := WhereObjectOptions{
		Property: "Email",
		Operator: FilterOperatorNe,
		Value:    nil,
	}
	result, err := whereObject(objects, opts)
	if err != nil {
		t.Fatalf("whereObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result))
	}
}

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		pattern  string
		value    string
		expected bool
	}{
		{"*", "anything", true},
		{"a*", "alice", true},
		{"a*", "bob", false},
		{"*e", "alice", true},
		{"*e", "bob", false},
		{"a*e", "alice", true},
		{"a*e", "abe", true},
		{"a?e", "abe", true},
		{"a?e", "alice", false},
		{"[abc]", "a", true},
		{"[abc]", "d", false},
	}

	for _, test := range tests {
		result := matchWildcard(test.value, test.pattern, false)
		if result != test.expected {
			t.Errorf("matchWildcard(%q, %q) = %v, expected %v", test.value, test.pattern, result, test.expected)
		}
	}
}

func TestExtractPropertyByPath(t *testing.T) {
	obj := map[string]any{
		"Name": "Alice",
		"Address": map[string]any{
			"City": "NYC",
			"State": map[string]any{
				"Code": "NY",
			},
		},
	}

	// Test single property
	val, err := extractPropertyByPath(obj, "Name")
	if err != nil {
		t.Fatalf("extractPropertyByPath failed: %v", err)
	}
	if val != "Alice" {
		t.Errorf("Expected Alice, got %v", val)
	}

	// Test nested property
	val, err = extractPropertyByPath(obj, "Address.City")
	if err != nil {
		t.Fatalf("extractPropertyByPath failed: %v", err)
	}
	if val != "NYC" {
		t.Errorf("Expected NYC, got %v", val)
	}

	// Test deeply nested property
	val, err = extractPropertyByPath(obj, "Address.State.Code")
	if err != nil {
		t.Fatalf("extractPropertyByPath failed: %v", err)
	}
	if val != "NY" {
		t.Errorf("Expected NY, got %v", val)
	}

	// Test missing property - should return error
	val, err = extractPropertyByPath(obj, "NonExistent")
	if err == nil {
		t.Errorf("Expected error for missing property, got nil")
	}
	if val != nil {
		t.Errorf("Expected nil for missing property, got %v", val)
	}
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		left     any
		right    any
		expected int
	}{
		{1, 2, -1},
		{2, 2, 0},
		{3, 2, 1},
		{1.5, 2.5, -1},
		{"a", "b", -1},
		{"b", "a", 1},
		{"a", "a", 0},
		{nil, nil, 0},
		{nil, 1, -1},
		{1, nil, 1},
	}

	for _, test := range tests {
		result := compareValues(test.left, test.right)
		if result != test.expected {
			t.Errorf("compareValues(%v, %v) = %d, expected %d", test.left, test.right, result, test.expected)
		}
	}
}
