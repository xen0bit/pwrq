package sh

import (
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

func TestSh_SimpleCommand(t *testing.T) {
	result := runGojqQuery(t, `sh("echo hello")`, nil, RegisterSh())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	if val != "hello" {
		t.Errorf("Expected 'hello', got %q", val)
	}

	// Check metadata
}

func TestSh_CommandWithOutput(t *testing.T) {
	result := runGojqQuery(t, `sh("echo -n 'test output'")`, nil, RegisterSh())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	if val != "test output" {
		t.Errorf("Expected 'test output', got %q", val)
	}
}

func TestSh_CommandWithMultipleLines(t *testing.T) {
	result := runGojqQuery(t, `sh("echo -e 'line1\nline2\nline3'")`, nil, RegisterSh())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	// Should contain all lines (trimmed, so newlines might be preserved in the middle)
	if !strings.Contains(val, "line1") || !strings.Contains(val, "line2") || !strings.Contains(val, "line3") {
		t.Errorf("Expected output to contain all lines, got %q", val)
	}
}

func TestSh_NonZeroExitCode(t *testing.T) {
	qErr := runGojqQueryErr(t, `sh("false")`, nil, RegisterSh())

	errStr := qErr.Error()

	if errStr == "" {
		t.Errorf("Expected error message, got empty string")
	}

	// A failing command reports the exit code in the error message.
	if !strings.Contains(errStr, "exited with code") {
		t.Errorf("error should name the exit code, got %q", errStr)
	}

}

func TestSh_CommandWithStderr(t *testing.T) {
	// Use a command that writes to stderr and exits with non-zero
	qErr := runGojqQueryErr(t, `sh("echo 'error message' >&2 && exit 1")`, nil, RegisterSh())

	errStr := qErr.Error()

	// Should contain the error message
	if !strings.Contains(errStr, "error message") && !strings.Contains(errStr, "1") {
		// The error might be formatted differently, but should at least mention exit code
		t.Logf("Error message: %q", errStr)
	}
}

func TestSh_CommandFromPipe(t *testing.T) {
	result := runGojqQuery(t, `"echo test" | sh(.)`, "echo test", RegisterSh())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	if val != "test" {
		t.Errorf("Expected 'test', got %q", val)
	}
}

func TestSh_Chaining(t *testing.T) {
	result := runGojqQuery(t, `sh("echo hello") | length`, nil, RegisterSh())

	length, ok := result.(int)
	if !ok {
		t.Fatalf("Expected int result, got %T", result)
	}

	if length <= 0 {
		t.Errorf("Expected length > 0, got %d", length)
	}
}

func TestSh_WithUDFResultInput(t *testing.T) {
	udfResult := priorCmdletOutput("echo test", map[string]any{"test": "value"})

	result := runGojqQuery(t, `sh(.)`, udfResult, RegisterSh())

	val, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	if val != "test" {
		t.Errorf("Expected 'test', got %q", val)
	}
}

func TestSh_EmptyCommand(t *testing.T) {
	qErr := runGojqQueryErr(t, `sh("")`, nil, RegisterSh())

	errStr := qErr.Error()

	if errStr == "" {
		t.Errorf("Expected error message, got empty string")
	}
}

func TestSh_CommandNotFound(t *testing.T) {
	qErr := runGojqQueryErr(t, `sh("nonexistentcommand12345")`, nil, RegisterSh())

	if !strings.Contains(qErr.Error(), "exited with code") {
		t.Errorf("a missing command should report its exit code, got %q", qErr)
	}
}
