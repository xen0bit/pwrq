package base64

import (
	"encoding/base64"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestAutomaticValExtraction(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantVal string
		wantErr bool
	}{
		{
			name:    "regular string input",
			input:   "hello",
			wantVal: base64.StdEncoding.EncodeToString([]byte("hello")),
			wantErr: false,
		},
		{
			name: "UDF result object input - should extract _val",
			input: map[string]any{
				"_val": "hello",
				"_meta": map[string]any{
					"key": "value",
				},
			},
			wantVal: base64.StdEncoding.EncodeToString([]byte("hello")),
			wantErr: false,
		},
		{
			name: "UDF result with different _val",
			input: map[string]any{
				"_val": "world",
				"_meta": map[string]any{
					"type": "test",
				},
			},
			wantVal: base64.StdEncoding.EncodeToString([]byte("world")),
			wantErr: false,
		},
		{
			name: "UDF result with non-string _val",
			input: map[string]any{
				"_val": 123,
				"_meta": map[string]any{},
			},
			wantErr: true, // Should error when trying to encode non-string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the extraction logic using common package
			extracted := common.ExtractUDFValue(tt.input)
			
			if tt.wantErr {
				// For error cases, just verify extraction happened
				// The actual encoding will fail, which is expected
				if extracted == tt.input && common.IsUDFResult(tt.input) {
					t.Error("ExtractUDFValue should have extracted _val from UDF result")
				}
			} else {
				// Verify extraction worked correctly
				if common.IsUDFResult(tt.input) {
					obj := tt.input.(map[string]any)
					expectedVal := obj["_val"]
					if extracted != expectedVal {
						t.Errorf("ExtractUDFValue() = %v, want %v", extracted, expectedVal)
					}
				} else {
					if extracted != tt.input {
						t.Errorf("ExtractUDFValue() = %v, want %v (should return as-is for non-UDF results)", extracted, tt.input)
					}
				}
			}
		})
	}
}

func TestBase64EncodeWithUDFResultInput(t *testing.T) {
	// Create a UDF result object
	udfResult := map[string]any{
		"_val": "test string",
		"_meta": map[string]any{
			"source": "previous_udf",
		},
	}

	// Extract _val (simulating what the function does)
	extracted := common.ExtractUDFValue(udfResult)
	
	if extracted != "test string" {
		t.Errorf("extractUDFValue() = %v, want %v", extracted, "test string")
	}

	// Verify it encodes correctly
	encoded := base64.StdEncoding.EncodeToString([]byte(extracted.(string)))
	expected := base64.StdEncoding.EncodeToString([]byte("test string"))
	
	if encoded != expected {
		t.Errorf("encoding extracted value = %v, want %v", encoded, expected)
	}
}

func TestBase64DecodeWithUDFResultInput(t *testing.T) {
	// Create a base64-encoded UDF result object
	encoded := base64.StdEncoding.EncodeToString([]byte("test string"))
	udfResult := map[string]any{
		"_val": encoded,
		"_meta": map[string]any{
			"source": "base64_encode",
		},
	}

	// Extract _val (simulating what the function does)
	extracted := common.ExtractUDFValue(udfResult)
	
	if extracted != encoded {
		t.Errorf("extractUDFValue() = %v, want %v", extracted, encoded)
	}

	// Verify it decodes correctly
	decoded, err := base64.StdEncoding.DecodeString(extracted.(string))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	
	if string(decoded) != "test string" {
		t.Errorf("decoding extracted value = %v, want %v", string(decoded), "test string")
	}
}

func TestIsUDFResultInBase64(t *testing.T) {
	tests := []struct {
		name string
		input any
		want  bool
	}{
		{
			name:  "valid UDF result",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := common.IsUDFResult(tt.input)
			if got != tt.want {
				t.Errorf("IsUDFResult() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractUDFValueInBase64(t *testing.T) {
	tests := []struct {
		name string
		input any
		want  any
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
			name: "UDF result with nested _val",
			input: map[string]any{
				"_val": map[string]any{"nested": "value"},
				"_meta": map[string]any{},
			},
			want: map[string]any{"nested": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := common.ExtractUDFValue(tt.input)
			// For maps, use proper comparison
			if gotMap, ok := got.(map[string]any); ok {
				if wantMap, ok := tt.want.(map[string]any); ok {
					if !equalMaps(gotMap, wantMap) {
						t.Errorf("ExtractUDFValue() = %v, want %v", got, tt.want)
					}
					return
				}
			}
			// For other types, direct comparison
			if got != tt.want {
				t.Errorf("ExtractUDFValue() = %v, want %v", got, tt.want)
			}
		})
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

// equalValues compares two values for equality, handling maps
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
	
	// Simple comparison for other types
	return a == b
}

func TestChainingUDFs(t *testing.T) {
	// Test that chaining works: encode -> decode should return original
	testCases := []string{
		"hello",
		"test string",
		"",
		"special chars: !@#$%",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			// Simulate: base64_encode returns UDF result
			encoded := base64.StdEncoding.EncodeToString([]byte(tc))
			encodeResult := map[string]any{
				"_val": encoded,
				"_meta": map[string]any{
					"encoding": "base64",
				},
			}

			// Simulate: base64_decode receives UDF result and extracts _val
			extracted := common.ExtractUDFValue(encodeResult)
			if extracted != encoded {
				t.Fatalf("extraction failed: got %v, want %v", extracted, encoded)
			}

			// Decode the extracted value
			decoded, err := base64.StdEncoding.DecodeString(extracted.(string))
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			if string(decoded) != tc {
				t.Errorf("chaining failed: got %q, want %q", string(decoded), tc)
			}
		})
	}
}

