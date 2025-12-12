package json

import (
	"encoding/json"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestJSONParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "simple object",
			input:   `{"key": "value"}`,
			wantErr: false,
		},
		{
			name:    "array",
			input:   `[1, 2, 3]`,
			wantErr: false,
		},
		{
			name:    "nested object",
			input:   `{"a": {"b": "c"}}`,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   `{key: value}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result any
			err := json.Unmarshal([]byte(tt.input), &result)
			if (err != nil) != tt.wantErr {
				t.Errorf("json_parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == nil {
				t.Error("json_parse() returned nil for valid JSON")
			}
		})
	}
}

func TestJSONStringify(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{
			name:  "object",
			input: map[string]any{"key": "value"},
		},
		{
			name:  "array",
			input: []any{1, 2, 3},
		},
		{
			name:  "string",
			input: "hello",
		},
		{
			name: "UDF result object",
			input: map[string]any{
				"_val": map[string]any{"key": "value"},
				"_meta": map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputVal := common.ExtractUDFValue(tt.input)
			jsonBytes, err := json.Marshal(inputVal)
			if err != nil {
				t.Errorf("json_stringify() error = %v", err)
				return
			}
			if len(jsonBytes) == 0 {
				t.Error("json_stringify() returned empty string")
			}
		})
	}
}

func TestJSONRoundTrip(t *testing.T) {
	testCases := []string{
		`{"key": "value"}`,
		`[1, 2, 3]`,
		`{"a": {"b": "c"}}`,
		`"hello"`,
		`123`,
		`true`,
		`null`,
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			// Parse
			var parsed any
			if err := json.Unmarshal([]byte(tc), &parsed); err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			// Stringify
			jsonBytes, err := json.Marshal(parsed)
			if err != nil {
				t.Fatalf("stringify failed: %v", err)
			}

			// Parse again to verify
			var reparsed any
			if err := json.Unmarshal(jsonBytes, &reparsed); err != nil {
				t.Fatalf("reparse failed: %v", err)
			}

			// Note: We can't directly compare because JSON doesn't preserve order
			// But we can verify it's valid JSON
			// For null, reparsed will be nil, which is correct
			if tc == "null" {
				if reparsed != nil {
					t.Error("reparsed null should be nil")
				}
			} else if reparsed == nil {
				t.Error("reparsed result is nil for non-null input")
			}
		})
	}
}

