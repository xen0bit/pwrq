package rm

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

func TestRm_File(t *testing.T) {
	// Create a temporary directory to test in
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(parentDir) }()

	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result := runGojqQuery(t, `rm("`+testFile+`"; "file")`, nil, RegisterRm())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	if val != testFile {
		t.Errorf("Expected path %q, got %q", testFile, val)
	}

	// Verify the file was removed
	if _, err := os.Stat(testFile); err == nil {
		t.Fatalf("File was not removed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Unexpected error checking file: %v", err)
	}

	// Check metadata
}

func TestRm_Folder(t *testing.T) {
	// Create a temporary directory to test in
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(parentDir) }()

	testDir := filepath.Join(parentDir, "testdir")
	err = os.MkdirAll(testDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create a file inside the directory
	testFile := filepath.Join(testDir, "nested.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create nested file: %v", err)
	}

	result := runGojqQuery(t, `rm("`+testDir+`"; "folder")`, nil, RegisterRm())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	if val != testDir {
		t.Errorf("Expected path %q, got %q", testDir, val)
	}

	// Verify the folder was removed
	if _, err := os.Stat(testDir); err == nil {
		t.Fatalf("Folder was not removed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Unexpected error checking folder: %v", err)
	}

	// Check metadata
}

func TestRm_NestedFolder(t *testing.T) {
	// Create a temporary directory to test in
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(parentDir) }()

	nestedDir := filepath.Join(parentDir, "level1", "level2", "level3")
	err = os.MkdirAll(nestedDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create nested directory: %v", err)
	}

	// Create files in nested directories
	_ = os.WriteFile(filepath.Join(parentDir, "level1", "file1.txt"), []byte("test"), 0644)
	_ = os.WriteFile(filepath.Join(parentDir, "level1", "level2", "file2.txt"), []byte("test"), 0644)
	_ = os.WriteFile(filepath.Join(nestedDir, "file3.txt"), []byte("test"), 0644)

	topLevelDir := filepath.Join(parentDir, "level1")
	_ = runGojqQuery(t, `rm("`+topLevelDir+`"; "folder")`, nil, RegisterRm())

	// Verify the entire nested structure was removed
	if _, err := os.Stat(topLevelDir); err == nil {
		t.Fatalf("Nested folder was not removed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Unexpected error checking folder: %v", err)
	}
}

func TestRm_FileNotFound(t *testing.T) {
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(parentDir) }()

	nonexistentFile := filepath.Join(parentDir, "nonexistent.txt")

	qErr := runGojqQueryErr(t, `rm("`+nonexistentFile+`"; "file")`, nil, RegisterRm())
	errStr := qErr.Error()

	if errStr == "" {
		t.Errorf("Expected error message, got empty string")
	}
}

func TestRm_TypeMismatch(t *testing.T) {
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(parentDir) }()

	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Try to remove file as folder
	qErr := runGojqQueryErr(t, `rm("`+testFile+`"; "folder")`, nil, RegisterRm())
	errStr := qErr.Error()

	if errStr == "" {
		t.Errorf("Expected error message, got empty string")
	}

	// File should still exist
	if _, err := os.Stat(testFile); err != nil {
		t.Fatalf("File should still exist after type mismatch error")
	}
}

func TestRm_InvalidType(t *testing.T) {
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(parentDir) }()

	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer func() { _ = os.Remove(testFile) }()

	qErr := runGojqQueryErr(t, `rm("`+testFile+`"; "invalid")`, nil, RegisterRm())
	errStr := qErr.Error()

	if errStr == "" {
		t.Errorf("Expected error message, got empty string")
	}
}

func TestRm_NoArgs(t *testing.T) {
	// This should fail because rm requires 2 arguments
	code, err := gojq.Parse("rm()")
	if err != nil {
		// Parser error is expected
		return
	}

	compiled, err := gojq.Compile(code, RegisterRm())
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

	t.Errorf("Expected error for rm() with no arguments, got %v", result)
}

func TestRm_Chaining(t *testing.T) {
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(parentDir) }()

	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test that rm can be chained
	result := runGojqQuery(t, `rm("`+testFile+`"; "file") | length`, nil, RegisterRm())

	// Should return the length of the path string
	length, ok := result.(int)
	if !ok {
		t.Fatalf("Expected int result, got %T", result)
	}

	if length <= 0 {
		t.Errorf("Expected path length > 0, got %d", length)
	}
}

func TestRm_WithUDFResultInput(t *testing.T) {
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(parentDir) }()

	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test that rm works with UDF result objects
	udfResult := priorCmdletOutput(testFile, map[string]any{"test": "value"})

	result := runGojqQuery(t, `rm(.; "file")`, udfResult, RegisterRm())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	// Verify the file was removed
	if _, err := os.Stat(val); err == nil {
		t.Fatalf("File was not removed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Unexpected error checking file: %v", err)
	}
}

func TestRm_CaseInsensitiveType(t *testing.T) {
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(parentDir) }()

	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test with uppercase type
	result := runGojqQuery(t, `rm("`+testFile+`"; "FILE")`, nil, RegisterRm())

	// Should succeed (case insensitive)
	_, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}

	// Verify the file was removed
	if _, err := os.Stat(testFile); err == nil {
		t.Fatalf("File was not removed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Unexpected error checking file: %v", err)
	}
}
