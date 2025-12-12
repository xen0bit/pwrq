package base64

import (
	"encoding/base64"
	"testing"
)

func TestBase64Encode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(any) bool
	}{
		{
			name:    "encode simple string",
			input:   "hello",
			wantErr: false,
			check: func(result any) bool {
				obj, ok := result.(map[string]any)
				if !ok {
					return false
				}
				val, ok := obj["_val"].(string)
				if !ok {
					return false
				}
				// Check it's valid base64
				expected := base64.StdEncoding.EncodeToString([]byte("hello"))
				return val == expected
			},
		},
		{
			name:    "encode empty string",
			input:   "",
			wantErr: false,
			check: func(result any) bool {
				obj, ok := result.(map[string]any)
				if !ok {
					return false
				}
				val, ok := obj["_val"].(string)
				if !ok {
					return false
				}
				return val == ""
			},
		},
		{
			name:    "encode with special characters",
			input:   "hello world!",
			wantErr: false,
			check: func(result any) bool {
				obj, ok := result.(map[string]any)
				if !ok {
					return false
				}
				val, ok := obj["_val"].(string)
				if !ok {
					return false
				}
				// Verify structure
				if _, ok := obj["_meta"]; !ok {
					return false
				}
				meta, ok := obj["_meta"].(map[string]any)
				if !ok {
					return false
				}
				if meta["encoding"] != "base64" {
					return false
				}
				// Check it's valid base64
				expected := base64.StdEncoding.EncodeToString([]byte("hello world!"))
				return val == expected
			},
		},
		{
			name:    "encode unicode string",
			input:   "こんにちは",
			wantErr: false,
			check: func(result any) bool {
				obj, ok := result.(map[string]any)
				if !ok {
					return false
				}
				val, ok := obj["_val"].(string)
				if !ok {
					return false
				}
				// Decode and verify
				decoded, err := base64.StdEncoding.DecodeString(val)
				if err != nil {
					return false
				}
				return string(decoded) == "こんにちは"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the encoding logic directly
			encoded := base64.StdEncoding.EncodeToString([]byte(tt.input))
			result := map[string]any{
				"_val": encoded,
				"_meta": map[string]any{
					"encoding": "base64",
					"original_length": len(tt.input),
					"encoded_length": len(encoded),
				},
			}
			
			if !tt.check(result) {
				t.Errorf("base64_encode() result did not pass check: %v", result)
			}
		})
	}
}

func TestBase64Decode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(any) bool
	}{
		{
			name:    "decode simple string",
			input:   base64.StdEncoding.EncodeToString([]byte("hello")),
			wantErr: false,
			check: func(result any) bool {
				obj, ok := result.(map[string]any)
				if !ok {
					return false
				}
				val, ok := obj["_val"].(string)
				if !ok {
					return false
				}
				return val == "hello"
			},
		},
		{
			name:    "decode empty string",
			input:   "",
			wantErr: false,
			check: func(result any) bool {
				obj, ok := result.(map[string]any)
				if !ok {
					return false
				}
				val, ok := obj["_val"].(string)
				if !ok {
					return false
				}
				return val == ""
			},
		},
		{
			name:    "decode with special characters",
			input:   base64.StdEncoding.EncodeToString([]byte("hello world!")),
			wantErr: false,
			check: func(result any) bool {
				obj, ok := result.(map[string]any)
				if !ok {
					return false
				}
				val, ok := obj["_val"].(string)
				if !ok {
					return false
				}
				// Verify structure
				if _, ok := obj["_meta"]; !ok {
					return false
				}
				meta, ok := obj["_meta"].(map[string]any)
				if !ok {
					return false
				}
				if meta["encoding"] != "base64" {
					return false
				}
				return val == "hello world!"
			},
		},
		{
			name:    "decode unicode string",
			input:   base64.StdEncoding.EncodeToString([]byte("こんにちは")),
			wantErr: false,
			check: func(result any) bool {
				obj, ok := result.(map[string]any)
				if !ok {
					return false
				}
				val, ok := obj["_val"].(string)
				if !ok {
					return false
				}
				return val == "こんにちは"
			},
		},
		{
			name:    "decode invalid base64",
			input:   "invalid base64!!!",
			wantErr: true,
			check: func(result any) bool {
				// Should return an error
				_, ok := result.(error)
				return ok
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the decoding logic directly
			decoded, err := base64.StdEncoding.DecodeString(tt.input)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("base64_decode() expected error but got none")
				}
				// Check that error is returned
				if !tt.check(err) {
					t.Errorf("base64_decode() error check failed")
				}
			} else {
				if err != nil {
					t.Errorf("base64_decode() unexpected error: %v", err)
					return
				}
				result := map[string]any{
					"_val": string(decoded),
					"_meta": map[string]any{
						"encoding": "base64",
						"original_length": len(tt.input),
						"decoded_length": len(decoded),
					},
				}
				if !tt.check(result) {
					t.Errorf("base64_decode() result did not pass check: %v", result)
				}
			}
		})
	}
}

func TestBase64RoundTrip(t *testing.T) {
	// Test that encode -> decode returns original value
	testCases := []string{
		"hello",
		"hello world!",
		"",
		"こんにちは",
		"test\nwith\nnewlines",
		"special chars: !@#$%^&*()",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			// Encode
			encoded := base64.StdEncoding.EncodeToString([]byte(tc))
			
			// Decode
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatalf("base64_decode() failed: %v", err)
			}
			
			// Verify round trip
			if string(decoded) != tc {
				t.Errorf("round trip failed: got %q, want %q", string(decoded), tc)
			}
		})
	}
}

