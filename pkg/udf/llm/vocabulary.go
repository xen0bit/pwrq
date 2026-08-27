package llm

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/queryrun"
)

// CommandDoc is one cmdlet as the agent is told about it.
type CommandDoc struct {
	Name        string
	Description string
	MinArgs     int
	MaxArgs     int
	Streaming   bool
	Examples    []string
}

// Vocabulary is the restricted pwrq the agent may write queries against: a
// runner compiled from the allowed cmdlets alone, and their documentation.
type Vocabulary struct {
	Runner   *queryrun.Runner
	Commands []CommandDoc
	// Options are the compiler options the runner was built from, so script
	// blocks inside an agent's query compile against the same restriction.
	Options []gojq.CompilerOption
}

// compilerOptions is what script blocks inside an agent's query compile
// against. It falls back to the runner's own options, which are the same set
// plus whatever the host added.
func (v *Vocabulary) compilerOptions() []gojq.CompilerOption {
	if len(v.Options) > 0 {
		return v.Options
	}
	if v.Runner != nil {
		return v.Runner.Options
	}
	return nil
}

// VocabularyFunc builds a Vocabulary from a list of cmdlet names.
//
// It is a hook rather than a direct call because the registry imports this
// package, so this package cannot import the registry. discovery.SetCatalog
// solves the same problem the same way.
type VocabularyFunc func(allow []string) (*Vocabulary, error)

var (
	vocabularyMu sync.RWMutex
	vocabulary   VocabularyFunc
)

// SetVocabulary supplies the builder. The CLI sets it at startup.
func SetVocabulary(fn VocabularyFunc) {
	vocabularyMu.Lock()
	defer vocabularyMu.Unlock()
	vocabulary = fn
}

func getVocabulary() VocabularyFunc {
	vocabularyMu.RLock()
	defer vocabularyMu.RUnlock()
	return vocabulary
}

// defaultAllow is what an agent may call when the caller does not say.
//
// It is deny-by-default, which is the one place this package departs from
// OpenCode on purpose: OpenCode's base ruleset is "allow everything, ask a
// human" because a human is watching. Nobody is watching a pipeline, so the
// default set reads and never writes — no sh, no rm, no out_file, no http, and
// no cmdlet that starts a process.
//
// The object cmdlets that take a jq script block are absent for a subtler
// reason: a script block is a whole query, so allowing where_object would let
// an agent reach anything from inside `{script: "..."}`. invoke_agent narrows
// script blocks to this same vocabulary while it runs, but leaving them out of
// the default keeps the blast radius small even if that ever regresses.
var defaultAllow = []string{
	// Look around the filesystem
	"find", "cat", "get_childitem", "test_path", "read_archive",
	// Search inside files
	"select_string", "head", "tail", "grep_lines", "wc_lines", "line_count",
	// Paths
	"basename", "dirname", "file_extension", "join_path", "split_path",
	"normalize_path", "relative_path", "stem",
	// Parse what was found
	"json_parse", "jsonl_parse", "yaml_parse", "csv_parse", "tsv_parse",
	"xml_parse", "ini_parse", "logfmt_parse", "properties_parse",
	"json_stringify", "get_path", "has_path", "json_pointer",
	// Identify and fingerprint
	"sha256", "md5", "entropy", "file_type", "is_binary", "is_utf8", "ssdeep",
	// Summarise
	"mean", "median", "summary", "percentile", "value_counts", "count_by",
	"group_by_key", "sum_by", "avg_by", "top_by", "summarize_by",
	// Tidy text
	"strip_ansi", "normalize_whitespace", "word_count", "extract_emails",
	"extract_urls", "extract_ips", "dedupe", "chunks",
	// Read a SQLite database. The query cmdlet opens it read-only and
	// out_sqlite is absent, so this stays a vocabulary that reads and never
	// writes.
	"invoke_sqlite_query", "get_sqlite_table", "get_sqlite_schema",
	// Present, and discover the rest
	"format_table", "get_command", "get_help",
}

// forbiddenPrefixes are the cmdlets an agent may never be given, whatever the
// caller asks for.
//
// An agent that could call invoke_llm could spend the budget in a loop no
// ceiling anticipates, and one that could call invoke_agent could nest until
// something ran out. The ceiling is a backstop; this is the actual answer.
var forbiddenPrefixes = []string{"invoke_llm", "invoke_agent", "get_llm_"}

// resolveAllow checks the requested vocabulary and returns it sorted.
func resolveAllow(op string, requested []string) ([]string, error) {
	allow := requested
	if len(allow) == 0 {
		allow = defaultAllow
	}
	seen := make(map[string]bool, len(allow))
	out := make([]string, 0, len(allow))
	for _, name := range allow {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		for _, prefix := range forbiddenPrefixes {
			if strings.HasPrefix(name, prefix) {
				return nil, fmt.Errorf("%s: %q cannot be in Allow; an agent that can call a model can spend without limit", op, name)
			}
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: Allow is empty; an agent with no cmdlets can only guess", op)
	}
	sort.Strings(out)
	return out, nil
}
