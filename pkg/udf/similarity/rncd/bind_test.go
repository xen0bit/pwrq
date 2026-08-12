package rncd

import (
	"encoding/json"
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

func TestBindBytesAcceptsEveryByteSource(t *testing.T) {
	cases := []struct {
		name  string
		input any
		want  string
	}{
		{"string", "hello", "hello"},
		{"byte slice", []byte("hello"), "hello"},
		{"content property", map[string]any{"Content": "hello"}, "hello"},
		{"bytes property", map[string]any{"Bytes": "hello"}, "hello"},
		{"data property", map[string]any{"Data": "hello"}, "hello"},
		{"named content", map[string]any{"Name": "greeting", "Content": "hello"}, "hello"},
		// A cmdlet's scalar output arrives wrapped, and BindValue unwraps it,
		// so one cmdlet's result feeds this one without a property access.
		{"psobject scalar", psobject.NewPSObject("hello"), "hello"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := bindBytes(tc.input)
			if !ok {
				t.Fatalf("did not bind %#v", tc.input)
			}
			if string(got) != tc.want {
				t.Errorf("bound %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBindBytesPreservesInvalidUTF8 is what makes a binary file usable as a
// value: Go strings are byte strings, so bytes that are not valid UTF-8 have
// to survive the round trip rather than becoming replacement characters.
func TestBindBytesPreservesInvalidUTF8(t *testing.T) {
	raw := string([]byte{0x00, 0xff, 0xfe, 0x7f, 0x80})
	got, ok := bindBytes(raw)
	if !ok {
		t.Fatal("did not bind a binary string")
	}
	if string(got) != raw {
		t.Errorf("bound % x, want % x", got, raw)
	}
}

func TestBindBytesRejectsWhatIsNotBytes(t *testing.T) {
	for _, input := range []any{
		nil,
		42,
		true,
		[]any{1, 2, 3},
		map[string]any{"Name": "no content here"},
		// One level only: a content property holding another object is a
		// mistake to report, not a structure to chase.
		map[string]any{"Content": map[string]any{"Content": "hello"}},
	} {
		if _, ok := bindBytes(input); ok {
			t.Errorf("bound %#v, which is not bytes", input)
		}
	}
}

func TestBindNameReadsEveryLabel(t *testing.T) {
	cases := []struct {
		input any
		want  string
	}{
		{map[string]any{"Name": "a", "Content": "x"}, "a"},
		{map[string]any{"FullName": "/tmp/a", "Content": "x"}, "/tmp/a"},
		{map[string]any{"Path": "/tmp/a", "Content": "x"}, "/tmp/a"},
		// Name wins over the path properties: it is the one a caller writes.
		{map[string]any{"Name": "a", "FullName": "/tmp/a", "Content": "x"}, "a"},
		{"a bare string", ""},
		{map[string]any{"Content": "x"}, ""},
	}
	for _, tc := range cases {
		if got := bindName(tc.input); got != tc.want {
			t.Errorf("bindName(%#v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestDescribeSpeaksJqsVocabulary keeps the rejection message pointing at what
// the caller wrote. A number reaches a UDF as int, float64 or json.Number
// depending on where it came from, and "a json.Number" names none of them.
func TestDescribeSpeaksJqsVocabulary(t *testing.T) {
	cases := []struct {
		input any
		want  string
	}{
		{nil, "null"},
		{42, "a number"},
		{4.2, "a number"},
		{json.Number("42"), "a number"},
		{true, "a boolean"},
		{[]any{1, 2}, "an array"},
		{map[string]any{"Length": 1}, "an object with no Content property"},
	}
	for _, tc := range cases {
		if got := describe(tc.input); got != tc.want {
			t.Errorf("describe(%#v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
