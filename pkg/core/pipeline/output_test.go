package pipeline

import (
	"strings"
	"testing"
)

// TestTableFormatter tests tabular output formatting.
func TestTableFormatter(t *testing.T) {
	t.Run("formats single object as table", func(t *testing.T) {
		formatter := &TableFormatter{
			Properties: []string{"Name", "Value"},
		}

		obj := map[string]any{
			"Name":  "test",
			"Value": "123",
		}

		result, err := formatter.Format(obj)
		if err != nil {
			t.Fatalf("Format failed: %v", err)
		}

		output := string(result)
		lines := strings.Split(strings.TrimSpace(output), "\n")

		if len(lines) < 3 {
			t.Fatalf("expected at least 3 lines (header, separator, data), got %d", len(lines))
		}

		// Check header contains column names
		if !strings.Contains(lines[0], "Name") || !strings.Contains(lines[0], "Value") {
			t.Errorf("header missing column names: %s", lines[0])
		}

		// Check data row
		if !strings.Contains(lines[2], "test") || !strings.Contains(lines[2], "123") {
			t.Errorf("data row missing values: %s", lines[2])
		}
	})

	t.Run("formats slice of objects", func(t *testing.T) {
		formatter := &TableFormatter{
			Properties: []string{"ID", "Name"},
		}

		objs := []map[string]any{
			{"ID": "1", "Name": "Alice"},
			{"ID": "2", "Name": "Bob"},
		}

		result, err := formatter.Format(objs)
		if err != nil {
			t.Fatalf("Format failed: %v", err)
		}

		output := string(result)
		lines := strings.Split(strings.TrimSpace(output), "\n")

		// Should have header, separator, and 2 data rows
		if len(lines) != 4 {
			t.Errorf("expected 4 lines, got %d: %s", len(lines), output)
		}

		if !strings.Contains(output, "Alice") || !strings.Contains(output, "Bob") {
			t.Errorf("missing data rows: %s", output)
		}
	})

	t.Run("auto-extracts properties when empty", func(t *testing.T) {
		formatter := &TableFormatter{
			Properties: nil,
		}

		obj := map[string]any{
			"Foo": "bar",
			"Baz": "qux",
		}

		result, err := formatter.Format(obj)
		if err != nil {
			t.Fatalf("Format failed: %v", err)
		}

		output := string(result)
		if !strings.Contains(output, "Foo") || !strings.Contains(output, "Baz") {
			t.Errorf("missing auto-extracted properties: %s", output)
		}
	})
}

// TestListFormatter tests list output formatting.
func TestListFormatter(t *testing.T) {
	t.Run("formats single object as list", func(t *testing.T) {
		formatter := &ListFormatter{
			Properties: []string{"Name", "Value"},
		}

		obj := map[string]any{
			"Name":  "test",
			"Value": "123",
		}

		result, err := formatter.Format(obj)
		if err != nil {
			t.Fatalf("Format failed: %v", err)
		}

		output := string(result)
		if !strings.Contains(output, "Name") || !strings.Contains(output, "test") {
			t.Errorf("missing Name property: %s", output)
		}
		if !strings.Contains(output, "Value") || !strings.Contains(output, "123") {
			t.Errorf("missing Value property: %s", output)
		}
	})

	t.Run("formats slice as multiple lists", func(t *testing.T) {
		formatter := &ListFormatter{
			Properties: []string{"ID"},
		}

		objs := []map[string]any{
			{"ID": "1", "Name": "Alice"},
			{"ID": "2", "Name": "Bob"},
		}

		result, err := formatter.Format(objs)
		if err != nil {
			t.Fatalf("Format failed: %v", err)
		}

		output := string(result)
		// Should contain both IDs
		if !strings.Contains(output, "1") || !strings.Contains(output, "2") {
			t.Errorf("missing list entries: %s", output)
		}
	})

	t.Run("formats struct properties", func(t *testing.T) {
		formatter := &ListFormatter{
			Properties: []string{"Name", "Age"},
		}

		type Person struct {
			Name string
			Age  int
		}

		obj := Person{Name: "John", Age: 30}

		result, err := formatter.Format(obj)
		if err != nil {
			t.Fatalf("Format failed: %v", err)
		}

		output := string(result)
		if !strings.Contains(output, "Name") || !strings.Contains(output, "John") {
			t.Errorf("missing Name: %s", output)
		}
		if !strings.Contains(output, "Age") || !strings.Contains(output, "30") {
			t.Errorf("missing Age: %s", output)
		}
	})
}

// TestFormatList tests the FormatList helper function.
func TestFormatList(t *testing.T) {
	t.Run("formats map properties", func(t *testing.T) {
		obj := map[string]any{
			"Key1": "Value1",
			"Key2": "Value2",
		}

		result := FormatList(obj, nil)
		if !strings.Contains(result, "Key1") || !strings.Contains(result, "Value1") {
			t.Errorf("missing properties: %s", result)
		}
	})

	t.Run("formats struct fields", func(t *testing.T) {
		type Config struct {
			Host string
			Port int
		}

		obj := Config{Host: "localhost", Port: 8080}

		result := FormatList(obj, nil)
		if !strings.Contains(result, "Host") || !strings.Contains(result, "localhost") {
			t.Errorf("missing Host: %s", result)
		}
		if !strings.Contains(result, "Port") || !strings.Contains(result, "8080") {
			t.Errorf("missing Port: %s", result)
		}
	})
}
