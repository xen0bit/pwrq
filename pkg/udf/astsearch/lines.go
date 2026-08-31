package astsearch

import (
	"bytes"
	"sort"
)

// Lines turns byte offsets in one file into the line and column a person would
// open the file at.
//
// The obvious way to do that is to count the newlines before the offset, and
// that is what this replaced. It is fine for one offset and quadratic for a
// file's worth: a match reports its own span and a span for every hole in the
// pattern, so a file with a thousand matches in it scanned its own bytes
// several thousand times, from the top, once per number reported.
//
// The starts are found once per file instead, and each offset is a binary
// search over them. Building the index is one pass the file already pays for
// by being read.
type Lines struct {
	source []byte
	// starts holds the offset of the first byte of each line. starts[0] is 0,
	// so the length is the number of lines and a search for an offset lands on
	// its line's index.
	starts []int
}

// Index reads the line starts of a file.
func Index(source []byte) *Lines {
	starts := make([]int, 1, bytes.Count(source, newline)+1)
	for i := 0; ; {
		next := bytes.IndexByte(source[i:], '\n')
		if next < 0 {
			break
		}
		i += next + 1
		starts = append(starts, i)
	}
	return &Lines{source: source, starts: starts}
}

var newline = []byte{'\n'}

// At is the one-based line and column of a byte offset.
//
// The column counts bytes rather than runes, which is what an editor's
// :line:col wants and what every other pwrq path-and-position pair reports. An
// offset past the end of the file is the end of the file, which is what a
// span's exclusive end is when a match runs to the last byte.
func (l *Lines) At(offset int) (line, column int) {
	if l == nil {
		return 1, 1
	}
	if offset > len(l.source) {
		offset = len(l.source)
	}
	if offset < 0 {
		offset = 0
	}
	// The line is the last one starting at or before the offset.
	i := sort.Search(len(l.starts), func(i int) bool { return l.starts[i] > offset })
	return i, offset - l.starts[i-1] + 1
}
