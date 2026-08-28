package udf

import (
	"strings"
	"testing"
)

// TestDecoderFailuresNameTheirInput pins the exact errors a model hit over
// MCP, each of which said what was wrong with the input while showing none
// of it.
//
// The first cost the most. `json_parse: invalid JSON: invalid character 'h'`
// came back from a plain-text endpoint, and the model went back to re-fetch
// and re-print the response body three separate times to find out what it had
// passed. The body began "hello world". One clause would have ended it.
func TestDecoderFailuresNameTheirInput(t *testing.T) {
	reg := DefaultRegistry()
	options := reg.Options()

	cases := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "json_parse on a plain-text body",
			query: `"hello world" | json_parse`,
			want:  `input was "hello world"`,
		},
		{
			name:  "base64_decode on something that is not base64",
			query: `"roundtrip" | base64_decode`,
			want:  `input was "roundtrip"`,
		},
		{
			name:  "hex_decode on something that is not hex",
			query: `"nothex!!" | hex_decode`,
			want:  `input was "nothex!!"`,
		},
		{
			name:  "base64url_decode on something that is not base64url",
			query: `"not base64url ~~" | base64url_decode`,
			want:  `input was "not base64url ~~"`,
		},
		{
			name:  "zlib_decompress on text that is not compressed",
			query: `"hello world" | zlib_decompress`,
			want:  `input was "hello world"`,
		},
		{
			// The wrong *type*, which is what a caller gets from reading a key
			// that turned out to be a flag rather than a payload. This was the
			// literal shape of one failure: .gzipped is a boolean.
			name:  "a decompressor handed a boolean",
			query: `true | gzip_decompress`,
			want:  "input was true",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runProbe(options, tc.query)
			if err == nil {
				t.Fatalf("%s unexpectedly succeeded", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the error does not show the input:\n    got:  %s\n    want it to contain: %s",
					err, tc.want)
			}
		})
	}
}

// TestDecoderFailuresStayReadable checks the excerpt does not turn a decoder's
// error into the document it rejected.
func TestDecoderFailuresStayReadable(t *testing.T) {
	reg := DefaultRegistry()
	options := reg.Options()

	// A large text body, of the kind an HTTP fetch produces.
	_, err := runProbe(options, `("lorem ipsum dolor sit amet " * 400) | json_parse`)
	if err == nil {
		t.Fatal("the query unexpectedly succeeded")
	}
	if n := len(err.Error()); n > 300 {
		t.Errorf("the error grew to %d bytes:\n    %s", n, err)
	}
	if strings.ContainsAny(err.Error(), "\n\r") {
		t.Errorf("the error spans lines:\n    %s", err)
	}

	// And compressed bytes, which have no readable form at all.
	_, err = runProbe(options, `"hello" | zlib_compress | hex_decode | json_parse`)
	if err == nil {
		t.Fatal("the query unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "not valid text") {
		t.Errorf("raw bytes were not described as bytes:\n    %s", err)
	}
}
