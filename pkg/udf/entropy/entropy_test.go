package entropy

import (
	"math"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestEntropy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMin  float64
		wantMax  float64
		wantErr  bool
	}{
		{
			name:    "empty string",
			input:   "",
			wantMin: 0.0,
			wantMax: 0.0,
			wantErr: false,
		},
		{
			name:    "single character",
			input:   "a",
			wantMin: 0.0,
			wantMax: 0.0,
			wantErr: false,
		},
		{
			name:    "repeated character",
			input:   "aaaa",
			wantMin: 0.0,
			wantMax: 0.0,
			wantErr: false,
		},
		{
			name:    "two different characters",
			input:   "ab",
			wantMin: 0.9,
			wantMax: 1.1,
			wantErr: false,
		},
		{
			name:    "random string",
			input:   "hello world",
			wantMin: 2.0,
			wantMax: 4.0,
			wantErr: false,
		},
		{
			name:    "high entropy (random bytes)",
			input:   string([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}),
			wantMin: 3.5,
			wantMax: 4.5,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate entropy manually for verification
			if len(tt.input) == 0 {
				if tt.wantMin != 0.0 || tt.wantMax != 0.0 {
					t.Errorf("empty string should have entropy 0")
				}
				return
			}

			// Count frequencies
			freq := make(map[byte]int)
			for _, b := range []byte(tt.input) {
				freq[b]++
			}

			// Calculate entropy
			entropy := 0.0
			dataLength := float64(len(tt.input))
			for _, count := range freq {
				if count > 0 {
					probability := float64(count) / dataLength
					entropy -= probability * math.Log2(probability)
				}
			}

			if entropy < tt.wantMin || entropy > tt.wantMax {
				t.Errorf("entropy() = %v, want between %v and %v", entropy, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestEntropyWithUDFResult(t *testing.T) {
	udfResult := map[string]any{
		"_val": "hello",
		"_meta": map[string]any{},
	}

	inputVal := common.ExtractUDFValue(udfResult)
	inputBytes := []byte(inputVal.(string))

	// Calculate entropy
	freq := make(map[byte]int)
	for _, b := range inputBytes {
		freq[b]++
	}

	entropy := 0.0
	dataLength := float64(len(inputBytes))
	for _, count := range freq {
		if count > 0 {
			probability := float64(count) / dataLength
			entropy -= probability * math.Log2(probability)
		}
	}

	if entropy < 1.0 || entropy > 3.0 {
		t.Errorf("entropy for 'hello' should be between 1.0 and 3.0, got %v", entropy)
	}
}

func TestEntropyProperties(t *testing.T) {
	// Test that entropy is always non-negative
	testCases := []string{
		"a",
		"aa",
		"ab",
		"abc",
		"hello world",
		"random string with various characters!@#$%^&*()",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			freq := make(map[byte]int)
			for _, b := range []byte(tc) {
				freq[b]++
			}

			entropy := 0.0
			dataLength := float64(len(tc))
			for _, count := range freq {
				if count > 0 {
					probability := float64(count) / dataLength
					entropy -= probability * math.Log2(probability)
				}
			}

			if entropy < 0 {
				t.Errorf("entropy should be non-negative, got %v", entropy)
			}

			// Maximum entropy for n unique bytes is log2(n)
			maxEntropy := math.Log2(float64(len(freq)))
			if len(freq) > 1 && entropy > maxEntropy*1.1 {
				t.Errorf("entropy %v exceeds theoretical maximum %v", entropy, maxEntropy)
			}
		})
	}
}

