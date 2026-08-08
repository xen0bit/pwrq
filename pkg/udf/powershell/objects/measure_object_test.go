package objects

import (
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

func TestMeasureObject_BasicCount(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice"},
		map[string]any{"Name": "Bob"},
		map[string]any{"Name": "Charlie"},
	}

	opts := MeasureObjectOptions{}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	if result.Count != 3 {
		t.Errorf("Expected count 3, got %d", result.Count)
	}
}

func TestMeasureObject_EmptyInput(t *testing.T) {
	objects := []any{}

	opts := MeasureObjectOptions{}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	if result.Count != 0 {
		t.Errorf("Expected count 0, got %d", result.Count)
	}
}

func TestMeasureObject_Sum(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 25},
		map[string]any{"Name": "Bob", "Age": 30},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := MeasureObjectOptions{Property: "Age", Sum: true}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	if result.Sum != 90 {
		t.Errorf("Expected sum 90, got %f", result.Sum)
	}
}

func TestMeasureObject_Average(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 25},
		map[string]any{"Name": "Bob", "Age": 30},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := MeasureObjectOptions{Property: "Age", Average: true}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	if result.Average != 30 {
		t.Errorf("Expected average 30, got %f", result.Average)
	}
}

func TestMeasureObject_Minimum(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 25},
		map[string]any{"Name": "Bob", "Age": 30},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := MeasureObjectOptions{Property: "Age", Minimum: true}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	if result.Minimum != float64(25) {
		t.Errorf("Expected minimum 25, got %v", result.Minimum)
	}
}

func TestMeasureObject_Maximum(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Age": 25},
		map[string]any{"Name": "Bob", "Age": 30},
		map[string]any{"Name": "Charlie", "Age": 35},
	}

	opts := MeasureObjectOptions{Property: "Age", Maximum: true}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	if result.Maximum != float64(35) {
		t.Errorf("Expected maximum 35, got %v", result.Maximum)
	}
}

func TestMeasureObject_AllStats(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Score": 10},
		map[string]any{"Name": "Bob", "Score": 20},
		map[string]any{"Name": "Charlie", "Score": 30},
		map[string]any{"Name": "Diana", "Score": 40},
	}

	opts := MeasureObjectOptions{Property: "Score", Sum: true, Average: true, Minimum: true, Maximum: true}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	if result.Count != 4 {
		t.Errorf("Expected count 4, got %d", result.Count)
	}
	if result.Sum != 100 {
		t.Errorf("Expected sum 100, got %f", result.Sum)
	}
	if result.Average != 25 {
		t.Errorf("Expected average 25, got %f", result.Average)
	}
	if result.Minimum != float64(10) {
		t.Errorf("Expected minimum 10, got %v", result.Minimum)
	}
	if result.Maximum != float64(40) {
		t.Errorf("Expected maximum 40, got %v", result.Maximum)
	}
}

func TestMeasureObject_NestedProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice", "Address": map[string]any{"ZipCode": 10001}},
		map[string]any{"Name": "Bob", "Address": map[string]any{"ZipCode": 10002}},
		map[string]any{"Name": "Charlie", "Address": map[string]any{"ZipCode": 10003}},
	}

	opts := MeasureObjectOptions{Property: "Address.ZipCode", Sum: true}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	if result.Sum != 30006 {
		t.Errorf("Expected sum 30006, got %f", result.Sum)
	}
}

func TestMeasureObject_MissingProperty(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "Alice"},
		map[string]any{"Name": "Bob"},
	}

	opts := MeasureObjectOptions{Property: "NonExistent", Sum: true}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	// Should skip objects without the property
	if result.Count != 2 {
		t.Errorf("Expected count 2, got %d", result.Count)
	}
	if result.Sum != 0 {
		t.Errorf("Expected sum 0 (no values), got %f", result.Sum)
	}
}

func TestMeasureObject_MixedNumericTypes(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "A", "Value": 10},
		map[string]any{"Name": "B", "Value": 20.5},
		map[string]any{"Name": "C", "Value": 30},
	}

	opts := MeasureObjectOptions{Property: "Value", Sum: true, Average: true}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	if result.Sum != 60.5 {
		t.Errorf("Expected sum 60.5, got %f", result.Sum)
	}
	expectedAvg := 60.5 / 3
	if result.Average != expectedAvg {
		t.Errorf("Expected average %f, got %f", expectedAvg, result.Average)
	}
}

func TestMeasureObject_StringNumbers(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "A", "Value": "10"},
		map[string]any{"Name": "B", "Value": "20"},
		map[string]any{"Name": "C", "Value": "30"},
	}

	opts := MeasureObjectOptions{Property: "Value", Sum: true}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	if result.Sum != 60 {
		t.Errorf("Expected sum 60, got %f", result.Sum)
	}
}

func TestMeasureObject_BooleanValues(t *testing.T) {
	objects := []any{
		map[string]any{"Name": "A", "Active": true},
		map[string]any{"Name": "B", "Active": false},
		map[string]any{"Name": "C", "Active": true},
		map[string]any{"Name": "D", "Active": true},
	}

	opts := MeasureObjectOptions{Property: "Active", Sum: true}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	// true = 1, false = 0
	if result.Sum != 3 {
		t.Errorf("Expected sum 3, got %f", result.Sum)
	}
}

func TestMeasureObject_PSObjectInput(t *testing.T) {
	psobj1 := psobject.NewPSObjectWithTypeName(map[string]any{"Name": "Alice", "Age": 25}, "System.Object")
	psobj1.AddNoteProperty("Name", "Alice")
	psobj1.AddNoteProperty("Age", 25)

	psobj2 := psobject.NewPSObjectWithTypeName(map[string]any{"Name": "Bob", "Age": 30}, "System.Object")
	psobj2.AddNoteProperty("Name", "Bob")
	psobj2.AddNoteProperty("Age", 30)

	objects := []any{
		psobj1.ToMap(),
		psobj2.ToMap(),
	}

	opts := MeasureObjectOptions{Property: "Age", Sum: true, Average: true}
	result, err := measureObject(objects, opts)
	if err != nil {
		t.Fatalf("measureObject failed: %v", err)
	}

	if result.Sum != 55 {
		t.Errorf("Expected sum 55, got %f", result.Sum)
	}
	if result.Average != 27.5 {
		t.Errorf("Expected average 27.5, got %f", result.Average)
	}
}

func TestMeasureObject_FormatResult(t *testing.T) {
	result := &MeasurementResult{
		Count:   5,
		Sum:     100,
		Average: 20,
		Minimum: float64(10),
		Maximum: float64(30),
	}

	opts := MeasureObjectOptions{Property: "Value", Sum: true, Average: true, Minimum: true, Maximum: true}
	output := formatMeasurementResult(result, opts)

	if !psobject.IsPSObject(output) {
		t.Error("Expected PSObject output")
	}

	count := GetMeasurementCount(output)
	if count != 5 {
		t.Errorf("Expected count 5, got %d", count)
	}

	sum := GetMeasurementSum(output)
	if sum != 100 {
		t.Errorf("Expected sum 100, got %f", sum)
	}
}

func TestParseMeasureObjectArgs(t *testing.T) {
	args := []any{
		[]any{
			map[string]any{"Name": "Alice", "Age": 25},
			map[string]any{"Name": "Bob", "Age": 30},
		},
		map[string]any{
			"property":      "Age",
			"sum":           true,
			"average":       true,
			"minimum":       true,
			"maximum":       true,
			"casesensitive": false,
		},
	}

	objects, opts, err := ParseMeasureObjectArgs(args)
	if err != nil {
		t.Fatalf("ParseMeasureObjectArgs failed: %v", err)
	}

	if len(objects) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(objects))
	}

	if opts.Property != "Age" {
		t.Errorf("Expected Property=Age, got %s", opts.Property)
	}
	if !opts.Sum {
		t.Error("Expected Sum=true")
	}
	if !opts.Average {
		t.Error("Expected Average=true")
	}
	if !opts.Minimum {
		t.Error("Expected Minimum=true")
	}
	if !opts.Maximum {
		t.Error("Expected Maximum=true")
	}
}

func TestGetMeasurementCount(t *testing.T) {
	result := map[string]any{
		"Count": 5,
		"Sum":   100.0,
	}

	count := GetMeasurementCount(result)
	if count != 5 {
		t.Errorf("Expected count 5, got %d", count)
	}
}

func TestGetMeasurementCount_PSObject(t *testing.T) {
	psobj := psobject.NewPSObjectWithTypeName(map[string]any{
		"Count": 5,
		"Sum":   100.0,
	}, "Microsoft.PowerShell.Commands.GenericMeasureInfo")

	result := psobj.ToMap()

	count := GetMeasurementCount(result)
	if count != 5 {
		t.Errorf("Expected count 5, got %d", count)
	}
}

func TestGetMeasurementSum(t *testing.T) {
	result := map[string]any{
		"Count": 5,
		"Sum":   100.0,
	}

	sum := GetMeasurementSum(result)
	if sum != 100.0 {
		t.Errorf("Expected sum 100.0, got %f", sum)
	}
}

func TestGetMeasurementAverage(t *testing.T) {
	result := map[string]any{
		"Count":   5,
		"Average": 20.0,
	}

	avg := GetMeasurementAverage(result)
	if avg != 20.0 {
		t.Errorf("Expected average 20.0, got %f", avg)
	}
}

func TestGetMeasurementMinimum(t *testing.T) {
	result := map[string]any{
		"Count":   5,
		"Minimum": float64(10),
	}

	min := GetMeasurementMinimum(result)
	if min != float64(10) {
		t.Errorf("Expected minimum 10, got %v", min)
	}
}

func TestGetMeasurementMaximum(t *testing.T) {
	result := map[string]any{
		"Count":   5,
		"Maximum": float64(30),
	}

	max := GetMeasurementMaximum(result)
	if max != float64(30) {
		t.Errorf("Expected maximum 30, got %v", max)
	}
}

func TestConvertToFloat64_IntTypes(t *testing.T) {
	tests := []struct {
		input    any
		expected float64
	}{
		{int(42), 42.0},
		{int8(42), 42.0},
		{int16(42), 42.0},
		{int32(42), 42.0},
		{int64(42), 42.0},
		{uint(42), 42.0},
		{uint8(42), 42.0},
		{uint16(42), 42.0},
		{uint32(42), 42.0},
		{uint64(42), 42.0},
		{float64(42.5), 42.5},
	}

	for _, test := range tests {
		result, err := convertToFloat64(test.input)
		if err != nil {
			t.Errorf("convertToFloat64(%v) failed: %v", test.input, err)
		}
		if result != test.expected {
			t.Errorf("convertToFloat64(%v) = %f, expected %f", test.input, result, test.expected)
		}
	}
}

func TestConvertToFloat64_String(t *testing.T) {
	result, err := convertToFloat64("42.5")
	if err != nil {
		t.Fatalf("convertToFloat64 failed: %v", err)
	}
	if result != 42.5 {
		t.Errorf("Expected 42.5, got %f", result)
	}

	_, err = convertToFloat64("not_a_number")
	if err == nil {
		t.Error("Expected error for non-numeric string")
	}
}

func TestConvertToFloat64_Bool(t *testing.T) {
	result, err := convertToFloat64(true)
	if err != nil {
		t.Fatalf("convertToFloat64(true) failed: %v", err)
	}
	if result != 1 {
		t.Errorf("Expected 1, got %f", result)
	}

	result, err = convertToFloat64(false)
	if err != nil {
		t.Fatalf("convertToFloat64(false) failed: %v", err)
	}
	if result != 0 {
		t.Errorf("Expected 0, got %f", result)
	}
}
