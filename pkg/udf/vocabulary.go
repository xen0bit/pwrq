package udf

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"github.com/xen0bit/pwrq/pkg/udf/llm"
)

// This file builds the restricted pwrq an agent writes queries against.
//
// The restriction is structural rather than a rule checked at call time: the
// runner is compiled from the allowed cmdlets' registrations alone, so calling
// a denied cmdlet is a compile error in the agent's query. There is no list to
// consult, nothing to keep in step, and nothing a model can argue with.
//
// Which cmdlet a compiler option registers is discovered the same way
// Registry.Names does it — by asking gojq what `builtins` reports with that
// option applied — so an option's membership cannot drift from what it
// actually registers.

var (
	optionIndexOnce sync.Once
	optionIndex     []optionEntry
	optionIndexErr  error
)

type optionEntry struct {
	option gojq.CompilerOption
	names  []string
}

// buildOptionIndex maps each registered option to the names it provides.
func buildOptionIndex() ([]optionEntry, error) {
	optionIndexOnce.Do(func() {
		base, err := evalBuiltins()
		if err != nil {
			optionIndexErr = err
			return
		}
		for _, option := range DefaultRegistry().Options() {
			sigs, err := evalBuiltins(option)
			if err != nil {
				optionIndexErr = err
				return
			}
			var names []string
			seen := make(map[string]bool)
			for sig := range sigs {
				if !base[sig] && !seen[sig.Name] {
					seen[sig.Name] = true
					names = append(names, sig.Name)
				}
			}
			if len(names) > 0 {
				optionIndex = append(optionIndex, optionEntry{option: option, names: names})
			}
		}
	})
	return optionIndex, optionIndexErr
}

// VocabularyFor builds the runner and the documentation for a set of cmdlets.
//
// An option is included only when *every* name it registers is allowed. An
// option that registered two cmdlets would otherwise smuggle the second one in
// beside the one that was asked for.
func VocabularyFor(allow []string) (*llm.Vocabulary, error) {
	index, err := buildOptionIndex()
	if err != nil {
		return nil, fmt.Errorf("building the restricted vocabulary: %w", err)
	}

	permitted := make(map[string]bool, len(allow))
	for _, name := range allow {
		permitted[name] = true
	}

	options := make([]gojq.CompilerOption, 0, len(allow))
	granted := make(map[string]bool, len(allow))
	for _, entry := range index {
		ok := true
		for _, name := range entry.names {
			if !permitted[name] {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		options = append(options, entry.option)
		for _, name := range entry.names {
			granted[name] = true
		}
	}

	var missing []string
	for _, name := range allow {
		if !granted[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("no such cmdlet: %s (get_command lists them)", strings.Join(missing, ", "))
	}

	// The agent's queries carry no environment loader and no stderr sink. env
	// would hand it the API keys of the process running it, and a query that
	// wrote to stderr would interleave with whatever the user is reading.
	documented := make(map[string]FunctionMetadata, len(allow))
	for _, meta := range GetFunctionMetadata() {
		documented[meta.Name] = meta
	}
	commands := make([]llm.CommandDoc, 0, len(allow))
	for _, name := range allow {
		meta, ok := documented[name]
		if !ok {
			continue
		}
		streaming, _ := common.IsStreaming(name)
		commands = append(commands, llm.CommandDoc{
			Name:        meta.Name,
			Description: meta.Description,
			MinArgs:     meta.MinArgs,
			MaxArgs:     meta.MaxArgs,
			Streaming:   streaming,
			Examples:    meta.Examples,
		})
	}

	return &llm.Vocabulary{
		Runner:   &queryrun.Runner{Options: options},
		Commands: commands,
		Options:  options,
	}, nil
}
