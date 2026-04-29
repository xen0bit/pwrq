package sessionstate

import (
	"testing"
)

func TestSetVariable_Basic(t *testing.T) {
	ss := NewSessionState()

	err := ss.SetVariable("count", 42, None)
	if err != nil {
		t.Fatalf("SetVariable failed: %v", err)
	}

	val, err := ss.GetVariable("count")
	if err != nil {
		t.Fatalf("GetVariable failed: %v", err)
	}

	if val != 42 {
		t.Errorf("Expected 42, got %v", val)
	}
}

func TestSetVariable_ReadOnly(t *testing.T) {
	ss := NewSessionState()

	// Set a read-only variable
	err := ss.SetVariable("readonly_var", "initial", ReadOnly)
	if err != nil {
		t.Fatalf("SetVariable failed: %v", err)
	}

	// Try to overwrite - should fail
	err = ss.SetVariable("readonly_var", "modified", None)
	if err == nil {
		t.Error("Expected error when overwriting read-only variable, got nil")
	}

	// Verify original value is intact
	val, err := ss.GetVariable("readonly_var")
	if err != nil {
		t.Fatalf("GetVariable failed: %v", err)
	}
	if val != "initial" {
		t.Errorf("Expected 'initial', got %v", val)
	}
}

func TestSetVariable_Constant(t *testing.T) {
	ss := NewSessionState()

	// Set a constant variable
	err := ss.SetVariable("constant_var", "fixed", Constant)
	if err != nil {
		t.Fatalf("SetVariable failed: %v", err)
	}

	// Try to overwrite - should fail
	err = ss.SetVariable("constant_var", "changed", None)
	if err == nil {
		t.Error("Expected error when overwriting constant variable, got nil")
	}

	// Verify original value is intact
	val, err := ss.GetVariable("constant_var")
	if err != nil {
		t.Fatalf("GetVariable failed: %v", err)
	}
	if val != "fixed" {
		t.Errorf("Expected 'fixed', got %v", val)
	}
}

func TestSetVariable_ValidateName_Empty(t *testing.T) {
	ss := NewSessionState()

	err := ss.SetVariable("", 42, None)
	if err == nil {
		t.Error("Expected error for empty variable name, got nil")
	}
}

func TestSetVariable_ValidateName_InvalidStart(t *testing.T) {
	ss := NewSessionState()

	err := ss.SetVariable("1invalid", 42, None)
	if err == nil {
		t.Error("Expected error for variable name starting with digit, got nil")
	}

	// Note: $invalid is actually valid because $ is stripped before validation
	// To test invalid start, use a digit or special char directly
	err = ss.SetVariable("@invalid", 42, None)
	if err == nil {
		t.Error("Expected error for variable name starting with @, got nil")
	}
}

func TestSetVariable_ValidateName_InvalidChars(t *testing.T) {
	ss := NewSessionState()

	err := ss.SetVariable("my-var", 42, None)
	if err == nil {
		t.Error("Expected error for variable name with hyphen, got nil")
	}

	err = ss.SetVariable("my.var", 42, None)
	if err == nil {
		t.Error("Expected error for variable name with dot, got nil")
	}

	err = ss.SetVariable("my var", 42, None)
	if err == nil {
		t.Error("Expected error for variable name with space, got nil")
	}
}

func TestSetVariable_ValidateName_Valid(t *testing.T) {
	ss := NewSessionState()

	validNames := []string{"count", "_private", "var123", "MyVar", "test_var_1"}

	for _, name := range validNames {
		err := ss.SetVariable(name, 42, None)
		if err != nil {
			t.Errorf("Valid name %q failed: %v", name, err)
		}
	}
}

func TestSetVariable_ScopePrefix_Global(t *testing.T) {
	ss := NewSessionState()

	// Create a script scope
	ss.NewScriptScope()

	// Set a global variable from script scope
	err := ss.SetVariable("global:globalVar", "global_value", None)
	if err != nil {
		t.Fatalf("SetVariable with global: prefix failed: %v", err)
	}

	// Pop back to global scope
	ss.PopScope()

	// Should be accessible from global scope
	val, err := ss.GetVariable("globalVar")
	if err != nil {
		t.Fatalf("GetVariable failed: %v", err)
	}
	if val != "global_value" {
		t.Errorf("Expected 'global_value', got %v", val)
	}
}

func TestSetVariable_ScopePrefix_Local(t *testing.T) {
	ss := NewSessionState()

	// Set in global first
	ss.SetVariable("testVar", "global", None)

	// Create a local scope
	ss.NewLocalScope()

	// Set a local variable
	err := ss.SetVariable("local:testVar", "local_value", None)
	if err != nil {
		t.Fatalf("SetVariable with local: prefix failed: %v", err)
	}

	// Should get local value
	val, err := ss.GetVariable("testVar")
	if err != nil {
		t.Fatalf("GetVariable failed: %v", err)
	}
	if val != "local_value" {
		t.Errorf("Expected 'local_value', got %v", val)
	}

	// Pop back to global
	ss.PopScope()

	// Should get global value
	val, err = ss.GetVariable("testVar")
	if err != nil {
		t.Fatalf("GetVariable failed: %v", err)
	}
	if val != "global" {
		t.Errorf("Expected 'global', got %v", val)
	}
}

func TestSetVariable_LeadingDollarSign(t *testing.T) {
	ss := NewSessionState()

	// Set with leading $
	err := ss.SetVariable("$myVar", 100, None)
	if err != nil {
		t.Fatalf("SetVariable with $ prefix failed: %v", err)
	}

	// Get without $
	val, err := ss.GetVariable("myVar")
	if err != nil {
		t.Fatalf("GetVariable failed: %v", err)
	}
	if val != 100 {
		t.Errorf("Expected 100, got %v", val)
	}
}

func TestSetVariable_VariableDrive(t *testing.T) {
	ss := NewSessionState()

	err := ss.SetVariable("driveTest", "drive_value", None)
	if err != nil {
		t.Fatalf("SetVariable failed: %v", err)
	}

	// Check Variable: drive
	val, err := ss.GetDriveItem("Variable", "driveTest")
	if err != nil {
		t.Fatalf("GetDriveItem failed: %v", err)
	}
	if val != "drive_value" {
		t.Errorf("Expected 'drive_value' in Variable: drive, got %v", val)
	}
}

func TestSetVariable_AllScope(t *testing.T) {
	ss := NewSessionState()

	// Set an AllScope variable in global
	err := ss.SetVariable("allScopeVar", "shared", AllScope)
	if err != nil {
		t.Fatalf("SetVariable failed: %v", err)
	}

	// Create child scopes
	ss.NewScriptScope()
	ss.NewLocalScope()

	// Should be visible in nested scope
	val, err := ss.GetVariable("allScopeVar")
	if err != nil {
		t.Fatalf("GetVariable in nested scope failed: %v", err)
	}
	if val != "shared" {
		t.Errorf("Expected 'shared', got %v", val)
	}
}

func TestValidateVariableName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"valid", false},
		{"_underscore", false},
		{"var123", false},
		{"", true},
		{"1starts_with_digit", true},
		{"has-dash", true},
		{"has.dot", true},
		{"has space", true},
		{"has!bang", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVariableName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateVariableName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestParseScopeFromName(t *testing.T) {
	ss := NewSessionState()

	tests := []struct {
		input       string
		wantScope   ScopeType
		wantVarName string
	}{
		{"global:myVar", ScopeGlobal, "myVar"},
		{"script:myVar", ScopeScript, "myVar"},
		{"local:myVar", ScopeLocal, "myVar"},
		{"private:myVar", ScopePrivate, "myVar"},
		{"noPrefix", ss.CurrentScope.Type, "noPrefix"},
		{"GLOBAL:uppercase", ScopeGlobal, "uppercase"},
	}

	for _, tt := range tests {
		scopeType, varName := ss.parseScopeFromName(tt.input)
		if scopeType != tt.wantScope {
			t.Errorf("parseScopeFromName(%q) scope = %v, want %v", tt.input, scopeType, tt.wantScope)
		}
		if varName != tt.wantVarName {
			t.Errorf("parseScopeFromName(%q) varName = %v, want %v", tt.input, varName, tt.wantVarName)
		}
	}
}

func TestVariableOptions_Bitmask(t *testing.T) {
	// Test that options are proper bitflags
	tests := []struct {
		name     string
		opts     VariableOptions
		wantBits []VariableOptions
	}{
		{"None", None, nil},
		{"ReadOnly", ReadOnly, []VariableOptions{ReadOnly}},
		{"Constant", Constant, []VariableOptions{Constant}},
		{"Private", Private, []VariableOptions{Private}},
		{"AllScope", AllScope, []VariableOptions{AllScope}},
		{"ReadOnly|AllScope", ReadOnly | AllScope, []VariableOptions{ReadOnly, AllScope}},
		{"Constant|Private", Constant | Private, []VariableOptions{Constant, Private}},
		{"ReadOnly|Constant|AllScope", ReadOnly | Constant | AllScope, []VariableOptions{ReadOnly, Constant, AllScope}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, bit := range tt.wantBits {
				if tt.opts&bit == 0 {
					t.Errorf("Expected option %v to be set in %v", bit, tt.opts)
				}
			}
			// Verify no extra bits are set
			allBits := ReadOnly | Constant | Private | AllScope
			extraBits := tt.opts &^ allBits
			if extraBits != 0 {
				t.Errorf("Unexpected bits set: %v", extraBits)
			}
		})
	}
}

func TestSetVariable_CombinedOptions(t *testing.T) {
	ss := NewSessionState()

	// Set a variable with combined ReadOnly and AllScope options
	err := ss.SetVariable("combinedVar", "value", ReadOnly|AllScope)
	if err != nil {
		t.Fatalf("SetVariable with combined options failed: %v", err)
	}

	// Verify both options are set
	entry, err := ss.GetVariableEntry("combinedVar")
	if err != nil {
		t.Fatalf("GetVariableEntry failed: %v", err)
	}

	if entry.Options&ReadOnly == 0 {
		t.Error("Expected ReadOnly option to be set")
	}
	if entry.Options&AllScope == 0 {
		t.Error("Expected AllScope option to be set")
	}
	if entry.Options&Constant != 0 {
		t.Error("Did not expect Constant option to be set")
	}
	if entry.Options&Private != 0 {
		t.Error("Did not expect Private option to be set")
	}

	// Try to overwrite - should fail due to ReadOnly
	err = ss.SetVariable("combinedVar", "modified", None)
	if err == nil {
		t.Error("Expected error when overwriting read-only variable, got nil")
	}
}
