package sha256

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestSHA256(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{
			name:    "simple string",
			input:   "hello",
			want:    fmt.Sprintf("%x", sha256.Sum256([]byte("hello"))),
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			want:    fmt.Sprintf("%x", sha256.Sum256([]byte(""))),
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
			want:    fmt.Sprintf("%x", sha256.Sum256([]byte("hello"))),
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

			// Compute SHA256
			hash := sha256.Sum256(inputBytes)
			got := fmt.Sprintf("%x", hash)

			if got != tt.want {
				t.Errorf("sha256() = %v, want %v", got, tt.want)
			}
		})
	}
}

