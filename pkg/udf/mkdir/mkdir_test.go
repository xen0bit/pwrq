package mkdir

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/itchyny/gojq"
)

func runGojqQuery(t *testing.T, query string, input any, options ...gojq.CompilerOption) any {
	code, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("Failed to parse query %q: %v", query, err)
	}

	compiled, err := gojq.Compile(code, options...)
	if err != nil {
		t.Fatalf("Failed to compile query %q: %v", query, err)
	}

	iter := compiled.Run(input)
	result, ok := iter.Next()
	if !ok {
		t.Fatalf("Query returned no result")
	}

	if err, ok := result.(error); ok {
		t.Fatalf("Query returned error: %v", err)
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

// priorCmdletOutput models what an upstream cmdlet now puts on the pipeline:
// the value itself, with no envelope around it.
func priorCmdletOutput(value any, _ map[string]any) any { return value }

func TestMkdir_Basic(t *testing.T) {
	// Create a temporary directory to test in
	parentDir, err := os.MkdirTemp("", "pwrq_mkdir_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	testDir := filepath.Join(parentDir, "testdir")

	result := runGojqQuery(t, `mkdir("`+testDir+`")`, nil, RegisterMkdir())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	// Verify the directory exists
	if _, err := os.Stat(val); err != nil {
		t.Fatalf("Created directory does not exist: %v", err)
	}

	// Verify it's actually a directory
	info, err := os.Stat(val)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("Created path is not a directory")
	}

	// Check metadata
}

func TestMkdir_NestedPath(t *testing.T) {
	// Create a temporary directory to test in
	parentDir, err := os.MkdirTemp("", "pwrq_mkdir_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	nestedDir := filepath.Join(parentDir, "level1", "level2", "level3")

	result := runGojqQuery(t, `mkdir("`+nestedDir+`")`, nil, RegisterMkdir())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	// Verify the nested directory exists
	if _, err := os.Stat(val); err != nil {
		t.Fatalf("Created nested directory does not exist: %v", err)
	}

	// Verify all parent directories were created
	if _, err := os.Stat(filepath.Join(parentDir, "level1")); err != nil {
		t.Fatalf("Parent directory level1 was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parentDir, "level1", "level2")); err != nil {
		t.Fatalf("Parent directory level2 was not created: %v", err)
	}
}

func TestMkdir_AlreadyExists(t *testing.T) {
	// Create a temporary directory to test in
	parentDir, err := os.MkdirTemp("", "pwrq_mkdir_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	testDir := filepath.Join(parentDir, "existingdir")
	err = os.MkdirAll(testDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create existing directory: %v", err)
	}

	// Try to create it again
	result := runGojqQuery(t, `mkdir("`+testDir+`")`, nil, RegisterMkdir())

	// Should succeed (like mkdir -p)
	val, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}

	if val != testDir {
		t.Errorf("Expected path %q, got %q", testDir, val)
	}

	// Check metadata indicates it already existed
}

func TestMkdir_NoArgs(t *testing.T) {
	// This should fail because mkdir requires an argument
	code, err := gojq.Parse("mkdir()")
	if err != nil {
		// Parser error is expected
		return
	}

	compiled, err := gojq.Compile(code, RegisterMkdir())
	if err != nil {
		// Compilation error is expected
		return
	}

	iter := compiled.Run(nil)
	result, ok := iter.Next()
	if !ok {
		// No result is acceptable (parser/compiler should catch this)
		return
	}

	// If we get here, check that it's an error
	resultMap, ok := result.(map[string]any)
	if ok {
		if errVal, hasErr := resultMap["_err"]; hasErr {
			errStr, ok := errVal.(string)
			if ok && errStr != "" {
				// Error is expected
				return
			}
		}
	}

	t.Errorf("Expected error for mkdir() with no arguments, got %v", result)
}

func TestMkdir_InvalidPath(t *testing.T) {
	parentDir, err := os.MkdirTemp("", "pwrq_mkdir_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	// A file cannot be a parent directory.
	testFile := filepath.Join(parentDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	qErr := runGojqQueryErr(t, `mkdir("`+filepath.Join(testFile, "subdir")+`")`, nil, RegisterMkdir())
	if qErr.Error() == "" {
		t.Errorf("Expected error message, got empty string")
	}
}

func TestMkdir_Chaining(t *testing.T) {
	parentDir, err := os.MkdirTemp("", "pwrq_mkdir_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	testDir := filepath.Join(parentDir, "chaintest")

	// Test that mkdir can be chained
	result := runGojqQuery(t, `mkdir("`+testDir+`") | length`, nil, RegisterMkdir())

	// Should return the length of the path string
	length, ok := result.(int)
	if !ok {
		t.Fatalf("Expected int result, got %T", result)
	}

	if length <= 0 {
		t.Errorf("Expected path length > 0, got %d", length)
	}
}

func TestMkdir_FromPipe(t *testing.T) {
	parentDir, err := os.MkdirTemp("", "pwrq_mkdir_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	testDir := filepath.Join(parentDir, "pipetest")

	// Test that mkdir works with input from pipe (though it requires argument)
	result := runGojqQuery(t, `"`+testDir+`" | mkdir(.)`, testDir, RegisterMkdir())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	// Verify the directory exists
	if _, err := os.Stat(val); err != nil {
		t.Fatalf("Created directory does not exist: %v", err)
	}
}

func TestMkdir_WithUDFResultInput(t *testing.T) {
	parentDir, err := os.MkdirTemp("", "pwrq_mkdir_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	testDir := filepath.Join(parentDir, "udfresulttest")

	// Test that mkdir works with UDF result objects
	udfResult := priorCmdletOutput(testDir, map[string]any{"test": "value"})

	result := runGojqQuery(t, `mkdir(.)`, udfResult, RegisterMkdir())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	// Verify the directory exists
	if _, err := os.Stat(val); err != nil {
		t.Fatalf("Created directory does not exist: %v", err)
	}
}
