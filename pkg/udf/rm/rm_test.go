package rm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
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

func TestRm_File(t *testing.T) {
	// Create a temporary directory to test in
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result := runGojqQuery(t, `rm("`+testFile+`"; "file")`, nil, RegisterRm())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
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
	meta, ok := resultMap["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("Expected _meta to be map, got %T", resultMap["_meta"])
	}
	if meta["operation"] != "rm" {
		t.Errorf("Expected operation to be 'rm', got %v", meta["operation"])
	}
	if meta["type"] != "file" {
		t.Errorf("Expected type to be 'file', got %v", meta["type"])
	}
	if meta["removed"] != true {
		t.Errorf("Expected removed to be true, got %v", meta["removed"])
	}
}

func TestRm_Folder(t *testing.T) {
	// Create a temporary directory to test in
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

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

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
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
	meta, ok := resultMap["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("Expected _meta to be map, got %T", resultMap["_meta"])
	}
	if meta["type"] != "folder" {
		t.Errorf("Expected type to be 'folder', got %v", meta["type"])
	}
}

func TestRm_NestedFolder(t *testing.T) {
	// Create a temporary directory to test in
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	nestedDir := filepath.Join(parentDir, "level1", "level2", "level3")
	err = os.MkdirAll(nestedDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create nested directory: %v", err)
	}

	// Create files in nested directories
	os.WriteFile(filepath.Join(parentDir, "level1", "file1.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(parentDir, "level1", "level2", "file2.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(nestedDir, "file3.txt"), []byte("test"), 0644)

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
	defer os.RemoveAll(parentDir)

	nonexistentFile := filepath.Join(parentDir, "nonexistent.txt")

	result := runGojqQuery(t, `rm("`+nonexistentFile+`"; "file")`, nil, RegisterRm())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	// Should have an error
	errVal, ok := resultMap["_err"]
	if !ok {
		t.Fatalf("Expected _err field in result")
	}

	errStr, ok := errVal.(string)
	if !ok {
		t.Fatalf("Expected _err to be string, got %T", errVal)
	}

	if errStr == "" {
		t.Errorf("Expected error message, got empty string")
	}
}

func TestRm_TypeMismatch(t *testing.T) {
	parentDir, err := os.MkdirTemp("", "pwrq_rm_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Try to remove file as folder
	result := runGojqQuery(t, `rm("`+testFile+`"; "folder")`, nil, RegisterRm())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	// Should have an error
	errVal, ok := resultMap["_err"]
	if !ok {
		t.Fatalf("Expected _err field in result")
	}

	errStr, ok := errVal.(string)
	if !ok {
		t.Fatalf("Expected _err to be string, got %T", errVal)
	}

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
	defer os.RemoveAll(parentDir)

	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)

	result := runGojqQuery(t, `rm("`+testFile+`"; "invalid")`, nil, RegisterRm())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	// Should have an error
	errVal, ok := resultMap["_err"]
	if !ok {
		t.Fatalf("Expected _err field in result")
	}

	errStr, ok := errVal.(string)
	if !ok {
		t.Fatalf("Expected _err to be string, got %T", errVal)
	}

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
	defer os.RemoveAll(parentDir)

	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test that rm can be chained
	result := runGojqQuery(t, `rm("`+testFile+`"; "file") | ._val | length`, nil, RegisterRm())

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
	defer os.RemoveAll(parentDir)

	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test that rm works with UDF result objects
	udfResult := common.MakeUDFSuccessResult(testFile, map[string]any{"test": "value"})

	result := runGojqQuery(t, `rm(._val; "file")`, udfResult, RegisterRm())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
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
	defer os.RemoveAll(parentDir)

	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test with uppercase type
	result := runGojqQuery(t, `rm("`+testFile+`"; "FILE")`, nil, RegisterRm())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	// Should succeed (case insensitive)
	_, ok = resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
	}

	// Verify the file was removed
	if _, err := os.Stat(testFile); err == nil {
		t.Fatalf("File was not removed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Unexpected error checking file: %v", err)
	}
}

