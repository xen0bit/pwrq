package queryrun

import (
	"encoding/json"
	"io"
	"strings"
)

// decodeInputs reads the request's input as the stream of values the program
// will run over, applying -R and -s.
//
// JSON input decodes as a stream, matching how the CLI reads a file:
// `{"a":1} {"a":2}` is two inputs, not a syntax error. It decodes with
// UseNumber because the CLI does — json.Number is a fmt.Stringer, and without
// it every number in the input is silently turned into a string somewhere
// downstream.
//
// Empty input is a single null, so a generator query like
// `[limit(3; repeat(1))]` still runs with nothing piped in.
func decodeInputs(req *Request) ([]any, error) {
	if req.RawInput {
		return rawInputs(req), nil
	}

	if isBlank(req.Input) {
		return []any{nil}, nil
	}

	dec := json.NewDecoder(strings.NewReader(req.Input))
	dec.UseNumber()

	var inputs []any
	for {
		var v any
		if err := dec.Decode(&v); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		inputs = append(inputs, v)
	}
	if len(inputs) == 0 {
		return []any{nil}, nil
	}
	if req.Slurp {
		return []any{inputs}, nil
	}
	return inputs, nil
}

// rawInputs reads the input as text rather than JSON (jq -R). Slurped, that is
// the input verbatim, trailing newline and all; otherwise it is one value per
// line, and the final newline ends the last line rather than starting an empty
// one.
func rawInputs(req *Request) []any {
	if req.Input == "" {
		return nil
	}
	if req.Slurp {
		return []any{req.Input}
	}

	text := strings.TrimSuffix(req.Input, "\n")
	text = strings.TrimSuffix(text, "\r")
	lines := strings.Split(text, "\n")
	inputs := make([]any, len(lines))
	for i, line := range lines {
		inputs[i] = strings.TrimSuffix(line, "\r")
	}
	return inputs
}

// isBlank reports whether a string holds nothing but whitespace.
func isBlank(s string) bool { return strings.TrimSpace(s) == "" }
