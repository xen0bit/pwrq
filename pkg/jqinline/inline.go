// Package jqinline expands a query's function definitions at their call
// sites.
//
// It is the third thing a query box wants beside laying a query out and
// putting it on one line: showing what a definition actually does, in place,
// so a pipeline can be read without jumping back to the top. Each call is
// replaced by a copy of the body, so a function used three times appears
// three times.
//
// Inlining is meant to be meaning-preserving, and that takes more than
// pasting text. jq's function arguments are filters, evaluated where they are
// used but with the caller's variables, so
//
//	def f(g): 9 as $x | [g]; 1 as $x | f($x)
//
// is [1]. A body that binds a name the argument also uses is renamed apart
// before the argument is put in, which is what keeps that [1]. And a body
// that reads a name from outside itself - an enclosing `as` binding, or
// another definition - is only expanded where that name still means the same
// thing; where it would not, the definition is left standing and the call
// left alone, with a note saying so.
//
// Three things it does not attempt. A definition that calls itself cannot be
// unfolded into a finite query, so it stays a definition. A definition nothing
// calls simply disappears, since inlining is what would have given it a place
// in the output. And duplication compounds - `def a: .; def b: a | a;` doubles
// per definition - so expansion stops at a size cap and says that it did,
// rather than return more query than anything can open.
package jqinline

import (
	"strings"

	"github.com/itchyny/gojq"
)

// Result is the inlined query and an account of what inlining could not do.
type Result struct {
	// Query is the rewritten query. It is a copy: the one passed in is left
	// alone.
	Query *gojq.Query
	// Expanded counts the call sites replaced by a body.
	Expanded int
	// Kept names each definition that is still a definition and gives the
	// short reason why, for a caller to report. It is empty when everything
	// was expanded.
	Kept []string
}

// Inline expands every function definition it safely can at the places the
// query calls them.
//
// It runs to a fixed point, because expanding one definition can unblock
// another: in `def r: <recursive>; def f: r; def r: 99; [f, r]` the second r
// is what stops f travelling, and it is gone once it has been expanded
// itself. Passes stop as soon as one changes nothing, so calling Inline on
// its own output returns that output.
func Inline(q *gojq.Query) Result {
	if q == nil {
		return Result{}
	}
	out := cloneQuery(q)
	r := Result{Query: out}
	for pass := 0; pass < maxPasses; pass++ {
		o := &inlineOps{
			env:  env{vars: idmap{}, funcs: idmap{}, labels: idmap{}},
			defs: map[int]*def{},
			gen:  &namer{used: mentionedNames(out, map[string]bool{})},
			// Each pass starts its budget from what the query already costs,
			// so the cap bounds the result rather than one pass's share of it.
			grown: len(out.String()),
		}
		walkQuery(out, o)
		r.Expanded += o.expanded
		// The notes describe the query as it now stands, so only the last
		// pass's are worth keeping: an earlier one may have complained about
		// a definition this pass went on to expand.
		r.Kept = o.kept
		if o.expanded == 0 {
			break
		}
	}
	return r
}

const (
	// maxPasses bounds the fixed-point loop. Each pass consumes at least one
	// definition, so a handful is far more than any query needs; the bound is
	// only there so that a case nobody thought of cannot spin.
	maxPasses = 8

	// maxGrowth caps how much text inlining may produce. Copying a body at
	// every call site is duplication by design, and duplication compounds:
	// `def a: .; def b: a | a; def c: b | b;` doubles per definition, so a
	// couple of dozen lines can ask for more query than a browser will hold.
	// Past the cap the remaining calls are left alone and said to be, which
	// beats handing back something nothing can open.
	maxGrowth = 1 << 20
)

// idmap records what a name means, as the identity of the binding it resolves
// to rather than the name itself. That is what lets a call site be compared
// with a definition site: the body may say `$x`, but it is only safe to move
// the body to the call if `$x` is the same `$x` in both places.
type idmap = scoped[int]

type env struct{ vars, funcs, labels idmap }

func (e env) snapshot() env {
	return env{vars: e.vars.clone(), funcs: e.funcs.clone(), labels: e.labels.clone()}
}

// def is one definition the walk has passed, with everything a call site
// needs to decide whether the body can come to it.
type def struct {
	fd    *gojq.FuncDef
	key   string
	env   env   // what names meant where it was defined
	free  names // what its body reads from outside itself
	size  int   // how much text one copy of its body costs
	done  bool  // its body has been walked, so it can be expanded
	rec   bool  // it calls itself, so it never can be
	keep  bool  // it must survive as a definition
	noted bool
}

type inlineOps struct {
	env      env
	defs     map[int]*def
	nextID   int
	gen      *namer
	expanded int
	grown    int
	kept     []string
}

func (o *inlineOps) id() int { o.nextID++; return o.nextID }

// funcDefs registers a node's definitions, walks their bodies, and on the way
// back out drops the ones every call site took a copy of.
func (o *inlineOps) funcDefs(q *gojq.Query, walk func(*gojq.Query)) func() {
	pops := make([]func(), 0, len(q.FuncDefs))
	records := make([]*def, 0, len(q.FuncDefs))
	for _, fd := range q.FuncDefs {
		d := &def{fd: fd, key: funcKey(fd.Name, len(fd.Args))}
		id := o.id()
		o.defs[id] = d
		// In scope inside its own body, and for the definitions after it.
		pops = append(pops, o.env.funcs.set(d.key, id))
		d.env = o.env.snapshot()
		pop := o.pushParams(fd)
		walk(fd.Body)
		pop()
		d.done, d.free = true, freeNames(fd.Body, fd.Args)
		d.size = len(fd.Body.String())
		records = append(records, d)
	}
	pop := popAll(pops)
	return func() {
		pop()
		kept := q.FuncDefs[:0]
		for i, d := range records {
			if d.keep {
				kept = append(kept, q.FuncDefs[i])
			}
		}
		q.FuncDefs = kept
		if len(q.FuncDefs) == 0 {
			q.FuncDefs = nil
		}
	}
}

// pushParams puts a definition's parameters in scope while its body is
// walked. They are bindings with no definition behind them, so a call to one
// inside the body finds a name that cannot be expanded - which is right: what
// it stands for is only known at the call site.
func (o *inlineOps) pushParams(fd *gojq.FuncDef) func() {
	pops := make([]func(), 0, 2*len(fd.Args))
	for _, p := range fd.Args {
		if strings.HasPrefix(p, "$") {
			pops = append(pops, o.env.vars.set(p, o.id()), o.env.funcs.set(funcKey(p[1:], 0), o.id()))
			continue
		}
		pops = append(pops, o.env.funcs.set(funcKey(p, 0), o.id()))
	}
	return popAll(pops)
}

func (o *inlineOps) bindVars(names []string) (map[string]string, func()) {
	pops := make([]func(), 0, len(names))
	for _, n := range names {
		pops = append(pops, o.env.vars.set(n, o.id()))
	}
	return nil, popAll(pops)
}

func (o *inlineOps) bindLabel(name string) (string, func()) {
	return name, o.env.labels.set(name, o.id())
}

func (o *inlineOps) brk(name string) string { return name }

func (o *inlineOps) ref(t *gojq.Term) *gojq.Term {
	name := t.Func.Name
	if strings.HasPrefix(name, "$") {
		return t // a variable, or $__loc__: nothing to expand
	}
	id, ok := o.env.funcs[funcKey(name, len(t.Func.Args))]
	if !ok {
		return t // a builtin or a cmdlet
	}
	d := o.defs[id]
	if d == nil {
		return t // a parameter of the definition being walked
	}
	if !d.done || d.rec {
		d.rec, d.keep = true, true
		o.note(d, "calls itself")
		return t
	}
	if !o.reaches(d) {
		d.keep = true
		o.note(d, "reads a name that means something else where it is called")
		return t
	}
	if o.grown+d.size > maxGrowth {
		d.keep = true
		o.note(d, "would make the query too large to expand any further")
		return t
	}
	// The arguments belong to the call site, so they are inlined here, in
	// this scope, before they travel into the body.
	for _, a := range t.Func.Args {
		walkQuery(a, o)
	}
	for _, s := range t.SuffixList {
		walkSuffix(s, o)
	}
	o.expanded++
	o.grown += d.size
	return o.expand(d, t.Func.Args, t.SuffixList)
}

// reaches reports whether a body can be moved to the current point without
// any of the names it reads from outside itself changing meaning.
func (o *inlineOps) reaches(d *def) bool {
	for n := range d.free.vars {
		if o.env.vars[n] != d.env.vars[n] {
			return false
		}
	}
	for k := range d.free.funcs {
		if o.env.funcs[k] != d.env.funcs[k] {
			return false
		}
	}
	for n := range d.free.labels {
		if o.env.labels[n] != d.env.labels[n] {
			return false
		}
	}
	return true
}

// note records why a definition had to stay. Two definitions can share a name
// and a reason, and saying it twice helps nobody, so the text is deduplicated
// rather than the definition.
func (o *inlineOps) note(d *def, why string) {
	if d.noted {
		return
	}
	d.noted = true
	msg := d.key + " " + why
	for _, seen := range o.kept {
		if seen == msg {
			return
		}
	}
	o.kept = append(o.kept, msg)
}

// expand makes one copy of a body with the call's arguments in it.
func (o *inlineOps) expand(d *def, args []*gojq.Query, suffixes []*gojq.Suffix) *gojq.Term {
	conflict := map[string]bool{}
	for _, a := range args {
		mentionedNames(a, conflict)
	}
	s := &substOps{conflict: conflict, gen: o.gen, vars: strmap{}, labels: strmap{}, funcs: funcmap{}}

	// A value parameter - `def f($a): ...` - is jq's own shorthand for
	// binding the argument first, so it expands to that binding. The filter
	// form of the name stays available inside the body, as it does in jq.
	type valueParam struct {
		name string
		arg  *gojq.Query
	}
	var bindings []valueParam
	for i, p := range d.fd.Args {
		if strings.HasPrefix(p, "$") {
			name := p
			if conflict[p] {
				name = o.gen.fresh(p)
			}
			s.vars[p] = name
			s.funcs[funcKey(p[1:], 0)] = funcRepl{query: args[i]}
			bindings = append(bindings, valueParam{name: name, arg: args[i]})
			continue
		}
		s.funcs[funcKey(p, 0)] = funcRepl{query: args[i]}
	}

	body := cloneQuery(d.fd.Body)
	walkQuery(body, s)
	for i := len(bindings) - 1; i >= 0; i-- {
		body = &gojq.Query{
			Op:       gojq.OpPipe,
			Left:     &gojq.Query{Term: asTerm(cloneQuery(bindings[i].arg))},
			Patterns: []*gojq.Pattern{{Name: bindings[i].name}},
			Right:    body,
		}
	}
	return spliceTerm(body, suffixes)
}
