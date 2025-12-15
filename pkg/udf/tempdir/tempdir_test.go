package tempdir

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

func TestTempDir_NoArgs(t *testing.T) {
	result := runGojqQuery(t, "tempdir", nil, RegisterTempDir())

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

	// Cleanup
	os.RemoveAll(val)

	// Check metadata
	meta, ok := resultMap["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("Expected _meta to be map, got %T", resultMap["_meta"])
	}
	if meta["operation"] != "tempdir" {
		t.Errorf("Expected operation to be 'tempdir', got %v", meta["operation"])
	}
}

func TestTempDir_WithPrefix(t *testing.T) {
	result := runGojqQuery(t, `tempdir("pwrq_test_")`, nil, RegisterTempDir())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
	}

	// Verify the directory exists and has the prefix
	if _, err := os.Stat(val); err != nil {
		t.Fatalf("Created directory does not exist: %v", err)
	}

	dirName := filepath.Base(val)
	if len(dirName) < len("pwrq_test_") || dirName[:len("pwrq_test_")] != "pwrq_test_" {
		t.Errorf("Directory name should start with 'pwrq_test_', got %q", dirName)
	}

	// Cleanup
	os.RemoveAll(val)

	// Check metadata
	meta, ok := resultMap["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("Expected _meta to be map, got %T", resultMap["_meta"])
	}
	if meta["prefix"] != "pwrq_test_" {
		t.Errorf("Expected prefix in metadata to be 'pwrq_test_', got %v", meta["prefix"])
	}
}

func TestTempDir_WithDir(t *testing.T) {
	// Create a temporary directory to use as the parent
	parentDir, err := os.MkdirTemp("", "pwrq_test_parent_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	result := runGojqQuery(t, `tempdir(""; "`+parentDir+`")`, nil, RegisterTempDir())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
	}

	// Verify the directory exists and is in the parent directory
	if _, err := os.Stat(val); err != nil {
		t.Fatalf("Created directory does not exist: %v", err)
	}

	// Verify it's in the parent directory
	parentAbs, _ := filepath.Abs(parentDir)
	valAbs, _ := filepath.Abs(val)
	if !filepath.HasPrefix(valAbs, parentAbs) {
		t.Errorf("Created directory %q is not in parent directory %q", valAbs, parentAbs)
	}

	// Cleanup
	os.RemoveAll(val)
}

func TestTempDir_WithPrefixAndDir(t *testing.T) {
	// Create a temporary directory to use as the parent
	parentDir, err := os.MkdirTemp("", "pwrq_test_parent_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	result := runGojqQuery(t, `tempdir("pwrq_test_"; "`+parentDir+`")`, nil, RegisterTempDir())

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

	// Verify it has the prefix
	dirName := filepath.Base(val)
	if len(dirName) < len("pwrq_test_") || dirName[:len("pwrq_test_")] != "pwrq_test_" {
		t.Errorf("Directory name should start with 'pwrq_test_', got %q", dirName)
	}

	// Verify it's in the parent directory
	parentAbs, _ := filepath.Abs(parentDir)
	valAbs, _ := filepath.Abs(val)
	if !filepath.HasPrefix(valAbs, parentAbs) {
		t.Errorf("Created directory %q is not in parent directory %q", valAbs, parentAbs)
	}

	// Cleanup
	os.RemoveAll(val)
}

func TestTempDir_InvalidDir(t *testing.T) {
	result := runGojqQuery(t, `tempdir(""; "/nonexistent/directory/path")`, nil, RegisterTempDir())

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

func TestTempDir_Chaining(t *testing.T) {
	// Test that tempdir can be chained
	result := runGojqQuery(t, `tempdir | ._val | length`, nil, RegisterTempDir())

	// Should return the length of the path string
	length, ok := result.(int)
	if !ok {
		t.Fatalf("Expected int result, got %T", result)
	}

	if length <= 0 {
		t.Errorf("Expected path length > 0, got %d", length)
	}
}

func TestTempDir_FromPipe(t *testing.T) {
	// Test that we can use tempdir with input from pipe (though it doesn't use it)
	result := runGojqQuery(t, `"test" | tempdir`, "test", RegisterTempDir())

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

	// Cleanup
	os.RemoveAll(val)
}

func TestTempDir_WithUDFResultInput(t *testing.T) {
	// Test that tempdir works with UDF result objects
	udfResult := common.MakeUDFSuccessResult("pwrq_test_", map[string]any{"test": "value"})

	result := runGojqQuery(t, `tempdir(._val)`, udfResult, RegisterTempDir())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
	}

	// Verify the directory exists and has the prefix
	if _, err := os.Stat(val); err != nil {
		t.Fatalf("Created directory does not exist: %v", err)
	}

	dirName := filepath.Base(val)
	if len(dirName) < len("pwrq_test_") || dirName[:len("pwrq_test_")] != "pwrq_test_" {
		t.Errorf("Directory name should start with 'pwrq_test_', got %q", dirName)
	}

	// Cleanup
	os.RemoveAll(val)
}

