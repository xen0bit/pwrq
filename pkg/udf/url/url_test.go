package url

import (
	"net/url"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestURLEncode(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{
			name:    "simple string",
			input:   "hello world",
			want:    url.QueryEscape("hello world"),
			wantErr: false,
		},
		{
			name:    "string with special characters",
			input:   "hello & world = test",
			want:    url.QueryEscape("hello & world = test"),
			wantErr: false,
		},
		{
			name:    "unicode string",
			input:   "こんにちは",
			want:    url.QueryEscape("こんにちは"),
			wantErr: false,
		},
		{
			name: "UDF result object input",
			input: map[string]any{
				"_val": "hello world",
				"_meta": map[string]any{},
			},
			want:    url.QueryEscape("hello world"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputVal := common.ExtractUDFValue(tt.input)

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

			got := url.QueryEscape(input)
			if got != tt.want {
				t.Errorf("url_encode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestURLDecode(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{
			name:    "simple encoded string",
			input:   url.QueryEscape("hello world"),
			want:    "hello world",
			wantErr: false,
		},
		{
			name:    "encoded with special characters",
			input:   url.QueryEscape("hello & world = test"),
			want:    "hello & world = test",
			wantErr: false,
		},
		{
			name:    "unicode encoded",
			input:   url.QueryEscape("こんにちは"),
			want:    "こんにちは",
			wantErr: false,
		},
		{
			name: "UDF result object input",
			input: map[string]any{
				"_val": url.QueryEscape("hello world"),
				"_meta": map[string]any{},
			},
			want:    "hello world",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputVal := common.ExtractUDFValue(tt.input)

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

			decoded, err := url.QueryUnescape(input)
			if (err != nil) != tt.wantErr {
				t.Errorf("url_decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && decoded != tt.want {
				t.Errorf("url_decode() = %v, want %v", decoded, tt.want)
			}
		})
	}
}

func TestURLRoundTrip(t *testing.T) {
	testCases := []string{
		"hello world",
		"test & example = value",
		"",
		"こんにちは",
		"special: !@#$%^&*()",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			encoded := url.QueryEscape(tc)
			decoded, err := url.QueryUnescape(encoded)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			if decoded != tc {
				t.Errorf("round-trip failed: got %q, want %q", decoded, tc)
			}
		})
	}
}

