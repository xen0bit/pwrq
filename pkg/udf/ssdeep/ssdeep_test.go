package ssdeep

import (
	"strings"
	"testing"

	"github.com/glaslos/ssdeep"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestSSDeepHash(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "long string",
			input:   "This is a longer string that should produce a valid ssdeep hash. It needs to be at least 4096 bytes for ssdeep to work properly, so let's make it longer. " + strings.Repeat("A", 4000),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := ssdeep.FuzzyBytes([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ssdeep.HashBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && hash == "" {
				t.Error("ssdeep.HashBytes() returned empty hash")
			}
		})
	}
}

func TestSSDeepCompare(t *testing.T) {
	tests := []struct {
		name    string
		input1  string
		input2  string
		wantErr bool
	}{
		{
			name:    "identical strings",
			input1:  strings.Repeat("This is a test string. ", 200),
			input2:  strings.Repeat("This is a test string. ", 200),
			wantErr: false,
		},
		{
			name:    "similar strings",
			input1:  strings.Repeat("This is a test string. ", 200),
			input2:  strings.Repeat("This is a test stringX ", 200),
			wantErr: false,
		},
		{
			name:    "different strings",
			input1:  strings.Repeat("This is a test string. ", 200),
			input2:  strings.Repeat("Completely different content here. ", 200),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1, err1 := ssdeep.FuzzyBytes([]byte(tt.input1))
			if err1 != nil {
				t.Fatalf("failed to hash input1: %v", err1)
			}

			hash2, err2 := ssdeep.FuzzyBytes([]byte(tt.input2))
			if err2 != nil {
				t.Fatalf("failed to hash input2: %v", err2)
			}

			score, err := ssdeep.Distance(hash1, hash2)
			if (err != nil) != tt.wantErr {
				t.Errorf("ssdeep.Distance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if score < 0 || score > 100 {
					t.Errorf("ssdeep.Distance() score = %v, expected between 0 and 100", score)
				}
			}
		})
	}
}

func TestSSDeepWithUDFResult(t *testing.T) {
	// SSDeep requires at least 4096 bytes
	longString := strings.Repeat("This is a test string for ssdeep. ", 200)
	udfResult := map[string]any{
		"_val": longString,
		"_meta": map[string]any{},
	}

	inputVal := common.ExtractUDFValue(udfResult)
	inputBytes := []byte(inputVal.(string))

	hash, err := ssdeep.FuzzyBytes(inputBytes)
	if err != nil {
		t.Fatalf("failed to hash: %v", err)
	}

	if hash == "" {
		t.Error("hash should not be empty")
	}
}

func TestSSDeepCompareIdentical(t *testing.T) {
	// SSDeep requires at least 4096 bytes
	input := strings.Repeat("test string for ssdeep comparison. ", 200)
	
	hash1, err1 := ssdeep.FuzzyBytes([]byte(input))
	if err1 != nil {
		t.Fatalf("failed to hash: %v", err1)
	}

	hash2, err2 := ssdeep.FuzzyBytes([]byte(input))
	if err2 != nil {
		t.Fatalf("failed to hash: %v", err2)
	}

	score, err := ssdeep.Distance(hash1, hash2)
	if err != nil {
		t.Fatalf("failed to compare: %v", err)
	}

	// Identical inputs should have a high similarity score
	if score < 90 {
		t.Errorf("identical inputs should have high similarity, got score %v", score)
	}
}

