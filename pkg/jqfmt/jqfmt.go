// Package jqfmt renders a parsed jq query as text.
//
// It offers the two forms a query box wants: Format lays the query out over
// multiple lines, breaking top-level pipelines so each stage starts its own
// line and folding long objects, arrays and conditionals under indentation,
// and Minify is the canonical single-line form. Both render the same parse
// tree, so neither ever changes what the query means - parens already in the
// tree are kept, and nothing else is added.
//
// Format is a layout, not a rewrite. It breaks where the reader would break:
// a top-level pipeline becomes one stage per line, an object that does not
// fit spreads its entries across lines, and an array whose body is a pipeline
// hangs the continuation stages under the opening bracket. Everything short
// stays on the line it started on.
package jqfmt

import (
	"strings"

	"github.com/itchyny/gojq"
)

// width is the line length Format breaks at. It is a layout target, not a
// hard bound: a single token longer than the width, or a nested pipe that
// stays on one line by rule, is allowed to exceed it.
const width = 100

// Format lays a parsed query out over multiple lines.
func Format(q *gojq.Query) string {
	f := &formatter{}
	var b strings.Builder
	f.program(&b, q)
	return b.String()
}

// Minify renders a parsed query on a single line.
func Minify(q *gojq.Query) string {
	return q.String()
}

// formatter carries the formatting state. It is stateless beyond the width,
// so a single value serves the whole render.
type formatter struct{}

// program renders a whole query: its module header and imports, its
// definitions each on their own line, then the main expression with its
// top-level pipelines broken stage by stage.
func (f *formatter) program(b *strings.Builder, q *gojq.Query) {
	if q == nil {
		return
	}
	if q.Meta != nil {
		b.WriteString("module ")
		b.WriteString(q.Meta.String())
		b.WriteString(";\n")
	}
	for _, im := range q.Imports {
		b.WriteString(im.String())
	}
	for i, fd := range q.FuncDefs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("def ")
		b.WriteString(fd.Name)
		if len(fd.Args) > 0 {
			b.WriteString("(")
			b.WriteString(strings.Join(fd.Args, "; "))
			b.WriteByte(')')
		}
		b.WriteString(": ")
		b.WriteString(f.renderQuery(fd.Body, len("def ")+len(fd.Name), false))
		b.WriteByte(';')
	}
	if len(q.FuncDefs) > 0 {
		b.WriteByte('\n')
		b.WriteByte('\n')
	}
	b.WriteString(f.renderQuery(q, 0, true))
}

// renderQuery renders a query node. breakPipes decides whether a top-level
// pipeline at this node breaks its stages onto separate lines; it is true for
// the program and for array bodies, where a long pipeline is exactly what
// wants spreading, and false everywhere a pipe is a value - inside a function
// argument, an object entry or parentheses - where the stages stay inline
// just as they were written.
func (f *formatter) renderQuery(q *gojq.Query, col int, breakPipes bool) string {
	if q == nil {
		return ""
	}
	switch q.Op {
	case gojq.OpPipe:
		stages := pipeStages(q)
		if !breakPipes {
			if fits(col, q.String()) {
				return q.String()
			}
			parts := make([]string, len(stages))
			for i, s := range stages {
				parts[i] = f.renderStage(s, col)
			}
			return strings.Join(parts, " | ")
		}
		parts := make([]string, len(stages))
		for i, s := range stages {
			// The first stage begins the line at col; every later stage is
			// preceded by "| ", so its content starts at col+2 and anything
			// it spreads aligns under it.
			start := col
			if i > 0 {
				start = col + 2
			}
			parts[i] = f.renderStage(s, start)
		}
		return strings.Join(parts, "\n"+indent(col)+"| ")
	case 0:
		if q.Term == nil {
			return ""
		}
		return f.renderTerm(q.Term, col)
	default:
		// Any other operator - comma, arithmetic, comparison, and/or, //,
		// assignment - reads as a single expression, so it stays inline.
		return q.String()
	}
}

// stage is one element of a pipeline, with any "as $x" bindings that follow
// it attached. A stage is never itself a pipe.
type stage struct {
	q        *gojq.Query
	patterns []*gojq.Pattern
}

// pipeStages flattens a left-associative pipeline into its stages. The "as"
// bindings live on the pipe node and apply to everything to its left, so they
// are attached to the last stage of the flattened left side, which is what
// gojq's own renderer does.
func pipeStages(q *gojq.Query) []stage {
	if q.Op != gojq.OpPipe {
		return []stage{{q: q}}
	}
	left := pipeStages(q.Left)
	if len(q.Patterns) > 0 {
		left[len(left)-1].patterns = append(left[len(left)-1].patterns, q.Patterns...)
	}
	return append(left, pipeStages(q.Right)...)
}

func (f *formatter) renderStage(s stage, col int) string {
	text := f.renderQuery(s.q, col, false)
	if len(s.patterns) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString(text)
	for i, p := range s.patterns {
		if i == 0 {
			b.WriteString(" as ")
		} else {
			b.WriteString(" ?// ")
		}
		b.WriteString(p.String())
	}
	return b.String()
}

// renderTerm renders a term. Objects and arrays decide their own layout -
// their compact forms differ from the parser's spaced String - and every
// other construct is kept inline when it fits, with only the ones worth
// spreading broken and their suffixes appended to the result.
func (f *formatter) renderTerm(t *gojq.Term, col int) string {
	if t == nil {
		return ""
	}

	var base string
	switch t.Type {
	case gojq.TermTypeObject:
		base = f.renderObject(t.Object, col)
	case gojq.TermTypeArray:
		base = f.renderArray(t.Array, col)
	}
	if base != "" {
		for _, s := range t.SuffixList {
			base += f.renderSuffix(s)
		}
		return base
	}

	inline := t.String()
	if fits(col, inline) {
		return inline
	}

	var b strings.Builder
	switch t.Type {
	case gojq.TermTypeIf:
		b.WriteString(f.renderIf(t.If, col))
	case gojq.TermTypeTry:
		b.WriteString(f.renderTry(t.Try, col))
	case gojq.TermTypeReduce:
		b.WriteString(f.renderReduce(t.Reduce, col))
	case gojq.TermTypeForeach:
		b.WriteString(f.renderForeach(t.Foreach, col))
	case gojq.TermTypeLabel:
		b.WriteString("label ")
		b.WriteString(t.Label.Ident)
		b.WriteString(" | ")
		b.WriteString(f.renderQuery(t.Label.Body, col+len("label ")+len(t.Label.Ident)+len(" | "), false))
	case gojq.TermTypeQuery:
		b.WriteByte('(')
		b.WriteString(f.renderQuery(t.Query, col+1, false))
		b.WriteByte(')')
	default:
		// Identity, index, function call, string, number, unary, format,
		// break, recurse, null, true and false have nothing to spread, so a
		// long one just stays long.
		return inline
	}
	for _, s := range t.SuffixList {
		b.WriteString(f.renderSuffix(s))
	}
	return b.String()
}

// renderObject lays an object out in the compact style the example queries
// are written in: no space after the braces. When every entry is single-line
// and the whole object fits it stays on one line; when it does not, the
// entries fill across lines at the width with a trailing comma, and when an
// entry had to spread itself the whole object gives every entry its own line.
func (f *formatter) renderObject(o *gojq.Object, col int) string {
	if len(o.KeyVals) == 0 {
		return "{}"
	}
	entries := make([]string, len(o.KeyVals))
	allInline := true
	for i, kv := range o.KeyVals {
		entries[i] = f.renderKeyVal(kv, col+2)
		if strings.Contains(entries[i], "\n") {
			allInline = false
		}
	}

	oneLine := "{" + strings.Join(entries, ", ") + "}"
	if allInline && fits(col, oneLine) {
		return oneLine
	}

	if allInline {
		var lines []string
		cur := "{"
		for _, e := range entries {
			if cur == "{" {
				cur += e
			} else if len(cur)+len(", ")+len(e) <= width {
				cur += ", " + e
			} else {
				cur += ","
				lines = append(lines, cur)
				cur = indent(col+2) + e
			}
		}
		lines = append(lines, cur+"}")
		return strings.Join(lines, "\n")
	}

	var b strings.Builder
	b.WriteString("{\n")
	for i, e := range entries {
		b.WriteString(indent(col + 2))
		b.WriteString(e)
		if i < len(entries)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(indent(col))
	b.WriteByte('}')
	return b.String()
}

func (f *formatter) renderKeyVal(kv *gojq.ObjectKeyVal, col int) string {
	key := kv.Key
	if kv.KeyString != nil {
		key = kv.KeyString.String()
	} else if kv.KeyQuery != nil {
		key = "(" + f.renderQuery(kv.KeyQuery, col, false) + ")"
	}
	if kv.Val == nil {
		return key
	}
	return key + ": " + f.renderQuery(kv.Val, col+len(key)+2, false)
}

// renderArray lays an array out. A short body stays on one line; a long body
// spreads according to its shape - a pipeline hangs its continuation stages
// under the opening bracket, a comma list gives each element a line, and
// anything else is indented on its own line.
func (f *formatter) renderArray(a *gojq.Array, col int) string {
	if a == nil || a.Query == nil {
		return "[]"
	}
	body := a.Query

	if fits(col, "["+body.String()+"]") {
		return "[" + body.String() + "]"
	}

	var b strings.Builder
	switch body.Op {
	case gojq.OpPipe:
		stages := pipeStages(body)
		b.WriteByte('[')
		b.WriteString(f.renderStage(stages[0], col+1))
		for _, s := range stages[1:] {
			b.WriteByte('\n')
			b.WriteString(indent(col + 2))
			b.WriteString("| ")
			b.WriteString(f.renderStage(s, col+2))
		}
		b.WriteByte(']')
	case gojq.OpComma:
		elems := flattenComma(body)
		b.WriteString("[\n")
		for i, e := range elems {
			b.WriteString(indent(col + 1))
			b.WriteString(f.renderQuery(e, col+1, false))
			if i < len(elems)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(indent(col))
		b.WriteByte(']')
	default:
		b.WriteString("[\n")
		b.WriteString(indent(col + 1))
		b.WriteString(f.renderQuery(body, col+1, false))
		b.WriteByte('\n')
		b.WriteString(indent(col))
		b.WriteByte(']')
	}
	return b.String()
}

// flattenComma splits a right-nested comma chain into its elements.
func flattenComma(q *gojq.Query) []*gojq.Query {
	if q.Op == gojq.OpComma {
		return append(flattenComma(q.Left), flattenComma(q.Right)...)
	}
	return []*gojq.Query{q}
}

// renderIf spreads a conditional that does not fit onto indented branches.
func (f *formatter) renderIf(ifq *gojq.If, col int) string {
	var b strings.Builder
	ind := indent(col)
	writeBranch := func(kw string, cond, then *gojq.Query) {
		b.WriteString(kw)
		b.WriteByte(' ')
		b.WriteString(f.renderQuery(cond, col+len(kw)+1, false))
		b.WriteString(" then\n")
		b.WriteString(indent(col + 2))
		b.WriteString(f.renderQuery(then, col+2, false))
	}
	writeBranch("if", ifq.Cond, ifq.Then)
	for _, el := range ifq.Elif {
		b.WriteByte('\n')
		b.WriteString(ind)
		writeBranch("elif", el.Cond, el.Then)
	}
	if ifq.Else != nil {
		b.WriteByte('\n')
		b.WriteString(ind)
		b.WriteString("else\n")
		b.WriteString(indent(col + 2))
		b.WriteString(f.renderQuery(ifq.Else, col+2, false))
	}
	b.WriteByte('\n')
	b.WriteString(ind)
	b.WriteString("end")
	return b.String()
}

func (f *formatter) renderTry(t *gojq.Try, col int) string {
	body := f.renderQuery(t.Body, col+len("try "), false)
	if t.Catch == nil {
		return "try " + body
	}
	return "try " + body + "\n" + indent(col) + "catch " +
		f.renderQuery(t.Catch, col+len("catch "), false)
}

func (f *formatter) renderReduce(r *gojq.Reduce, col int) string {
	var b strings.Builder
	b.WriteString("reduce ")
	b.WriteString(f.renderQuery(r.Query, col+len("reduce "), false))
	b.WriteString(" as ")
	b.WriteString(r.Pattern.String())
	b.WriteString(" (\n")
	b.WriteString(indent(col + 2))
	b.WriteString(f.renderQuery(r.Start, col+2, false))
	b.WriteString(";\n")
	b.WriteString(indent(col + 2))
	b.WriteString(f.renderQuery(r.Update, col+2, false))
	b.WriteString("\n")
	b.WriteString(indent(col))
	b.WriteByte(')')
	return b.String()
}

func (f *formatter) renderForeach(fe *gojq.Foreach, col int) string {
	var b strings.Builder
	b.WriteString("foreach ")
	b.WriteString(f.renderQuery(fe.Query, col+len("foreach "), false))
	b.WriteString(" as ")
	b.WriteString(fe.Pattern.String())
	b.WriteString(" (\n")
	b.WriteString(indent(col + 2))
	b.WriteString(f.renderQuery(fe.Start, col+2, false))
	b.WriteString(";\n")
	b.WriteString(indent(col + 2))
	b.WriteString(f.renderQuery(fe.Update, col+2, false))
	if fe.Extract != nil {
		b.WriteString(";\n")
		b.WriteString(indent(col + 2))
		b.WriteString(f.renderQuery(fe.Extract, col+2, false))
	}
	b.WriteString("\n")
	b.WriteString(indent(col))
	b.WriteByte(')')
	return b.String()
}

// renderSuffix renders one trailing operator of a term - an index, iteration
// or the optional marker - after a base that spread across lines. A bracketed
// index carries no leading dot, exactly as it does in the source.
func (f *formatter) renderSuffix(s *gojq.Suffix) string {
	if s.Index == nil {
		if s.Iter {
			return "[]"
		}
		if s.Optional {
			return "?"
		}
		return ""
	}
	if s.Index.Name != "" || s.Index.Str != nil {
		return s.Index.String()
	}
	var b strings.Builder
	b.WriteByte('[')
	if s.Index.IsSlice {
		if s.Index.Start != nil {
			b.WriteString(s.Index.Start.String())
		}
		b.WriteByte(':')
		if s.Index.End != nil {
			b.WriteString(s.Index.End.String())
		}
	} else {
		b.WriteString(s.Index.Start.String())
	}
	b.WriteByte(']')
	return b.String()
}

func indent(n int) string {
	return strings.Repeat(" ", n)
}

// fits reports whether a single-line text of this length fits in the width
// remaining after col.
func fits(col int, text string) bool {
	if strings.Contains(text, "\n") {
		return false
	}
	return col+len(text) <= width
}
