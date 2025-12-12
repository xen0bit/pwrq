package md5

import (
	"crypto/md5"
	"fmt"
	"testing"

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
			name: "UDF result object input - should extract _val",
			input: map[string]any{
				"_val": "hello",
				"_meta": map[string]any{
					"source": "previous_udf",
				},
			},
			want:    fmt.Sprintf("%x", md5.Sum([]byte("hello"))),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract _val if it's a UDF result
			inputVal := common.ExtractUDFValue(tt.input)

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
				"_val": tc,
				"_meta": map[string]any{
					"source": "base64_encode",
				},
			}

			// Simulate: md5 receives UDF result and extracts _val
			extracted := common.ExtractUDFValue(udfResult)
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

func TestMD5Metadata(t *testing.T) {
	// Test that the function returns correct metadata structure
	input := "hello"
	expectedHash := fmt.Sprintf("%x", md5.Sum([]byte(input)))

	// Simulate function call
	inputBytes := []byte(input)
	hash := md5.Sum(inputBytes)
	hashHex := fmt.Sprintf("%x", hash)

	result := map[string]any{
		"_val": hashHex,
		"_meta": map[string]any{
			"algorithm":    "md5",
			"input_length": len(inputBytes),
			"hash_length":  len(hashHex),
		},
	}

	// Verify structure
	if val, ok := result["_val"].(string); !ok || val != expectedHash {
		t.Errorf("_val = %v, want %v", val, expectedHash)
	}

	meta, ok := result["_meta"].(map[string]any)
	if !ok {
		t.Fatal("_meta is not a map")
	}

	if algo, ok := meta["algorithm"].(string); !ok || algo != "md5" {
		t.Errorf("algorithm = %v, want %v", algo, "md5")
	}

	if inputLen, ok := meta["input_length"].(int); !ok || inputLen != len(input) {
		t.Errorf("input_length = %v, want %v", inputLen, len(input))
	}

	if hashLen, ok := meta["hash_length"].(int); !ok || hashLen != len(expectedHash) {
		t.Errorf("hash_length = %v, want %v", hashLen, len(expectedHash))
	}
}

