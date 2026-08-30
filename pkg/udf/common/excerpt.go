package common

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// excerptRunes is how much of a rejected input is worth showing. Long enough
// that a caller recognises what they passed - "hello world", "<!DOCTYPE HTML",
// "789cca48" - and short enough that an error stays one line.
const excerptRunes = 48

// Excerpt renders the input a cmdlet rejected, as a clause to append to the
// error that rejected it.
//
// Every runtime failure in one recorded MCP session was a decoder turning down
// its input, and not one of them showed the input:
//
//	json_parse: invalid JSON: invalid character 'h' looking for beginning of value
//	base64_decode: invalid base64 string: illegal base64 data at input byte 8
//	zlib_decompress: failed to create reader: zlib: invalid header
//
// The first of those sent a model back to re-fetch and re-print the response
// body three separate times to find out what it had passed. The body began
// "hello world" - it was a plain-text endpoint - and the whole detour would
// have been one call had the message said so.
//
// The clause leads with "; " so it appends cleanly:
//
//	fmt.Errorf("json_parse: invalid JSON: %v%s", err, common.Excerpt(input))
//
// It returns "" for an input there is nothing useful to say about, so a caller
// can append it unconditionally.
func Excerpt(v any) string {
	described := describeInput(v)
	if described == "" {
		return ""
	}
	return "; input was " + described
}

// InputHint is the clause to append to a decoder's failure: the file-flag
// advice when the input turns out to be a path on disk, and otherwise the
// input itself.
//
// The two never both belong. When FileFlagHint fires, the "input" is a
// filename the caller meant as data, and quoting it a second time under
// "input was" would only restate what the hint just explained.
func InputHint(fn string, input any) string {
	if hint := FileFlagHint(fn, input); hint != "" {
		return hint
	}
	return Excerpt(input)
}

func describeInput(v any) string {
	switch val := BindValue(v).(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(val)
	case string:
		return describeString(val)
	case []any:
		return fmt.Sprintf("an array of %d", len(val))
	case map[string]any:
		return fmt.Sprintf("an object with %d keys", len(val))
	default:
		// Numbers and anything else gojq can hold. %v is right for these and
		// they are short enough to print whole.
		return fmt.Sprintf("%v", val)
	}
}

// describeString renders a string input, which is the case that matters: it is
// what every decoder takes and every decoder rejected.
func describeString(s string) string {
	if s == "" {
		return "the empty string"
	}
	// Bytes that are not text render as replacement characters, which look
	// like the data is corrupt when they are an artefact of the display. Say
	// what it is instead, and how long, since that is what a caller holding
	// compressed or encrypted bytes actually needs.
	if !utf8.ValidString(s) {
		return fmt.Sprintf("%d bytes, not valid text", len(s))
	}

	// One line. A decoder's input is often a whole document, and pasting its
	// newlines into an error breaks the one-error-one-line reading that makes
	// a stream of them scannable.
	flat := strings.Join(strings.Fields(s), " ")
	if flat == "" {
		return fmt.Sprintf("%d characters of whitespace", len([]rune(s)))
	}

	runes := []rune(flat)
	if len(runes) <= excerptRunes {
		if len(runes) == len([]rune(s)) {
			return strconv.Quote(flat)
		}
		// Whitespace was collapsed, so say the length of the real input rather
		// than let the quoted form imply it.
		return fmt.Sprintf("%s (%d characters in all)", strconv.Quote(flat), len([]rune(s)))
	}
	return fmt.Sprintf("%s... (%d characters in all)",
		strconv.Quote(string(runes[:excerptRunes])), len([]rune(s)))
}
