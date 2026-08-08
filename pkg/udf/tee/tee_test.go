package tee

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
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

// runGojqQueryErr runs a query that is expected to fail and returns the error.
// UDF failures now travel on jq's error channel rather than in-band, so a test
// that wants a failure has to look for one there.
func runGojqQueryErr(t *testing.T, query string, input any, options ...gojq.CompilerOption) error {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("Failed to parse query %q: %v", query, err)
	}
	code, err := gojq.Compile(q, options...)
	if err != nil {
		t.Fatalf("Failed to compile query %q: %v", query, err)
	}
	iter := code.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return err
		}
	}
	t.Fatalf("expected query %q to fail, but it succeeded", query)
	return nil
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

	// tee passes its input through unchanged
	if !reflect.DeepEqual(result, input) {
		t.Errorf("tee must pass its input through unchanged, got %v", result)
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

	// Check _val matches input
	if !reflect.DeepEqual(result, input) {
		t.Errorf("tee must pass its input through unchanged, got %v", result)
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
		"PSPath":     "test_value",
		"PSTypeName": "System.String",
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
	invalidPath := "/nonexistent/directory/file.json"

	qErr := runGojqQueryErr(t, "tee(\""+invalidPath+"\")", "test", RegisterTee())
	if !strings.Contains(qErr.Error(), invalidPath) {
		t.Errorf("error should name the path it could not write, got %q", qErr)
	}
}

func TestTeeChaining(t *testing.T) {
	// Test that tee can be chained
	input := "hello"

	result := runGojqQuery(t, "tee", input, RegisterTee())
	if err, ok := result.(error); ok {
		t.Fatalf("tee function returned an error: %v", err)
	}

	// tee is transparent: what goes in comes out
	if result != input {
		t.Errorf("expected tee to pass through %q, got %q", input, result)
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
			query:    `tee`,
			expected: map[string]any{"test": "value"},
			options:  []gojq.CompilerOption{RegisterTee()},
		},
		{
			name:     "tee with file path",
			input:    "test_string",
			query:    `tee("/tmp/pwrq_test_tee.json")`,
			expected: "test_string",
			options:  []gojq.CompilerOption{RegisterTee()},
		},
		{
			name:     "tee in pipeline",
			input:    "hello",
			query:    `tee | upper`,
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
