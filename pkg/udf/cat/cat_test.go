package cat

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/itchyny/gojq"
)

// Helper to compile and run a gojq query
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

func TestCatFromPipe(t *testing.T) {
	// Create a temporary file with test content
	tmpFile, err := os.CreateTemp("", "pwrq_cat_test_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	testContent := "Hello, World!\nThis is a test file."
	_, err = tmpFile.WriteString(testContent)
	if err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Test cat with file path from pipe
	result := runGojqQuery(t, "cat", tmpFile.Name(), RegisterCat())
	if err, ok := result.(error); ok {
		t.Fatalf("cat function returned an error: %v", err)
	}

	// Check that result is a UDF result object
	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Check _val matches file content
	val, ok := resMap["_val"].(string)
	if !ok {
		t.Fatalf("expected _val to be string, got %T", resMap["_val"])
	}

	if val != testContent {
		t.Errorf("expected _val to be %q, got %q", testContent, val)
	}

	// Check _meta
	meta, ok := resMap["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected _meta to be map[string]any, got %T", resMap["_meta"])
	}

	if meta["operation"] != "cat" {
		t.Errorf("expected operation 'cat', got %v", meta["operation"])
	}

	absPath, err := filepath.Abs(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	if meta["file_path"] != absPath {
		t.Errorf("expected file_path %q, got %q", absPath, meta["file_path"])
	}
}

func TestCatFromArgument(t *testing.T) {
	// Create a temporary file with test content
	tmpFile, err := os.CreateTemp("", "pwrq_cat_test_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	testContent := "Test content from argument"
	_, err = tmpFile.WriteString(testContent)
	if err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Test cat with file path as argument
	result := runGojqQuery(t, "cat(\""+tmpFile.Name()+"\")", nil, RegisterCat())
	if err, ok := result.(error); ok {
		t.Fatalf("cat function returned an error: %v", err)
	}

	// Check that result is a UDF result object
	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Check _val matches file content
	val, ok := resMap["_val"].(string)
	if !ok {
		t.Fatalf("expected _val to be string, got %T", resMap["_val"])
	}

	if val != testContent {
		t.Errorf("expected _val to be %q, got %q", testContent, val)
	}
}

func TestCatFileNotFound(t *testing.T) {
	// Test cat with non-existent file
	result := runGojqQuery(t, "cat(\"/nonexistent/file.txt\")", nil, RegisterCat())

	// Should return error result
	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any error result, got %T", result)
	}

	if errStr, hasErr := resMap["_err"].(string); !hasErr {
		t.Errorf("expected _err field in result, got %v", result)
	} else if errStr == "" {
		t.Errorf("expected non-empty error message, got empty string")
	}

	// _val should be null on error
	if resMap["_val"] != nil {
		t.Errorf("expected _val to be null on error, got %v", resMap["_val"])
	}
}

func TestCatDirectoryError(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "pwrq_cat_test_*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test cat with directory path
	result := runGojqQuery(t, "cat(\""+tmpDir+"\")", nil, RegisterCat())

	// Should return error result
	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any error result, got %T", result)
	}

	if errStr, hasErr := resMap["_err"].(string); !hasErr {
		t.Errorf("expected _err field in result, got %v", result)
	} else if errStr == "" {
		t.Errorf("expected non-empty error message, got empty string")
	}
}

func TestCatWithUDFResult(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "pwrq_cat_test_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	testContent := "UDF result test"
	_, err = tmpFile.WriteString(testContent)
	if err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Test cat with UDF result as input
	udfResult := map[string]any{
		"_val":  tmpFile.Name(),
		"_meta": map[string]any{"source": "previous_udf"},
	}

	// Test without ._val to get the full UDF result object
	result := runGojqQuery(t, "cat", udfResult, RegisterCat())
	if err, ok := result.(error); ok {
		t.Fatalf("cat function returned an error: %v", err)
	}

	// Check that result is a UDF result object
	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Check _val matches file content
	val, ok := resMap["_val"].(string)
	if !ok {
		t.Fatalf("expected _val to be string, got %T", resMap["_val"])
	}

	if val != testContent {
		t.Errorf("expected _val to be %q, got %q", testContent, val)
	}
}

func TestCatChaining(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "pwrq_cat_test_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	testContent := "hello world"
	_, err = tmpFile.WriteString(testContent)
	if err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Test that cat can be chained
	result := runGojqQuery(t, "cat", tmpFile.Name(), RegisterCat())
	if err, ok := result.(error); ok {
		t.Fatalf("cat function returned an error: %v", err)
	}

	// Extract value
	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	val := resMap["_val"]
	if val != testContent {
		t.Errorf("expected _val to be %q, got %q", testContent, val)
	}
}

func TestCatGojqIntegration(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "pwrq_cat_test_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	testContent := "integration test"
	_, err = tmpFile.WriteString(testContent)
	if err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	tests := []struct {
		name     string
		input    any
		query    string
		expected string
		options  []gojq.CompilerOption
	}{
		{
			name:     "cat from pipe",
			input:    tmpFile.Name(),
			query:    `cat | ._val`,
			expected: testContent,
			options:  []gojq.CompilerOption{RegisterCat()},
		},
		{
			name:     "cat from argument",
			input:    nil,
			query:    `cat("` + tmpFile.Name() + `") | ._val`,
			expected: testContent,
			options:  []gojq.CompilerOption{RegisterCat()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runGojqQuery(t, tt.query, tt.input, tt.options...)

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

