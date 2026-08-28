package jqinline

import "github.com/itchyny/gojq"

// Every expansion needs its own copy of the body it expands, because the
// copies are then renamed and substituted apart from one another. These are
// the deep copies that make that safe. Nodes that no pass ever writes to -
// module metadata, imports, and the constant terms they are built from - are
// shared rather than copied.

func cloneQuery(q *gojq.Query) *gojq.Query {
	if q == nil {
		return nil
	}
	c := *q
	if q.FuncDefs != nil {
		c.FuncDefs = make([]*gojq.FuncDef, len(q.FuncDefs))
		for i, fd := range q.FuncDefs {
			c.FuncDefs[i] = cloneFuncDef(fd)
		}
	}
	c.Term = cloneTerm(q.Term)
	c.Left = cloneQuery(q.Left)
	c.Right = cloneQuery(q.Right)
	if q.Patterns != nil {
		c.Patterns = make([]*gojq.Pattern, len(q.Patterns))
		for i, p := range q.Patterns {
			c.Patterns[i] = clonePattern(p)
		}
	}
	return &c
}

func cloneFuncDef(fd *gojq.FuncDef) *gojq.FuncDef {
	if fd == nil {
		return nil
	}
	c := *fd
	c.Args = append([]string(nil), fd.Args...)
	c.Body = cloneQuery(fd.Body)
	return &c
}

func cloneTerm(t *gojq.Term) *gojq.Term {
	if t == nil {
		return nil
	}
	c := *t
	c.Index = cloneIndex(t.Index)
	if t.Func != nil {
		f := *t.Func
		if t.Func.Args != nil {
			f.Args = make([]*gojq.Query, len(t.Func.Args))
			for i, a := range t.Func.Args {
				f.Args[i] = cloneQuery(a)
			}
		}
		c.Func = &f
	}
	if t.Object != nil {
		o := gojq.Object{KeyVals: make([]*gojq.ObjectKeyVal, len(t.Object.KeyVals))}
		for i, kv := range t.Object.KeyVals {
			e := *kv
			e.KeyString = cloneString(kv.KeyString)
			e.KeyQuery = cloneQuery(kv.KeyQuery)
			e.Val = cloneQuery(kv.Val)
			o.KeyVals[i] = &e
		}
		c.Object = &o
	}
	if t.Array != nil {
		c.Array = &gojq.Array{Query: cloneQuery(t.Array.Query)}
	}
	if t.Unary != nil {
		c.Unary = &gojq.Unary{Op: t.Unary.Op, Term: cloneTerm(t.Unary.Term)}
	}
	c.Str = cloneString(t.Str)
	if t.If != nil {
		i := gojq.If{Cond: cloneQuery(t.If.Cond), Then: cloneQuery(t.If.Then), Else: cloneQuery(t.If.Else)}
		if t.If.Elif != nil {
			i.Elif = make([]*gojq.IfElif, len(t.If.Elif))
			for n, el := range t.If.Elif {
				i.Elif[n] = &gojq.IfElif{Cond: cloneQuery(el.Cond), Then: cloneQuery(el.Then)}
			}
		}
		c.If = &i
	}
	if t.Try != nil {
		c.Try = &gojq.Try{Body: cloneQuery(t.Try.Body), Catch: cloneQuery(t.Try.Catch)}
	}
	if t.Reduce != nil {
		c.Reduce = &gojq.Reduce{
			Query:   cloneQuery(t.Reduce.Query),
			Pattern: clonePattern(t.Reduce.Pattern),
			Start:   cloneQuery(t.Reduce.Start),
			Update:  cloneQuery(t.Reduce.Update),
		}
	}
	if t.Foreach != nil {
		c.Foreach = &gojq.Foreach{
			Query:   cloneQuery(t.Foreach.Query),
			Pattern: clonePattern(t.Foreach.Pattern),
			Start:   cloneQuery(t.Foreach.Start),
			Update:  cloneQuery(t.Foreach.Update),
			Extract: cloneQuery(t.Foreach.Extract),
		}
	}
	if t.Label != nil {
		c.Label = &gojq.Label{Ident: t.Label.Ident, Body: cloneQuery(t.Label.Body)}
	}
	c.Query = cloneQuery(t.Query)
	if t.SuffixList != nil {
		c.SuffixList = make([]*gojq.Suffix, len(t.SuffixList))
		for i, s := range t.SuffixList {
			c.SuffixList[i] = &gojq.Suffix{Index: cloneIndex(s.Index), Iter: s.Iter, Optional: s.Optional}
		}
	}
	return &c
}

func cloneIndex(i *gojq.Index) *gojq.Index {
	if i == nil {
		return nil
	}
	c := *i
	c.Str = cloneString(i.Str)
	c.Start = cloneQuery(i.Start)
	c.End = cloneQuery(i.End)
	return &c
}

func cloneString(s *gojq.String) *gojq.String {
	if s == nil {
		return nil
	}
	c := *s
	if s.Queries != nil {
		c.Queries = make([]*gojq.Query, len(s.Queries))
		for i, q := range s.Queries {
			c.Queries[i] = cloneQuery(q)
		}
	}
	return &c
}

func clonePattern(p *gojq.Pattern) *gojq.Pattern {
	if p == nil {
		return nil
	}
	c := *p
	if p.Array != nil {
		c.Array = make([]*gojq.Pattern, len(p.Array))
		for i, e := range p.Array {
			c.Array[i] = clonePattern(e)
		}
	}
	if p.Object != nil {
		c.Object = make([]*gojq.PatternObject, len(p.Object))
		for i, e := range p.Object {
			o := *e
			o.KeyString = cloneString(e.KeyString)
			o.KeyQuery = cloneQuery(e.KeyQuery)
			o.Val = clonePattern(e.Val)
			c.Object[i] = &o
		}
	}
	return &c
}
