package ideengine

import (
	"strings"

	"github.com/itchyny/gojq"
)

// ValidateRequest asks whether a query parses. Args names the variables the
// query may read, so one that mentions $name compiles as it would in a run;
// only the names are used, and only by an engine that compiles on validate.
type ValidateRequest struct {
	Query string `json:"query"`
	Args  []Arg  `json:"args,omitempty"`
}

// ValidateResponse reports the answer, and where it went wrong when it did.
// The position is what lets the editor underline the offending token instead
// of colouring the whole box red.
type ValidateResponse struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Offset    int    `json:"offset,omitempty"`
	Line      int    `json:"line,omitempty"`
	Column    int    `json:"column,omitempty"`
	Token     string `json:"token,omitempty"`
	Start     int    `json:"start,omitempty"`
	End       int    `json:"end,omitempty"`
	Empty     bool   `json:"empty,omitempty"`
	Formatted string `json:"formatted,omitempty"`
}

// Validate parses a query, and compiles it too when the engine is configured
// to, and reports what it found.
func (e *Engine) Validate(req ValidateRequest) ValidateResponse {
	if strings.TrimSpace(req.Query) == "" {
		return ValidateResponse{Empty: true}
	}

	query, err := gojq.Parse(req.Query)
	if err != nil {
		return parseFailure(req.Query, err)
	}

	// Rendered before compiling, which prepends the aliases' definitions:
	// what comes back is the user's query, not the vocabulary.
	formatted := query.String()
	if e.config.CompileOnValidate {
		if err := e.compile(query, req.Args); err != nil {
			return ValidateResponse{Error: err.Error(), Formatted: formatted}
		}
	}
	return ValidateResponse{OK: true, Formatted: formatted}
}

// compile compiles a parsed query against the engine's vocabulary exactly as
// a run would, so the two cannot disagree about what resolves.
func (e *Engine) compile(query *gojq.Query, args []Arg) error {
	if len(e.aliasDefs) > 0 {
		query.FuncDefs = append(append([]*gojq.FuncDef{}, e.aliasDefs...), query.FuncDefs...)
	}

	options := append([]gojq.CompilerOption{}, e.runner.Options...)
	if names := variableNames(args); len(names) > 0 {
		options = append(options, gojq.WithVariables(names))
	}

	_, err := gojq.Compile(query, options...)
	return err
}

// variableNames renders the caller's named args the way gojq wants them, with
// the leading dollar it may or may not have been given.
func variableNames(args []Arg) []string {
	names := make([]string, 0, len(args))
	for _, arg := range args {
		name := strings.TrimSpace(arg.Name)
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "$") {
			name = "$" + name
		}
		names = append(names, name)
	}
	return names
}

// parseFailure locates a parse error in the source. gojq reports a byte
// offset and the token it choked on; the editor wants a line, a column and a
// span it can highlight.
func parseFailure(src string, err error) ValidateResponse {
	resp := ValidateResponse{Error: err.Error()}

	var perr *gojq.ParseError
	if !asParseError(err, &perr) {
		return resp
	}

	offset := perr.Offset
	if offset > len(src) {
		offset = len(src)
	}
	if offset < 0 {
		offset = 0
	}

	end := offset
	start := offset - len(perr.Token)
	if start < 0 {
		start = 0
	}
	if start == end {
		// An unexpected EOF names no token, so there is nothing to underline
		// at the offset itself. The last character before it is what the
		// reader has to look at - the bracket that was never closed.
		switch {
		case end < len(src):
			end = start + 1
		case start > 0:
			start--
		}
	}

	line, column := lineColumn(src, start)
	resp.Offset = offset
	resp.Token = perr.Token
	resp.Start = start
	resp.End = end
	resp.Line = line
	resp.Column = column
	return resp
}

func asParseError(err error, target **gojq.ParseError) bool {
	for err != nil {
		if perr, ok := err.(*gojq.ParseError); ok {
			*target = perr
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}

// lineColumn converts a byte offset into a 1-based line and column.
func lineColumn(src string, offset int) (int, int) {
	if offset > len(src) {
		offset = len(src)
	}
	line, column := 1, 1
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return line, column
}
