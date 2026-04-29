package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSetContent_UTF8(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test_utf8.txt")

	opts := SetContentOptions{
		Path:     testPath,
		Value:    "Hello, 世界",
		Encoding: "utf8",
		Force:    false,
	}

	_, err := setContent(opts)
	if err != nil {
		t.Fatalf("setContent failed: %v", err)
	}

	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(content) != "Hello, 世界" {
		t.Errorf("expected 'Hello, 世界', got '%s'", string(content))
	}
}

func TestSetContent_UTF16LE(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test_utf16le.txt")

	opts := SetContentOptions{
		Path:     testPath,
		Value:    "Hello",
		Encoding: "utf16le",
		Force:    false,
	}

	_, err := setContent(opts)
	if err != nil {
		t.Fatalf("setContent failed: %v", err)
	}

	// UTF-16LE should be 2 bytes per character for ASCII
	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// "Hello" in UTF-16LE should be 10 bytes (5 chars * 2 bytes each)
	if len(content) != 10 {
		t.Errorf("expected 10 bytes for UTF-16LE 'Hello', got %d", len(content))
	}
}

func TestSetContent_Latin1(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test_latin1.txt")

	opts := SetContentOptions{
		Path:     testPath,
		Value:    "Café",
		Encoding: "latin1",
		Force:    false,
	}

	_, err := setContent(opts)
	if err != nil {
		t.Fatalf("setContent failed: %v", err)
	}

	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// 'é' in Latin-1 is 0xE9
	expected := []byte("Caf\xe9")
	if string(content) != string(expected) {
		t.Errorf("expected '%x', got '%x'", expected, content)
	}
}

func TestSetContent_Force(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "subdir", "nested", "test.txt")

	opts := SetContentOptions{
		Path:     testPath,
		Value:    "content",
		Encoding: "utf8",
		Force:    true,
	}

	writtenPath, err := setContent(opts)
	if err != nil {
		t.Fatalf("setContent with Force failed: %v", err)
	}

	if writtenPath != testPath {
		t.Errorf("expected path %s, got %s", testPath, writtenPath)
	}

	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(content) != "content" {
		t.Errorf("expected 'content', got '%s'", string(content))
	}
}

func TestSetContent_ArrayValue(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test_array.txt")

	opts := SetContentOptions{
		Path:     testPath,
		Value:    []any{"line1", "line2", "line3"},
		Encoding: "utf8",
		Force:    false,
	}

	_, err := setContent(opts)
	if err != nil {
		t.Fatalf("setContent failed: %v", err)
	}

	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expected := "line1\nline2\nline3"
	if string(content) != expected {
		t.Errorf("expected '%s', got '%s'", expected, string(content))
	}
}

func TestSetContent_InvalidEncoding(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.txt")

	opts := SetContentOptions{
		Path:     testPath,
		Value:    "content",
		Encoding: "invalid-encoding",
		Force:    false,
	}

	_, err := setContent(opts)
	if err == nil {
		t.Fatal("expected error for invalid encoding, got nil")
	}

	if !strings.Contains(err.Error(), "unsupported encoding") {
		t.Errorf("expected 'unsupported encoding' error, got: %v", err)
	}
}

func TestGetEncoding_Valid(t *testing.T) {
	tests := []struct {
		name     string
		encoding string
		wantErr  bool
	}{
		{"UTF-8", "utf8", false},
		{"UTF-8 variant", "utf-8", false},
		{"UTF-16LE", "utf16le", false},
		{"UTF-16BE", "utf16be", false},
		{"UTF-16", "utf16", false},
		{"ASCII", "ascii", false},
		{"Latin-1", "latin1", false},
		{"ISO-8859-1", "iso-8859-1", false},
		{"CP437", "cp437", false},
		{"CP850", "cp850", false},
		{"Windows-1252", "windows-1252", false},
		{"CP1252", "cp1252", false},
		{"Invalid", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getEncoding(tt.encoding)
			if (err != nil) != tt.wantErr {
				t.Errorf("getEncoding(%q) error = %v, wantErr %v", tt.encoding, err, tt.wantErr)
			}
		})
	}
}

func TestParseSetContentArgs_Basic(t *testing.T) {
	args := []any{"test.txt", "content"}
	opts, err := parseSetContentArgs(args)
	if err != nil {
		t.Fatalf("parseSetContentArgs failed: %v", err)
	}

	if opts.Path != "test.txt" {
		t.Errorf("expected path 'test.txt', got '%s'", opts.Path)
	}
	if opts.Value != "content" {
		t.Errorf("expected value 'content', got '%v'", opts.Value)
	}
	if opts.Encoding != "utf8" {
		t.Errorf("expected default encoding 'utf8', got '%s'", opts.Encoding)
	}
}

func TestParseSetContentArgs_WithOptions(t *testing.T) {
	args := []any{
		map[string]any{
			"Path":     "test.txt",
			"Value":    "content",
			"Encoding": "utf16le",
			"Force":    true,
		},
	}
	opts, err := parseSetContentArgs(args)
	if err != nil {
		t.Fatalf("parseSetContentArgs failed: %v", err)
	}

	if opts.Path != "test.txt" {
		t.Errorf("expected path 'test.txt', got '%s'", opts.Path)
	}
	if opts.Value != "content" {
		t.Errorf("expected value 'content', got '%v'", opts.Value)
	}
	if opts.Encoding != "utf16le" {
		t.Errorf("expected encoding 'utf16le', got '%s'", opts.Encoding)
	}
	if !opts.Force {
		t.Error("expected Force to be true")
	}
}

func TestParseSetContentArgs_MissingPath(t *testing.T) {
	args := []any{"", "content"}
	_, err := parseSetContentArgs(args)
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}

func TestParseSetContentArgs_MissingValue(t *testing.T) {
	args := []any{"test.txt"}
	_, err := parseSetContentArgs(args)
	if err == nil {
		t.Fatal("expected error for missing value, got nil")
	}
}

func TestExtractPSObjectValue(t *testing.T) {
	// Test with non-PSObject value
	result := extractPSObjectValue("plain string")
	if result != "plain string" {
		t.Errorf("expected 'plain string', got '%v'", result)
	}

	// Test with PSObject map
	psObjMap := map[string]any{
		"__psobject": true,
		"Value":      "wrapped value",
		"TypeName":   "System.String",
	}
	result = extractPSObjectValue(psObjMap)
	if result != "wrapped value" {
		t.Errorf("expected 'wrapped value', got '%v'", result)
	}

	// Test with nested PSObject
	nestedPsObj := map[string]any{
		"__psobject": true,
		"Value": map[string]any{
			"__psobject": true,
			"Value":      "deeply wrapped",
		},
	}
	result = extractPSObjectValue(nestedPsObj)
	if result != "deeply wrapped" {
		t.Errorf("expected 'deeply wrapped', got '%v'", result)
	}
}

func TestGetNewLine(t *testing.T) {
	newline := getNewLine()
	if runtime.GOOS == "windows" {
		if newline != "\r\n" {
			t.Errorf("expected '\\r\\n' on Windows, got '%s'", newline)
		}
	} else {
		if newline != "\n" {
			t.Errorf("expected '\\n' on Unix, got '%s'", newline)
		}
	}
}

func TestSetContent_ArrayWithPSObject(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test_psobj_array.txt")

	// Array containing PSObject maps
	psObjItem := map[string]any{
		"__psobject": true,
		"Value":      "extracted line",
	}
	opts := SetContentOptions{
		Path:     testPath,
		Value:    []any{"line1", psObjItem, "line3"},
		Encoding: "utf8",
		Force:    false,
	}

	_, err := setContent(opts)
	if err != nil {
		t.Fatalf("setContent failed: %v", err)
	}

	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// Should use platform-appropriate newlines
	expected := "line1" + getNewLine() + "extracted line" + getNewLine() + "line3"
	if string(content) != expected {
		t.Errorf("expected '%s', got '%s'", expected, string(content))
	}
}

func TestSetContent_StringValueWithPSObject(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test_psobj_string.txt")

	// PSObject wrapping a string
	psObjMap := map[string]any{
		"__psobject": true,
		"Value":      "unwrapped content",
		"TypeName":   "System.String",
	}
	opts := SetContentOptions{
		Path:     testPath,
		Value:    psObjMap,
		Encoding: "utf8",
		Force:    false,
	}

	_, err := setContent(opts)
	if err != nil {
		t.Fatalf("setContent failed: %v", err)
	}

	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(content) != "unwrapped content" {
		t.Errorf("expected 'unwrapped content', got '%s'", string(content))
	}
}

func TestValidatePath(t *testing.T) {
	// Valid path
	err := validatePath("/tmp/test.txt")
	if err != nil {
		t.Errorf("expected no error for valid path, got %v", err)
	}

	// Empty path
	err = validatePath("")
	if err == nil {
		t.Error("expected error for empty path, got nil")
	}

	// On Windows, test reserved names
	if runtime.GOOS == "windows" {
		err = validatePath("C:\\CON")
		if err == nil {
			t.Error("expected error for reserved name CON, got nil")
		}
		err = validatePath("C:\\NUL.txt")
		if err == nil {
			t.Error("expected error for reserved name NUL, got nil")
		}
		err = validatePath("C:\\COM1")
		if err == nil {
			t.Error("expected error for reserved name COM1, got nil")
		}
	}
}
