package astsearch

import (
	"sync"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/grep"
	"github.com/xen0bit/pwrq/pkg/core/filewalk"
)

// SearchTree is select_ast without the stream: every match of every pattern
// under root, collected, as the same objects select_ast emits.
//
// It exists for the rule vocabulary, which combines whole result sets rather
// than reading them one at a time - "this call, in a file that also imports
// that" is a question about all the matches at once - and which asks about
// several languages in one search, so most patterns being code in none of the
// files met is ordinary here rather than the mistake it is when a person typed
// one pattern. That is the one difference: the diagnostic select_ast ends its
// stream with is left to select_ast. See exhausted.
func SearchTree(root string, patterns []string, include string) ([]any, error) {
	walk, err := filewalk.New(root, include)
	if err != nil {
		return nil, err
	}
	it := &matchIter{patterns: patterns, opts: selectAstOpts{include: include}, walk: walk}
	var out []any
	for {
		path, ok, err := walk.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			return out, nil
		}
		found, err := it.searchFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
}

// readings caches the queries a pattern compiles to for one language, because
// a rule asks the same question of every match it has.
var readings sync.Map // language + "\x00" + pattern -> []*grep.CompiledPattern

// MatchesText reports whether a pattern matches a piece of source in a
// language.
//
// This is what a rule means by constraining a hole to a pattern of its own -
// "the argument, but only when it is a call to getenv" - and it is a different
// question from the one select_ast answers, because the thing being searched
// is a fragment a match already found rather than a file.
func MatchesText(pattern, language, source string) (bool, error) {
	key := language + "\x00" + pattern
	queries, cached := readings.Load(key)
	if !cached {
		c, err := compilePattern(pattern, language)
		if err != nil {
			return false, err
		}
		if err := c.validate(); err != nil {
			return false, err
		}
		queries = c.queries
		readings.Store(key, queries)
	}
	for _, text := range sourceReadings(language, source) {
		for _, q := range queries.([]*grep.CompiledPattern) {
			results, err := q.Match([]byte(text))
			if err != nil {
				return false, err
			}
			if len(results) > 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

// sourceReadings is the text to search: what the caller gave, and - when that
// is not a program in this language - the same thing inside the smallest
// wrapper that makes it one.
//
// A hole catches a fragment. `$X` in `md5($X)` catches `request.args["q"]`,
// which is an expression and not a Java file, and a rule constraining that
// hole to a pattern of its own is asking about the expression. Parsed on its
// own it is an ERROR node and matches nothing, so every such constraint
// silently kept nothing - the same silent-empty-result failure a pattern that
// is not code produces, on the other side of the search.
//
// The wrappers are the ones a pattern gets, for the same reason and by the
// same measurement: see scaffolds.
func sourceReadings(language, source string) []string {
	entry := grammars.DetectLanguageByName(language)
	if entry == nil || parses(entry.Language(), source) {
		return []string{source}
	}
	for _, sc := range scaffolds {
		if sc.sigil {
			// A sigil rewrites holes, and source has none.
			continue
		}
		wrapped := sc.before + source + sc.closing + sc.after
		if parses(entry.Language(), wrapped) {
			return []string{source, wrapped}
		}
	}
	return []string{source}
}

// parses reports whether a grammar could read the whole of a piece of source.
func parses(lang *gotreesitter.Language, source string) bool {
	tree, err := gotreesitter.NewParser(lang).Parse([]byte(source))
	if err != nil || tree == nil {
		return false
	}
	bound := gotreesitter.Bind(tree)
	defer bound.Release()
	root := bound.RootNode()
	return root != nil && !root.HasErrorOrMissing()
}
