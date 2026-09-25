package ideengine

import (
	"sort"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/graph"
	"github.com/xen0bit/pwrq/pkg/udf/discovery"
)

// Command is one callable name, as an editor's help and completion show it.
type Command struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	MinArgs     int      `json:"minArgs"`
	MaxArgs     int      `json:"maxArgs"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
	// Available reports whether this registry can actually evaluate the name.
	// The catalog lists the CLI's full vocabulary, so the cmdlets that need a
	// filesystem, process table or service manager are present but marked
	// unavailable rather than hidden where they cannot run.
	Available bool `json:"available"`
}

// AliasInfo is a short name and what it stands for.
type AliasInfo struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

// ClassStyle is one node class in the diagram legend, in both themes.
type ClassStyle struct {
	Name        string           `json:"name"`
	Label       string           `json:"label"`
	Description string           `json:"description"`
	Dark        graph.ClassStyle `json:"dark"`
	Light       graph.ClassStyle `json:"light"`
}

// CatalogResponse is everything an editor needs to describe its own
// vocabulary: completion, help, syntax highlighting and the diagram legend all
// read it.
type CatalogResponse struct {
	Version  string       `json:"version"`
	Commands []Command    `json:"commands"`
	Aliases  []AliasInfo  `json:"aliases"`
	Cmdlets  []string     `json:"cmdlets"`
	Builtins []string     `json:"builtins"`
	Classes  []ClassStyle `json:"classes"`
	Examples []Example    `json:"examples"`
}

// Catalog reports what can be called here.
//
// It is derived from the registry the engine evaluates against, not from a
// hand-kept list, so an editor can never offer a name it would then fail to
// run.
func (e *Engine) Catalog() CatalogResponse {
	resp := CatalogResponse{
		Version:  e.config.Version,
		Cmdlets:  e.names,
		Builtins: jqBuiltins(),
		Examples: Examples(),
	}

	for _, cmd := range discovery.Catalog() {
		resp.Commands = append(resp.Commands, Command{
			Name:        cmd.Name,
			Aliases:     cmd.Aliases,
			MinArgs:     cmd.MinArgs,
			MaxArgs:     cmd.MaxArgs,
			Category:    cmd.Category,
			Description: cmd.Description,
			Examples:    cmd.Examples,
			Available:   cmd.Available,
		})
	}
	sort.Slice(resp.Commands, func(i, j int) bool { return resp.Commands[i].Name < resp.Commands[j].Name })

	for _, alias := range e.aliases {
		resp.Aliases = append(resp.Aliases, AliasInfo{Name: alias.Name, Target: alias.Target})
	}

	dark, light := graph.PaletteFor("dark"), graph.PaletteFor("light")
	for _, class := range graph.Classes() {
		resp.Classes = append(resp.Classes, ClassStyle{
			Name:        class.Name,
			Label:       class.Label,
			Description: class.Description,
			Dark:        dark[class.Name],
			Light:       light[class.Name],
		})
	}

	return resp
}

// jqBuiltins lists jq's own functions, asked of gojq rather than kept in a
// list that would drift. The names carry no arity: completion offers a name,
// and jq dispatches on arity itself.
func jqBuiltins() []string {
	query, err := gojq.Parse("builtins")
	if err != nil {
		return nil
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil
	}
	iter := code.Run(nil)
	v, ok := iter.Next()
	if !ok {
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	seen := make(map[string]bool, len(list))
	names := make([]string, 0, len(list))
	for _, entry := range list {
		s, ok := entry.(string)
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(s, "/")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
