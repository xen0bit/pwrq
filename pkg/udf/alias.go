package udf

import (
	"fmt"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
)

// Alias is a PowerShell-style short name for a cmdlet.
//
// Aliases are compiled into the query as ordinary jq function definitions, so
// `gci` is the same kind of thing as any `def` a user writes. Resolving them at
// runtime is not an option: gojq binds function names at compile time and never
// consults session state, which is why the previous SessionState.SetAlias calls
// had no effect on anything.
type Alias struct {
	Name   string
	Target string
}

// StandardAliases are the PowerShell aliases pwrq ships with.
//
// Names that collide with a jq builtin are deliberately absent. PowerShell's
// `select`, `sort`, `group` and `measure` would shadow jq's own select/1 and
// sort/0 - and a definition that shadows a builtin does not error, it silently
// changes what existing jq programs mean. TestNoBuiltinShadowing enforces this.
// `?` and `%` are likewise absent: they are jq operators, not identifiers.
var StandardAliases = []Alias{
	// Filesystem
	{"gci", "get_childitem"},
	{"dir", "get_childitem"},
	{"gc", "cat"},
	{"gi", "get_childitem"},
	{"ni", "new_item"},
	{"ri", "rm"},
	{"cpi", "copy_item"},
	{"mi", "move_item"},
	{"rvpa", "resolve_path"},
	{"sc", "set_content"},

	// Location
	{"cd", "set_location"},
	{"sl", "set_location"},
	{"gl", "get_location"},
	{"pushd", "push_location"},
	{"popd", "pop_location"},

	// Variables
	{"gv", "get_variable"},
	{"sv", "set_variable"},
	{"rv", "remove_variable"},

	// Objects and formatting
	{"fl", "format_list"},
	{"ft", "format_table"},

	// Processes and services
	{"gps", "get_process"},
	{"spps", "stop_process"},
	{"saps", "start_process"},
	{"gsv", "get_service"},
	{"sasv", "start_service"},
	{"spsv", "stop_service"},

	// Web
	{"iwr", "invoke_web_request"},
	{"irm", "invoke_rest_method"},

	// Date
	{"gd", "get_date"},
}

// KnownAliases narrows an alias table to the ones this registry can actually
// resolve.
//
// AliasFuncDefs deliberately errors on an alias naming a function that is not
// registered - for the CLI that is a bug worth failing the build over, and
// TestAliasesResolve enforces it. A curated registry is the case where it is
// not a bug: WebRegistry leaves out every filesystem and process cmdlet, so
// `gci` has nothing to name and simply should not exist there.
func (r *Registry) KnownAliases(aliases []Alias) ([]Alias, error) {
	sigs, err := r.Signatures()
	if err != nil {
		return nil, err
	}
	registered := make(map[string]bool, len(sigs))
	for sig := range sigs {
		registered[sig.Name] = true
	}

	known := make([]Alias, 0, len(aliases))
	for _, alias := range aliases {
		if registered[alias.Target] {
			known = append(known, alias)
		}
	}
	return known, nil
}

// AliasFuncDefs compiles the alias table into jq function definitions, one per
// arity the target accepts, so that `gci` and `gci("src"; {Recurse: true})` both
// resolve. The arities come from the registry itself, so an alias cannot fall
// out of step with the cmdlet it names.
func (r *Registry) AliasFuncDefs(aliases []Alias) ([]*gojq.FuncDef, error) {
	sigs, err := r.Signatures()
	if err != nil {
		return nil, err
	}

	arities := make(map[string][]int)
	for sig := range sigs {
		arities[sig.Name] = append(arities[sig.Name], sig.Arity)
	}
	for name := range arities {
		sort.Ints(arities[name])
	}

	var src strings.Builder
	for _, alias := range aliases {
		targetArities, ok := arities[alias.Target]
		if !ok {
			return nil, fmt.Errorf("alias %q names %q, which is not a registered function", alias.Name, alias.Target)
		}
		for _, arity := range targetArities {
			params := make([]string, arity)
			for i := range params {
				params[i] = fmt.Sprintf("a%d", i)
			}
			if arity == 0 {
				fmt.Fprintf(&src, "def %s: %s; ", alias.Name, alias.Target)
				continue
			}
			joined := strings.Join(params, "; ")
			fmt.Fprintf(&src, "def %s(%s): %s(%s); ", alias.Name, joined, alias.Target, joined)
		}
	}
	if src.Len() == 0 {
		return nil, nil
	}

	// Parse the definitions as a real query so they are validated here rather
	// than failing inside whatever the user happened to type.
	query, err := gojq.Parse(src.String() + ".")
	if err != nil {
		return nil, fmt.Errorf("compiling aliases: %w", err)
	}
	return query.FuncDefs, nil
}
