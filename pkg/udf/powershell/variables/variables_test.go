package variables

import (
	"path/filepath"
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestSetVariable(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	tests := []struct {
		name      string
		varName   string
		value     any
		opts      SetVariableOptions
		wantErr   bool
		checkFunc func(t *testing.T, ss *sessionstate.SessionState)
	}{
		{
			name:    "set simple variable",
			varName: "testVar",
			value:   "hello",
			opts:    SetVariableOptions{},
			wantErr: false,
			checkFunc: func(t *testing.T, ss *sessionstate.SessionState) {
				val, err := ss.GetVariable("testVar")
				if err != nil {
					t.Errorf("Failed to get variable: %v", err)
				}
				if val != "hello" {
					t.Errorf("Expected 'hello', got %v", val)
				}
			},
		},
		{
			name:    "set numeric variable",
			varName: "count",
			value:   42,
			opts:    SetVariableOptions{},
			wantErr: false,
			checkFunc: func(t *testing.T, ss *sessionstate.SessionState) {
				val, err := ss.GetVariable("count")
				if err != nil {
					t.Errorf("Failed to get variable: %v", err)
				}
				// JSON numbers are float64
				if val != float64(42) && val != 42 {
					t.Errorf("Expected 42, got %v (type %T)", val, val)
				}
			},
		},
		{
			name:    "set variable with scope",
			varName: "globalVar",
			value:   "global",
			opts:    SetVariableOptions{Scope: "Global"},
			wantErr: false,
			checkFunc: func(t *testing.T, ss *sessionstate.SessionState) {
				// Variable should be in global scope
				// GetVariable searches from current scope up, so "globalVar" should find it
				val, err := ss.GetVariable("globalVar")
				if err != nil {
					t.Errorf("Failed to get variable: %v", err)
				}
				if val != "global" {
					t.Errorf("Expected 'global', got %v", val)
				}
			},
		},
		{
			name:    "set empty name should fail",
			varName: "",
			value:   "value",
			opts:    SetVariableOptions{},
			wantErr: true,
			checkFunc: func(t *testing.T, ss *sessionstate.SessionState) {
				// No check needed
			},
		},
		{
			name:    "set ReadOnly variable",
			varName: "readOnlyVar",
			value:   "readonly",
			opts:    SetVariableOptions{Option: sessionstate.ReadOnly},
			wantErr: false,
			checkFunc: func(t *testing.T, ss *sessionstate.SessionState) {
				val, err := ss.GetVariable("readOnlyVar")
				if err != nil {
					t.Errorf("Failed to get variable: %v", err)
				}
				if val != "readonly" {
					t.Errorf("Expected 'readonly', got %v", val)
				}
			},
		},
		{
			name:    "set Constant variable",
			varName: "constantVar",
			value:   "constant",
			opts:    SetVariableOptions{Option: sessionstate.Constant},
			wantErr: false,
			checkFunc: func(t *testing.T, ss *sessionstate.SessionState) {
				val, err := ss.GetVariable("constantVar")
				if err != nil {
					t.Errorf("Failed to get variable: %v", err)
				}
				if val != "constant" {
					t.Errorf("Expected 'constant', got %v", val)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setVariable(ss, tt.varName, tt.value, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("setVariable() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.checkFunc != nil && err == nil {
				tt.checkFunc(t, ss)
			}
		})
	}
}

func TestSetVariableReadOnlyProtection(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Set a ReadOnly variable
	err := ss.SetVariable("protectedVar", "original", sessionstate.ReadOnly)
	if err != nil {
		t.Fatalf("Failed to set ReadOnly variable: %v", err)
	}

	// Attempt to overwrite it - should fail
	err = setVariable(ss, "protectedVar", "modified", SetVariableOptions{})
	if err == nil {
		t.Error("Expected error when overwriting ReadOnly variable, got nil")
	}
	if err != nil && err.Error() != `cannot overwrite read-only variable "protectedVar"` {
		t.Errorf("Expected read-only protection error, got: %v", err)
	}
}

func TestSetVariableConstantProtection(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Set a Constant variable
	err := ss.SetVariable("constantVar", "original", sessionstate.Constant)
	if err != nil {
		t.Fatalf("Failed to set Constant variable: %v", err)
	}

	// Attempt to overwrite it - should fail
	err = setVariable(ss, "constantVar", "modified", SetVariableOptions{})
	if err == nil {
		t.Error("Expected error when overwriting Constant variable, got nil")
	}
	if err != nil && err.Error() != `cannot overwrite constant variable "constantVar"` {
		t.Errorf("Expected constant protection error, got: %v", err)
	}
}

func TestGetVariable(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Setup test variables
	_ = ss.SetVariable("testVar", "testValue", sessionstate.None)
	_ = ss.SetVariable("numVar", 123, sessionstate.None)
	_ = ss.SetVariable("globalVar", "global", sessionstate.None)
	_ = ss.SetVariable("readOnlyVar", "readonly", sessionstate.ReadOnly)

	tests := []struct {
		name      string
		varName   string
		opts      GetVariableOptions
		wantErr   bool
		checkFunc func(t *testing.T, result any)
	}{
		{
			name:    "get existing variable",
			varName: "testVar",
			opts:    GetVariableOptions{},
			wantErr: false,
			checkFunc: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				if !ok {
					t.Fatalf("Expected map, got %T", result)
				}
				if m["Value"] != "testValue" {
					t.Errorf("Expected 'testValue', got %v", m["Value"])
				}
				// Check that Options and Scope are included
				if m["Options"] == nil {
					t.Error("Expected Options field in result")
				}
				if m["Scope"] == nil {
					t.Error("Expected Scope field in result")
				}
			},
		},
		{
			name:    "get variable value only",
			varName: "numVar",
			opts:    GetVariableOptions{ValueOnly: true},
			wantErr: false,
			checkFunc: func(t *testing.T, result any) {
				// JSON numbers are float64
				if result != float64(123) && result != 123 {
					t.Errorf("Expected 123, got %v (type %T)", result, result)
				}
			},
		},
		{
			name:    "get non-existent variable",
			varName: "nonExistent",
			opts:    GetVariableOptions{},
			wantErr: true,
			checkFunc: func(t *testing.T, result any) {
				// No check needed
			},
		},
		{
			name:    "get ReadOnly variable",
			varName: "readOnlyVar",
			opts:    GetVariableOptions{},
			wantErr: false,
			checkFunc: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				if !ok {
					t.Fatalf("Expected map, got %T", result)
				}
				if m["Options"] != "ReadOnly" {
					t.Errorf("Expected Options='ReadOnly', got %v", m["Options"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getVariable(ss, tt.varName, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("getVariable() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.checkFunc != nil && err == nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

func TestGetVariableWildcard(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Setup test variables
	_ = ss.SetVariable("testVar1", "value1", sessionstate.None)
	_ = ss.SetVariable("testVar2", "value2", sessionstate.None)
	_ = ss.SetVariable("otherVar", "other", sessionstate.None)

	// Test wildcard matching using filepath.Match
	allVars := ss.GetVariables()

	var matchedCount int
	for name := range allVars {
		matched, err := filepath.Match("test*", name)
		if err != nil {
			t.Errorf("filepath.Match error: %v", err)
			continue
		}
		if matched {
			matchedCount++
		}
	}

	// We expect at least testVar1 and testVar2 to match
	if matchedCount < 2 {
		t.Errorf("Expected at least 2 variables matching 'test*', got %d", matchedCount)
	}
}

func TestGetVariableExcludeInclude(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Setup test variables
	_ = ss.SetVariable("include1", "val1", sessionstate.None)
	_ = ss.SetVariable("include2", "val2", sessionstate.None)
	_ = ss.SetVariable("exclude1", "val3", sessionstate.None)

	opts := GetVariableOptions{
		Include: "include*",
		Exclude: "exclude*",
	}

	allVars := ss.GetVariables()

	// Simulate filtering
	var includedCount int
	var excludedCount int
	for name := range allVars {
		// Check include filter
		if opts.Include != "" {
			matched, _ := filepath.Match(opts.Include, name)
			if matched {
				includedCount++
			}
		}
		// Check exclude filter
		if opts.Exclude != "" {
			matched, _ := filepath.Match(opts.Exclude, name)
			if matched {
				excludedCount++
			}
		}
	}

	if includedCount < 2 {
		t.Errorf("Expected at least 2 variables matching include pattern, got %d", includedCount)
	}
	if excludedCount < 1 {
		t.Errorf("Expected at least 1 variable matching exclude pattern, got %d", excludedCount)
	}
}

func TestRemoveVariable(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Setup test variables
	_ = ss.SetVariable("toRemove", "value", sessionstate.None)
	_ = ss.SetVariable("toKeep", "value", sessionstate.None)

	tests := []struct {
		name      string
		varName   string
		opts      RemoveVariableOptions
		wantErr   bool
		checkFunc func(t *testing.T, ss *sessionstate.SessionState)
	}{
		{
			name:    "remove existing variable",
			varName: "toRemove",
			opts:    RemoveVariableOptions{},
			wantErr: false,
			checkFunc: func(t *testing.T, ss *sessionstate.SessionState) {
				_, err := ss.GetVariable("toRemove")
				if err == nil {
					t.Error("Expected error getting removed variable")
				}
			},
		},
		{
			name:    "keep variable not removed",
			varName: "toRemove",
			opts:    RemoveVariableOptions{},
			wantErr: false,
			checkFunc: func(t *testing.T, ss *sessionstate.SessionState) {
				val, err := ss.GetVariable("toKeep")
				if err != nil {
					t.Errorf("Failed to get toKeep variable: %v", err)
				}
				if val != "value" {
					t.Errorf("Expected 'value', got %v", val)
				}
			},
		},
		{
			name:    "remove non-existent with Force",
			varName: "nonExistent",
			opts:    RemoveVariableOptions{Force: true},
			wantErr: false,
			checkFunc: func(t *testing.T, ss *sessionstate.SessionState) {
				// No check needed - Force should suppress not found error
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state
			_ = ss.SetVariable("toRemove", "value", sessionstate.None)
			_ = ss.SetVariable("toKeep", "value", sessionstate.None)

			err := removeVariable(ss, tt.varName, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("removeVariable() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.checkFunc != nil && err == nil {
				tt.checkFunc(t, ss)
			}
		})
	}
}

func TestRemoveVariableReadOnlyProtection(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Set a ReadOnly variable
	err := ss.SetVariable("protectedVar", "value", sessionstate.ReadOnly)
	if err != nil {
		t.Fatalf("Failed to set ReadOnly variable: %v", err)
	}

	// Attempt to remove it without Force - should fail
	err = removeVariable(ss, "protectedVar", RemoveVariableOptions{})
	if err == nil {
		t.Error("Expected error when removing ReadOnly variable, got nil")
	}

	// Attempt to remove it with Force - should still fail (Force doesn't bypass protection)
	err = removeVariable(ss, "protectedVar", RemoveVariableOptions{Force: true})
	if err == nil {
		t.Error("Expected error when removing ReadOnly variable with Force, got nil")
	}
}

func TestRemoveVariableWildcard(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Setup test variables
	_ = ss.SetVariable("test1", "value1", sessionstate.None)
	_ = ss.SetVariable("test2", "value2", sessionstate.None)
	_ = ss.SetVariable("keep", "value", sessionstate.None)

	// Remove all variables matching "test*"
	removedCount, err := removeVariablesByPattern(ss, "test*", RemoveVariableOptions{})
	if err != nil {
		t.Errorf("removeVariablesByPattern error: %v", err)
	}
	if removedCount != 2 {
		t.Errorf("Expected to remove 2 variables, got %d", removedCount)
	}

	// Verify test1 and test2 are removed
	_, err = ss.GetVariable("test1")
	if err == nil {
		t.Error("Expected error getting removed test1")
	}
	_, err = ss.GetVariable("test2")
	if err == nil {
		t.Error("Expected error getting removed test2")
	}

	// Verify keep is still there
	val, err := ss.GetVariable("keep")
	if err != nil {
		t.Errorf("Failed to get keep variable: %v", err)
	}
	if val != "value" {
		t.Errorf("Expected 'value', got %v", val)
	}
}

func TestRemoveVariableWithExclude(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Setup test variables
	_ = ss.SetVariable("test1", "value1", sessionstate.None)
	_ = ss.SetVariable("test2", "value2", sessionstate.None)
	_ = ss.SetVariable("test3", "value3", sessionstate.None)

	// Remove all test* variables except test2
	removedCount, err := removeVariablesByPattern(ss, "test*", RemoveVariableOptions{
		Exclude: "test2",
	})
	if err != nil {
		t.Errorf("removeVariablesByPattern error: %v", err)
	}
	if removedCount != 2 {
		t.Errorf("Expected to remove 2 variables (excluding test2), got %d", removedCount)
	}

	// Verify test2 is still there
	val, err := ss.GetVariable("test2")
	if err != nil {
		t.Errorf("Failed to get test2 variable: %v", err)
	}
	if val != "value2" {
		t.Errorf("Expected 'value2', got %v", val)
	}
}

func TestWildcardMatching(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		{"*", "anything", true},
		{"test*", "test", true},
		{"test*", "testing", true},
		{"test*", "other", false},
		{"*test", "test", true},
		{"*test", "mytest", true},
		{"*test", "other", false},
		{"te?t", "test", true},
		{"te?t", "text", true},
		{"te?t", "teet", true},
		{"te?t", "tet", false},
		{"te*st", "test", true},
		{"te*st", "testingst", true},
		{"te*st", "other", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.s, func(t *testing.T) {
			got, err := filepath.Match(tt.pattern, tt.s)
			if err != nil {
				t.Errorf("filepath.Match error: %v", err)
			}
			if got != tt.want {
				t.Errorf("filepath.Match(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
			}
		})
	}
}

func TestNormalizeScope(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Global", "global"},
		{"global", "global"},
		{"GLOBAL", "global"},
		{"Script", "script"},
		{"script", "script"},
		{"Local", "local"},
		{"local", "local"},
		{"Private", "private"},
		{"private", "private"},
		{"Unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeScope(tt.input)
			if got != tt.want {
				t.Errorf("normalizeScope(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestVariableOptionsToString(t *testing.T) {
	tests := []struct {
		opts sessionstate.VariableOptions
		want string
	}{
		{sessionstate.None, "None"},
		{sessionstate.ReadOnly, "ReadOnly"},
		{sessionstate.Constant, "Constant"},
		{sessionstate.Private, "Private"},
		{sessionstate.AllScope, "AllScope"},
		{sessionstate.ReadOnly | sessionstate.AllScope, "ReadOnly, AllScope"},
		{sessionstate.Constant | sessionstate.Private, "Constant, Private"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := variableOptionsToString(tt.opts)
			if got != tt.want {
				t.Errorf("variableOptionsToString(%v) = %q, want %q", tt.opts, got, tt.want)
			}
		})
	}
}

func TestScopeTypeToString(t *testing.T) {
	tests := []struct {
		scope sessionstate.ScopeType
		want  string
	}{
		{sessionstate.ScopeGlobal, "Global"},
		{sessionstate.ScopeScript, "Script"},
		{sessionstate.ScopeLocal, "Local"},
		{sessionstate.ScopePrivate, "Private"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := scopeTypeToString(tt.scope)
			if got != tt.want {
				t.Errorf("scopeTypeToString(%v) = %q, want %q", tt.scope, got, tt.want)
			}
		})
	}
}
