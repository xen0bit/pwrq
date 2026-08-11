package common

import (
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
)

func TestSetGlobalSessionState(t *testing.T) {
	// Clear any existing session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()

	ss := sessionstate.NewSessionState()
	SetGlobalSessionState(ss)

	got := GetSessionState()
	if got != ss {
		t.Errorf("GetSessionState() returned different instance than set")
	}
}

func TestGetVariable(t *testing.T) {
	// Clear any existing session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()

	ss := sessionstate.NewSessionState()
	_ = ss.SetVariable("testVar", "testValue", 0)
	SetGlobalSessionState(ss)

	// Test getting existing variable
	val := GetVariable("testVar")
	if val != "testValue" {
		t.Errorf("GetVariable(testVar) = %v, want testValue", val)
	}

	// Test getting non-existing variable
	val = GetVariable("nonExisting")
	if val != nil {
		t.Errorf("GetVariable(nonExisting) = %v, want nil", val)
	}

	// Test with no session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()
	val = GetVariable("testVar")
	if val != nil {
		t.Errorf("GetVariable with no session = %v, want nil", val)
	}
}

func TestSetVariable(t *testing.T) {
	// Clear any existing session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()

	ss := sessionstate.NewSessionState()
	SetGlobalSessionState(ss)

	// Test setting variable
	err := SetVariable("newVar", "newValue")
	if err != nil {
		t.Errorf("SetVariable returned error: %v", err)
	}

	val, err := ss.GetVariable("newVar")
	if err != nil {
		t.Fatalf("Failed to get variable: %v", err)
	}
	if val != "newValue" {
		t.Errorf("SetVariable did not set value correctly, got %v", val)
	}

	// Test with no session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()
	err = SetVariable("testVar", "testValue")
	if err != nil {
		t.Errorf("SetVariable with no session returned error: %v", err)
	}
}

func TestGetAlias(t *testing.T) {
	// Clear any existing session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()

	ss := sessionstate.NewSessionState()
	ss.SetAlias("test", "actual_command")
	SetGlobalSessionState(ss)

	// Test getting existing alias
	cmd := GetAlias("test")
	if cmd != "actual_command" {
		t.Errorf("GetAlias(test) = %v, want actual_command", cmd)
	}

	// Test getting non-existing alias
	cmd = GetAlias("nonExisting")
	if cmd != "" {
		t.Errorf("GetAlias(nonExisting) = %v, want empty string", cmd)
	}

	// Test with no session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()
	cmd = GetAlias("test")
	if cmd != "" {
		t.Errorf("GetAlias with no session = %v, want empty string", cmd)
	}
}

func TestResolveAlias(t *testing.T) {
	// Clear any existing session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()

	ss := sessionstate.NewSessionState()
	ss.SetAlias("gci", "get_childitem")
	ss.SetAlias("ls", "gci") // Nested alias
	SetGlobalSessionState(ss)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"direct alias", "gci", "get_childitem"},
		{"nested alias", "ls", "get_childitem"},
		{"non-alias", "get_childitem", "get_childitem"},
		{"unknown command", "unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAlias(tt.input)
			if got != tt.expected {
				t.Errorf("ResolveAlias(%s) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}

	// Test with no session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()
	got := ResolveAlias("test")
	if got != "test" {
		t.Errorf("ResolveAlias with no session = %v, want test", got)
	}
}

func TestGetPreferenceVariable(t *testing.T) {
	// Clear any existing session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()

	ss := sessionstate.NewSessionState()
	_ = ss.SetVariable("TestPreference", "TestValue", 0)
	SetGlobalSessionState(ss)

	// Test getting existing preference
	val := GetPreferenceVariable("TestPreference", "default")
	if val != "TestValue" {
		t.Errorf("GetPreferenceVariable(TestPreference) = %v, want TestValue", val)
	}

	// Test getting non-existing preference (should return default)
	val = GetPreferenceVariable("NonExisting", "defaultValue")
	if val != "defaultValue" {
		t.Errorf("GetPreferenceVariable(NonExisting) = %v, want defaultValue", val)
	}
}

func TestGetErrorActionPreference(t *testing.T) {
	// Clear any existing session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()

	ss := sessionstate.NewSessionState()
	_ = ss.SetVariable("ErrorActionPreference", "Stop", 0)
	SetGlobalSessionState(ss)

	val := GetErrorActionPreference()
	if val != "Stop" {
		t.Errorf("GetErrorActionPreference() = %v, want Stop", val)
	}

	// Test default
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()
	val = GetErrorActionPreference()
	if val != "Continue" {
		t.Errorf("GetErrorActionPreference() with no session = %v, want Continue", val)
	}
}

func TestGetVerbosePreference(t *testing.T) {
	// Clear any existing session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()

	ss := sessionstate.NewSessionState()
	_ = ss.SetVariable("VerbosePreference", "SilentlyContinue", 0)
	SetGlobalSessionState(ss)

	val := GetVerbosePreference()
	if val != "SilentlyContinue" {
		t.Errorf("GetVerbosePreference() = %v, want SilentlyContinue", val)
	}
}

func TestGetDebugPreference(t *testing.T) {
	// Clear any existing session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()

	ss := sessionstate.NewSessionState()
	_ = ss.SetVariable("DebugPreference", "Inquire", 0)
	SetGlobalSessionState(ss)

	val := GetDebugPreference()
	if val != "Inquire" {
		t.Errorf("GetDebugPreference() = %v, want Inquire", val)
	}
}

func TestGetWarningPreference(t *testing.T) {
	// Clear any existing session state
	sessionStateMu.Lock()
	globalSession = nil
	sessionStateMu.Unlock()

	ss := sessionstate.NewSessionState()
	_ = ss.SetVariable("WarningPreference", "Ignore", 0)
	SetGlobalSessionState(ss)

	val := GetWarningPreference()
	if val != "Ignore" {
		t.Errorf("GetWarningPreference() = %v, want Ignore", val)
	}
}
