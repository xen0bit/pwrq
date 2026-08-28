package jqinline

import (
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
)

// funcKey names a definition the way jq resolves one: by name and arity, so
// `def f:` and `def f(a):` are two different functions.
func funcKey(name string, arity int) string {
	return name + "/" + strconv.Itoa(arity)
}

// names is what a scan reports: the names a query uses without binding them.
// Variables and labels are keyed as written, with the dollar; functions by
// name and arity.
type names struct {
	vars   map[string]bool
	funcs  map[string]bool
	labels map[string]bool
}

func newNames() names {
	return names{vars: map[string]bool{}, funcs: map[string]bool{}, labels: map[string]bool{}}
}

// scoped is what every pass here keeps: a map from a name to what it currently
// means, where entering a binding can be undone. set and unset both hand back
// the function that restores what the name meant before - unset being how a
// binding that needs no rewriting still shadows one that does.
type scoped[V any] map[string]V

func (m scoped[V]) set(k string, v V) func() {
	old, had := m[k]
	m[k] = v
	return func() {
		if had {
			m[k] = old
		} else {
			delete(m, k)
		}
	}
}

func (m scoped[V]) unset(k string) func() {
	old, had := m[k]
	if !had {
		return func() {}
	}
	delete(m, k)
	return func() { m[k] = old }
}

func (m scoped[V]) clone() scoped[V] {
	c := make(scoped[V], len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// popAll undoes a run of bindings, innermost first, which is the order a
// scope has to come apart in.
func popAll(pops []func()) func() {
	return func() {
		for i := len(pops) - 1; i >= 0; i-- {
			pops[i]()
		}
	}
}

// counter is a set of names that are in scope, counted so that a shadowing
// binding can be popped without losing the one it shadowed.
type counter map[string]int

func (c counter) push(names ...string) func() {
	for _, n := range names {
		c[n]++
	}
	return func() {
		for _, n := range names {
			if c[n]--; c[n] <= 0 {
				delete(c, n)
			}
		}
	}
}

func (c counter) has(n string) bool { return c[n] > 0 }

// scanOps observes a query without changing it. It answers the two questions
// the inliner asks about a subtree: which names does it leave free - those
// have to still mean the same thing wherever the subtree is moved to - and
// which names does it mention at all - those are what an expansion must
// rename around.
type scanOps struct {
	vars, funcs, labels counter
	free                names
	all                 map[string]bool
}

func newScanOps() *scanOps {
	return &scanOps{
		vars: counter{}, funcs: counter{}, labels: counter{},
		free: newNames(), all: map[string]bool{},
	}
}

// freeNames reports the names a definition's body leaves free, with the
// definition's own parameters treated as bound.
func freeNames(body *gojq.Query, params []string) names {
	o := newScanOps()
	pop := o.pushParams(params)
	walkQuery(body, o)
	pop()
	return o.free
}

// mentionedNames reports every name a query spells, bound or free. It is the
// set an expansion has to rename its own bindings away from.
func mentionedNames(q *gojq.Query, into map[string]bool) map[string]bool {
	o := newScanOps()
	o.all = into
	walkQuery(q, o)
	return into
}

func (o *scanOps) note(n ...string) {
	for _, s := range n {
		o.all[s] = true
	}
}

func (o *scanOps) pushParams(params []string) func() {
	var vars, funcs []string
	for _, p := range params {
		o.note(p)
		if strings.HasPrefix(p, "$") {
			// A value parameter binds both forms: `$a` for the value and `a`
			// for the filter it came from.
			o.note(p[1:])
			vars = append(vars, p)
			funcs = append(funcs, funcKey(p[1:], 0))
		} else {
			funcs = append(funcs, funcKey(p, 0))
		}
	}
	popVars, popFuncs := o.vars.push(vars...), o.funcs.push(funcs...)
	return func() { popFuncs(); popVars() }
}

func (o *scanOps) funcDefs(q *gojq.Query, walk func(*gojq.Query)) func() {
	pops := make([]func(), 0, len(q.FuncDefs))
	for _, fd := range q.FuncDefs {
		o.note(fd.Name)
		// A definition is in scope inside its own body, and in scope for the
		// definitions that follow it - but not for the ones before it.
		pops = append(pops, o.funcs.push(funcKey(fd.Name, len(fd.Args))))
		pop := o.pushParams(fd.Args)
		walk(fd.Body)
		pop()
	}
	return popAll(pops)
}

func (o *scanOps) bindVars(names []string) (map[string]string, func()) {
	o.note(names...)
	return nil, o.vars.push(names...)
}

func (o *scanOps) bindLabel(name string) (string, func()) {
	o.note(name)
	return name, o.labels.push(name)
}

func (o *scanOps) ref(t *gojq.Term) *gojq.Term {
	name := t.Func.Name
	if strings.HasPrefix(name, "$") {
		o.note(name)
		if !o.vars.has(name) {
			o.free.vars[name] = true
		}
		return t
	}
	o.note(name)
	if key := funcKey(name, len(t.Func.Args)); !o.funcs.has(key) {
		o.free.funcs[key] = true
	}
	return t
}

func (o *scanOps) brk(name string) string {
	o.note(name)
	if !o.labels.has(name) {
		o.free.labels[name] = true
	}
	return name
}
