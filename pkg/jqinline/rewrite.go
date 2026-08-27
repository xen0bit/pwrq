package jqinline

import (
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
)

// This is the pass that makes one expansion of a body: it puts the call's
// arguments where the parameters were, and renames anything the body binds
// that would otherwise swallow a name the arguments carry.
//
// The renaming is what keeps inlining honest. jq's arguments are filters
// evaluated at the point of use but with the caller's variables, so
//
//	def f(g): 9 as $x | [g]; 1 as $x | f($x)
//
// is [1], not [9]. Substituting `$x` into a body that binds `$x` would say
// [9], so the body's binding is renamed out of the way first.

// funcRepl is what a function name is rewritten to: either a query that
// replaces the call - a parameter standing for its argument - or a new name,
// when a definition inside the body had to be renamed.
type funcRepl struct {
	query *gojq.Query
	name  string
}

type (
	strmap  = scoped[string]
	funcmap = scoped[funcRepl]
)

// namer hands out names that appear nowhere in the query being inlined, so a
// renamed binding can never collide with one that was already there.
type namer struct{ used map[string]bool }

func (n *namer) fresh(base string) string {
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s_%d", base, i)
		if !n.used[cand] {
			n.used[cand] = true
			return cand
		}
	}
}

// substOps rewrites one cloned body. conflict holds the names the arguments
// mention; a binding in the body that spells one of them is renamed, and any
// other binding simply shadows whatever substitution was in flight, exactly
// as it did before the body moved.
type substOps struct {
	conflict map[string]bool
	gen      *namer
	vars     strmap
	labels   strmap
	funcs    funcmap
}

func (o *substOps) funcDefs(q *gojq.Query, walk func(*gojq.Query)) func() {
	pops := make([]func(), 0, len(q.FuncDefs))
	for _, fd := range q.FuncDefs {
		key := funcKey(fd.Name, len(fd.Args))
		if o.conflict[fd.Name] {
			fd.Name = o.gen.fresh(fd.Name)
			pops = append(pops, o.funcs.set(key, funcRepl{name: fd.Name}))
		} else {
			pops = append(pops, o.funcs.unset(key))
		}
		pop := o.pushParams(fd)
		walk(fd.Body)
		pop()
	}
	return popAll(pops)
}

// pushParams handles a definition nested inside the body being expanded. Its
// parameters shadow the outer substitution, and are themselves renamed when
// they spell a name the arguments carry.
func (o *substOps) pushParams(fd *gojq.FuncDef) func() {
	pops := make([]func(), 0, 2*len(fd.Args))
	for i, p := range fd.Args {
		if strings.HasPrefix(p, "$") {
			if o.conflict[p] || o.conflict[p[1:]] {
				fd.Args[i] = o.gen.fresh(p)
				pops = append(pops, o.vars.set(p, fd.Args[i]))
				pops = append(pops, o.funcs.set(funcKey(p[1:], 0), funcRepl{name: fd.Args[i][1:]}))
				continue
			}
			pops = append(pops, o.vars.unset(p), o.funcs.unset(funcKey(p[1:], 0)))
			continue
		}
		if o.conflict[p] {
			fd.Args[i] = o.gen.fresh(p)
			pops = append(pops, o.funcs.set(funcKey(p, 0), funcRepl{name: fd.Args[i]}))
			continue
		}
		pops = append(pops, o.funcs.unset(funcKey(p, 0)))
	}
	return popAll(pops)
}

func (o *substOps) bindVars(names []string) (map[string]string, func()) {
	var rename map[string]string
	pops := make([]func(), 0, len(names))
	for _, n := range names {
		if o.conflict[n] {
			fresh := o.gen.fresh(n)
			if rename == nil {
				rename = map[string]string{}
			}
			rename[n] = fresh
			pops = append(pops, o.vars.set(n, fresh))
			continue
		}
		pops = append(pops, o.vars.unset(n))
	}
	return rename, popAll(pops)
}

func (o *substOps) bindLabel(name string) (string, func()) {
	if o.conflict[name] {
		fresh := o.gen.fresh(name)
		return fresh, o.labels.set(name, fresh)
	}
	return name, o.labels.unset(name)
}

func (o *substOps) ref(t *gojq.Term) *gojq.Term {
	name := t.Func.Name
	if strings.HasPrefix(name, "$") {
		if n, ok := o.vars[name]; ok {
			t.Func.Name = n
		}
		return t
	}
	repl, ok := o.funcs[funcKey(name, len(t.Func.Args))]
	if !ok {
		return t
	}
	if repl.query == nil {
		t.Func.Name = repl.name
		return t
	}
	// A parameter: the argument takes the call's place. It comes from the
	// call site already in its final form, so it is copied in rather than
	// walked - nothing in this body's renaming applies to it.
	for _, s := range t.SuffixList {
		walkSuffix(s, o)
	}
	return spliceTerm(cloneQuery(repl.query), t.SuffixList)
}

func (o *substOps) brk(name string) string {
	if n, ok := o.labels[name]; ok {
		return n
	}
	return name
}

// asTerm reduces a query to the term it already is, or parenthesises it. jq's
// grammar wants a term in the places an expansion lands - the left of an
// `as`, the thing a suffix hangs off - and a term is atomic, so dropping the
// parentheses around one can never change how the surrounding query groups.
func asTerm(q *gojq.Query) *gojq.Term {
	if q.Meta == nil && len(q.Imports) == 0 && len(q.FuncDefs) == 0 && q.Op == 0 && q.Term != nil {
		return q.Term
	}
	return &gojq.Term{Type: gojq.TermTypeQuery, Query: q}
}

// spliceTerm turns an expanded body into the term that replaces a call,
// carrying over the suffixes the call had: `f[0]` becomes `(body)[0]`.
func spliceTerm(q *gojq.Query, suffixes []*gojq.Suffix) *gojq.Term {
	t := asTerm(q)
	if len(suffixes) > 0 {
		t.SuffixList = append(append([]*gojq.Suffix(nil), t.SuffixList...), suffixes...)
	}
	return t
}
