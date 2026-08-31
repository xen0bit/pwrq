package astsearch

import (
	"fmt"
	"runtime"
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
	var paths []string
	for {
		path, ok, err := walk.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		paths = append(paths, path)
	}

	it := &matchIter{patterns: patterns, opts: selectAstOpts{include: include}, walk: walk}
	found := make([][]any, len(paths))
	errs := make([]error, len(paths))

	// Files are searched in parallel, which is worth doing here and not in
	// select_ast: this collects everything before it returns anyway, where
	// select_ast promises a lazy walk that stops at the first match a caller
	// asked for. Searching a file is parsing it, so this is most of what a
	// rule spends its time on and none of it is shared between files.
	//
	// The order the results come back in is the order the walk found them,
	// whatever order they finished in, because a report reads file by file.
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < searchWorkers(len(paths)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				found[i], errs[i] = it.searchFile(paths[i])
			}
		}()
	}
	for i := range paths {
		next <- i
	}
	close(next)
	wg.Wait()

	var out []any
	for i := range paths {
		// The first failure in file order, which is the one a sequential
		// search would have stopped at.
		if errs[i] != nil {
			return nil, errs[i]
		}
		out = append(out, found[i]...)
	}
	return out, nil
}

// searchWorkers is how many files to have in flight at once.
//
// Not one per core: a worker holds a parse tree, and a parse tree is tens of
// megabytes for a large file, so the limit is memory rather than arithmetic.
// Eight is where the wall-clock gain flattens on the trees this was measured
// against, and it keeps the peak within a few hundred megabytes.
func searchWorkers(files int) int {
	const most = 8
	n := runtime.GOMAXPROCS(0)
	if n > most {
		n = most
	}
	if n > files {
		n = files
	}
	if n < 1 {
		n = 1
	}
	return n
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
	entry := grammars.DetectLanguageByName(language)
	if entry == nil {
		return false, fmt.Errorf("no grammar for language %q in this build", language)
	}
	texts := sourceReadings(entry, source)
	// Every reading's tree is given back, including the ones a match found
	// before reaching them.
	defer func() {
		for _, r := range texts {
			r.tree.release()
		}
	}()
	for i, r := range texts {
		if r.tree == nil {
			// One parse per reading rather than one per query: the readings
			// differ, the queries asked of each do not. See match.go.
			tree, err := parseOnce(entry, []byte(r.text))
			if err != nil {
				return false, err
			}
			texts[i].tree, r.tree = tree, tree
		}
		for _, q := range queries.([]*grep.CompiledPattern) {
			results, err := r.tree.match(q)
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

// reading is one text to search and, where the check that produced it parsed
// the text the way the search would, the tree that check built.
//
// Deciding whether a fragment is a program in a language means parsing it, and
// the search that follows parses it again. Instantiating a parser is
// milliseconds whatever the fragment's size, so the second parse was most of
// what this cost. It is skipped where the two parses are provably the same
// one: grep lexes a few grammars - C and C++ among them - through a token
// source rather than the DFA, and for those the check stays a check.
type reading struct {
	text string
	tree *parsed
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
func sourceReadings(entry *grammars.LangEntry, source string) []reading {
	plain, clean := readSource(entry, source)
	if clean {
		return []reading{plain}
	}
	for _, sc := range scaffolds {
		if sc.sigil {
			// A sigil rewrites holes, and source has none.
			continue
		}
		wrapped, ok := readSource(entry, sc.before+source+sc.closing+sc.after)
		if ok {
			return []reading{plain, wrapped}
		}
		wrapped.tree.release()
	}
	return []reading{plain}
}

// readSource parses a candidate reading and reports whether the grammar could
// follow the whole of it, keeping the tree where the search can reuse it.
func readSource(entry *grammars.LangEntry, source string) (reading, bool) {
	if entry.TokenSourceFactory != nil {
		// The search will parse this differently, so the tree built here
		// answers the question and is thrown away.
		return reading{text: source}, parses(entry.Language(), source)
	}
	tree, err := parseOnce(entry, []byte(source))
	if err != nil {
		return reading{text: source}, false
	}
	if tree == nil {
		// Nothing to read, which no grammar objects to.
		return reading{text: source}, true
	}
	return reading{text: source, tree: tree}, !tree.root.HasErrorOrMissing()
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
