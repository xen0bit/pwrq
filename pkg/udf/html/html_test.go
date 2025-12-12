package html

import (
	"html"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestHTMLEncode(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{
			name:    "simple string",
			input:   "hello world",
			want:    html.EscapeString("hello world"),
			wantErr: false,
		},
		{
			name:    "string with HTML characters",
			input:   "<div>hello & world</div>",
			want:    html.EscapeString("<div>hello & world</div>"),
			wantErr: false,
		},
		{
			name: "UDF result object input",
			input: map[string]any{
				"_val": "<div>test</div>",
				"_meta": map[string]any{},
			},
			want:    html.EscapeString("<div>test</div>"),
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

			got := html.EscapeString(input)
			if got != tt.want {
				t.Errorf("html_encode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHTMLDecode(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{
			name:    "simple encoded string",
			input:   html.EscapeString("hello world"),
			want:    "hello world",
			wantErr: false,
		},
		{
			name:    "encoded HTML",
			input:   html.EscapeString("<div>hello & world</div>"),
			want:    "<div>hello & world</div>",
			wantErr: false,
		},
		{
			name: "UDF result object input",
			input: map[string]any{
				"_val": html.EscapeString("<div>test</div>"),
				"_meta": map[string]any{},
			},
			want:    "<div>test</div>",
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

			decoded := html.UnescapeString(input)
			if decoded != tt.want {
				t.Errorf("html_decode() = %v, want %v", decoded, tt.want)
			}
		})
	}
}

func TestHTMLRoundTrip(t *testing.T) {
	testCases := []string{
		"hello world",
		"<div>test</div>",
		"hello & world",
		"",
		"special: <>&\"'",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			encoded := html.EscapeString(tc)
			decoded := html.UnescapeString(encoded)

			if decoded != tc {
				t.Errorf("round-trip failed: got %q, want %q", decoded, tc)
			}
		})
	}
}

