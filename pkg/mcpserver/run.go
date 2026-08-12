package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// Limits the server runs under. An MCP client has no Ctrl-C: a query like
// `repeat(1)` is an infinite stream, and without a bound the server would be
// stuck for ever. The defaults are generous for a local tool, and a call can
// relax them up to the caps.
const (
	defaultMaxResults = 1000
	maxMaxResults     = 100000
	defaultTimeout    = 30 * time.Second
	maxTimeout        = 10 * time.Minute
	// maxOutputBytes stops a query whose *values* are enormous even though
	// there are few of them - `[range(1e7)]` is one result.
	maxOutputBytes = 64 << 20
)

// execute runs a query to completion under the engine's vocabulary. It is
// called with the engine mutex held, and installs a fresh, private session
// state for the duration of the run: cmdlets like set_variable and set_location
// work within a single call and never leak into the next.
func (e *engine) execute(req runQueryArgs) runQueryResult {
	var resp runQueryResult
	resp.Values = []string{}
	started := time.Now()
	defer func() { resp.ElapsedMs = float64(time.Since(started).Microseconds()) / 1000 }()

	if strings.TrimSpace(req.Query) == "" {
		resp.Error = "query is empty"
		resp.Kind = "parse"
		return resp
	}

	query, err := gojq.Parse(req.Query)
	if err != nil {
		resp.Error = err.Error()
		resp.Kind = "parse"
		return resp
	}
	if len(e.aliasDefs) > 0 {
		query.FuncDefs = append(append([]*gojq.FuncDef{}, e.aliasDefs...), query.FuncDefs...)
	}

	names, values, err := bindArgs(req.Args)
	if err != nil {
		resp.Error = err.Error()
		resp.Kind = "args"
		return resp
	}

	inputs, err := decodeInputs(req)
	if err != nil {
		resp.Error = fmt.Sprintf("input is not JSON: %v", err)
		resp.Kind = "input"
		return resp
	}

	options := append([]gojq.CompilerOption{}, e.options...)
	options = append(options, gojq.WithEnvironLoader(os.Environ))
	if len(names) > 0 {
		options = append(options, gojq.WithVariables(names))
	}
	// `input` and `inputs` read whatever the program was not handed, which is
	// how jq behaves and what makes -n useful on a multi-value input.
	remaining := &sliceIter{}
	options = append(options, gojq.WithInputIter(remaining))
	// debug and stderr are registered here exactly as the CLI registers them;
	// gojq has no such builtins. stderr is the only safe sink: it never
	// corrupts the JSON-RPC channel the client is listening on.
	options = append(options,
		gojq.WithFunction("debug", 0, 0, func(v any, _ []any) any {
			if b, err := gojq.Marshal(v); err == nil {
				_, _ = os.Stderr.Write(append(b, '\n'))
			}
			return v
		}),
		gojq.WithFunction("stderr", 0, 0, func(v any, _ []any) any {
			if b, err := gojq.Marshal(v); err == nil {
				_, _ = os.Stderr.Write(append(b, '\n'))
			}
			return v
		}),
	)

	code, err := gojq.Compile(query, options...)
	if err != nil {
		resp.Error = err.Error()
		resp.Kind = "compile"
		return resp
	}

	limit := req.Limit
	if limit <= 0 {
		limit = defaultMaxResults
	}
	if limit > maxMaxResults {
		limit = maxMaxResults
	}

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if timeout > maxTimeout {
		timeout = maxTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// A private session per run: the cmdlets that touch variables, aliases and
	// drives see one another within this query but nothing after it.
	common.SetGlobalSessionState(sessionstate.NewSessionState())
	defer common.SetGlobalSessionState(nil)

	program := inputs
	if req.NullInput {
		// The program runs once, on null; `inputs` still sees the input.
		program = []any{nil}
		remaining.values = decodedOrEmpty(req)
	}

	bytesOut := 0
	for i, input := range program {
		// Whatever this run has not consumed yet is what `input` returns.
		if !req.NullInput {
			remaining.values = program[i+1:]
			remaining.pos = 0
		}

		iter := code.RunWithContext(ctx, input, values...)
		for {
			v, ok := iter.Next()
			if !ok {
				break
			}

			if err, isErr := v.(error); isErr {
				// `halt` and `halt_error` end the run deliberately, so what
				// they carry is the message, not a failure of the server.
				if halt, ok := err.(*gojq.HaltError); ok {
					resp.Kind = "halt"
					resp.Error = haltMessage(halt)
					return resp
				}
				if ctx.Err() == context.DeadlineExceeded {
					resp.Kind = "timeout"
					resp.Error = fmt.Sprintf("stopped after %s; the query is still running", timeout)
					return resp
				}
				resp.Kind = "runtime"
				resp.Error = err.Error()
				return resp
			}

			text, err := encodeValue(v, req)
			if err != nil {
				resp.Kind = "runtime"
				resp.Error = err.Error()
				return resp
			}
			resp.Values = append(resp.Values, text)
			resp.Count++
			bytesOut += len(text)

			if resp.Count >= limit {
				resp.Truncated = true
				resp.Error = fmt.Sprintf("stopped after %d results; the query may produce an unbounded stream", limit)
				resp.Kind = "limit"
				return resp
			}
			if bytesOut >= maxOutputBytes {
				resp.Truncated = true
				resp.Error = fmt.Sprintf("stopped after %d MB of output", maxOutputBytes>>20)
				resp.Kind = "limit"
				return resp
			}
		}
	}

	return resp
}

// bindArgs turns named JSON arguments into the variable names and values gojq
// wants. Names may be written with or without the dollar.
func bindArgs(args []namedArg) ([]string, []any, error) {
	if len(args) == 0 {
		return nil, nil, nil
	}
	names := make([]string, 0, len(args))
	values := make([]any, 0, len(args))
	seen := make(map[string]bool, len(args))

	for _, arg := range args {
		name := strings.TrimSpace(arg.Name)
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "$") {
			name = "$" + name
		}
		if !validVariableName(name) {
			return nil, nil, fmt.Errorf("%q is not a valid variable name", arg.Name)
		}
		if seen[name] {
			return nil, nil, fmt.Errorf("%s is bound twice", name)
		}
		seen[name] = true

		text := strings.TrimSpace(arg.Value)
		if text == "" {
			text = "null"
		}
		dec := json.NewDecoder(strings.NewReader(text))
		dec.UseNumber()
		var value any
		if err := dec.Decode(&value); err != nil {
			return nil, nil, fmt.Errorf("%s is not JSON: %w", name, err)
		}
		names = append(names, name)
		values = append(values, value)
	}
	return names, values, nil
}

func validVariableName(name string) bool {
	if len(name) < 2 || name[0] != '$' {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9' && i > 1:
		default:
			return false
		}
	}
	return true
}

// decodeInputs reads the input as a stream of values, matching how the CLI
// reads a file: `{"a":1} {"a":2}` is two inputs, not a syntax error. JSON input
// decodes with UseNumber so big numbers survive. Empty input is a single null,
// so a generator query like `[limit(3; repeat(1))]` still runs.
func decodeInputs(req runQueryArgs) ([]any, error) {
	if req.RawInput {
		if req.Input == "" {
			return nil, nil
		}
		if req.Slurp {
			return []any{req.Input}, nil
		}
		text := req.Input
		text = strings.TrimSuffix(text, "\n")
		text = strings.TrimSuffix(text, "\r")
		raw := strings.Split(text, "\n")
		inputs := make([]any, len(raw))
		for i, line := range raw {
			inputs[i] = strings.TrimSuffix(line, "\r")
		}
		return inputs, nil
	}

	if strings.TrimSpace(req.Input) == "" {
		return []any{nil}, nil
	}

	dec := json.NewDecoder(bytes.NewReader([]byte(req.Input)))
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
	return inputs, nil
}

// decodedOrEmpty is decodeInputs for the -n case, where a malformed input has
// already been reported and must not stop the program from running.
func decodedOrEmpty(req runQueryArgs) []any {
	inputs, err := decodeInputs(req)
	if err != nil {
		return nil
	}
	if strings.TrimSpace(req.Input) == "" {
		return nil
	}
	return inputs
}

// encodeValue renders one result according to the request's flags: strings come
// out bare under Raw, everything is single-line under Compact, and indentation
// is honored otherwise.
func encodeValue(v any, req runQueryArgs) (string, error) {
	if req.Raw {
		if s, ok := v.(string); ok {
			return s, nil
		}
	}
	var (
		b   []byte
		err error
	)
	if req.Compact {
		b, err = json.Marshal(v)
	} else {
		indent := req.Indent
		if indent <= 0 {
			indent = 2
		}
		b, err = json.MarshalIndent(v, "", strings.Repeat(" ", indent))
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// sliceIter feeds `input` and `inputs` from a slice.
type sliceIter struct {
	values []any
	pos    int
}

func (s *sliceIter) Next() (any, bool) {
	if s.pos >= len(s.values) {
		return nil, false
	}
	v := s.values[s.pos]
	s.pos++
	return v, true
}

// haltMessage renders what a halt carried, mirroring how the CLI writes it to
// stderr: a string goes out as text, anything else as JSON, and a bare `halt`
// carries nothing at all.
func haltMessage(halt *gojq.HaltError) string {
	v := halt.Value()
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	encoded, err := gojq.Marshal(v)
	if err != nil {
		return halt.Error()
	}
	return string(encoded)
}
