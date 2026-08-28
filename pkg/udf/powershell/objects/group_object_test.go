package objects

import (
	"testing"
)

func TestGroupObject_Basic(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Department": "Engineering"},
		map[string]any{"Name": "Bob", "Department": "Engineering"},
		map[string]any{"Name": "Charlie", "Department": "Sales"},
	}

	opts := GroupObjectOptions{Property: "Department"}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(result))
	}
}

func TestGroupObject_ByProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Department": "Engineering"},
		map[string]any{"Name": "Bob", "Department": "Engineering"},
		map[string]any{"Name": "Charlie", "Department": "Sales"},
		map[string]any{"Name": "Diana", "Department": "Sales"},
		map[string]any{"Name": "Eve", "Department": "Sales"},
	}

	opts := GroupObjectOptions{Property: "Department"}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(result))
	}

	// Find Engineering group
	engineeringGroup := GetGroupByName(result, "Engineering")
	if engineeringGroup == nil {
		t.Fatal("Engineering group not found")
	}
	if engineeringGroup.Count != 2 {
		t.Errorf("Expected Engineering group to have 2 items, got %d", engineeringGroup.Count)
	}

	// Find Sales group
	salesGroup := GetGroupByName(result, "Sales")
	if salesGroup == nil {
		t.Fatal("Sales group not found")
	}
	if salesGroup.Count != 3 {
		t.Errorf("Expected Sales group to have 3 items, got %d", salesGroup.Count)
	}
}

func TestGroupObject_NoProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice"},
		map[string]any{"Name": "Alice"},
		map[string]any{"Name": "Bob"},
	}

	opts := GroupObjectOptions{}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	// Should group by entire object value
	if len(result) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(result))
	}
}

func TestGroupObject_CaseSensitive(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "alice"},
		map[string]any{"Name": "Alice"},
		map[string]any{"Name": "ALICE"},
		map[string]any{"Name": "bob"},
	}

	// Case insensitive (default)
	opts := GroupObjectOptions{Property: "Name", CaseSensitive: false}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 groups (case-insensitive), got %d", len(result))
	}

	// Case sensitive
	opts.CaseSensitive = true
	result, err = groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	if len(result) != 4 {
		t.Errorf("Expected 4 groups (case-sensitive), got %d", len(result))
	}
}

func TestGroupObject_NoElement(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Department": "Engineering"},
		map[string]any{"Name": "Bob", "Department": "Engineering"},
		map[string]any{"Name": "Charlie", "Department": "Sales"},
	}

	opts := GroupObjectOptions{Property: "Department", NoElement: true}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(result))
	}

	// Verify groups don't have Group property
	for _, group := range result {
		if groupMap, ok := group.(map[string]any); ok {
			innerMap := groupMap

			if innerMap == nil {
				t.Error("Could not extract group data")
				continue
			}

			if _, hasGroup := innerMap["Group"]; hasGroup {
				t.Error("Group should not have Group property when NoElement is true")
			}
			if _, hasName := innerMap["Name"]; !hasName {
				t.Error("Group should have Name property")
			}
			if _, hasCount := innerMap["Count"]; !hasCount {
				t.Error("Group should have Count property")
			}
		}
	}
}

func TestGroupObject_NoGroup(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Department": "Engineering"},
		map[string]any{"Name": "Bob", "Department": "Engineering"},
		map[string]any{"Name": "Charlie", "Department": "Sales"},
	}

	opts := GroupObjectOptions{Property: "Department", NoGroup: true}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	// Should return unique values only
	if len(result) != 2 {
		t.Errorf("Expected 2 unique values, got %d", len(result))
	}
}

func TestGroupObject_EmptyInput(t *testing.T) {
	objects := []any{}

	opts := GroupObjectOptions{Property: "Name"}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected 0 groups, got %d", len(result))
	}
}

func TestGroupObject_MissingProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice"},
		map[string]any{"Name": "Bob"},
	}

	opts := GroupObjectOptions{Property: "NonExistent"}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	// Should return empty result when property doesn't exist
	if len(result) != 0 {
		t.Errorf("Expected 0 groups (property doesn't exist), got %d", len(result))
	}
}

func TestGroupObject_GroupStructure(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Department": "Engineering"},
		map[string]any{"Name": "Bob", "Department": "Engineering"},
	}

	opts := GroupObjectOptions{Property: "Department"}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 group, got %d", len(result))
	}

	group := result[0].(map[string]any)

	// Handle typed format: {_val: {Name, Count, Group}, _meta: {...}}
	innerMap := group

	// Verify structure
	if _, ok := innerMap["Name"]; !ok {
		t.Error("Group missing Name property")
	}
	if _, ok := innerMap["Count"]; !ok {
		t.Error("Group missing Count property")
	}
	if _, ok := innerMap["Group"]; !ok {
		t.Error("Group missing Group property")
	}

	// Verify Count matches Group length
	count := innerMap["Count"].(int)
	groupItems := innerMap["Group"].([]any)
	if count != len(groupItems) {
		t.Errorf("Count (%d) doesn't match Group length (%d)", count, len(groupItems))
	}
}

func TestGroupObject_NestedProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Address": map[string]any{"City": "NYC"}},
		map[string]any{"Name": "Bob", "Address": map[string]any{"City": "NYC"}},
		map[string]any{"Name": "Charlie", "Address": map[string]any{"City": "LA"}},
	}

	opts := GroupObjectOptions{Property: "Address.City"}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(result))
	}

	nycGroup := GetGroupByName(result, "NYC")
	if nycGroup == nil {
		t.Fatal("NYC group not found")
	}
	if nycGroup.Count != 2 {
		t.Errorf("Expected NYC group to have 2 items, got %d", nycGroup.Count)
	}
}

func TestGroupObject_SortGroupsByCount(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Department": "Engineering"},
		map[string]any{"Name": "Bob", "Department": "Engineering"},
		map[string]any{"Name": "Charlie", "Department": "Engineering"},
		map[string]any{"Name": "Diana", "Department": "Sales"},
		map[string]any{"Name": "Eve", "Department": "Sales"},
		map[string]any{"Name": "Frank", "Department": "HR"},
	}

	opts := GroupObjectOptions{Property: "Department"}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	// Sort descending by count
	sorted := SortGroupsByCount(result, true)
	if len(sorted) != 3 {
		t.Fatalf("Expected 3 groups, got %d", len(sorted))
	}

	// First should be Engineering (3 items)
	firstName := getGroupName(sorted[0])
	if firstName != "Engineering" {
		t.Errorf("Expected first group to be Engineering, got %s", firstName)
	}

	// Last should be HR (1 item)
	lastName := getGroupName(sorted[2])
	if lastName != "HR" {
		t.Errorf("Expected last group to be HR, got %s", lastName)
	}
}

func TestGroupObject_SortGroupsByName(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Department": "Sales"},
		map[string]any{"Name": "Bob", "Department": "Engineering"},
		map[string]any{"Name": "Charlie", "Department": "HR"},
	}

	opts := GroupObjectOptions{Property: "Department"}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	// Sort alphabetically
	sorted := SortGroupsByName(result, false)
	if len(sorted) != 3 {
		t.Fatalf("Expected 3 groups, got %d", len(sorted))
	}

	// Should be: Engineering, HR, Sales
	expectedOrder := []string{"Engineering", "HR", "Sales"}
	for i, expected := range expectedOrder {
		name := getGroupName(sorted[i])
		if name != expected {
			t.Errorf("Position %d: expected %s, got %s", i, expected, name)
		}
	}
}

func TestParseGroupObjectArgs(t *testing.T) {
	args := []any{
		[]any{
			map[string]any{"Name": "Alice"},
			map[string]any{"Name": "Bob"},
		},
		map[string]any{
			"property":      "Name",
			"casesensitive": true,
			"noelement":     true,
		},
	}

	objects, opts, err := ParseGroupObjectArgs(args)
	if err != nil {
		t.Fatalf("ParseGroupObjectArgs failed: %v", err)
	}

	if len(objects) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(objects))
	}

	if opts.Property != "Name" {
		t.Errorf("Expected Property=Name, got %s", opts.Property)
	}

	if !opts.CaseSensitive {
		t.Error("Expected CaseSensitive=true")
	}

	if !opts.NoElement {
		t.Error("Expected NoElement=true")
	}
}

func TestGetGroupCount(t *testing.T) {
	groups := []any{
		map[string]any{"Name": "A", "Count": 5},
		map[string]any{"Name": "B", "Count": 3},
		map[string]any{"Name": "C", "Count": 7},
	}

	count := GetGroupCount(groups)
	if count != 3 {
		t.Errorf("Expected 3 groups, got %d", count)
	}
}

func TestGetGroupByName_NotFound(t *testing.T) {
	groups := []any{
		map[string]any{"Name": "Engineering", "Count": 2},
		map[string]any{"Name": "Sales", "Count": 3},
	}

	result := GetGroupByName(groups, "NonExistent")
	if result != nil {
		t.Error("Expected nil for non-existent group")
	}
}

func TestGroupObject_WithNumericValues(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 25},
		map[string]any{"Name": "Bob", "Age": 30},
		map[string]any{"Name": "Charlie", "Age": 25},
		map[string]any{"Name": "Diana", "Age": 30},
		map[string]any{"Name": "Eve", "Age": 25},
	}

	opts := GroupObjectOptions{Property: "Age"}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(result))
	}

	// Find age 25 group
	age25Group := GetGroupByName(result, "25")
	if age25Group == nil {
		t.Fatal("Age 25 group not found")
	}
	if age25Group.Count != 3 {
		t.Errorf("Expected age 25 group to have 3 items, got %d", age25Group.Count)
	}
}

func TestGroupObject_WithBooleanValues(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Active": true},
		map[string]any{"Name": "Bob", "Active": false},
		map[string]any{"Name": "Charlie", "Active": true},
	}

	opts := GroupObjectOptions{Property: "Active"}
	result, err := groupObject(objects, opts)
	if err != nil {
		t.Fatalf("groupObject failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(result))
	}
}
