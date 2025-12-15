package mkdir

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

func TestMkdir_Basic(t *testing.T) {
	// Create a temporary directory to test in
	parentDir, err := os.MkdirTemp("", "pwrq_mkdir_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	testDir := filepath.Join(parentDir, "testdir")

	result := runGojqQuery(t, `mkdir("`+testDir+`")`, nil, RegisterMkdir())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
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
	meta, ok := resultMap["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("Expected _meta to be map, got %T", resultMap["_meta"])
	}
	if meta["operation"] != "mkdir" {
		t.Errorf("Expected operation to be 'mkdir', got %v", meta["operation"])
	}
	if meta["created"] != true {
		t.Errorf("Expected created to be true, got %v", meta["created"])
	}
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

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
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

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	// Should succeed (like mkdir -p)
	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
	}

	if val != testDir {
		t.Errorf("Expected path %q, got %q", testDir, val)
	}

	// Check metadata indicates it already existed
	meta, ok := resultMap["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("Expected _meta to be map, got %T", resultMap["_meta"])
	}
	if meta["existed"] != true {
		t.Errorf("Expected existed to be true, got %v", meta["existed"])
	}
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
	// Try to create a directory in a non-existent parent (should fail without MkdirAll, but we use MkdirAll)
	// Actually, with MkdirAll this should succeed, so let's test with a file path instead
	parentDir, err := os.MkdirTemp("", "pwrq_mkdir_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	// Create a file
	testFile := filepath.Join(parentDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Try to create a directory with the same name as the file
	invalidDir := filepath.Join(parentDir, "testfile.txt", "subdir")

	result := runGojqQuery(t, `mkdir("`+invalidDir+`")`, nil, RegisterMkdir())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	// Should have an error (can't create subdir of a file)
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

func TestMkdir_Chaining(t *testing.T) {
	parentDir, err := os.MkdirTemp("", "pwrq_mkdir_test_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	testDir := filepath.Join(parentDir, "chaintest")

	// Test that mkdir can be chained
	result := runGojqQuery(t, `mkdir("`+testDir+`") | ._val | length`, nil, RegisterMkdir())

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

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
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
	udfResult := common.MakeUDFSuccessResult(testDir, map[string]any{"test": "value"})

	result := runGojqQuery(t, `mkdir(._val)`, udfResult, RegisterMkdir())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
	}

	// Verify the directory exists
	if _, err := os.Stat(val); err != nil {
		t.Fatalf("Created directory does not exist: %v", err)
	}
}

