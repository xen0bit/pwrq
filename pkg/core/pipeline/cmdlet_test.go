package pipeline

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
)

// TestWriteObject tests the WriteObject method with enumeration.
func TestWriteObject(t *testing.T) {
	t.Run("writes single object", func(t *testing.T) {
		var outputs []any
		base := &CmdletBase{
			OutputWriter: func(obj any) {
				outputs = append(outputs, obj)
			},
		}

		base.WriteObject("hello", false)

		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}
		if outputs[0] != "hello" {
			t.Errorf("expected 'hello', got %v", outputs[0])
		}
	})

	t.Run("enumerates slice when true", func(t *testing.T) {
		var outputs []any
		base := &CmdletBase{
			OutputWriter: func(obj any) {
				outputs = append(outputs, obj)
			},
		}

		base.WriteObject([]string{"a", "b", "c"}, true)

		if len(outputs) != 3 {
			t.Fatalf("expected 3 outputs, got %d", len(outputs))
		}
		if outputs[0] != "a" || outputs[1] != "b" || outputs[2] != "c" {
			t.Errorf("expected [a, b, c], got %v", outputs)
		}
	})

	t.Run("does not enumerate when false", func(t *testing.T) {
		var outputs []any
		base := &CmdletBase{
			OutputWriter: func(obj any) {
				outputs = append(outputs, obj)
			},
		}

		base.WriteObject([]string{"a", "b", "c"}, false)

		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}
		if _, ok := outputs[0].([]string); !ok {
			t.Errorf("expected slice, got %T", outputs[0])
		}
	})

	t.Run("handles nil output writer", func(t *testing.T) {
		base := &CmdletBase{
			OutputWriter: nil,
		}

		// Should not panic
		base.WriteObject("test", false)
	})
}

// TestWriteVerbose tests verbose output based on preferences.
func TestWriteVerbose(t *testing.T) {
	t.Run("writes when Continue", func(t *testing.T) {
		var buf bytes.Buffer
		ss := sessionstate.NewSessionState()
		ss.SetVariable("VerbosePreference", "Continue", 0)
		ss.Stderr = &buf

		base := &CmdletBase{
			SessionState: ss,
		}

		base.WriteVerbose("test message")

		output := buf.String()
		if !strings.Contains(output, "VERBOSE: test message") {
			t.Errorf("expected verbose output, got: %s", output)
		}
	})

	t.Run("does not write when SilentlyContinue", func(t *testing.T) {
		var buf bytes.Buffer
		ss := sessionstate.NewSessionState()
		ss.SetVariable("VerbosePreference", "SilentlyContinue", 0)
		ss.Stderr = &buf

		base := &CmdletBase{
			SessionState: ss,
		}

		base.WriteVerbose("test message")

		if buf.Len() > 0 {
			t.Errorf("expected no output, got: %s", buf.String())
		}
	})

	t.Run("handles nil session state", func(t *testing.T) {
		base := &CmdletBase{
			SessionState: nil,
		}

		// Should not panic
		base.WriteVerbose("test")
	})
}

// TestWriteDebug tests debug output based on preferences.
func TestWriteDebug(t *testing.T) {
	t.Run("writes when Continue", func(t *testing.T) {
		var buf bytes.Buffer
		ss := sessionstate.NewSessionState()
		ss.SetVariable("DebugPreference", "Continue", 0)
		ss.Stderr = &buf

		base := &CmdletBase{
			SessionState: ss,
		}

		base.WriteDebug("debug message")

		output := buf.String()
		if !strings.Contains(output, "DEBUG: debug message") {
			t.Errorf("expected debug output, got: %s", output)
		}
	})

	t.Run("does not write when SilentlyContinue", func(t *testing.T) {
		var buf bytes.Buffer
		ss := sessionstate.NewSessionState()
		ss.SetVariable("DebugPreference", "SilentlyContinue", 0)
		ss.Stderr = &buf

		base := &CmdletBase{
			SessionState: ss,
		}

		base.WriteDebug("debug message")

		if buf.Len() > 0 {
			t.Errorf("expected no output, got: %s", buf.String())
		}
	})
}

// TestWriteWarning tests warning output based on preferences.
func TestWriteWarning(t *testing.T) {
	t.Run("writes when Continue", func(t *testing.T) {
		var buf bytes.Buffer
		ss := sessionstate.NewSessionState()
		ss.SetVariable("WarningPreference", "Continue", 0)
		ss.Stderr = &buf

		base := &CmdletBase{
			SessionState: ss,
		}

		base.WriteWarning("warning message")

		output := buf.String()
		if !strings.Contains(output, "WARNING: warning message") {
			t.Errorf("expected warning output, got: %s", output)
		}
	})

	t.Run("writes by default", func(t *testing.T) {
		// Default behavior should write warnings
		var buf bytes.Buffer
		ss := sessionstate.NewSessionState()
		ss.Stderr = &buf

		base := &CmdletBase{
			SessionState: ss,
		}

		base.WriteWarning("warning message")

		output := buf.String()
		if !strings.Contains(output, "WARNING: warning message") {
			t.Errorf("expected warning output by default, got: %s", output)
		}
	})
}

// TestWriteError tests error output.
func TestWriteError(t *testing.T) {
	t.Run("writes error", func(t *testing.T) {
		var errors []error
		base := &CmdletBase{
			ErrorWriter: func(err error) {
				errors = append(errors, err)
			},
		}

		testErr := &testError{"test error"}
		base.WriteError(testErr)

		if len(errors) != 1 {
			t.Fatalf("expected 1 error, got %d", len(errors))
		}
		if errors[0].Error() != "test error" {
			t.Errorf("expected 'test error', got %v", errors[0])
		}
	})

	t.Run("handles nil error writer", func(t *testing.T) {
		base := &CmdletBase{
			ErrorWriter: nil,
		}

		// Should not panic
		base.WriteError(&testError{"test"})
	})
}

// testError is a simple error implementation for testing.
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
