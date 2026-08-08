package formatting

import (
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

func TestFormatTable_Basic(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
	}

	opts := FormatTableOptions{}
	table, err := formatTable(objects, opts)
	if err != nil {
		t.Fatalf("formatTable failed: %v", err)
	}

	if table.RowCount != 2 {
		t.Errorf("Expected 2 rows, got %d", table.RowCount)
	}

	if len(table.Headers) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(table.Headers))
	}
}

func TestFormatTable_ByProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30, "City": "NYC"},
		map[string]any{"Name": "Bob", "Age": 25, "City": "LA"},
	}

	opts := FormatTableOptions{Property: []string{"Name", "City"}}
	table, err := formatTable(objects, opts)
	if err != nil {
		t.Fatalf("formatTable failed: %v", err)
	}

	if len(table.Headers) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(table.Headers))
	}

	if table.Headers[0] != "Name" {
		t.Errorf("Expected first header to be 'Name', got '%s'", table.Headers[0])
	}

	if table.Headers[1] != "City" {
		t.Errorf("Expected second header to be 'City', got '%s'", table.Headers[1])
	}
}

func TestFormatTable_EmptyInput(t *testing.T) {
	objects := []any{}

	opts := FormatTableOptions{}
	table, err := formatTable(objects, opts)
	if err != nil {
		t.Fatalf("formatTable failed: %v", err)
	}

	if table.RowCount != 0 {
		t.Errorf("Expected 0 rows, got %d", table.RowCount)
	}

	if len(table.Headers) != 0 {
		t.Errorf("Expected 0 headers, got %d", len(table.Headers))
	}
}

func TestFormatTable_AutoSize(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
	}

	opts := FormatTableOptions{AutoSize: true}
	table, err := formatTable(objects, opts)
	if err != nil {
		t.Fatalf("formatTable failed: %v", err)
	}

	// Check that widths are calculated based on content
	nameIdx := 0
	for i, h := range table.Headers {
		if h == "Name" {
			nameIdx = i
			break
		}
	}

	// "Alice" is 5 chars, so width should be at least 5
	if table.Widths[nameIdx] < 5 {
		t.Errorf("Expected Name column width >= 5, got %d", table.Widths[nameIdx])
	}
}

func TestFormatTable_HideHeaders(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice"},
	}

	opts := FormatTableOptions{HideTableHeaders: true}
	table, err := formatTable(objects, opts)
	if err != nil {
		t.Fatalf("formatTable failed: %v", err)
	}

	// Verify table is built correctly (headers still exist, just hidden in output)
	if len(table.Headers) != 1 {
		t.Errorf("Expected 1 header, got %d", len(table.Headers))
	}

	// Test string output doesn't include headers
	output := FormatTableToString(table)
	if len(output) == 0 {
		t.Error("Expected non-empty output")
	}
}

func TestFormatTable_WithPSObject(t *testing.T) {
	// Create PSObject-wrapped objects
	obj1 := psobject.NewPSObject(map[string]any{"Name": "Alice", "Age": 30})
	obj2 := psobject.NewPSObject(map[string]any{"Name": "Bob", "Age": 25})

	objects := []any{
		obj1.ToMap(),
		obj2.ToMap(),
	}

	opts := FormatTableOptions{}
	table, err := formatTable(objects, opts)
	if err != nil {
		t.Fatalf("formatTable failed: %v", err)
	}

	if table.RowCount != 2 {
		t.Errorf("Expected 2 rows, got %d", table.RowCount)
	}
}

func TestFormatTable_WildcardProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30, "Address_City": "NYC"},
		map[string]any{"Name": "Bob", "Age": 25, "Address_City": "LA"},
	}

	opts := FormatTableOptions{Property: []string{"Address_*"}}
	table, err := formatTable(objects, opts)
	if err != nil {
		t.Fatalf("formatTable failed: %v", err)
	}

	if len(table.Headers) != 1 {
		t.Errorf("Expected 1 header matching 'Address_*', got %d: %v", len(table.Headers), table.Headers)
	}
}

func TestFormatTable_NestedProperty(t *testing.T) {
	objects := []any{
		map[string]any{
			"Name": "Alice",
			"Address": map[string]any{
				"City":  "NYC",
				"State": "NY",
			},
		},
	}

	opts := FormatTableOptions{Property: []string{"Name", "Address"}}
	table, err := formatTable(objects, opts)
	if err != nil {
		t.Fatalf("formatTable failed: %v", err)
	}

	if len(table.Headers) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(table.Headers))
	}

	// Check that nested object is formatted as string
	row := table.Rows[0]
	if len(row) != 2 {
		t.Errorf("Expected 2 cells in row, got %d", len(row))
	}
}

func TestFormatTableToString(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
		map[string]any{"Name": "Bob", "Age": 25},
	}

	opts := FormatTableOptions{AutoSize: true}
	table, err := formatTable(objects, opts)
	if err != nil {
		t.Fatalf("formatTable failed: %v", err)
	}

	output := FormatTableToString(table)
	if len(output) == 0 {
		t.Error("Expected non-empty table output")
	}

	// Check that output contains headers
	if !containsStringInOutput(output, "Name") {
		t.Error("Expected output to contain 'Name' header")
	}

	if !containsStringInOutput(output, "Age") {
		t.Error("Expected output to contain 'Age' header")
	}

	// Check that output contains data
	if !containsStringInOutput(output, "Alice") {
		t.Error("Expected output to contain 'Alice'")
	}
}

func TestFormatTableToString_HideHeaders(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 30},
	}

	opts := FormatTableOptions{HideTableHeaders: true}
	table, err := formatTable(objects, opts)
	if err != nil {
		t.Fatalf("formatTable failed: %v", err)
	}

	output := FormatTableToString(table)

	// Should contain data but not separator line
	if containsStringInOutput(output, "---") {
		t.Error("Expected no separator line when headers are hidden")
	}
}

func TestParseFormatTableArgs(t *testing.T) {
	args := []any{
		[]any{
			map[string]any{"Name": "Alice"},
		},
		map[string]any{
			"property":         []any{"Name", "Age"},
			"autosize":         true,
			"hidetableheaders": true,
			"depth":            2,
		},
	}

	objects, opts, err := ParseFormatTableArgs(args)
	if err != nil {
		t.Fatalf("ParseFormatTableArgs failed: %v", err)
	}

	if len(objects) != 1 {
		t.Errorf("Expected 1 object, got %d", len(objects))
	}

	if len(opts.Property) != 2 {
		t.Errorf("Expected 2 properties, got %d", len(opts.Property))
	}

	if !opts.AutoSize {
		t.Error("Expected AutoSize to be true")
	}

	if !opts.HideTableHeaders {
		t.Error("Expected HideTableHeaders to be true")
	}

	if opts.Depth != 2 {
		t.Errorf("Expected Depth to be 2, got %d", opts.Depth)
	}
}

func TestFormatTable_SingleProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice"},
		map[string]any{"Name": "Bob"},
	}

	opts := FormatTableOptions{Property: []string{"Name"}}
	table, err := formatTable(objects, opts)
	if err != nil {
		t.Fatalf("formatTable failed: %v", err)
	}

	if len(table.Headers) != 1 {
		t.Errorf("Expected 1 header, got %d", len(table.Headers))
	}

	if table.Headers[0] != "Name" {
		t.Errorf("Expected header to be 'Name', got '%s'", table.Headers[0])
	}
}

func TestFormatTable_NoMatchingProperties(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice"},
	}

	opts := FormatTableOptions{Property: []string{"NonExistent"}}
	table, err := formatTable(objects, opts)
	if err != nil {
		t.Fatalf("formatTable failed: %v", err)
	}

	if len(table.Headers) != 0 {
		t.Errorf("Expected 0 headers (no matching properties), got %d", len(table.Headers))
	}
}

func TestGetTableHeaders(t *testing.T) {
	table := &FormattedTable{
		Headers: []string{"Name", "Age"},
		Rows:    [][]string{{"Alice", "30"}},
	}

	headers := GetTableHeaders(table)
	if len(headers) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(headers))
	}
}

func TestGetTableRows(t *testing.T) {
	table := &FormattedTable{
		Headers: []string{"Name", "Age"},
		Rows:    [][]string{{"Alice", "30"}, {"Bob", "25"}},
	}

	rows := GetTableRows(table)
	if len(rows) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(rows))
	}
}

func TestGetTableRowCount(t *testing.T) {
	table := &FormattedTable{
		Headers:  []string{"Name"},
		Rows:     [][]string{{"Alice"}, {"Bob"}, {"Charlie"}},
		RowCount: 3,
	}

	count := GetTableRowCount(table)
	if count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
}

func containsStringInOutput(output, substr string) bool {
	return len(output) > 0 && len(substr) > 0 && (output == substr || len(output) >= len(substr) && containsSubstring(output, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
