// Package astsearch searches source code by its syntax rather than its text.
package astsearch

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/grep"
)

// A pattern is code with holes in it. `func $NAME($$$ARGS) error` is a
// pattern, and it matches every Go function returning an error, however it is
// spaced, wrapped or commented - because the match is against the parse tree,
// not the bytes.
//
// This file is the part that decides whether a pattern means anything, which
// the engine underneath does not.

// errorNode is what tree-sitter calls the node it produces where the grammar
// could not follow the input.
const errorNode = "(ERROR"

// capturePrefix is what the engine rewrites `$NAME` into so a pattern with
// holes in it still parses as code. Finding it in the finished query means the
// rewrite was never undone. See patternProblem.
const capturePrefix = "__GREP_CAP_"

// literalPredicate matches the `(#eq? @_lit_1 "...")` clauses the compiler
// emits for the parts of a pattern that are text rather than holes.
var literalPredicate = regexp.MustCompile(`\(#eq\? @[A-Za-z0-9_]+ "((?:[^"\\]|\\.)*)"\)`)

// metaVarUse matches the metavariable forms a pattern can write, so that a
// name used twice can be counted. It mirrors the engine's own scanner, whose
// results cannot be used for this: the engine keys its table by name, so by
// the time it has finished the second use of a name is indistinguishable from
// the first.
var metaVarUse = regexp.MustCompile(`\$\$\$([A-Za-z_]\w*)|\$(_)\b|\$([A-Za-z_]\w*)`)

// oneWord matches a pattern that is a single bare identifier. For those - and
// only those - a query that is one text comparison is the search the caller
// asked for rather than a symptom of one that failed. See patternProblem.
var oneWord = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// compiled is a pattern that has been turned into a tree-sitter query, along
// with the two facts a caller needs about it: whether it can match code at
// all, and what it became.
type compiled struct {
	pattern  string
	language string
	// queries are the readings of the pattern the grammar allows, most likely
	// first. There is usually one; see statementReading for the language where
	// the same characters mean two things.
	queries []*grep.CompiledPattern
	// problem is empty when the pattern is structural code in this language,
	// and otherwise says what went wrong, in the caller's terms.
	problem string
}

// query is the reading a caller is shown and asked about. A pattern that
// would not compile has none.
func (c *compiled) query() *grep.CompiledPattern {
	if len(c.queries) == 0 {
		return nil
	}
	return c.queries[0]
}

// valid reports whether the pattern can match code at all.
func (c *compiled) valid() bool { return c.problem == "" }

// compilePattern turns a pattern into a query against a named language.
func compilePattern(pattern, language string) (*compiled, error) {
	entry := grammars.DetectLanguageByName(language)
	if entry == nil {
		return nil, fmt.Errorf("no grammar for language %q in this build; get_ast_language lists the %d it has",
			language, len(grammars.AllLanguages()))
	}
	query, err := grep.Compile(entry.Language(), pattern)
	if err != nil {
		// A pattern that will not compile for this language is not an error,
		// for the same reason a pattern that is not code in it is not: a
		// search runs over a tree, and a tree is written in several languages.
		// `$S:string` names a node type JavaScript has and Java does not, so
		// compiling it for Java fails - and a JavaScript rule must not die on
		// the Java beside it. Whether the pattern is code in anything at all
		// is decided once, at the end, by exhausted.
		return &compiled{
			pattern:  pattern,
			language: entry.Name,
			problem: fmt.Sprintf("pattern %q will not compile for %s: %v", pattern,
				entry.Name, err),
		}, nil
	}
	c := &compiled{
		pattern:  pattern,
		language: entry.Name,
		queries:  []*grep.CompiledPattern{query},
		problem:  patternProblem(pattern, entry.Name, query.SExpr),
	}
	if !c.valid() {
		return c, nil
	}
	if name := siblingVariadic(query.SExpr, variadicNames(query.MetaVars)); name != "" {
		c.problem = fmt.Sprintf("pattern %q writes $$$%s beside other children, and there it "+
			"matches one node rather than the rest of them: a capture holds a single node, so "+
			"there is nothing for a run of them to be bound to. Write $$$_ where you mean "+
			"\"and anything else here\" - `f($A, $$$_)` - and name the children you need",
			pattern, name)
		return c, nil
	}
	if anchored, err := anchorPattern(entry.Language(), pattern, query); err == nil {
		c.queries[0] = anchored
	}
	if stmt := statementReading(entry.Language(), pattern, query); stmt != nil {
		c.queries = append([]*grep.CompiledPattern{stmt}, c.queries...)
	}
	return c, nil
}

// statementReading is the pattern read as a statement rather than as whatever
// the grammar makes of it standing alone, or nil when that is the same thing.
//
// In C it is not the same thing, and the difference is total. `gets($BUF)`
// parses as a declaration - `gets` a type, `($BUF)` a declarator - because
// that is what those characters mean at the top of a file. The query compiles,
// reports no problem, and matches nothing in any C ever written. `gets($BUF);`
// parses as the call. Every C pattern in a rule corpus is a call, so every one
// of them silently found nothing.
//
// Adding the terminator changes nothing in Go, Java, Python or JavaScript,
// where a semicolon is optional or a separator, so the second reading is kept
// only where it is genuinely a second reading. A pattern that already parses
// badly is refused before this is reached, which is what stops `$A + $B` in C
// - a fragment C has no context for at all - from being rescued into the
// nonsense the terminator makes of it.
func statementReading(lang *gotreesitter.Language, pattern string, query *grep.CompiledPattern) *grep.CompiledPattern {
	if strings.HasSuffix(strings.TrimSpace(pattern), ";") {
		return nil
	}
	stmt, err := grep.Compile(lang, pattern+";")
	if err != nil || stmt.SExpr == query.SExpr || patternProblem(pattern, "", stmt.SExpr) != "" {
		return nil
	}
	anchored, err := anchorPattern(lang, pattern, stmt)
	if err != nil {
		return nil
	}
	return anchored
}

// variadicNames lists the holes a pattern wrote as $$$NAME, by the name the
// caller gave them. The map is keyed by placeholder, so the names are in the
// values.
func variadicNames(mvars map[string]*grep.MetaVar) map[string]bool {
	names := map[string]bool{}
	for _, mv := range mvars {
		if mv != nil && mv.Variadic && mv.Name != "" && mv.Name != ellipsisName {
			names[mv.Name] = true
		}
	}
	return names
}

// anchorPattern recompiles a query with its sibling lists anchored, so that a
// pattern naming two arguments matches a call that has two. See sexp.go for
// what the engine does without it.
func anchorPattern(lang *gotreesitter.Language, pattern string, query *grep.CompiledPattern) (*grep.CompiledPattern, error) {
	sexp := anchorQuery(query.SExpr, bubbleDepths(lang, pattern, query))
	if sexp == query.SExpr {
		return query, nil
	}
	q, err := gotreesitter.NewQuery(sexp, lang)
	if err != nil {
		return nil, err
	}
	return &grep.CompiledPattern{Query: q, MetaVars: query.MetaVars, Lang: lang, SExpr: sexp}, nil
}

// patternProblem is the check the engine underneath does not make: whether the
// query a pattern compiled to searches for code or for text.
//
// It is the whole reason a pattern goes through this package rather than
// straight to grep.Match. A grammar that cannot follow a pattern does not say
// so. It produces a query that is well-formed, runs against every file and
// matches none of them, so a typo and an honest absence come back identical
// and the caller reads the wrong one.
//
// One idea covers the three ways it happens: a query that still contains the
// pattern's own text did not understand the pattern.
//
//   - An ERROR node, where the grammar gave up. `func $$$(` compiles to
//     `(ERROR (ERROR (ERROR) @_lit_1))`.
//
//   - A leaked placeholder. `$NAME` is rewritten to __GREP_CAP_NAME__ so that
//     the pattern parses as code, then turned back into a capture. When it
//     survives into a text comparison the hole was read as part of a word.
//     PHP does this to every pattern, and reports no error at all: a PHP file
//     is HTML until `<?php`, so `md5($X)` parses as the literal text
//     "md5(__GREP_CAP_X__)".
//
//   - The whole pattern as a single literal. `foo(1)` against markdown or YAML
//     compiles to a query for a paragraph whose text is `foo(1)`. Nothing was
//     parsed; the pattern was quoted. The exception is a pattern that is one
//     bare word, where comparing text is exactly the search intended -
//     `select_ast("."; "md5")` looks for that identifier.
//
// What none of the three can catch is a pattern that parses as the wrong
// construct. Python's `except $E: $$$B` compiles cleanly, because "except"
// also reads as an identifier, into a query for an assignment. That is what
// ast_pattern's Query field is for.
func patternProblem(pattern, language, sexp string) string {
	if strings.Contains(sexp, errorNode) {
		return fmt.Sprintf("pattern %q does not parse as %s code, so it can never match; "+
			"write the pattern as code you could compile, with $NAME where a value varies "+
			"and $$$NAME where a list does", pattern, language)
	}

	if name := repeatedMetaVariable(pattern); name != "" {
		return fmt.Sprintf("pattern %q uses $%s more than once, and the query cannot require "+
			"the two to be the same thing: a match keeps one capture per name, so the pattern "+
			"matches wherever the shape fits whatever the text is - `$X == $X` matches `a == b`. "+
			"Give each hole its own name and compare what they caught afterwards, with "+
			"select(.Captures.A == .Captures.B)", pattern, name)
	}

	literals := literalPredicate.FindAllStringSubmatch(sexp, -1)
	for _, m := range literals {
		if strings.Contains(m[1], capturePrefix) {
			return fmt.Sprintf("pattern %q is not code in %s: the grammar read its $ holes as "+
				"part of the surrounding text, so the query looks for the literal characters of "+
				"the pattern and can only ever match a file that contains them verbatim; "+
				"ast_pattern shows what it compiled to", pattern, language)
		}
	}

	// One literal holding the entire pattern means the grammar took the
	// pattern as a run of text and parsed nothing inside it.
	if len(literals) == 1 && literals[0][1] == strings.TrimSpace(pattern) && !oneWord.MatchString(literals[0][1]) {
		return fmt.Sprintf("pattern %q is not code in %s: the whole pattern compiled to a single "+
			"piece of text, so the query looks for those characters rather than for the construct "+
			"they spell; ast_pattern shows what it compiled to", pattern, language)
	}
	return ""
}

// repeatedMetaVariable returns the first hole a pattern names twice, or "" if
// it names each of them once. Anonymous wildcards are exempt: $_ says "one
// node, I do not care which", and two of them were never a claim that the two
// nodes are equal.
func repeatedMetaVariable(pattern string) string {
	seen := map[string]bool{}
	for _, m := range metaVarUse.FindAllStringSubmatch(pattern, -1) {
		name := m[1]
		if name == "" {
			name = m[3]
		}
		if name == "" || name == "_" {
			continue
		}
		if seen[name] {
			return name
		}
		seen[name] = true
	}
	return ""
}

// validate is the error a caller sees for a pattern that cannot match code.
func (c *compiled) validate() error {
	if c.valid() {
		return nil
	}
	return fmt.Errorf("%s", c.problem)
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
	if c.query() == nil {
		return nil
	}
	names := make([]string, 0, len(c.query().MetaVars))
	for _, meta := range c.query().MetaVars {
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
