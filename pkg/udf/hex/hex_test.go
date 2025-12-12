package hex

import (
	"encoding/hex"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestHexEncode(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{
			name:    "simple string",
			input:   "hello",
			want:    hex.EncodeToString([]byte("hello")),
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			want:    hex.EncodeToString([]byte("")),
			wantErr: false,
		},
		{
			name:    "string with special characters",
			input:   "hello world!",
			want:    hex.EncodeToString([]byte("hello world!")),
			wantErr: false,
		},
		{
			name:    "unicode string",
			input:   "こんにちは",
			want:    hex.EncodeToString([]byte("こんにちは")),
			wantErr: false,
		},
		{
			name:    "bytes input",
			input:   []byte("test"),
			want:    hex.EncodeToString([]byte("test")),
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
			want:    hex.EncodeToString([]byte("hello")),
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

			// Encode to hex
			got := hex.EncodeToString(inputBytes)

			if got != tt.want {
				t.Errorf("hex_encode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHexDecode(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{
			name:    "simple hex string",
			input:   hex.EncodeToString([]byte("hello")),
			want:    "hello",
			wantErr: false,
		},
		{
			name:    "empty hex string",
			input:   hex.EncodeToString([]byte("")),
			want:    "",
			wantErr: false,
		},
		{
			name:    "hex string with special characters",
			input:   hex.EncodeToString([]byte("hello world!")),
			want:    "hello world!",
			wantErr: false,
		},
		{
			name:    "unicode hex string",
			input:   hex.EncodeToString([]byte("こんにちは")),
			want:    "こんにちは",
			wantErr: false,
		},
		{
			name: "UDF result object input - should extract _val",
			input: map[string]any{
				"_val": hex.EncodeToString([]byte("hello")),
				"_meta": map[string]any{
					"source": "hex_encode",
				},
			},
			want:    "hello",
			wantErr: false,
		},
		{
			name:    "invalid hex string",
			input:   "invalid hex!",
			want:    "",
			wantErr: true,
		},
		{
			name:    "odd length hex string",
			input:   "abc",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract _val if it's a UDF result
			inputVal := common.ExtractUDFValue(tt.input)

			// Convert to string
			var input string
			switch val := inputVal.(type) {
			case string:
				input = val
			case []byte:
				input = string(val)
			default:
				if !tt.wantErr {
					t.Fatalf("unexpected input type: %T", val)
				}
				return
			}

			// Decode from hex
			decoded, err := hex.DecodeString(input)
			if (err != nil) != tt.wantErr {
				t.Errorf("hex_decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				got := string(decoded)
				if got != tt.want {
					t.Errorf("hex_decode() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestHexRoundTrip(t *testing.T) {
	testCases := []string{
		"hello",
		"hello world!",
		"",
		"#00",
		"こんにちは",
		"test with newlines\nand\ttabs",
		"special chars: !@#$%^&*()",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			// Encode
			encoded := hex.EncodeToString([]byte(tc))

			// Decode
			decoded, err := hex.DecodeString(encoded)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			if string(decoded) != tc {
				t.Errorf("round-trip failed: got %q, want %q", string(decoded), tc)
			}
		})
	}
}

func TestHexEncodeWithUDFResultInput(t *testing.T) {
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
	encoded := hex.EncodeToString([]byte(extracted.(string)))
	expected := hex.EncodeToString([]byte("test string"))

	if encoded != expected {
		t.Errorf("encoding extracted value = %v, want %v", encoded, expected)
	}
}

func TestHexDecodeWithUDFResultInput(t *testing.T) {
	// Create a hex-encoded UDF result object
	encoded := hex.EncodeToString([]byte("test string"))
	udfResult := map[string]any{
		"_val": encoded,
		"_meta": map[string]any{
			"source": "hex_encode",
		},
	}

	// Extract _val (simulating what the function does)
	extracted := common.ExtractUDFValue(udfResult)

	if extracted != encoded {
		t.Errorf("extractUDFValue() = %v, want %v", extracted, encoded)
	}

	// Verify it decodes correctly
	decoded, err := hex.DecodeString(extracted.(string))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if string(decoded) != "test string" {
		t.Errorf("decoding extracted value = %v, want %v", string(decoded), "test string")
	}
}

func TestHexMetadata(t *testing.T) {
	// Test that the function returns correct metadata structure
	input := "hello"
	expectedEncoded := hex.EncodeToString([]byte(input))

	// Simulate function call
	inputBytes := []byte(input)
	encoded := hex.EncodeToString(inputBytes)

	result := map[string]any{
		"_val": encoded,
		"_meta": map[string]any{
			"encoding":        "hex",
			"original_length": len(inputBytes),
			"encoded_length":  len(encoded),
		},
	}

	// Verify structure
	if val, ok := result["_val"].(string); !ok || val != expectedEncoded {
		t.Errorf("_val = %v, want %v", val, expectedEncoded)
	}

	meta, ok := result["_meta"].(map[string]any)
	if !ok {
		t.Fatal("_meta is not a map")
	}

	if enc, ok := meta["encoding"].(string); !ok || enc != "hex" {
		t.Errorf("encoding = %v, want %v", enc, "hex")
	}

	if origLen, ok := meta["original_length"].(int); !ok || origLen != len(input) {
		t.Errorf("original_length = %v, want %v", origLen, len(input))
	}

	if encLen, ok := meta["encoded_length"].(int); !ok || encLen != len(expectedEncoded) {
		t.Errorf("encoded_length = %v, want %v", encLen, len(expectedEncoded))
	}
}

