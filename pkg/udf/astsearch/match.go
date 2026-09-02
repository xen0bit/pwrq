package astsearch

import (
	"fmt"
	"strings"
	"sync"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/grep"
)

// Running many queries over one file, having parsed it once.
//
// grep.CompiledPattern.Match parses the source it is given, so matching a file
// against n patterns parsed that file n times - and a pattern has more than
// one reading, so it was really n times the readings. Parsing is almost the
// whole cost of a search: a 60KB Go file parses in 50ms and the query over the
// finished tree runs in under one, so a rule with four patterns spent 95% of
// its time reading the same file eight times.
//
// The tree is the same tree whichever query runs against it, so this parses
// once and executes each query against that. It is what select_ast's
// documentation already promised - "the tree is walked once and each file
// parsed once however many patterns there are" - and now what it does.
//
// The conversion from a tree-sitter match to a grep.Result is grep's, and is
// reproduced here because grep exposes no way to run a compiled pattern
// against a tree somebody else parsed. TestParsedMatchesGrep pins the two
// together: it runs both paths over the same files and fails if they disagree,
// so an upstream change to the conversion is caught here rather than in a
// rule's output.

// parsed is one file, read once, ready for any number of queries.
type parsed struct {
	bound  *gotreesitter.BoundTree
	root   *gotreesitter.Node
	source []byte
}

// parseOnce parses source the way grep does, which is the way it has to be
// parsed for a query compiled by grep to mean the same thing over it.
//
// The one detail that matters is TokenSourceFactory: a few grammars - C and
// C++ among them - lex through a token source rather than the DFA, and a tree
// parsed without it is a different tree. grep finds the entry by scanning the
// registry for the one whose Language matches; the caller here already has it.
func parseOnce(entry *grammars.LangEntry, source []byte) (*parsed, error) {
	lang := entry.Language()
	if lang == nil {
		return nil, fmt.Errorf("no grammar for %s in this build", entry.Name)
	}
	// From a pool rather than a fresh parser: see parserPools.
	pool := parserFor(lang)
	var (
		tree *gotreesitter.Tree
		err  error
	)
	if entry.TokenSourceFactory != nil {
		tree, err = pool.ParseWithTokenSource(source, entry.TokenSourceFactory(source, lang))
	} else {
		tree, err = pool.Parse(source)
	}
	if err != nil {
		return nil, err
	}
	bound := gotreesitter.Bind(tree)
	root := bound.RootNode()
	if root == nil {
		// An empty file is not a failure; it simply matches nothing.
		bound.Release()
		return nil, nil
	}
	return &parsed{bound: bound, root: root, source: source}, nil
}

// parserPools holds one pool per language, for the length of the process.
//
// A parser is not a small object: it builds a dozen lookup tables from the
// grammar and carries the stacks a GLR parse forks, and it costs about ten
// megabytes whatever the size of the file it is about to read. Building one
// per file meant a corpus of rules over a repository built one for every file
// of every rule - a hundred thousand of them for a few hundred files - and
// under a CPU profile the construction alone was 44% of the run.
//
// Pooling the file parses took a fifty-rule search over certbot from 23.6s to
// 16.8s before the tree cache below was there to remove most of the parses
// outright. What is left of that 44% is upstream: compiling a pattern parses
// the pattern text through grep, which builds its own parser and is not ours
// to pool.
//
// gotreesitter provides the pool for exactly this, and resets the mutable
// state - logger, timeouts, cancellation flag, included ranges - on checkout,
// so nothing carries from one parse to the next. A pool is safe to share
// between goroutines, which the workers in SearchTree need.
//
// Keyed by the Language rather than by its name: the registry hands out one
// Language per grammar and hands out the same one every time, and some of the
// callers here have the Language without the entry it came from.
var parserPools sync.Map // *gotreesitter.Language -> *gotreesitter.ParserPool

// parserFor is the pool to parse this language with. Every parse in this
// package goes through it, and a bare NewParser here is a bug.
func parserFor(lang *gotreesitter.Language) *gotreesitter.ParserPool {
	if p, ok := parserPools.Load(lang); ok {
		return p.(*gotreesitter.ParserPool)
	}
	// Two callers racing here build two pools and one is dropped, which is
	// cheaper than holding a lock across the construction.
	actual, _ := parserPools.LoadOrStore(lang, gotreesitter.NewParserPool(lang))
	return actual.(*gotreesitter.ParserPool)
}

// release gives the tree back. A parsed file is used and dropped within one
// call, so this is always a defer at the point of parsing.
func (p *parsed) release() {
	if p != nil && p.bound != nil {
		p.bound.Release()
	}
}

// match runs one compiled pattern over the parsed file.
func (p *parsed) match(cp *grep.CompiledPattern) ([]grep.Result, error) {
	if p == nil {
		return nil, nil
	}
	if cp == nil || cp.Query == nil || cp.Lang == nil {
		return nil, fmt.Errorf("execute: nil compiled pattern or query")
	}
	found := cp.Query.ExecuteNode(p.root, cp.Lang, p.source)
	results := make([]grep.Result, 0, len(found))
	for _, m := range found {
		results = append(results, result(m, p.source))
	}
	return results, nil
}

// result turns one tree-sitter match into the value grep would have returned.
//
// The span is the union of every capture's span, including the literal ones
// the compiler adds for the parts of a pattern that are text - `md5.New()` is
// all literals, and without them the match would have no span at all. Those
// captures are then left out of the result, because they are not holes anybody
// wrote.
//
// grep maps capture names through its metavariable table before reporting
// them. That table maps each name to itself, so the name a caller sees is the
// name in the query either way; the mapping is not reproduced because
// reproducing an identity function only invites a reader to wonder what it
// changes.
func result(m gotreesitter.QueryMatch, source []byte) grep.Result {
	r := grep.Result{StartByte: ^uint32(0), Captures: make(map[string]grep.Capture)}
	for _, qc := range m.Captures {
		node := qc.Node
		if node == nil {
			continue
		}
		start, end := node.StartByte(), node.EndByte()
		if start < r.StartByte {
			r.StartByte = start
		}
		if end > r.EndByte {
			r.EndByte = end
		}
		if strings.HasPrefix(qc.Name, "_lit_") {
			continue
		}
		r.Captures[qc.Name] = grep.Capture{
			Name:      qc.Name,
			Text:      []byte(qc.Text(source)),
			StartByte: start,
			EndByte:   end,
			Node:      node,
		}
	}
	if r.StartByte == ^uint32(0) {
		// Nothing contributed a span, so there is no span.
		r.StartByte, r.EndByte = 0, 0
	}
	return r
}
