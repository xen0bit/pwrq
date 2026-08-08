package common

import (
	"testing"
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
