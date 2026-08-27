package cli

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// Truncated input has to surface as io.ErrUnexpectedEOF whatever the toolchain
// calls it: the CLI's error formatter keys off that sentinel to underline past
// the end of the line, and Go 1.27 changed Token's answer out from under it.
func TestJSONStreamTruncatedInput(t *testing.T) {
	for _, src := range []string{`{"a":1,`, `{"a":`, `[1,`, `[1`, `{`, `[`, `{"a"`} {
		t.Run(src, func(t *testing.T) {
			s := newJSONStream(newTestDecoder(src))
			var err error
			for err == nil {
				_, err = s.next()
			}
			if err != io.ErrUnexpectedEOF {
				t.Errorf("got %v (%T), want io.ErrUnexpectedEOF", err, err)
			}
		})
	}
}

// A malformed value is not a truncated one, so it must keep its own error
// rather than being folded into the end-of-input case.
func TestJSONStreamSyntaxErrorIsNotTruncation(t *testing.T) {
	for _, src := range []string{`{"a":1,]`, `[1,}`, `{,}`} {
		t.Run(src, func(t *testing.T) {
			s := newJSONStream(newTestDecoder(src))
			var err error
			for err == nil {
				_, err = s.next()
			}
			if err == io.ErrUnexpectedEOF {
				t.Errorf("%s: reported as truncation, want a syntax error", src)
			}
			var serr *json.SyntaxError
			if !errors.As(err, &serr) {
				t.Errorf("got %v (%T), want *json.SyntaxError", err, err)
			}
		})
	}
}

// Complete input still terminates on io.EOF, not on the truncation sentinel.
func TestJSONStreamCompleteInput(t *testing.T) {
	s := newJSONStream(newTestDecoder(`{"a":1}`))
	var err error
	for err == nil {
		_, err = s.next()
	}
	if err != io.EOF {
		t.Errorf("got %v (%T), want io.EOF", err, err)
	}
}

func newTestDecoder(src string) *json.Decoder {
	dec := json.NewDecoder(strings.NewReader(src))
	dec.UseNumber()
	return dec
}
