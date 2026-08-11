package tempdir

import (
	"os"
	"path/filepath"
	"strings"
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

func TestTempDir_NoArgs(t *testing.T) {
	result := runGojqQuery(t, "tempdir", nil, RegisterTempDir())

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

	// Cleanup
	os.RemoveAll(val)

	// Check metadata
}

func TestTempDir_WithPrefix(t *testing.T) {
	result := runGojqQuery(t, `tempdir("pwrq_test_")`, nil, RegisterTempDir())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
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
}

func TestTempDir_WithDir(t *testing.T) {
	// Create a temporary directory to use as the parent
	parentDir, err := os.MkdirTemp("", "pwrq_test_parent_")
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	defer os.RemoveAll(parentDir)

	result := runGojqQuery(t, `tempdir(""; "`+parentDir+`")`, nil, RegisterTempDir())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	// Verify the directory exists and is in the parent directory
	if _, err := os.Stat(val); err != nil {
		t.Fatalf("Created directory does not exist: %v", err)
	}

	// Verify it's in the parent directory
	parentAbs, _ := filepath.Abs(parentDir)
	valAbs, _ := filepath.Abs(val)
	if rel, err := filepath.Rel(parentAbs, valAbs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
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

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
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
	if rel, err := filepath.Rel(parentAbs, valAbs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Errorf("Created directory %q is not in parent directory %q", valAbs, parentAbs)
	}

	// Cleanup
	os.RemoveAll(val)
}

func TestTempDir_InvalidDir(t *testing.T) {
	qErr := runGojqQueryErr(t, `tempdir(""; "/nonexistent/directory/path")`, nil, RegisterTempDir())

	errStr := qErr.Error()

	if errStr == "" {
		t.Errorf("Expected error message, got empty string")
	}
}

func TestTempDir_Chaining(t *testing.T) {
	// Test that tempdir can be chained
	result := runGojqQuery(t, `tempdir | length`, nil, RegisterTempDir())

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

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
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
	udfResult := priorCmdletOutput("pwrq_test_", map[string]any{"test": "value"})

	result := runGojqQuery(t, `tempdir(.)`, udfResult, RegisterTempDir())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
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
