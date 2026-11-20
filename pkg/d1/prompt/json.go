// Package prompt is d1's prompt and readout, ported from the checkpoint's
// own prompt.py, attention.py, pointer.py, api.py, box.py, grid.py and
// calibration.py. It builds the exact rows the model was trained on - token
// ids, the question each token belongs to, and the positions answers are
// read at - and turns the decoder's logits and hidden states back into typed
// answers. It knows nothing of ONNX Runtime, so all of it is checked in CI
// against fixtures the Python code wrote.
//
// Python indexes strings by code point, and the reference's offsets, spans
// and slot arithmetic are all in those units; this package keeps them in the
// same units ([]rune indices) so the port can be read against the original
// line by line.
package prompt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// Member is one key of an Object.
type Member struct {
	Key   string
	Value any
}

// Object is a JSON object that keeps its keys' order. A state from a pwrq
// pipeline is a map, rendered with its keys sorted as jq prints them; an
// Object is how the tests hand over the reference's states, whose keys
// Python kept in insertion order.
type Object []Member

// DecodeOrdered decodes JSON keeping object key order (objects become
// Object) and numbers as json.Number.
func DecodeOrdered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data after JSON value")
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := Object{}
			for dec.More() {
				k, err := dec.Token()
				if err != nil {
					return nil, err
				}
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				obj = append(obj, Member{k.(string), v})
			}
			_, err := dec.Token()
			return obj, err
		case '[':
			arr := []any{}
			for dec.More() {
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			_, err := dec.Token()
			return arr, err
		}
	}
	return tok, nil
}

// members returns an object's members in rendering order: an Object's own,
// a map's sorted by key. ok is false for anything that is not an object.
func members(v any) (Object, bool) {
	switch o := v.(type) {
	case Object:
		return o, true
	case map[string]any:
		keys := make([]string, 0, len(o))
		for k := range o {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(Object, len(keys))
		for i, k := range keys {
			out[i] = Member{k, o[k]}
		}
		return out, true
	}
	return nil, false
}

// lookup returns an object's value for key.
func lookup(v any, key string) (any, bool) {
	if o, ok := members(v); ok {
		for _, m := range o {
			if m.Key == key {
				return m.Value, true
			}
		}
	}
	return nil, false
}

// PythonJSON renders v as Python's json.dumps(v, ensure_ascii=False,
// indent=indent) does (indent < 0: no indentation, Python's default
// separators). d1 was trained on states written this way, so the bytes
// matter: Python writes 2.0 for a float two, leaves "<" and "&" alone, and
// spells floats as repr does.
func PythonJSON(v any, indent int) (string, error) {
	var b strings.Builder
	if err := writeJSON(&b, v, indent, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

func writeJSON(b *strings.Builder, v any, indent, depth int) error {
	newline := func(d int) {
		if indent >= 0 {
			b.WriteByte('\n')
			b.WriteString(strings.Repeat(" ", indent*d))
		}
	}
	itemSep, keySep := ", ", ": "
	if indent >= 0 {
		itemSep = ","
	}
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		writeString(b, x)
	case json.Number:
		s, err := pythonNumber(x)
		if err != nil {
			return err
		}
		b.WriteString(s)
	case int:
		b.WriteString(strconv.Itoa(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case *big.Int:
		b.WriteString(x.String())
	case float64:
		s, err := PythonFloat(x)
		if err != nil {
			return err
		}
		b.WriteString(s)
	case []any:
		if len(x) == 0 {
			b.WriteString("[]")
			return nil
		}
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteString(itemSep)
			}
			newline(depth + 1)
			if err := writeJSON(b, e, indent, depth+1); err != nil {
				return err
			}
		}
		newline(depth)
		b.WriteByte(']')
	default:
		o, ok := members(v)
		if !ok {
			return fmt.Errorf("cannot render %T as JSON", v)
		}
		if len(o) == 0 {
			b.WriteString("{}")
			return nil
		}
		b.WriteByte('{')
		for i, m := range o {
			if i > 0 {
				b.WriteString(itemSep)
			}
			newline(depth + 1)
			writeString(b, m.Key)
			b.WriteString(keySep)
			if err := writeJSON(b, m.Value, indent, depth+1); err != nil {
				return err
			}
		}
		newline(depth)
		b.WriteByte('}')
	}
	return nil
}

// writeString escapes as Python's json module does with ensure_ascii=False:
// quotes, backslashes and control characters only.
func writeString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}

// pythonNumber writes a JSON number literal as Python would after reading
// it: an integer as itself, anything else as the float repr of its value.
func pythonNumber(n json.Number) (string, error) {
	s := string(n)
	if !strings.ContainsAny(s, ".eE") {
		if _, ok := new(big.Int).SetString(s, 10); ok {
			return s, nil
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return "", err
	}
	return PythonFloat(f)
}

// PythonFloat is Python's repr of a float: the shortest digits that read
// back, in fixed notation while the decimal point sits within 16 places,
// else scientific with a two-digit exponent; a whole float keeps ".0".
func PythonFloat(f float64) (string, error) {
	switch {
	case math.IsNaN(f):
		return "NaN", nil
	case math.IsInf(f, 1):
		return "Infinity", nil
	case math.IsInf(f, -1):
		return "-Infinity", nil
	}
	if f == 0 {
		if math.Signbit(f) {
			return "-0.0", nil
		}
		return "0.0", nil
	}
	sci := strconv.FormatFloat(f, 'e', -1, 64) // e.g. -1.2345e+17
	neg := strings.HasPrefix(sci, "-")
	sci = strings.TrimPrefix(sci, "-")
	mant, expPart, _ := strings.Cut(sci, "e")
	exp, _ := strconv.Atoi(expPart)
	digits := strings.Replace(mant, ".", "", 1)
	decpt := exp + 1
	var out string
	if -4 < decpt && decpt <= 16 {
		switch {
		case decpt <= 0:
			out = "0." + strings.Repeat("0", -decpt) + digits
		case decpt >= len(digits):
			out = digits + strings.Repeat("0", decpt-len(digits)) + ".0"
		default:
			out = digits[:decpt] + "." + digits[decpt:]
		}
	} else {
		m := digits[:1]
		if len(digits) > 1 {
			m += "." + digits[1:]
		}
		sign := "+"
		if exp < 0 {
			sign, exp = "-", -exp
		}
		out = fmt.Sprintf("%se%s%02d", m, sign, exp)
	}
	if neg {
		out = "-" + out
	}
	return out, nil
}

// pyG is Python's format(v, "g"): six significant digits, trailing zeros
// dropped, scientific outside 1e-4..1e6.
func pyG(v float64) string {
	if v == 0 {
		if math.Signbit(v) {
			return "-0"
		}
		return "0"
	}
	exp := int(math.Floor(math.Log10(math.Abs(v))))
	// Rounding to six digits can carry into the next power (999999.5).
	if r, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'e', 5, 64), 64); r != 0 {
		exp = int(math.Floor(math.Log10(math.Abs(r))))
	}
	if exp < -4 || exp >= 6 {
		s := strconv.FormatFloat(v, 'e', 5, 64)
		mant, e, _ := strings.Cut(s, "e")
		if strings.Contains(mant, ".") {
			mant = strings.TrimRight(strings.TrimRight(mant, "0"), ".")
		}
		sign := e[0]
		n, _ := strconv.Atoi(e[1:])
		return fmt.Sprintf("%se%c%02d", mant, sign, n)
	}
	s := strconv.FormatFloat(v, 'f', max(0, 5-exp), 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}
