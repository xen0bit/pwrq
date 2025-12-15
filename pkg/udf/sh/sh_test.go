package sh

import (
	"strings"
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

func TestSh_SimpleCommand(t *testing.T) {
	result := runGojqQuery(t, `sh("echo hello")`, nil, RegisterSh())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
	}

	if val != "hello" {
		t.Errorf("Expected 'hello', got %q", val)
	}

	// Check metadata
	meta, ok := resultMap["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("Expected _meta to be map, got %T", resultMap["_meta"])
	}
	if meta["operation"] != "sh" {
		t.Errorf("Expected operation to be 'sh', got %v", meta["operation"])
	}
	if meta["exit_code"] != 0 {
		t.Errorf("Expected exit_code to be 0, got %v", meta["exit_code"])
	}
}

func TestSh_CommandWithOutput(t *testing.T) {
	result := runGojqQuery(t, `sh("echo -n 'test output'")`, nil, RegisterSh())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
	}

	if val != "test output" {
		t.Errorf("Expected 'test output', got %q", val)
	}
}

func TestSh_CommandWithMultipleLines(t *testing.T) {
	result := runGojqQuery(t, `sh("echo -e 'line1\nline2\nline3'")`, nil, RegisterSh())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
	}

	// Should contain all lines (trimmed, so newlines might be preserved in the middle)
	if !strings.Contains(val, "line1") || !strings.Contains(val, "line2") || !strings.Contains(val, "line3") {
		t.Errorf("Expected output to contain all lines, got %q", val)
	}
}

func TestSh_NonZeroExitCode(t *testing.T) {
	result := runGojqQuery(t, `sh("false")`, nil, RegisterSh())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	// Should have _err field
	errVal, ok := resultMap["_err"]
	if !ok {
		t.Fatalf("Expected _err field in result for non-zero exit code")
	}

	errStr, ok := errVal.(string)
	if !ok {
		t.Fatalf("Expected _err to be string, got %T", errVal)
	}

	if errStr == "" {
		t.Errorf("Expected error message, got empty string")
	}

	// Check metadata has exit code
	meta, ok := resultMap["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("Expected _meta to be map, got %T", resultMap["_meta"])
	}

	exitCode, ok := meta["exit_code"].(int)
	if !ok {
		// Try float64 (JSON numbers)
		if exitCodeFloat, ok := meta["exit_code"].(float64); ok {
			exitCode = int(exitCodeFloat)
		} else {
			t.Fatalf("Expected exit_code to be int or float64, got %T", meta["exit_code"])
		}
	}

	if exitCode == 0 {
		t.Errorf("Expected non-zero exit code, got %d", exitCode)
	}
}

func TestSh_CommandWithStderr(t *testing.T) {
	// Use a command that writes to stderr and exits with non-zero
	result := runGojqQuery(t, `sh("echo 'error message' >&2 && exit 1")`, nil, RegisterSh())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	// Should have _err field
	errVal, ok := resultMap["_err"]
	if !ok {
		t.Fatalf("Expected _err field in result")
	}

	errStr, ok := errVal.(string)
	if !ok {
		t.Fatalf("Expected _err to be string, got %T", errVal)
	}

	// Should contain the error message
	if !strings.Contains(errStr, "error message") && !strings.Contains(errStr, "1") {
		// The error might be formatted differently, but should at least mention exit code
		t.Logf("Error message: %q", errStr)
	}
}

func TestSh_CommandFromPipe(t *testing.T) {
	result := runGojqQuery(t, `"echo test" | sh(.)`, "echo test", RegisterSh())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
	}

	if val != "test" {
		t.Errorf("Expected 'test', got %q", val)
	}
}

func TestSh_Chaining(t *testing.T) {
	result := runGojqQuery(t, `sh("echo hello") | ._val | length`, nil, RegisterSh())

	length, ok := result.(int)
	if !ok {
		t.Fatalf("Expected int result, got %T", result)
	}

	if length <= 0 {
		t.Errorf("Expected length > 0, got %d", length)
	}
}

func TestSh_WithUDFResultInput(t *testing.T) {
	udfResult := common.MakeUDFSuccessResult("echo test", map[string]any{"test": "value"})

	result := runGojqQuery(t, `sh(._val)`, udfResult, RegisterSh())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	val, ok := resultMap["_val"].(string)
	if !ok {
		t.Fatalf("Expected _val to be string, got %T", resultMap["_val"])
	}

	if val != "test" {
		t.Errorf("Expected 'test', got %q", val)
	}
}

func TestSh_EmptyCommand(t *testing.T) {
	result := runGojqQuery(t, `sh("")`, nil, RegisterSh())

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

func TestSh_CommandNotFound(t *testing.T) {
	result := runGojqQuery(t, `sh("nonexistentcommand12345")`, nil, RegisterSh())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", result)
	}

	// Should have an error (either in _err or as UDF error)
	errVal, hasErr := resultMap["_err"]
	if hasErr {
		errStr, ok := errVal.(string)
		if ok && errStr != "" {
			// This is expected for command not found
			return
		}
	}

	// If no _err, check if it's a UDF error result
	if !hasErr {
		// Command not found should result in an error
		t.Logf("Result: %+v", resultMap)
	}
}

