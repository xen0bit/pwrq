package queryrun

import (
	"strings"

	"github.com/itchyny/gojq"
)

// encoder renders a result value the way the CLI's flags would.
//
// Values are marshalled by gojq itself, so numbers, string escapes and key
// order match what the command line would print; only the whitespace is this
// package's business. encoding/json is not a substitute: it escapes <, > and &
// into \u sequences, and refuses NaN and infinity outright where jq prints
// null.
type encoder struct {
	raw     bool
	compact bool
	indent  string
}

func newEncoder(req *Request) *encoder {
	e := &encoder{raw: req.Raw, compact: req.Compact}
	switch {
	case req.Compact:
	case req.Tab:
		e.indent = "\t"
	case req.Indent > 0:
		e.indent = strings.Repeat(" ", min(req.Indent, 8))
	default:
		e.indent = "  "
	}
	return e
}

func (e *encoder) encode(v any) (string, error) {
	// jq -r prints a string result as its contents. Anything else is JSON,
	// because there is no other honest rendering of it.
	if e.raw {
		if s, ok := v.(string); ok {
			return s, nil
		}
	}

	encoded, err := gojq.Marshal(v)
	if err != nil {
		return "", err
	}
	if e.compact || e.indent == "" {
		return string(encoded), nil
	}
	return reindent(string(encoded), e.indent), nil
}

// reindent lays compact JSON out over multiple lines.
//
// It works on the text rather than on the value because the text is already
// correct: re-encoding from a decoded value would risk changing a number's
// spelling, and jq's contract is that it does not.
func reindent(compact, indent string) string {
	var b strings.Builder
	b.Grow(len(compact) * 2)

	depth := 0
	inString := false
	escaped := false

	newline := func() {
		b.WriteByte('\n')
		for i := 0; i < depth; i++ {
			b.WriteString(indent)
		}
	}

	for i := 0; i < len(compact); i++ {
		c := compact[i]

		if inString {
			b.WriteByte(c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
			b.WriteByte(c)
		case '{', '[':
			// An empty collection stays on one line, as jq prints it.
			if i+1 < len(compact) && (compact[i+1] == '}' || compact[i+1] == ']') {
				b.WriteByte(c)
				b.WriteByte(compact[i+1])
				i++
				continue
			}
			b.WriteByte(c)
			depth++
			newline()
		case '}', ']':
			depth--
			newline()
			b.WriteByte(c)
		case ',':
			b.WriteByte(c)
			newline()
		case ':':
			b.WriteString(": ")
		default:
			b.WriteByte(c)
		}
	}

	return b.String()
}
