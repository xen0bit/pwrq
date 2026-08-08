package udf

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
)

// Signature identifies a function the way jq does: by name and arity. jq
// dispatches on both, so "sort" and "sort_by/1" are different functions and
// only the pair can answer "does this name collide".
type Signature struct {
	Name  string
	Arity int
}

func (s Signature) String() string { return fmt.Sprintf("%s/%d", s.Name, s.Arity) }

// jqBuiltins is the set of functions jq itself provides, discovered by asking
// gojq rather than maintaining a list that would drift with the dependency.
//
// The query must be compiled with no CompilerOptions: `builtins` reports
// builtins *and* registered custom functions, so applying the registry's
// options here would return the union and defeat the check it exists for.
func jqBuiltins() (map[Signature]bool, error) {
	return evalBuiltins()
}

// Signatures reports every function this registry adds to jq, discovered from
// gojq itself. Nothing has to declare its own name, so this cannot drift from
// what is actually registered.
func (r *Registry) Signatures() (map[Signature]bool, error) {
	base, err := evalBuiltins()
	if err != nil {
		return nil, err
	}
	all, err := evalBuiltins(r.Options()...)
	if err != nil {
		return nil, err
	}
	custom := make(map[Signature]bool, len(all)-len(base))
	for sig := range all {
		if !base[sig] {
			custom[sig] = true
		}
	}
	return custom, nil
}

// Names reports the distinct function names this registry adds, sorted.
func (r *Registry) Names() ([]string, error) {
	sigs, err := r.Signatures()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(sigs))
	names := make([]string, 0, len(sigs))
	for sig := range sigs {
		if !seen[sig.Name] {
			seen[sig.Name] = true
			names = append(names, sig.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// evalBuiltins runs jq's own `builtins` under the given options.
func evalBuiltins(options ...gojq.CompilerOption) (map[Signature]bool, error) {
	query, err := gojq.Parse("builtins")
	if err != nil {
		return nil, fmt.Errorf("parsing builtins: %w", err)
	}
	code, err := gojq.Compile(query, options...)
	if err != nil {
		return nil, fmt.Errorf("compiling builtins: %w", err)
	}

	result := make(map[Signature]bool)
	iter := code.Run(nil)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return nil, fmt.Errorf("evaluating builtins: %w", err)
		}
		entries, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("builtins returned %T, want an array", v)
		}
		for _, entry := range entries {
			s, ok := entry.(string)
			if !ok {
				continue
			}
			sig, err := parseSignature(s)
			if err != nil {
				return nil, err
			}
			result[sig] = true
		}
	}
	return result, nil
}

// parseSignature splits jq's "name/arity" notation.
func parseSignature(s string) (Signature, error) {
	i := strings.LastIndex(s, "/")
	if i < 0 {
		return Signature{}, fmt.Errorf("malformed builtin %q, want name/arity", s)
	}
	arity, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return Signature{}, fmt.Errorf("malformed builtin %q: %w", s, err)
	}
	return Signature{Name: s[:i], Arity: arity}, nil
}
