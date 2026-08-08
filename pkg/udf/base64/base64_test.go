package base64

import (
	"encoding/base64"
	"testing"

	"github.com/itchyny/gojq"
)

// runBase64 drives the registered function so these tests exercise the real
// UDF rather than a local restatement of what it is supposed to do.
func runBase64(t *testing.T, fn string, input any, opt gojq.CompilerOption) any {
	t.Helper()
	q, err := gojq.Parse(fn)
	if err != nil {
		t.Fatalf("parse %s: %v", fn, err)
	}
	code, err := gojq.Compile(q, opt)
	if err != nil {
		t.Fatalf("compile %s: %v", fn, err)
	}
	v, _ := code.Run(input).Next()
	return v
}

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
				val, ok := result.(string)
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
				val, ok := result.(string)
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
				val, ok := result.(string)
				if !ok {
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
				val, ok := result.(string)
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
			result := runBase64(t, "base64_encode", tt.input, RegisterBase64Encode())
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
				val, ok := result.(string)
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
				val, ok := result.(string)
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
				val, ok := result.(string)
				if !ok {
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
				val, ok := result.(string)
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
			result := runBase64(t, "base64_decode", tt.input, RegisterBase64Decode())
			err, isErr := result.(error)

			if tt.wantErr {
				if !isErr {
					t.Errorf("base64_decode() expected error but got none")
					return
				}
				if !tt.check(err) {
					t.Errorf("base64_decode() error check failed")
				}
				return
			}
			if isErr {
				t.Errorf("base64_decode() unexpected error: %v", err)
				return
			}
			if !tt.check(result) {
				t.Errorf("base64_decode() result did not pass check: %v", result)
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
