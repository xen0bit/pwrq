package common

import (
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/typed"
)

func TestEnsureObject(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		wantNil  bool
		wantType string
		wantVal  any
	}{
		{
			name:     "nil input returns nil",
			input:    nil,
			wantNil:  true,
			wantType: "",
			wantVal:  nil,
		},
		{
			name:     "already object returned unchanged",
			input:    typed.New("test"),
			wantNil:  false,
			wantType: "string",
			wantVal:  "test",
		},
		{
			name: "typed cmdlet output keeps its type",
			input: map[string]any{
				"PwrqValue": 42,
				"PwrqType":  "number",
			},
			wantNil:  false,
			wantType: "number",
			wantVal:  map[string]any{"PwrqValue": 42, "PwrqType": "number"},
		},
		{
			name:     "raw string wrapped",
			input:    "hello",
			wantNil:  false,
			wantType: "string",
			wantVal:  "hello",
		},
		{
			name:     "raw int wrapped",
			input:    123,
			wantNil:  false,
			wantType: "number",
			wantVal:  123,
		},
		{
			name:     "raw float wrapped",
			input:    3.14,
			wantNil:  false,
			wantType: "number",
			wantVal:  3.14,
		},
		{
			name:     "raw bool wrapped",
			input:    true,
			wantNil:  false,
			wantType: "boolean",
			wantVal:  true,
		},
		{
			// Any JSON object is a typed object; one carrying no PwrqType is simply
			// which is what PowerShell calls an object with no type of its own.
			name:     "plain JSON object reports the object type",
			input:    map[string]any{"key": "value"},
			wantNil:  false,
			wantType: "object",
			wantVal:  map[string]any{"key": "value"},
		},
		{
			name:     "raw slice wrapped",
			input:    []any{1, 2, 3},
			wantNil:  false,
			wantType: "array",
			wantVal:  []any{1, 2, 3},
		},
		{
			name: "an object with no PwrqType still converts",
			input: map[string]any{
				"_val": "incomplete",
			},
			wantNil:  false,
			wantType: "object",
			wantVal:  map[string]any{"_val": "incomplete"},
		},
		{
			name: "object carrying only a type is still that type",
			input: map[string]any{
				"PwrqType": "string",
			},
			wantNil:  false,
			wantType: "string",
			wantVal:  map[string]any{"PwrqType": "string"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnsureObject(tt.input)

			if tt.wantNil {
				if got != nil {
					t.Errorf("EnsureObject() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatalf("EnsureObject() returned nil, expected non-nil")
			}

			if got.TypeName != tt.wantType {
				t.Errorf("EnsureObject().TypeName = %v, want %v", got.TypeName, tt.wantType)
			}

			// For map comparisons, we need special handling
			if wantMap, ok := tt.wantVal.(map[string]any); ok {
				if gotMap, ok := got.Value.(map[string]any); ok {
					if !equalMaps(gotMap, wantMap) {
						t.Errorf("EnsureObject().Value = %v, want %v", got.Value, tt.wantVal)
					}
					return
				}
			}

			// For slice comparisons
			if wantSlice, ok := tt.wantVal.([]any); ok {
				if gotSlice, ok := got.Value.([]any); ok {
					if !equalSlices(gotSlice, wantSlice) {
						t.Errorf("EnsureObject().Value = %v, want %v", got.Value, tt.wantVal)
					}
					return
				}
			}

			// Direct comparison for other types
			if got.Value != tt.wantVal {
				t.Errorf("EnsureObject().Value = %v (%T), want %v (%T)", got.Value, got.Value, tt.wantVal, tt.wantVal)
			}
		})
	}
}

func TestTryEnsureObject(t *testing.T) {
	tests := []struct {
		name        string
		input       any
		wantErr     bool
		wantNil     bool
		wantType    string
		wantVal     any
		errContains string
	}{
		{
			name:    "nil input returns nil no error",
			input:   nil,
			wantErr: false,
			wantNil: true,
		},
		{
			name: "valid object's wire form converted",
			input: map[string]any{
				"PwrqValue": "hello",
				"PwrqType":  "string",
			},
			wantErr:  false,
			wantNil:  false,
			wantType: "string",
			wantVal:  "hello",
		},
		{
			name:     "plain JSON object reports the object type",
			input:    map[string]any{"key": "value"},
			wantErr:  false,
			wantNil:  false,
			wantType: "object",
			wantVal:  map[string]any{"key": "value"},
		},
		{
			name:     "raw value wrapped successfully",
			input:    42,
			wantErr:  false,
			wantNil:  false,
			wantType: "number",
			wantVal:  42,
		},
		{
			name:     "already object returned",
			input:    typed.New("test"),
			wantErr:  false,
			wantNil:  false,
			wantType: "string",
			wantVal:  "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TryEnsureObject(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("TryEnsureObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if tt.errContains != "" && err != nil {
					if !contains(err.Error(), tt.errContains) {
						t.Errorf("TryEnsureObject() error = %v, want error containing %q", err, tt.errContains)
					}
				}
				return
			}

			if tt.wantNil {
				if got != nil {
					t.Errorf("TryEnsureObject() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatalf("TryEnsureObject() returned nil, expected non-nil")
			}

			if got.TypeName != tt.wantType {
				t.Errorf("TryEnsureObject().TypeName = %v, want %v", got.TypeName, tt.wantType)
			}

			// Handle map comparison specially (maps are not comparable with !=)
			if wantMap, ok := tt.wantVal.(map[string]any); ok {
				if gotMap, ok := got.Value.(map[string]any); ok {
					if !equalMaps(gotMap, wantMap) {
						t.Errorf("TryEnsureObject().Value = %v, want %v", got.Value, tt.wantVal)
					}
				} else {
					t.Errorf("TryEnsureObject().Value type mismatch: got %T, want map[string]any", got.Value)
				}
			} else if got.Value != tt.wantVal {
				t.Errorf("TryEnsureObject().Value = %v, want %v", got.Value, tt.wantVal)
			}
		})
	}
}

func TestTypeNameOf(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{
			name:  "*typed.Object extracts TypeName",
			input: typed.NewWithType("test", "Test.CustomType"),
			want:  "Test.CustomType",
		},
		{
			name: "object's wire form extracts type",
			input: map[string]any{
				"PwrqValue": 42,
				"PwrqType":  "number",
			},
			want: "number",
		},
		{
			name:  "raw string",
			input: "hello",
			want:  "string",
		},
		{
			name:  "raw int",
			input: 123,
			want:  "number",
		},
		{
			name:  "raw float",
			input: 3.14,
			want:  "number",
		},
		{
			name:  "raw bool",
			input: true,
			want:  "boolean",
		},
		{
			name:  "nil",
			input: nil,
			want:  "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TypeNameOf(tt.input)
			if got != tt.want {
				t.Errorf("TypeNameOf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddNoteProperty(t *testing.T) {
	obj := typed.New("test")
	result := AddNoteProperty(obj, "NewProp", "value")

	if result != obj {
		t.Errorf("AddNoteProperty() did not return same object")
	}

	member, ok := obj.GetMember("NewProp")
	if !ok {
		t.Fatalf("AddNoteProperty() member not found")
	}

	if member.MemberType != typed.MemberTypeNoteProperty {
		t.Errorf("AddNoteProperty() wrong member type: %v", member.MemberType)
	}

	if member.Value != "value" {
		t.Errorf("AddNoteProperty() wrong value: %v", member.Value)
	}
}

func TestConvertObject(t *testing.T) {
	tests := []struct {
		name       string
		input      any
		targetType string
		wantErr    bool
		wantType   string
	}{
		{
			name:       "string to number",
			input:      "42",
			targetType: "number",
			wantErr:    false,
			wantType:   "number",
		},
		{
			name:       "int to string",
			input:      123,
			targetType: "string",
			wantErr:    false,
			wantType:   "string",
		},
		{
			name:       "string to bool",
			input:      "true",
			targetType: "bool",
			wantErr:    false,
			wantType:   "boolean",
		},
		{
			name:       "invalid string to number",
			input:      "not_a_number",
			targetType: "number",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConvertObject(tt.input, tt.targetType)

			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got.TypeName != tt.wantType {
				t.Errorf("ConvertObject().TypeName = %v, want %v", got.TypeName, tt.wantType)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
