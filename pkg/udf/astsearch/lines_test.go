package astsearch

import (
	"bytes"
	"testing"
)

// Lines replaced counting the newlines before an offset, so what it must do is
// agree with that count everywhere - including at the ends, where the two ways
// of writing this differ.

// counted is the reading Lines replaced: the line is one more than the
// newlines before the offset, the column one more than the bytes since the
// last of them.
func counted(source []byte, offset int) (int, int) {
	if offset > len(source) {
		offset = len(source)
	}
	if offset < 0 {
		offset = 0
	}
	before := source[:offset]
	return bytes.Count(before, []byte{'\n'}) + 1,
		offset - (bytes.LastIndexByte(before, '\n') + 1) + 1
}

func TestLinesAgreesWithCounting(t *testing.T) {
	sources := []string{
		"",
		"one line, no newline",
		"first\nsecond\nthird\n",
		"\n\n\nleading blanks\n",
		"trailing newline then nothing\n",
		"no trailing newline\nat all",
		"unicode: héllo wörld\nsecond ünicode line\n",
	}
	for _, s := range sources {
		source := []byte(s)
		lines := Index(source)
		// Every offset in the file, plus one past the end - which is what a
		// span's exclusive end is when a match runs to the last byte.
		for offset := 0; offset <= len(source)+2; offset++ {
			wantLine, wantColumn := counted(source, offset)
			gotLine, gotColumn := lines.At(offset)
			if gotLine != wantLine || gotColumn != wantColumn {
				t.Fatalf("%q at %d: counting says %d:%d, the index says %d:%d",
					s, offset, wantLine, wantColumn, gotLine, gotColumn)
			}
		}
		// A negative offset is not something a span produces, and answering it
		// with the start of the file is better than panicking.
		if line, column := lines.At(-1); line != 1 || column != 1 {
			t.Errorf("%q at -1: got %d:%d, want 1:1", s, line, column)
		}
	}
}

func TestNilLinesIsTheStartOfTheFile(t *testing.T) {
	var lines *Lines
	if line, column := lines.At(40); line != 1 || column != 1 {
		t.Errorf("got %d:%d, want 1:1", line, column)
	}
}
