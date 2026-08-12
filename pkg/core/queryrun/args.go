package queryrun

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Arg binds one named variable. Value is JSON text, so any value can be bound
// — which is what makes a query reusable across cases: the program is the
// same, the arguments carry the case.
type Arg struct {
	Name  string
	Value string
}

// bindArgs turns named JSON arguments into the variable names and values gojq
// wants. Names may be written with or without the dollar.
func bindArgs(args []Arg) ([]string, []any, error) {
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
