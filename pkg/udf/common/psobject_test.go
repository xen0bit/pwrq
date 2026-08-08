package common

import (
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

func TestEnsurePSObject(t *testing.T) {
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
			name:     "already PSObject returned unchanged",
			input:    psobject.NewPSObject("test"),
			wantNil:  false,
			wantType: "System.String",
			wantVal:  "test",
		},
		{
			name: "typed cmdlet output keeps its type",
			input: map[string]any{
				"PSPath":     42,
				"PSTypeName": "System.Int32",
			},
			wantNil:  false,
			wantType: "System.Int32",
			wantVal:  map[string]any{"PSPath": 42, "PSTypeName": "System.Int32"},
		},
		{
			name:     "raw string wrapped",
			input:    "hello",
			wantNil:  false,
			wantType: "System.String",
			wantVal:  "hello",
		},
		{
			name:     "raw int wrapped",
			input:    123,
			wantNil:  false,
			wantType: "System.Int32",
			wantVal:  123,
		},
		{
			name:     "raw float wrapped",
			input:    3.14,
			wantNil:  false,
			wantType: "System.Double",
			wantVal:  3.14,
		},
		{
			name:     "raw bool wrapped",
			input:    true,
			wantNil:  false,
			wantType: "System.Boolean",
			wantVal:  true,
		},
		{
			// Any JSON object is a PSObject; an untyped one is a PSCustomObject,
			// which is what PowerShell calls an object with no type of its own.
			name:     "plain JSON object becomes a PSCustomObject",
			input:    map[string]any{"key": "value"},
			wantNil:  false,
			wantType: "System.Management.Automation.PSCustomObject",
			wantVal:  map[string]any{"key": "value"},
		},
		{
			name:     "raw slice wrapped",
			input:    []any{1, 2, 3},
			wantNil:  false,
			wantType: "System.Object[]",
			wantVal:  []any{1, 2, 3},
		},
		{
			name: "malformed PSObject map (missing _meta) wrapped as raw value",
			input: map[string]any{
				"_val": "incomplete",
			},
			wantNil:  false,
			wantType: "System.Management.Automation.PSCustomObject",
			wantVal:  map[string]any{"_val": "incomplete"},
		},
		{
			name: "object carrying only a type is still that type",
			input: map[string]any{
				"PSTypeName": "System.String",
			},
			wantNil:  false,
			wantType: "System.String",
			wantVal:  map[string]any{"PSTypeName": "System.String"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnsurePSObject(tt.input)

			if tt.wantNil {
				if got != nil {
					t.Errorf("EnsurePSObject() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatalf("EnsurePSObject() returned nil, expected non-nil")
			}

			if got.TypeName != tt.wantType {
				t.Errorf("EnsurePSObject().TypeName = %v, want %v", got.TypeName, tt.wantType)
			}

			// For map comparisons, we need special handling
			if wantMap, ok := tt.wantVal.(map[string]any); ok {
				if gotMap, ok := got.Value.(map[string]any); ok {
					if !equalMaps(gotMap, wantMap) {
						t.Errorf("EnsurePSObject().Value = %v, want %v", got.Value, tt.wantVal)
					}
					return
				}
			}

			// For slice comparisons
			if wantSlice, ok := tt.wantVal.([]any); ok {
				if gotSlice, ok := got.Value.([]any); ok {
					if !equalSlices(gotSlice, wantSlice) {
						t.Errorf("EnsurePSObject().Value = %v, want %v", got.Value, tt.wantVal)
					}
					return
				}
			}

			// Direct comparison for other types
			if got.Value != tt.wantVal {
				t.Errorf("EnsurePSObject().Value = %v (%T), want %v (%T)", got.Value, got.Value, tt.wantVal, tt.wantVal)
			}
		})
	}
}

func TestTryEnsurePSObject(t *testing.T) {
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
			name: "valid PSObject map converted",
			input: map[string]any{
				"PSPath":     "hello",
				"PSTypeName": "System.String",
			},
			wantErr:  false,
			wantNil:  false,
			wantType: "System.String",
			wantVal:  "hello",
		},
		{
			name:     "plain JSON object becomes a PSCustomObject",
			input:    map[string]any{"key": "value"},
			wantErr:  false,
			wantNil:  false,
			wantType: "System.Management.Automation.PSCustomObject",
			wantVal:  map[string]any{"key": "value"},
		},
		{
			name:     "raw value wrapped successfully",
			input:    42,
			wantErr:  false,
			wantNil:  false,
			wantType: "System.Int32",
			wantVal:  42,
		},
		{
			name:     "already PSObject returned",
			input:    psobject.NewPSObject("test"),
			wantErr:  false,
			wantNil:  false,
			wantType: "System.String",
			wantVal:  "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TryEnsurePSObject(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("TryEnsurePSObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if tt.errContains != "" && err != nil {
					if !contains(err.Error(), tt.errContains) {
						t.Errorf("TryEnsurePSObject() error = %v, want error containing %q", err, tt.errContains)
					}
				}
				return
			}

			if tt.wantNil {
				if got != nil {
					t.Errorf("TryEnsurePSObject() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatalf("TryEnsurePSObject() returned nil, expected non-nil")
			}

			if got.TypeName != tt.wantType {
				t.Errorf("TryEnsurePSObject().TypeName = %v, want %v", got.TypeName, tt.wantType)
			}

			// Handle map comparison specially (maps are not comparable with !=)
			if wantMap, ok := tt.wantVal.(map[string]any); ok {
				if gotMap, ok := got.Value.(map[string]any); ok {
					if !equalMaps(gotMap, wantMap) {
						t.Errorf("TryEnsurePSObject().Value = %v, want %v", got.Value, tt.wantVal)
					}
				} else {
					t.Errorf("TryEnsurePSObject().Value type mismatch: got %T, want map[string]any", got.Value)
				}
			} else if got.Value != tt.wantVal {
				t.Errorf("TryEnsurePSObject().Value = %v, want %v", got.Value, tt.wantVal)
			}
		})
	}
}

func TestGetPSTypeName(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{
			name:  "*PSObject extracts TypeName",
			input: psobject.NewPSObjectWithTypeName("test", "System.CustomType"),
			want:  "System.CustomType",
		},
		{
			name: "PSObject map extracts type",
			input: map[string]any{
				"PSPath":     42,
				"PSTypeName": "System.Int32",
			},
			want: "System.Int32",
		},
		{
			name:  "raw string",
			input: "hello",
			want:  "System.String",
		},
		{
			name:  "raw int",
			input: 123,
			want:  "System.Int32",
		},
		{
			name:  "raw float",
			input: 3.14,
			want:  "System.Double",
		},
		{
			name:  "raw bool",
			input: true,
			want:  "System.Boolean",
		},
		{
			name:  "nil",
			input: nil,
			want:  "System.Object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPSTypeName(tt.input)
			if got != tt.want {
				t.Errorf("GetPSTypeName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddNoteProperty(t *testing.T) {
	psobj := psobject.NewPSObject("test")
	result := AddNoteProperty(psobj, "NewProp", "value")

	if result != psobj {
		t.Errorf("AddNoteProperty() did not return same PSObject")
	}

	member, ok := psobj.GetMember("NewProp")
	if !ok {
		t.Fatalf("AddNoteProperty() member not found")
	}

	if member.MemberType != psobject.MemberTypeNoteProperty {
		t.Errorf("AddNoteProperty() wrong member type: %v", member.MemberType)
	}

	if member.Value != "value" {
		t.Errorf("AddNoteProperty() wrong value: %v", member.Value)
	}
}

func TestConvertPSObject(t *testing.T) {
	tests := []struct {
		name       string
		input      any
		targetType string
		wantErr    bool
		wantType   string
	}{
		{
			name:       "string to int",
			input:      "42",
			targetType: "System.Int32",
			wantErr:    false,
			wantType:   "System.Int32",
		},
		{
			name:       "int to string",
			input:      123,
			targetType: "System.String",
			wantErr:    false,
			wantType:   "System.String",
		},
		{
			name:       "string to bool",
			input:      "true",
			targetType: "System.Boolean",
			wantErr:    false,
			wantType:   "System.Boolean",
		},
		{
			name:       "invalid string to int",
			input:      "not_a_number",
			targetType: "System.Int32",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConvertPSObject(tt.input, tt.targetType)

			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertPSObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got.TypeName != tt.wantType {
				t.Errorf("ConvertPSObject().TypeName = %v, want %v", got.TypeName, tt.wantType)
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
