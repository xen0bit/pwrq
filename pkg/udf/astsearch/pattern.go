// Package astsearch searches source code by its syntax rather than its text.
package astsearch

import (
	"fmt"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/grep"
)

// A pattern is code with holes in it. `func $NAME($$$ARGS) error { $$$BODY }`
// is a pattern, and it matches every Go function returning an error, however
// it is spaced, wrapped or commented - because the match is against the parse
// tree, not the bytes.
//
// This file is the part that decides whether a pattern means anything, which
// the engine underneath does not.

// errorNode is what tree-sitter calls the node it produces where the grammar
// could not follow the input.
const errorNode = "(ERROR"

// compiled is a pattern that has been turned into a tree-sitter query, along
// with the two facts a caller needs about it: whether it parsed, and what it
// became.
type compiled struct {
	pattern  string
	language string
	query    *grep.CompiledPattern
	// valid is false when the pattern did not parse as code. See validate.
	valid bool
}

// compilePattern turns a pattern into a query against a named language.
func compilePattern(pattern, language string) (*compiled, error) {
	entry := grammars.DetectLanguageByName(language)
	if entry == nil {
		return nil, fmt.Errorf("no grammar for language %q in this build; get_ast_language lists the %d it has",
			language, len(grammars.AllLanguages()))
	}
	query, err := grep.Compile(entry.Language(), pattern)
	if err != nil {
		return nil, fmt.Errorf("cannot compile pattern %q for %s: %v", pattern, entry.Name, err)
	}
	return &compiled{
		pattern:  pattern,
		language: entry.Name,
		query:    query,
		valid:    !strings.Contains(query.SExpr, errorNode),
	}, nil
}

// validate is the check the engine underneath does not make.
//
// A pattern that is not valid code compiles anyway. `func $$$(` becomes
// `(ERROR (ERROR (ERROR) @_lit_1))`, a query that is well-formed, runs against
// every file, and matches nothing - so a typo and an honest absence come back
// identical, and the caller reads "no matches" as an answer about their
// codebase.
//
// Refusing those is the whole reason a pattern goes through this package
// rather than straight to grep.Match. The test is the ERROR node itself:
// across every valid pattern tried against Go, Python and JavaScript none
// produced one, and every malformed pattern did.
//
// What it cannot catch is a pattern that parses as the wrong thing. Python's
// `except $E: $$$B` compiles cleanly, because "except" also reads as an
// identifier, into a query for an assignment - so it is well-formed, wrong,
// and silent. That is what the Query field is for: a caller who expected
// matches and got none can read what their pattern actually became.
func (c *compiled) validate() error {
	if c.valid {
		return nil
	}
	return fmt.Errorf("pattern %q does not parse as %s code, so it can never match; "+
		"write the pattern as code you could compile, with $NAME where a value varies "+
		"and $$$NAME where a list does", c.pattern, c.language)
}

// metaVariables lists the holes in a pattern, in a stable order.
//
// The map is keyed by the placeholder the engine rewrote each hole into -
// $NAME becomes __GREP_CAP_NAME__ so that the pattern still parses as code -
// and it is the Name inside that the caller wrote and that the matches come
// back under. Reading the keys instead reports the machinery.
//
// Anonymous wildcards are left out. $_ says "something goes here and I do not
// care what", so listing it as a name to look up would be answering a question
// the caller declined to ask.
func (c *compiled) metaVariables() []any {
	seen := map[string]bool{}
	names := make([]string, 0, len(c.query.MetaVars))
	for _, meta := range c.query.MetaVars {
		if meta == nil || meta.Wildcard || meta.Name == "" || meta.Name == "_" || seen[meta.Name] {
			continue
		}
		seen[meta.Name] = true
		names = append(names, meta.Name)
	}
	sort.Strings(names)
	out := make([]any, len(names))
	for i, name := range names {
		out[i] = name
	}
	return out
}

// languageFor decides which grammar to parse a file with: the one the caller
// named, or the one the extension implies.
func languageFor(path, requested string) (*grammars.LangEntry, error) {
	if requested != "" {
		entry := grammars.DetectLanguageByName(requested)
		if entry == nil {
			return nil, fmt.Errorf("no grammar for language %q in this build; "+
				"get_ast_language lists the %d it has", requested, len(grammars.AllLanguages()))
		}
		return entry, nil
	}
	return grammars.DetectLanguage(path), nil
}
