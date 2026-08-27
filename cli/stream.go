package cli

import (
	"encoding/json"
	"errors"
	"io"
)

type jsonStream struct {
	dec    *json.Decoder
	path   []any
	states []int
}

func newJSONStream(dec *json.Decoder) *jsonStream {
	return &jsonStream{dec: dec, states: []int{jsonStateTopValue}, path: []any{}}
}

// errEndOfJSONInput is the message encoding/json's scanner uses for input that
// stops in the middle of a value. It is only ever produced at the end of the
// input, so matching it cannot swallow a syntax error from somewhere earlier.
const errEndOfJSONInput = "unexpected end of JSON input"

// isEndOfJSONInput reports whether err means the reader ran out mid-value.
//
// Which error that is depends on the toolchain. Through Go 1.26, Decoder.Token
// returned io.EOF. Go 1.27's encoding/json returns a *json.SyntaxError instead,
// but only once Decoder.More has already read past the last token - which is
// exactly what next does to keep the path in step. So `{"a":1,` arrives here as
// io.EOF or as a syntax error depending on the Go the binary was built with,
// and both mean the same thing: the value was truncated.
func isEndOfJSONInput(err error) bool {
	if err == io.EOF {
		return true
	}
	var serr *json.SyntaxError
	return errors.As(err, &serr) && serr.Error() == errEndOfJSONInput
}

const (
	jsonStateTopValue = iota
	jsonStateArrayStart
	jsonStateArrayValue
	jsonStateArrayEnd
	jsonStateArrayEmptyEnd
	jsonStateObjectStart
	jsonStateObjectKey
	jsonStateObjectValue
	jsonStateObjectEnd
	jsonStateObjectEmptyEnd
)

func (s *jsonStream) next() (any, error) {
	switch s.states[len(s.states)-1] {
	case jsonStateArrayEnd, jsonStateObjectEnd:
		s.path = s.path[:len(s.path)-1]
		fallthrough
	case jsonStateArrayEmptyEnd, jsonStateObjectEmptyEnd:
		s.states = s.states[:len(s.states)-1]
	}
	if s.dec.More() {
		switch s.states[len(s.states)-1] {
		case jsonStateArrayValue:
			s.path[len(s.path)-1] = s.path[len(s.path)-1].(int) + 1
		case jsonStateObjectValue:
			s.path = s.path[:len(s.path)-1]
		}
	}
	for {
		token, err := s.dec.Token()
		if err != nil {
			if isEndOfJSONInput(err) && s.states[len(s.states)-1] != jsonStateTopValue {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
		if d, ok := token.(json.Delim); ok {
			switch d {
			case '[', '{':
				switch s.states[len(s.states)-1] {
				case jsonStateArrayStart:
					s.states[len(s.states)-1] = jsonStateArrayValue
				case jsonStateObjectKey:
					s.states[len(s.states)-1] = jsonStateObjectValue
				}
				if d == '[' {
					s.states = append(s.states, jsonStateArrayStart)
					s.path = append(s.path, 0)
				} else {
					s.states = append(s.states, jsonStateObjectStart)
				}
			case ']':
				if s.states[len(s.states)-1] == jsonStateArrayStart {
					s.states[len(s.states)-1] = jsonStateArrayEmptyEnd
					s.path = s.path[:len(s.path)-1]
					return []any{s.copyPath(), []any{}}, nil
				}
				s.states[len(s.states)-1] = jsonStateArrayEnd
				return []any{s.copyPath()}, nil
			case '}':
				if s.states[len(s.states)-1] == jsonStateObjectStart {
					s.states[len(s.states)-1] = jsonStateObjectEmptyEnd
					return []any{s.copyPath(), map[string]any{}}, nil
				}
				s.states[len(s.states)-1] = jsonStateObjectEnd
				return []any{s.copyPath()}, nil
			}
		} else {
			switch s.states[len(s.states)-1] {
			case jsonStateArrayStart:
				s.states[len(s.states)-1] = jsonStateArrayValue
				fallthrough
			case jsonStateArrayValue:
				return []any{s.copyPath(), token}, nil
			case jsonStateObjectStart, jsonStateObjectValue:
				s.states[len(s.states)-1] = jsonStateObjectKey
				s.path = append(s.path, token)
			case jsonStateObjectKey:
				s.states[len(s.states)-1] = jsonStateObjectValue
				return []any{s.copyPath(), token}, nil
			default:
				s.states[len(s.states)-1] = jsonStateTopValue
				return []any{s.copyPath(), token}, nil
			}
		}
	}
}

func (s *jsonStream) copyPath() []any {
	path := make([]any, len(s.path))
	copy(path, s.path)
	return path
}
