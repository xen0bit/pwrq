package tee

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/itchyny/gojq"
	stringudf "github.com/xen0bit/pwrq/pkg/udf/string"
)

// Helper to compile and run a gojq query with tee and other UDFs
func runGojqQuery(t *testing.T, query string, input any, options ...gojq.CompilerOption) any {
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("Failed to parse query %q: %v", query, err)
	}

	code, err := gojq.Compile(q, options...)
	if err != nil {
		t.Fatalf("Failed to compile query %q: %v", query, err)
	}

	var result any
	iter := code.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			t.Fatalf("Query execution failed: %v", err)
		}
		result = v
	}
	return result
}

func TestTeeToStderr(t *testing.T) {

	input := map[string]any{
		"test": "value",
		"num":  42,
	}

	result := runGojqQuery(t, "tee", input, RegisterTee())
	if err, ok := result.(error); ok {
		t.Fatalf("tee function returned an error: %v", err)
	}

	// Check that result is a UDF result object
	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Check _val matches input
	if !reflect.DeepEqual(resMap["_val"], input) {
		t.Errorf("expected _val to match input, got %v", resMap["_val"])
	}

	// Check _meta
	meta, ok := resMap["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected _meta to be map[string]any, got %T", resMap["_meta"])
	}

	if meta["operation"] != "tee" {
		t.Errorf("expected operation 'tee', got %v", meta["operation"])
	}

	if meta["written_to"] != "stderr" {
		t.Errorf("expected written_to 'stderr', got %v", meta["written_to"])
	}
}

func TestTeeToFile(t *testing.T) {
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "pwrq_tee_test_*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	input := map[string]any{
		"test": "file_write",
		"data": []any{float64(1), float64(2), float64(3)}, // JSON numbers unmarshal as float64
	}

	result := runGojqQuery(t, "tee(\""+tmpFile.Name()+"\")", input, RegisterTee())
	if err, ok := result.(error); ok {
		t.Fatalf("tee function returned an error: %v", err)
	}

	// Check that result is a UDF result object
	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Check _val matches input
	if !reflect.DeepEqual(resMap["_val"], input) {
		t.Errorf("expected _val to match input, got %v", resMap["_val"])
	}

	// Check _meta
	meta, ok := resMap["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected _meta to be map[string]any, got %T", resMap["_meta"])
	}

	if meta["operation"] != "tee" {
		t.Errorf("expected operation 'tee', got %v", meta["operation"])
	}

	if meta["written"] != true {
		t.Errorf("expected written true, got %v", meta["written"])
	}

	absPath, err := filepath.Abs(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	if meta["file_path"] != absPath {
		t.Errorf("expected file_path %q, got %q", absPath, meta["file_path"])
	}

	// Verify file was written
	fileData, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	var fileContent map[string]any
	if err := json.Unmarshal(fileData, &fileContent); err != nil {
		t.Fatalf("failed to unmarshal file content: %v", err)
	}

	// JSON unmarshaling may reorder map keys, so we need to compare values
	// Check that all keys and values match
	for k, v := range input {
		if fileV, ok := fileContent[k]; !ok {
			t.Errorf("file content missing key %q", k)
		} else if !reflect.DeepEqual(fileV, v) {
			t.Errorf("file content value for key %q doesn't match: got %v, want %v", k, fileV, v)
		}
	}
	// Check that file doesn't have extra keys
	for k := range fileContent {
		if _, ok := input[k]; !ok {
			t.Errorf("file content has extra key %q", k)
		}
	}
}

func TestTeeWithUDFResult(t *testing.T) {
	// Test that tee passes through UDF results correctly
	udfResult := map[string]any{
		"_val":  "test_value",
		"_meta": map[string]any{"source": "previous_udf"},
	}

	result := runGojqQuery(t, "tee", udfResult, RegisterTee())
	if err, ok := result.(error); ok {
		t.Fatalf("tee function returned an error: %v", err)
	}

	// Should return the UDF result as-is
	if !reflect.DeepEqual(result, udfResult) {
		t.Errorf("expected UDF result to pass through unchanged, got %v", result)
	}
}

func TestTeeErrorHandling(t *testing.T) {
	// Test with invalid file path (directory that doesn't exist)
	invalidPath := "/nonexistent/directory/file.json"

	result := runGojqQuery(t, "tee(\""+invalidPath+"\")", "test", RegisterTee())
	if err, ok := result.(error); ok {
		// This is expected - should return error in _err format
		t.Logf("Got expected error: %v", err)
	} else {
		// Check if it's an error result object
		resMap, ok := result.(map[string]any)
		if ok {
			if errStr, hasErr := resMap["_err"].(string); hasErr {
				t.Logf("Got error in _err field: %s", errStr)
				return
			}
		}
		t.Errorf("expected error for invalid file path, got %v", result)
	}
}

func TestTeeChaining(t *testing.T) {
	// Test that tee can be chained
	input := "hello"

	result := runGojqQuery(t, "tee", input, RegisterTee())
	if err, ok := result.(error); ok {
		t.Fatalf("tee function returned an error: %v", err)
	}

	// Extract value and pass to another operation
	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	val := resMap["_val"]
	if val != input {
		t.Errorf("expected _val to be %q, got %q", input, val)
	}
}

func TestTeeGojqIntegration(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		query    string
		expected any
		options  []gojq.CompilerOption
	}{
		{
			name:     "tee to stderr",
			input:    map[string]any{"test": "value"},
			query:    `tee | ._val`,
			expected: map[string]any{"test": "value"},
			options:  []gojq.CompilerOption{RegisterTee()},
		},
		{
			name:     "tee with file path",
			input:    "test_string",
			query:    `tee("/tmp/pwrq_test_tee.json") | ._val`,
			expected: "test_string",
			options:  []gojq.CompilerOption{RegisterTee()},
		},
		{
			name:     "tee in pipeline",
			input:    "hello",
			query:    `tee | upper | ._val`,
			expected: "HELLO",
			options:  []gojq.CompilerOption{RegisterTee(), stringudf.RegisterUpper()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runGojqQuery(t, tt.query, tt.input, tt.options...)

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
