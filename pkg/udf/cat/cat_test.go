package cat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestFile(t *testing.T, content string) (string, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "cat_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	tmpFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create temp file: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return tmpFile, cleanup
}

func TestCat_BasicRead(t *testing.T) {
	content := "line1\nline2\nline3\n"
	tmpFile, cleanup := setupTestFile(t, content)
	defer cleanup()

	result := cat(tmpFile, []any{})
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	if _, hasErr := resultMap["_err"]; hasErr {
		t.Fatalf("Expected success, got error: %v", resultMap["_err"])
	}

	value, _ := resultMap["_val"].(string)
	if value != content {
		t.Errorf("Expected content %q, got %q", content, value)
	}
}

func TestCat_FileNotFound(t *testing.T) {
	result := cat("/nonexistent/path/file.txt", []any{})
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	errMsg, hasErr := resultMap["_err"].(string)
	if !hasErr {
		t.Fatalf("Expected error for nonexistent file, got success")
	}

	if !strings.Contains(errMsg, "does not exist") {
		t.Errorf("Expected 'does not exist' error, got: %v", errMsg)
	}
}

func TestCat_DirectoryError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cat_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	result := cat(tmpDir, []any{})
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	errMsg, hasErr := resultMap["_err"].(string)
	if !hasErr {
		t.Fatalf("Expected error for directory, got success")
	}

	if !strings.Contains(errMsg, "is a directory") {
		t.Errorf("Expected 'is a directory' error, got: %v", errMsg)
	}
}

func TestCat_WithTail(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\n"
	tmpFile, cleanup := setupTestFile(t, content)
	defer cleanup()

	// cat function: inputPath from pipe (simulated), args[0] is options
	result := cat(tmpFile, []any{map[string]any{"tail": float64(2)}})
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	if _, hasErr := resultMap["_err"]; hasErr {
		t.Fatalf("Expected success, got error: %v", resultMap["_err"])
	}

	value, _ := resultMap["_val"].(string)
	expected := "line4\nline5\n"
	if value != expected {
		t.Errorf("Expected tail output %q, got %q", expected, value)
	}
}

func TestCat_WithTotalCount(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\n"
	tmpFile, cleanup := setupTestFile(t, content)
	defer cleanup()

	result := cat(tmpFile, []any{map[string]any{"totalcount": float64(3)}})
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	if _, hasErr := resultMap["_err"]; hasErr {
		t.Fatalf("Expected success, got error: %v", resultMap["_err"])
	}

	value, _ := resultMap["_val"].(string)
	expected := "line1\nline2\nline3\n"
	if value != expected {
		t.Errorf("Expected totalcount output %q, got %q", expected, value)
	}
}

func TestCat_WithTailAndTotalCount(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\n"
	tmpFile, cleanup := setupTestFile(t, content)
	defer cleanup()

	result := cat(tmpFile, []any{map[string]any{"totalcount": float64(4), "tail": float64(2)}})
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	if _, hasErr := resultMap["_err"]; hasErr {
		t.Fatalf("Expected success, got error: %v", resultMap["_err"])
	}

	value, _ := resultMap["_val"].(string)
	expected := "line3\nline4\n"
	if value != expected {
		t.Errorf("Expected combined output %q, got %q", expected, value)
	}
}

func TestCat_WithEncoding(t *testing.T) {
	content := "Hello, 世界\n"
	tmpFile, cleanup := setupTestFile(t, content)
	defer cleanup()

	result := cat(tmpFile, []any{map[string]any{"encoding": "utf8"}})
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	if _, hasErr := resultMap["_err"]; hasErr {
		t.Fatalf("Expected success, got error: %v", resultMap["_err"])
	}

	value, _ := resultMap["_val"].(string)
	if value != content {
		t.Errorf("Expected content %q, got %q", content, value)
	}

	meta, _ := resultMap["_meta"].(map[string]any)
	if meta["encoding"] != "utf8" {
		t.Errorf("Expected encoding 'utf8' in metadata, got %v", meta["encoding"])
	}
}

func TestCat_InvalidEncoding(t *testing.T) {
	content := "test\n"
	tmpFile, cleanup := setupTestFile(t, content)
	defer cleanup()

	// Invalid encoding should be caught
	result := cat(tmpFile, []any{map[string]any{"encoding": "invalid"}})
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	errMsg, hasErr := resultMap["_err"].(string)
	if !hasErr {
		t.Fatalf("Expected error for invalid encoding, got success")
	}

	if !strings.Contains(errMsg, "unsupported encoding") {
		t.Errorf("Expected 'unsupported encoding' error, got: %v", errMsg)
	}
}

func TestCat_WithTailAsString(t *testing.T) {
	content := "line1\nline2\nline3\n"
	tmpFile, cleanup := setupTestFile(t, content)
	defer cleanup()

	result := cat(tmpFile, []any{map[string]any{"tail": "1"}})
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	if _, hasErr := resultMap["_err"]; hasErr {
		t.Fatalf("Expected success, got error: %v", resultMap["_err"])
	}

	value, _ := resultMap["_val"].(string)
	expected := "line3\n"
	if value != expected {
		t.Errorf("Expected tail output %q, got %q", expected, value)
	}
}

func TestCat_EmptyFile(t *testing.T) {
	content := ""
	tmpFile, cleanup := setupTestFile(t, content)
	defer cleanup()

	result := cat(tmpFile, []any{})
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	if _, hasErr := resultMap["_err"]; hasErr {
		t.Fatalf("Expected success for empty file, got error: %v", resultMap["_err"])
	}

	value, _ := resultMap["_val"].(string)
	if value != "" {
		t.Errorf("Expected empty string, got %q", value)
	}
}

func TestCat_Metadata(t *testing.T) {
	content := "test content\n"
	tmpFile, cleanup := setupTestFile(t, content)
	defer cleanup()

	result := cat(tmpFile, []any{})
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	if _, hasErr := resultMap["_err"]; hasErr {
		t.Fatalf("Expected success, got error")
	}

	meta, _ := resultMap["_meta"].(map[string]any)

	if meta["operation"] != "cat" {
		t.Errorf("Expected operation 'cat', got %v", meta["operation"])
	}

	if meta["file_path"] != tmpFile {
		t.Errorf("Expected file_path %q, got %v", tmpFile, meta["file_path"])
	}

	if meta["file_size"] != len(content) {
		t.Errorf("Expected file_size %d, got %v", len(content), meta["file_size"])
	}
}

func TestParseCatArgs_Basic(t *testing.T) {
	args := []any{"test.txt"}
	path, opts, err := parseCatArgs(args)
	if err != nil {
		t.Fatalf("parseCatArgs failed: %v", err)
	}

	if path != "test.txt" {
		t.Errorf("expected path 'test.txt', got '%s'", path)
	}
	if opts.TailLines != -1 {
		t.Errorf("expected default TailLines -1, got %d", opts.TailLines)
	}
	if opts.TotalCount != -1 {
		t.Errorf("expected default TotalCount -1, got %d", opts.TotalCount)
	}
	if opts.Encoding != "utf8" {
		t.Errorf("expected default Encoding 'utf8', got '%s'", opts.Encoding)
	}
}

func TestParseCatArgs_WithOptions(t *testing.T) {
	args := []any{
		"test.txt",
		map[string]any{
			"tail":       float64(5),
			"totalcount": float64(10),
			"encoding":   "utf16le",
		},
	}
	path, opts, err := parseCatArgs(args)
	if err != nil {
		t.Fatalf("parseCatArgs failed: %v", err)
	}

	if path != "test.txt" {
		t.Errorf("expected path 'test.txt', got '%s'", path)
	}
	if opts.TailLines != 5 {
		t.Errorf("expected TailLines 5, got %d", opts.TailLines)
	}
	if opts.TotalCount != 10 {
		t.Errorf("expected TotalCount 10, got %d", opts.TotalCount)
	}
	if opts.Encoding != "utf16le" {
		t.Errorf("expected Encoding 'utf16le', got '%s'", opts.Encoding)
	}
}

func TestParseCatArgs_MissingPath(t *testing.T) {
	_, _, err := parseCatArgs([]any{})
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}
