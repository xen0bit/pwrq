package md5

import (
	"crypto/md5"
	"fmt"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestMD5(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{
			name:    "simple string",
			input:   "hello",
			want:    fmt.Sprintf("%x", md5.Sum([]byte("hello"))),
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			want:    fmt.Sprintf("%x", md5.Sum([]byte(""))),
			wantErr: false,
		},
		{
			name:    "string with special characters",
			input:   "hello world!",
			want:    fmt.Sprintf("%x", md5.Sum([]byte("hello world!"))),
			wantErr: false,
		},
		{
			name:    "unicode string",
			input:   "こんにちは",
			want:    fmt.Sprintf("%x", md5.Sum([]byte("こんにちは"))),
			wantErr: false,
		},
		{
			name:    "bytes input",
			input:   []byte("test"),
			want:    fmt.Sprintf("%x", md5.Sum([]byte("test"))),
			wantErr: false,
		},
		{
			name: "cmdlet output binds by property name",
			input: map[string]any{
				"PwrqValue": "hello",
				"PwrqType":  "string",
			},
			want:    fmt.Sprintf("%x", md5.Sum([]byte("hello"))),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract _val if it's a UDF result
			inputVal := common.BindValue(tt.input)

			// Convert to bytes
			var inputBytes []byte
			switch val := inputVal.(type) {
			case string:
				inputBytes = []byte(val)
			case []byte:
				inputBytes = val
			default:
				if !tt.wantErr {
					t.Fatalf("unexpected input type: %T", val)
				}
				return
			}

			// Compute MD5
			hash := md5.Sum(inputBytes)
			got := fmt.Sprintf("%x", hash)

			if got != tt.want {
				t.Errorf("md5() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMD5WithUDFResultInput(t *testing.T) {
	// Create a UDF result object
	udfResult := map[string]any{
		"PwrqValue": "test string",
		"PwrqType":  "string",
	}

	// Extract _val (simulating what the function does)
	extracted := common.BindValue(udfResult)

	if extracted != "test string" {
		t.Errorf("extractUDFValue() = %v, want %v", extracted, "test string")
	}

	// Verify it hashes correctly
	hash := md5.Sum([]byte(extracted.(string)))
	expected := fmt.Sprintf("%x", md5.Sum([]byte("test string")))

	if fmt.Sprintf("%x", hash) != expected {
		t.Errorf("hashing extracted value = %v, want %v", fmt.Sprintf("%x", hash), expected)
	}
}

func TestMD5Chaining(t *testing.T) {
	// Test that chaining works: base64_encode -> md5
	testCases := []string{
		"hello",
		"test string",
		"",
		"special chars: !@#$%",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			// Simulate: base64_encode returns UDF result
			// (We'll just test that md5 can extract from UDF results)
			udfResult := map[string]any{
				"PwrqValue": tc,
				"PwrqType":  "string",
			}

			// Simulate: md5 receives UDF result and extracts _val
			extracted := common.BindValue(udfResult)
			if extracted != tc {
				t.Fatalf("extraction failed: got %v, want %v", extracted, tc)
			}

			// Hash the extracted value
			hash := md5.Sum([]byte(extracted.(string)))
			expectedHash := md5.Sum([]byte(tc))

			if hash != expectedHash {
				t.Errorf("chaining failed: got %x, want %x", hash, expectedHash)
			}
		})
	}
}

func TestMD5ThroughPipeline(t *testing.T) {
	// Exercise the registered function, not a local re-implementation of it.
	q, err := gojq.Parse(`md5`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	code, err := gojq.Compile(q, RegisterMD5())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	v, _ := code.Run("hello").Next()
	if err, ok := v.(error); ok {
		t.Fatalf("md5: %v", err)
	}
	want := fmt.Sprintf("%x", md5.Sum([]byte("hello")))
	if v != want {
		t.Errorf("md5 = %#v, want %q", v, want)
	}
}
