package queryrun

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// maxEncodeDepth bounds how deeply a result value may nest before it is
// refused. NormalizeJSON and gojq.Marshal both recurse over the value, and an
// unbounded depth overflows the goroutine stack - a fatal error that no
// recover() can catch, taking the whole process (an MCP server among them)
// down. encoding/json refuses the same situation with a limit of 10000, which
// is also comfortably within both recursion budgets.
const maxEncodeDepth = 10000

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

func (e *encoder) encode(v any) (text string, err error) {
	// jq -r prints a string result as its contents. Anything else is JSON,
	// because there is no other honest rendering of it.
	if e.raw {
		if s, ok := v.(string); ok {
			return s, nil
		}
	}

	// A cmdlet may leak an internal representation into the output stream: a
	// *psobject.PSObject, a typed integer like int32, a time.Duration, a
	// []byte. NormalizeJSON converts those to the value space gojq marshals,
	// and gojq.Marshal panics on anything it does not know, so an
	// unnormalizable value must become a query error rather than crash the
	// process. The recover covers NormalizeJSON too: it walks the same
	// unvetted value and can panic on its own account.
	defer func() {
		if r := recover(); r != nil {
			text, err = "", &encodeError{value: v, detail: r}
		}
	}()
	if err := checkDepth(v, maxEncodeDepth); err != nil {
		return "", err
	}
	encoded, err := gojq.Marshal(psobject.NormalizeJSON(v))
	if err != nil {
		return "", err
	}
	if e.compact || e.indent == "" {
		return string(encoded), nil
	}
	return reindent(string(encoded), e.indent), nil
}

// encodeError reports that a result value had no JSON encoding. It is returned
// rather than letting the panic that gojq.Marshal raises kill the host.
type encodeError struct {
	value  any
	detail any
}

func (e *encodeError) Error() string {
	return fmt.Sprintf("cannot encode value of type %T (%v)", e.value, e.detail)
}

// checkDepth reports an error if any value under v nests deeper than limit.
//
// It walks depth-first with an explicit stack, so the check itself cannot
// overflow the goroutine stack the way the recursive code it protects does. It
// descends exactly the containers NormalizeJSON descends, so the depth it
// measures is the depth NormalizeJSON and gojq.Marshal will actually recurse
// to. Every descent counts as a level, which also means a value that somehow
// contains itself trips the limit instead of looping for ever.
//
// Depth-first matters for the common shapes: a wide, shallow result keeps only
// one branch's children on the stack at a time rather than all of them.
func checkDepth(v any, limit int) error {
	type frame struct {
		vals  []any
		next  int
		depth int
	}
	stack := []frame{{vals: []any{v}, depth: 1}}
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if top.next >= len(top.vals) {
			stack = stack[:len(stack)-1]
			continue
		}
		cur := top.vals[top.next]
		top.next++
		depth := top.depth
		if depth > limit {
			return &encodeError{value: v, detail: fmt.Sprintf("value nests more than %d levels deep", limit)}
		}

		children := childValues(cur)
		if len(children) > 0 {
			stack = append(stack, frame{vals: children, depth: depth + 1})
		}
	}
	return nil
}

// childValues returns the values nested one level inside v, or nil if v is a
// scalar. It mirrors psobject.NormalizeJSON, including its reflection fallback
// for named slice, map and pointer types.
func childValues(v any) []any {
	switch x := v.(type) {
	case nil, bool, string, int, float64, json.Number, *big.Int:
		// The overwhelmingly common case: a scalar gojq already understands.
		// Matched first so a large result is not put through reflection.
		return nil
	case *psobject.PSObject:
		children := make([]any, 0, len(x.Members)+1)
		children = append(children, x.Value)
		for _, m := range x.Members {
			if m.MemberType == psobject.MemberTypeNoteProperty {
				children = append(children, m.Value)
			}
		}
		return children
	case []any:
		return x
	case map[string]any:
		children := make([]any, 0, len(x))
		for _, item := range x {
			children = append(children, item)
		}
		return children
	}

	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil
	}
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return nil
		}
		return []any{rv.Elem().Interface()}
	case reflect.Slice, reflect.Array:
		children := make([]any, rv.Len())
		for i := range children {
			children[i] = rv.Index(i).Interface()
		}
		return children
	case reflect.Map:
		children := make([]any, 0, rv.Len())
		for iter := rv.MapRange(); iter.Next(); {
			children = append(children, iter.Value().Interface())
		}
		return children
	default:
		return nil
	}
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
