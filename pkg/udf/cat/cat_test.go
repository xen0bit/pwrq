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
	if err, isErr := result.(error); isErr {
		t.Fatalf("Expected success, got error: %v", err)
	}

	value, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
	if value != content {
		t.Errorf("Expected content %q, got %q", content, value)
	}
}

func TestCat_FileNotFound(t *testing.T) {
	result := cat("/nonexistent/path/file.txt", []any{})
	err, isErr := result.(error)
	if !isErr {
		t.Fatalf("Expected an error, got %T", result)
	}
	errMsg := err.Error()

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
	err, isErr := result.(error)
	if !isErr {
		t.Fatalf("Expected an error, got %T", result)
	}
	errMsg := err.Error()

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
	if err, isErr := result.(error); isErr {
		t.Fatalf("Expected success, got error: %v", err)
	}

	value, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
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
	if err, isErr := result.(error); isErr {
		t.Fatalf("Expected success, got error: %v", err)
	}

	value, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
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
	if err, isErr := result.(error); isErr {
		t.Fatalf("Expected success, got error: %v", err)
	}

	value, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
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
	if err, isErr := result.(error); isErr {
		t.Fatalf("Expected success, got error: %v", err)
	}

	value, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
	if value != content {
		t.Errorf("Expected content %q, got %q", content, value)
	}
}

func TestCat_InvalidEncoding(t *testing.T) {
	content := "test\n"
	tmpFile, cleanup := setupTestFile(t, content)
	defer cleanup()

	// Invalid encoding should be caught
	result := cat(tmpFile, []any{map[string]any{"encoding": "invalid"}})
	err, isErr := result.(error)
	if !isErr {
		t.Fatalf("Expected an error, got %T", result)
	}
	errMsg := err.Error()

	if !strings.Contains(errMsg, "unsupported encoding") {
		t.Errorf("Expected 'unsupported encoding' error, got: %v", errMsg)
	}
}

func TestCat_WithTailAsString(t *testing.T) {
	content := "line1\nline2\nline3\n"
	tmpFile, cleanup := setupTestFile(t, content)
	defer cleanup()

	result := cat(tmpFile, []any{map[string]any{"tail": "1"}})
	if err, isErr := result.(error); isErr {
		t.Fatalf("Expected success, got error: %v", err)
	}

	value, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
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
	if err, isErr := result.(error); isErr {
		t.Fatalf("Expected success, got error: %v", err)
	}

	value, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
	if value != "" {
		t.Errorf("Expected empty string, got %q", value)
	}
}

func TestCat_ReturnsFileContents(t *testing.T) {
	content := "test content\n"
	tmpFile, cleanup := setupTestFile(t, content)
	defer cleanup()

	result := cat(tmpFile, []any{})
	if err, isErr := result.(error); isErr {
		t.Fatalf("Expected success, got error: %v", err)
	}

	// cat is a transform: it returns the contents, not a description of them.
	if result != content {
		t.Errorf("cat = %#v, want %q", result, content)
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
