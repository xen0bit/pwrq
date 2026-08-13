package common

import (
	"os"
	"strings"
)

// Cmdlets in the codec and compression families take their input by value, and
// read it from a file only when a second argument says so:
//
//	gzip_decompress($bytes)          the value is the compressed data
//	gzip_decompress($path; true)     the value names a file to read
//
// Passing a path without the flag is the easy mistake, and it fails in the
// worst possible way: the path *string* is treated as the payload, so the
// cmdlet reports that the data is malformed. "gzip: invalid header" on a file
// that is a perfectly good archive reads as data corruption, and sends the
// caller looking for a bug in the bytes rather than at the missing argument.
//
// FileFlagHint recognizes that situation - the input is a string naming a file
// that exists on disk - and returns a suffix explaining the real problem. It
// only ever fires on the failure path, so a caller who genuinely passed a
// payload that happens to look like a filename sees nothing extra unless the
// operation already failed.
func FileFlagHint(fn string, input any) string {
	s, ok := input.(string)
	if !ok {
		return ""
	}
	// A payload is usually large and often contains bytes no path would hold.
	// Anything past a plausible path length, or spanning lines, is data.
	if s == "" || len(s) > 4096 || strings.ContainsAny(s, "\x00\n") {
		return ""
	}
	info, err := os.Stat(s)
	if err != nil || info.IsDir() {
		return ""
	}
	return "; " + s + " is a file on disk — to read it, pass the file flag: " +
		fn + "(\"" + s + "\"; true)"
}
