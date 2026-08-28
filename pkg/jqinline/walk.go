package jqinline

import (
	"strings"

	"github.com/itchyny/gojq"
)

// This file knows the grammar and nothing else. Three passes need to walk a
// query while keeping track of what each construct binds - the inliner, the
// renamer that keeps an expansion hygienic, and the scan that asks which
// names a definition's body leaves free - and they differ only in what they
// do at the points where a name is bound or used. So the walk lives here once
// and takes those points as an interface.
//
// The walk rewrites in place. A pass that only observes returns everything it
// is given unchanged.

type ops interface {
	// funcDefs is called at a query node that carries definitions, before
	// anything else at that node. The implementation registers them, walks
	// each body itself with walk - the scope a body sees is the pass's own
	// business, since jq puts a definition in scope inside its own body but
	// not inside its predecessors' - and returns the function that takes them
	// back out of scope.
	funcDefs(q *gojq.Query, walk func(*gojq.Query)) func()

	// bindVars is called with the variable names a pattern binds, before the
	// subtree they cover is walked. The returned map renames them; the walk
	// applies it to the pattern itself.
	bindVars(names []string) (map[string]string, func())

	// bindLabel is called for `label $x | body`, and returns the name the
	// label should carry.
	bindLabel(name string) (string, func())

	// ref is called for every function term: a call, a variable reference or
	// $__loc__. Returning a term other than the one passed in replaces it,
	// and the walk then leaves that subtree alone - the implementation has
	// taken responsibility for the arguments and suffixes.
	ref(t *gojq.Term) *gojq.Term

	// brk is called for `break $x`, and returns the label name to use.
	brk(name string) string
}

func walkQuery(q *gojq.Query, o ops) {
	if q == nil {
		return
	}
	if len(q.FuncDefs) > 0 {
		pop := o.funcDefs(q, func(b *gojq.Query) { walkQuery(b, o) })
		defer pop()
	}
	if q.Term != nil {
		q.Term = walkTerm(q.Term, o)
		return
	}
	if q.Right == nil {
		return
	}
	// The left side of a pipe is evaluated before the bindings the pipe
	// carries exist, so it is walked in the scope outside them.
	walkQuery(q.Left, o)
	if len(q.Patterns) == 0 {
		walkQuery(q.Right, o)
		return
	}
	var names []string
	for _, p := range q.Patterns {
		names = patternNames(p, names)
	}
	rename, pop := o.bindVars(names)
	defer pop()
	for _, p := range q.Patterns {
		renamePattern(p, rename)
		walkPatternQueries(p, o)
	}
	walkQuery(q.Right, o)
}

func walkTerm(t *gojq.Term, o ops) *gojq.Term {
	if t == nil {
		return nil
	}
	switch t.Type {
	case gojq.TermTypeFunc:
		if r := o.ref(t); r != t {
			return r
		}
		for _, a := range t.Func.Args {
			walkQuery(a, o)
		}
	case gojq.TermTypeIndex:
		walkIndex(t.Index, o)
	case gojq.TermTypeObject:
		for _, kv := range t.Object.KeyVals {
			walkString(kv.KeyString, o)
			walkQuery(kv.KeyQuery, o)
			walkQuery(kv.Val, o)
		}
	case gojq.TermTypeArray:
		if t.Array != nil {
			walkQuery(t.Array.Query, o)
		}
	case gojq.TermTypeUnary:
		t.Unary.Term = walkTerm(t.Unary.Term, o)
	case gojq.TermTypeFormat, gojq.TermTypeString:
		walkString(t.Str, o)
	case gojq.TermTypeIf:
		walkQuery(t.If.Cond, o)
		walkQuery(t.If.Then, o)
		for _, el := range t.If.Elif {
			walkQuery(el.Cond, o)
			walkQuery(el.Then, o)
		}
		walkQuery(t.If.Else, o)
	case gojq.TermTypeTry:
		walkQuery(t.Try.Body, o)
		walkQuery(t.Try.Catch, o)
	case gojq.TermTypeReduce:
		walkLoop(o, t.Reduce.Query, t.Reduce.Pattern, t.Reduce.Start, t.Reduce.Update, nil)
	case gojq.TermTypeForeach:
		walkLoop(o, t.Foreach.Query, t.Foreach.Pattern, t.Foreach.Start, t.Foreach.Update, t.Foreach.Extract)
	case gojq.TermTypeLabel:
		name, pop := o.bindLabel(t.Label.Ident)
		t.Label.Ident = name
		walkQuery(t.Label.Body, o)
		pop()
	case gojq.TermTypeBreak:
		t.Break = o.brk(t.Break)
	case gojq.TermTypeQuery:
		walkQuery(t.Query, o)
	}
	for _, s := range t.SuffixList {
		walkSuffix(s, o)
	}
	return t
}

// walkLoop covers reduce and foreach, which bind the same way: the source and
// the initial value are outside the binding, the update and the extractor
// inside it.
func walkLoop(o ops, source *gojq.Query, pat *gojq.Pattern, start, update, extract *gojq.Query) {
	walkQuery(source, o)
	walkQuery(start, o)
	rename, pop := o.bindVars(patternNames(pat, nil))
	defer pop()
	renamePattern(pat, rename)
	walkPatternQueries(pat, o)
	walkQuery(update, o)
	walkQuery(extract, o)
}

func walkSuffix(s *gojq.Suffix, o ops) {
	if s != nil {
		walkIndex(s.Index, o)
	}
}

func walkIndex(i *gojq.Index, o ops) {
	if i == nil {
		return
	}
	walkString(i.Str, o)
	walkQuery(i.Start, o)
	walkQuery(i.End, o)
}

func walkString(s *gojq.String, o ops) {
	if s == nil {
		return
	}
	for _, q := range s.Queries {
		walkQuery(q, o)
	}
}

// walkPatternQueries walks the computed keys inside a pattern. They are
// ordinary queries, and can call anything.
func walkPatternQueries(p *gojq.Pattern, o ops) {
	if p == nil {
		return
	}
	for _, e := range p.Array {
		walkPatternQueries(e, o)
	}
	for _, e := range p.Object {
		walkString(e.KeyString, o)
		walkQuery(e.KeyQuery, o)
		walkPatternQueries(e.Val, o)
	}
}

// patternNames collects every variable a pattern binds, including the ones
// the `{$a}` shorthand binds through a key.
func patternNames(p *gojq.Pattern, out []string) []string {
	if p == nil {
		return out
	}
	if p.Name != "" {
		out = append(out, p.Name)
	}
	for _, e := range p.Array {
		out = patternNames(e, out)
	}
	for _, e := range p.Object {
		if strings.HasPrefix(e.Key, "$") {
			out = append(out, e.Key)
		}
		out = patternNames(e.Val, out)
	}
	return out
}

// renamePattern applies a renaming to the variables a pattern binds. The
// `{$a}` shorthand names the key and binds the variable in one token, so
// renaming the variable there has to spell the key out: `{a: $a_1}`.
func renamePattern(p *gojq.Pattern, rename map[string]string) {
	if p == nil || len(rename) == 0 {
		return
	}
	if n, ok := rename[p.Name]; ok && p.Name != "" {
		p.Name = n
	}
	for _, e := range p.Array {
		renamePattern(e, rename)
	}
	for _, e := range p.Object {
		if strings.HasPrefix(e.Key, "$") {
			if n, ok := rename[e.Key]; ok {
				e.Key, e.Val = e.Key[1:], &gojq.Pattern{Name: n}
				continue
			}
		}
		renamePattern(e.Val, rename)
	}
}
