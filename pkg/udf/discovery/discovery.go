// Package discovery implements the Get-Command and Get-Help cmdlets.
//
// A cmdlet-oriented tool lives or dies by whether you can find the cmdlet you
// want, and `--udf-list` only answers that from a terminal. These make the same
// information queryable from inside a pipeline.
package discovery

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/shape"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// Command describes one callable name.
type Command struct {
	Name        string
	Aliases     []string
	MinArgs     int
	MaxArgs     int
	Category    string
	Description string
	Examples    []string
	// Available reports whether this command is registered in the registry
	// that published the catalog. A browser tab has no filesystem, process
	// table or service manager, so the commands that need one are documented
	// but marked unavailable rather than hidden.
	Available bool
	// Streaming reports whether the command emits a stream of values rather
	// than a single one. The registry fills this in from what the registration
	// wrappers observed, so it describes the code rather than restating it.
	Streaming bool
	// Shape is what one emitted value looks like, when the command declared
	// it. Read from the same registration the streaming flag comes from, so
	// the two cannot disagree about the same cmdlet.
	Shape *shape.Shape
	// Input describes where the command reads its input from, in the terms a
	// caller writing the call needs. Empty when the command has not said.
	Input string
}

// EmitsDescription explains the command's output cardinality, which is what
// decides whether a caller has to collect the results with [...].
func (c Command) EmitsDescription() string {
	if c.Streaming {
		return "a stream of values, one per result — collect with [...] to get an array"
	}
	return "a single value — already collected, do not wrap in [...]"
}

// catalog is supplied by the registry at startup. It is a hook rather than a
// direct import because the registry imports this package.
var catalog []Command

// SetCatalog publishes the command catalog these cmdlets report on.
func SetCatalog(commands []Command) {
	catalog = commands
}

// Catalog reports the published command catalog. The browser IDE reads it
// directly rather than through get_command, because completion and help need
// it as data rather than as a stream of PSObjects.
func Catalog() []Command {
	return append([]Command(nil), catalog...)
}

func (c Command) toObject() map[string]any {
	aliases := make([]any, len(c.Aliases))
	for i, a := range c.Aliases {
		aliases[i] = a
	}
	examples := make([]any, len(c.Examples))
	for i, e := range c.Examples {
		examples[i] = e
	}
	obj := map[string]any{
		"Name":        c.Name,
		"Aliases":     aliases,
		"MinArgs":     c.MinArgs,
		"MaxArgs":     c.MaxArgs,
		"Category":    c.Category,
		"Description": c.Description,
		"Examples":    examples,
		"Available":   c.Available,
		"Streaming":   c.Streaming,
		"Output":      c.EmitsDescription(),
		"Shape":       c.ShapeDescription(),
		"TypeName":    c.Shape.TypeName(),
		"Input":       c.Input,
	}
	return CommandInfoShape.Build(obj)
}

// ShapeDescription is the one-line summary of what the command emits, or "" for
// a command that emits no object.
func (c Command) ShapeDescription() string {
	return c.Shape.Compact()
}

// RegisterGetCommand registers get_command.
//
//	get_command                 every command
//	get_command("get_*")        wildcard match on name or alias
func RegisterGetCommand() gojq.CompilerOption {
	return common.WithIterFunctionOf("get_command", 0, 2, CommandInfoShape, func(v any, args []any) gojq.Iter {
		pattern := "*"
		if len(args) > 0 {
			if s, ok := common.BindValue(args[0]).(string); ok && s != "" {
				pattern = s
			}
		}

		matches := findCommands(pattern)
		results := make([]any, len(matches))
		for i, c := range matches {
			results[i] = c.toObject()
		}
		return common.SliceIter(results)
	})
}

// RegisterGetHelp registers get_help, which returns the same information as
// get_command but rendered for reading.
func RegisterGetHelp() gojq.CompilerOption {
	return common.WithFunction("get_help", 0, 2, func(v any, args []any) any {
		pattern := "*"
		if len(args) > 0 {
			if s, ok := common.BindValue(args[0]).(string); ok && s != "" {
				pattern = s
			}
		} else if s, ok := common.BindValue(v).(string); ok && s != "" {
			pattern = s
		}

		matches := findCommands(pattern)
		if len(matches) == 0 {
			return common.MakeUDFErrorResult(
				fmt.Errorf("get_help: no command matches %q", pattern), nil)
		}
		return renderHelp(matches)
	})
}

// findCommands returns commands whose name or alias matches the pattern.
func findCommands(pattern string) []Command {
	var matches []Command
	for _, c := range catalog {
		if matchesName(c, pattern) {
			matches = append(matches, c)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })
	return matches
}

func matchesName(c Command, pattern string) bool {
	lower := strings.ToLower(pattern)
	if ok, _ := path.Match(lower, strings.ToLower(c.Name)); ok {
		return true
	}
	for _, alias := range c.Aliases {
		if ok, _ := path.Match(lower, strings.ToLower(alias)); ok {
			return true
		}
	}
	return false
}

func renderHelp(commands []Command) string {
	var b strings.Builder
	for i, c := range commands {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "NAME\n    %s\n", c.Name)
		if c.Description != "" {
			fmt.Fprintf(&b, "\nSYNOPSIS\n    %s\n", c.Description)
		}
		fmt.Fprintf(&b, "\nSYNTAX\n    %s\n", syntax(c))
		if c.Input != "" {
			fmt.Fprintf(&b, "\nINPUT\n    %s\n", c.Input)
		}
		fmt.Fprintf(&b, "\nOUTPUT\n    %s\n", c.EmitsDescription())
		if described := c.Shape.Describe(); described != "" {
			fmt.Fprintf(&b, "    %s\n", strings.ReplaceAll(described, "\n", "\n    "))
		}
		if len(c.Aliases) > 0 {
			fmt.Fprintf(&b, "\nALIASES\n    %s\n", strings.Join(c.Aliases, ", "))
		}
		if len(c.Examples) > 0 {
			b.WriteString("\nEXAMPLES\n")
			for _, e := range c.Examples {
				fmt.Fprintf(&b, "    %s\n", e)
			}
		}
	}
	return b.String()
}

func syntax(c Command) string {
	switch {
	case c.MaxArgs == 0:
		return c.Name
	case c.MinArgs == c.MaxArgs:
		return fmt.Sprintf("%s(%s)", c.Name, args(c.MinArgs))
	default:
		return fmt.Sprintf("%s(%s)  [%d-%d arguments]", c.Name, args(c.MaxArgs), c.MinArgs, c.MaxArgs)
	}
}

func args(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("arg%d", i+1)
	}
	return strings.Join(parts, "; ")
}
