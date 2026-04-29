package common

import (
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

func TestNormalizeToSlice(t *testing.T) {
	t.Run("nil returns empty slice", func(t *testing.T) {
		result := NormalizeToSlice(nil)
		if len(result) != 0 {
			t.Errorf("Expected empty slice, got %d elements", len(result))
		}
	})

	t.Run("slice returned as-is", func(t *testing.T) {
		input := []any{1, 2, 3}
		result := NormalizeToSlice(input)
		if len(result) != 3 {
			t.Errorf("Expected 3 elements, got %d", len(result))
		}
	})

	t.Run("single map wrapped in slice", func(t *testing.T) {
		input := map[string]any{"Name": "Alice"}
		result := NormalizeToSlice(input)
		if len(result) != 1 {
			t.Errorf("Expected 1 element, got %d", len(result))
		}
	})

	t.Run("single value wrapped in slice", func(t *testing.T) {
		input := "hello"
		result := NormalizeToSlice(input)
		if len(result) != 1 {
			t.Errorf("Expected 1 element, got %d", len(result))
		}
		if result[0] != "hello" {
			t.Errorf("Expected 'hello', got %v", result[0])
		}
	})
}

func TestPreservePSObjectMetadata(t *testing.T) {
	t.Run("non-PSObject input returned as-is", func(t *testing.T) {
		input := map[string]any{"Name": "Alice"}
		result := PreservePSObjectMetadata(input, input)
		if m, ok := result.(map[string]any); !ok {
			t.Errorf("Expected map, got %T", result)
		} else if m["Name"] != "Alice" {
			t.Errorf("Expected Name=Alice, got %v", m["Name"])
		}
	})

	t.Run("PSObject TypeName preserved", func(t *testing.T) {
		psobj := psobject.NewPSObjectWithTypeName(map[string]any{"Name": "Alice"}, "Custom.Type")
		input := psobj.ToMap()
		result := PreservePSObjectMetadata(input, input)

		if m, ok := result.(map[string]any); !ok {
			t.Errorf("Expected map, got %T", result)
		} else {
			if meta, exists := m["_meta"].(map[string]any); !exists {
				t.Error("Expected _meta in result")
			} else if typeName, exists := meta["type"].(string); !exists || typeName != "Custom.Type" {
				t.Errorf("Expected type=Custom.Type, got %v", typeName)
			}
		}
	})

	t.Run("NoteProperty members preserved", func(t *testing.T) {
		psobj := psobject.NewPSObjectWithTypeName(map[string]any{"Name": "Alice", "Age": 30}, "Person")
		psobj.AddNoteProperty("DisplayName", "Alice Smith")
		input := psobj.ToMap()

		// Filter to just Name
		filtered := map[string]any{"Name": "Alice"}
		result := PreservePSObjectMetadata(input, filtered)

		if m, ok := result.(map[string]any); !ok {
			t.Errorf("Expected map, got %T", result)
		} else {
			// Check TypeName preserved
			if meta, exists := m["_meta"].(map[string]any); exists {
				if typeName, exists := meta["type"].(string); !exists || typeName != "Person" {
					t.Errorf("Expected type=Person, got %v", typeName)
				}
			}
			// Check DisplayName member preserved (it wasn't in filtered)
			if members, exists := m["_meta"].(map[string]any)["members"].(map[string]any); exists {
				if _, hasDisplayName := members["DisplayName"]; !hasDisplayName {
					t.Error("Expected DisplayName member to be preserved")
				}
			}
		}
	})
}

func TestExtractUDFValue(t *testing.T) {
	tests := []struct {
		name string
		input any
		want any
	}{
		{
			name:  "UDF result - extracts _val",
			input: map[string]any{"_val": "extracted", "_meta": map[string]any{"key": "value"}},
			want:  "extracted",
		},
		{
			name:  "regular string - returns as-is",
			input: "test",
			want:  "test",
		},
		{
			name:  "regular map - returns as-is",
			input: map[string]any{"key": "value"},
			want:  map[string]any{"key": "value"},
		},
		{
			name:  "number - returns as-is",
			input: 42,
			want:  42,
		},
		{
			name:  "nil - returns as-is",
			input: nil,
			want:  nil,
		},
		{
			name:  "boolean - returns as-is",
			input: true,
			want:  true,
		},
		{
			name: "UDF result with nested _val",
			input: map[string]any{
				"_val": map[string]any{"nested": "value"},
				"_meta": map[string]any{},
			},
			want: map[string]any{"nested": "value"},
		},
		{
			name: "UDF result with array _val",
			input: map[string]any{
				"_val": []any{1, 2, 3},
				"_meta": map[string]any{},
			},
			want: []any{1, 2, 3},
		},
		{
			name: "UDF result with number _val",
			input: map[string]any{
				"_val": 42,
				"_meta": map[string]any{},
			},
			want: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractUDFValue(tt.input)
			// For maps, we need to compare differently
			if gotMap, ok := got.(map[string]any); ok {
				if wantMap, ok := tt.want.(map[string]any); ok {
					if !equalMaps(gotMap, wantMap) {
						t.Errorf("ExtractUDFValue() = %v, want %v", got, tt.want)
					}
					return
				}
			}
			// For slices
			if gotSlice, ok := got.([]any); ok {
				if wantSlice, ok := tt.want.([]any); ok {
					if !equalSlices(gotSlice, wantSlice) {
						t.Errorf("ExtractUDFValue() = %v, want %v", got, tt.want)
					}
					return
				}
			}
			// For other types, direct comparison
			if got != tt.want {
				t.Errorf("ExtractUDFValue() = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestExtractUDFValueChaining(t *testing.T) {
	// Test chaining: UDF result -> extract -> UDF result -> extract
	firstResult := map[string]any{
		"_val": "hello",
		"_meta": map[string]any{
			"source": "first",
		},
	}

	// First extraction
	firstExtracted := ExtractUDFValue(firstResult)
	if firstExtracted != "hello" {
		t.Fatalf("first extraction failed: got %v, want %v", firstExtracted, "hello")
	}

	// Simulate second UDF that returns another UDF result
	secondResult := map[string]any{
		"_val": firstExtracted,
		"_meta": map[string]any{
			"source": "second",
		},
	}

	// Second extraction
	secondExtracted := ExtractUDFValue(secondResult)
	if secondExtracted != "hello" {
		t.Errorf("second extraction failed: got %v, want %v", secondExtracted, "hello")
	}
}

// equalMaps compares two maps for equality
func equalMaps(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok {
			return false
		} else if !equalValues(v, bv) {
			return false
		}
	}
	return true
}

// equalSlices compares two slices for equality
func equalSlices(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalValues(a[i], b[i]) {
			return false
		}
	}
	return true
}

// equalValues compares two values for equality, handling maps and slices
func equalValues(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Handle maps
	if am, ok := a.(map[string]any); ok {
		if bm, ok := b.(map[string]any); ok {
			return equalMaps(am, bm)
		}
		return false
	}

	// Handle slices
	if as, ok := a.([]any); ok {
		if bs, ok := b.([]any); ok {
			return equalSlices(as, bs)
		}
		return false
	}

	// Simple comparison for other types
	return a == b
}

