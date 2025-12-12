package common

import (
	"testing"
)

func TestIsUDFResult(t *testing.T) {
	tests := []struct {
		name string
		input any
		want bool
	}{
		{
			name:  "valid UDF result with both keys",
			input: map[string]any{"_val": "test", "_meta": map[string]any{}},
			want:  true,
		},
		{
			name:  "missing _meta",
			input: map[string]any{"_val": "test"},
			want:  false,
		},
		{
			name:  "missing _val",
			input: map[string]any{"_meta": map[string]any{}},
			want:  false,
		},
		{
			name:  "regular string",
			input: "test",
			want:  false,
		},
		{
			name:  "regular map without UDF keys",
			input: map[string]any{"key": "value"},
			want:  false,
		},
		{
			name:  "nil",
			input: nil,
			want:  false,
		},
		{
			name:  "number",
			input: 42,
			want:  false,
		},
		{
			name:  "array",
			input: []any{1, 2, 3},
			want:  false,
		},
		{
			name: "UDF result with nested values",
			input: map[string]any{
				"_val": map[string]any{"nested": "value"},
				"_meta": map[string]any{"type": "complex"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsUDFResult(tt.input)
			if got != tt.want {
				t.Errorf("IsUDFResult() = %v, want %v", got, tt.want)
			}
		})
	}
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

