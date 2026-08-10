package sniff

import (
	"fmt"
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query string) any {
	return runInput(t, query, nil)
}

func runInput(t *testing.T, query string, input any) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	iter := code.Run(input)
	v, ok := iter.Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

func TestFileType(t *testing.T) {
	if got := fmt.Sprint(run(t, `"hello world" | file_type`)); got != "text" {
		t.Errorf("text = %s", got)
	}
	if got := fmt.Sprint(run(t, `"" | file_type`)); got != "empty" {
		t.Errorf("empty = %s", got)
	}
	if got := fmt.Sprint(run(t, `"<root>hi</root>" | file_type`)); got != "xml" {
		t.Errorf("xml = %s", got)
	}

	// Magic-number cases need raw bytes, which a jq string cannot carry for
	// non-ASCII values.
	byteCases := map[string][]byte{
		"png":  {0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A},
		"gzip": {0x1F, 0x8B, 0x08, 0x00},
		"elf":  {0x7F, 'E', 'L', 'F'},
		"pe":   {'M', 'Z', 0x90, 0x00},
		"wasm": {0x00, 'a', 's', 'm'},
	}
	for want, data := range byteCases {
		if got := fmt.Sprint(runInput(t, `file_type`, data)); got != want {
			t.Errorf("file_type(% x) = %s, want %s", data, got, want)
		}
	}
}

func TestIsBinary(t *testing.T) {
	if got := run(t, `"plain text" | is_binary`); got != false {
		t.Error("text is_binary = true")
	}
	if got := runInput(t, `is_binary`, []byte{0x00, 0x01, 0x02}); got != true {
		t.Error("nul bytes is_binary = false")
	}
}

func TestIsUTF8(t *testing.T) {
	if got := run(t, `"héllo" | is_utf8`); got != true {
		t.Error("héllo is_utf8 = false")
	}
	if got := runInput(t, `is_utf8`, []byte{0xFF, 0xFE, 0xFD}); got != false {
		t.Error("invalid utf8 is_utf8 = true")
	}
}
