package graph

import (
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
)

// RenderD2 turns a parsed query into a D2 script.
//
// The diagram's job is to show where data goes, so the pipeline is the spine:
// every stage of `a | b | c` becomes a node, left to right. Structure that
// carries meaning - a function's arguments, the branches of an if, the keys of
// a constructed object, the operands of a comparison - expands into a nested
// container. Structure that does not is inlined as label text, because a
// diagram in which every `.a` is its own box shows less than the query did.
func RenderD2(query *gojq.Query) string {
	b := &builder{}
	b.line("direction: right")
	b.line("")

	if query == nil {
		b.node("empty", "(empty query)", "circle")
		return b.String()
	}

	// User-defined functions are context rather than pipeline, so they sit
	// above the flow instead of interrupting it.
	b.emitFuncDefs(query.FuncDefs)

	b.node("start", "Start", "circle")
	first, last := b.emitPipeline(query, "")
	if first == "" {
		// The query is only definitions; wire start straight to end.
		first, last = "start", "start"
	} else {
		b.edge("start", first, "")
	}
	_ = last

	b.node("end", "End", "circle")
	b.edge(last, "end", "")
	return b.String()
}

// opNone is the zero Operator, which gojq uses for a query that is just a
// term. The package exports no name for it (OpPipe starts at iota + 1).
const opNone = gojq.Operator(0)

type builder struct {
	sb    strings.Builder
	n     int
	depth int
}

func (b *builder) String() string { return b.sb.String() }

// id mints a node name. Names are local to the container they are declared in,
// because D2 resolves references relative to the enclosing block - writing a
// dotted path inside a block declares that whole path *under* it, which is what
// produced the stray "n1"/"n2" boxes in an earlier version.
func (b *builder) id(string) string {
	b.n++
	return fmt.Sprintf("n%d", b.n)
}

func (b *builder) line(s string) {
	b.sb.WriteString(strings.Repeat("  ", b.depth))
	b.sb.WriteString(s)
	b.sb.WriteByte('\n')
}

func (b *builder) node(id, label, shape string) {
	if shape == "" {
		b.line(fmt.Sprintf("%s: %s", id, quote(label)))
		return
	}
	b.line(fmt.Sprintf("%s: %s {shape: %s}", id, quote(label), shape))
}

// container opens a node that holds other nodes, and returns a function that
// closes it.
func (b *builder) container(id, label string) func() {
	b.line(fmt.Sprintf("%s: %s {", id, quote(label)))
	b.depth++
	return func() {
		b.depth--
		b.line("}")
	}
}

func (b *builder) edge(from, to, label string) {
	if from == "" || to == "" {
		return
	}
	if label == "" {
		b.line(fmt.Sprintf("%s -> %s", from, to))
		return
	}
	b.line(fmt.Sprintf("%s -> %s: %s", from, to, quote(label)))
}

func (b *builder) emitFuncDefs(defs []*gojq.FuncDef) {
	if len(defs) == 0 {
		return
	}
	close := b.container("defs", fmt.Sprintf("Definitions (%d)", len(defs)))
	for _, def := range defs {
		id := b.id("defs")
		sig := def.Name
		if len(def.Args) > 0 {
			sig += "(" + strings.Join(def.Args, "; ") + ")"
		}
		b.node(id, sig, "rectangle")
	}
	close()
	b.line("")
}

// emitPipeline lays out `a | b | c` as a chain and returns the first and last
// node ids so the caller can wire it into a larger flow.
func (b *builder) emitPipeline(q *gojq.Query, scope string) (string, string) {
	stages := flattenPipe(q)
	var first, last string
	for _, st := range stages {
		id := b.emitStage(st.query, scope, bindingLabel(st.bindings))
		if id == "" {
			continue
		}
		if first == "" {
			first = id
		} else {
			b.edge(last, id, "")
		}
		last = id
	}
	return first, last
}

// stage is one step of a pipeline, together with any pattern it binds its
// output to. `. as $x | rest` is a pipe whose bindings hang off the pipe node,
// so flattening without carrying them would drop the binding from the diagram.
type stage struct {
	query    *gojq.Query
	bindings []*gojq.Pattern
}

// flattenPipe turns a left-nested pipe chain into the stages it represents.
func flattenPipe(q *gojq.Query) []stage {
	if q == nil {
		return nil
	}
	if q.Op == gojq.OpPipe {
		left := flattenPipe(q.Left)
		if n := len(q.Patterns); n > 0 && len(left) > 0 {
			left[len(left)-1].bindings = q.Patterns
		}
		return append(left, flattenPipe(q.Right)...)
	}
	return []stage{{query: q}}
}

// bindingLabel renders the `as $x` part of a binding stage.
func bindingLabel(patterns []*gojq.Pattern) string {
	if len(patterns) == 0 {
		return ""
	}
	names := make([]string, len(patterns))
	for i, p := range patterns {
		names[i] = patternLabel(p)
	}
	return " as " + strings.Join(names, " ?// ")
}

// emitStage renders one stage of a pipeline as a single node, which may be a
// container if the stage has structure worth showing.
func (b *builder) emitStage(q *gojq.Query, scope, bind string) string {
	if q == nil {
		return ""
	}

	// A binary operator is a junction: both operands feed it.
	if q.Op != opNone && q.Op != gojq.OpPipe {
		if !needsExpansion(q) {
			id := b.id(scope)
			b.node(id, q.String()+bind, "rectangle")
			return id
		}
		id := b.id(scope)
		close := b.container(id, operatorLabel(q.Op)+bind)
		leftFirst, leftLast := b.emitOperand(q.Left, id, "left")
		rightFirst, rightLast := b.emitOperand(q.Right, id, "right")
		close()
		_, _ = leftLast, rightLast
		_, _ = leftFirst, rightFirst
		return id
	}

	if q.Term != nil {
		return b.emitTerm(q.Term, scope, "", bind)
	}

	id := b.id(scope)
	b.node(id, q.String()+bind, "rectangle")
	return id
}

// emitOperand renders one side of a binary operator inside its container.
func (b *builder) emitOperand(q *gojq.Query, scope, side string) (string, string) {
	if q == nil {
		return "", ""
	}
	if !needsExpansion(q) {
		id := b.id(scope)
		b.node(id, q.String(), "rectangle")
		return id, id
	}
	return b.emitPipeline(q, scope)
}

// emitTerm renders a single term, expanding the ones that contain a pipeline.
func (b *builder) emitTerm(t *gojq.Term, scope, prefix, bind string) string {
	if t == nil {
		return ""
	}

	suffix := suffixLabel(t.SuffixList) + bind

	switch t.Type {
	case gojq.TermTypeFunc:
		return b.emitFunc(t, scope, suffix, prefix)
	case gojq.TermTypeObject:
		return b.emitObject(t, scope, suffix, prefix)
	case gojq.TermTypeArray:
		return b.emitArray(t, scope, suffix, prefix)
	case gojq.TermTypeIf:
		return b.emitIf(t, scope, suffix, prefix)
	case gojq.TermTypeTry:
		return b.emitTry(t, scope, suffix, prefix)
	case gojq.TermTypeReduce:
		return b.emitReduce(t, scope, suffix, prefix)
	case gojq.TermTypeForeach:
		return b.emitForeach(t, scope, suffix, prefix)
	case gojq.TermTypeQuery:
		return b.emitGrouped(t, scope, suffix, prefix)
	case gojq.TermTypeUnary:
		return b.emitUnary(t, scope, suffix, prefix)
	case gojq.TermTypeLabel:
		return b.emitLabel(t, scope, suffix, prefix)
	}

	// A leaf renders as the jq that produced it; Term.String already carries
	// any suffixes, so only the binding needs appending.
	id := b.id(scope)
	b.node(id, withPrefix(prefix, t.String()+bind), "rectangle")
	return id
}

func (b *builder) emitFunc(t *gojq.Term, scope, suffix, prefix string) string {
	fn := t.Func
	if len(fn.Args) == 0 {
		id := b.id(scope)
		b.node(id, withPrefix(prefix, fn.Name+suffix), "rectangle")
		return id
	}

	// Arguments that are themselves simple read better on the node than as
	// boxes, so a call only opens a container when an argument has structure.
	if !anyNeedsExpansion(fn.Args) {
		id := b.id(scope)
		b.node(id, withPrefix(prefix, t.String()), "rectangle")
		return id
	}

	// A call whose one argument is a single structured term merges with it, so
	// `map({a, b})` reads "map: Object { }" instead of nesting an unlabelled
	// object inside a container that holds nothing else.
	if len(fn.Args) == 1 {
		arg := unwrapParens(fn.Args[0])
		if stages := flattenPipe(arg); len(stages) == 1 && len(stages[0].bindings) == 0 &&
			stages[0].query.Op == opNone && stages[0].query.Term != nil &&
			termNeedsExpansion(stages[0].query.Term) {
			return b.emitTerm(stages[0].query.Term, scope,
				withPrefix(prefix, fn.Name+suffix), "")
		}
	}

	id := b.id(scope)
	close := b.container(id, withPrefix(prefix, fn.Name+"( )"+suffix))
	if len(fn.Args) == 1 {
		// A single argument needs no name; the container already says whose
		// argument it is.
		b.emitPipeline(unwrapParens(fn.Args[0]), id)
	} else {
		for i, arg := range fn.Args {
			b.emitLabelledBranch(id, argLabel(fn.Name, i, len(fn.Args)), arg)
		}
	}
	close()
	return id
}

// argLabel names a call's argument. A couple of cmdlets have a conventional
// shape worth naming; the rest are numbered.
func argLabel(fnName string, i, total int) string {
	if total == 2 {
		switch fnName {
		case "get_childitem", "find", "cat", "http", "invoke_web_request":
			if i == 0 {
				return "path"
			}
			return "options"
		}
	}
	return fmt.Sprintf("arg %d", i+1)
}

func (b *builder) emitObject(t *gojq.Term, scope, suffix, prefix string) string {
	id := b.id(scope)
	close := b.container(id, withPrefix(prefix, "Object { }"+suffix))
	for _, kv := range t.Object.KeyVals {
		key := objectKeyLabel(kv)
		if kv.Val == nil {
			// Shorthand {Name} takes the value from the input, which the old
			// renderer dropped entirely.
			child := b.id(id)
			b.node(child, key+": ."+key, "rectangle")
			continue
		}
		b.emitLabelledBranch(id, key, kv.Val)
	}
	close()
	return id
}

func objectKeyLabel(kv *gojq.ObjectKeyVal) string {
	switch {
	case kv.Key != "":
		return kv.Key
	case kv.KeyString != nil:
		return strings.Trim(kv.KeyString.String(), `"`)
	case kv.KeyQuery != nil:
		return "(" + kv.KeyQuery.String() + ")"
	}
	return "?"
}

func (b *builder) emitArray(t *gojq.Term, scope, suffix, prefix string) string {
	if t.Array == nil || t.Array.Query == nil {
		id := b.id(scope)
		b.node(id, withPrefix(prefix, "[ ] empty array"+suffix), "rectangle")
		return id
	}
	if !needsExpansion(t.Array.Query) {
		id := b.id(scope)
		b.node(id, withPrefix(prefix, "Collect "+t.String()+suffix), "rectangle")
		return id
	}

	// Array construction was invisible before: `[ ... ] | sort_by(...)` showed
	// only its contents, so the collection step that makes sort_by possible
	// simply was not in the picture.
	id := b.id(scope)
	close := b.container(id, withPrefix(prefix, "Collect [ ]"+suffix))
	b.emitPipeline(t.Array.Query, id)
	close()
	return id
}

func (b *builder) emitIf(t *gojq.Term, scope, suffix, prefix string) string {
	id := b.id(scope)
	close := b.container(id, withPrefix(prefix, "if"+suffix))
	b.emitLabelledBranch(id, "if", t.If.Cond)
	b.emitLabelledBranch(id, "then", t.If.Then)
	for i, elif := range t.If.Elif {
		b.emitLabelledBranch(id, fmt.Sprintf("elif %d", i+1), elif.Cond)
		b.emitLabelledBranch(id, fmt.Sprintf("then %d", i+1), elif.Then)
	}
	if t.If.Else != nil {
		b.emitLabelledBranch(id, "else", t.If.Else)
	}
	close()
	return id
}

func (b *builder) emitTry(t *gojq.Term, scope, suffix, prefix string) string {
	id := b.id(scope)
	close := b.container(id, withPrefix(prefix, "try"+suffix))
	b.emitLabelledBranch(id, "try", t.Try.Body)
	if t.Try.Catch != nil {
		b.emitLabelledBranch(id, "catch", t.Try.Catch)
	}
	close()
	return id
}

// emitLabel renders `label $out | body`. The label itself is only meaningful
// because something inside the body breaks to it, so the body is what the
// container holds; `break $out` reaches the leaf case and renders as its jq.
func (b *builder) emitLabel(t *gojq.Term, scope, suffix, prefix string) string {
	id := b.id(scope)
	close := b.container(id, withPrefix(prefix, "label "+t.Label.Ident+suffix))
	b.emitLabelledBranch(id, "body", t.Label.Body)
	close()
	return id
}

func (b *builder) emitReduce(t *gojq.Term, scope, suffix, prefix string) string {
	id := b.id(scope)
	close := b.container(id, withPrefix(prefix, "reduce as "+patternLabel(t.Reduce.Pattern)+suffix))
	b.emitLabelledBranch(id, "source", t.Reduce.Query)
	b.emitLabelledBranch(id, "init", t.Reduce.Start)
	b.emitLabelledBranch(id, "update", t.Reduce.Update)
	close()
	return id
}

func (b *builder) emitForeach(t *gojq.Term, scope, suffix, prefix string) string {
	id := b.id(scope)
	close := b.container(id, withPrefix(prefix, "foreach as "+patternLabel(t.Foreach.Pattern)+suffix))
	b.emitLabelledBranch(id, "source", t.Foreach.Query)
	b.emitLabelledBranch(id, "init", t.Foreach.Start)
	b.emitLabelledBranch(id, "update", t.Foreach.Update)
	if t.Foreach.Extract != nil {
		b.emitLabelledBranch(id, "extract", t.Foreach.Extract)
	}
	close()
	return id
}

func (b *builder) emitGrouped(t *gojq.Term, scope, suffix, prefix string) string {
	if !needsExpansion(t.Query) {
		id := b.id(scope)
		b.node(id, withPrefix(prefix, t.String()), "rectangle")
		return id
	}
	id := b.id(scope)
	close := b.container(id, withPrefix(prefix, strings.TrimSpace("( )"+suffix)))
	b.emitPipeline(t.Query, id)
	close()
	return id
}

func (b *builder) emitUnary(t *gojq.Term, scope, suffix, prefix string) string {
	// The operand used to be dropped, leaving a bare "Unary: -".
	inner := &gojq.Query{Term: t.Unary.Term}
	if !needsExpansion(inner) {
		id := b.id(scope)
		b.node(id, withPrefix(prefix, t.String()), "rectangle")
		return id
	}
	id := b.id(scope)
	close := b.container(id, withPrefix(prefix, "unary "+t.Unary.Op.String()+suffix))
	b.emitPipeline(inner, id)
	close()
	return id
}

// emitLabelledBranch renders a named part of a construct - an argument, an
// object value, a branch of an if - as its own labelled sub-container when it
// has structure, or a single labelled node when it does not.
func (b *builder) emitLabelledBranch(scope, label string, q *gojq.Query) {
	if q == nil {
		return
	}
	// Parentheses that only exist to group a pipeline add a box that says
	// nothing; the branch's own label already marks the boundary.
	q = unwrapParens(q)

	if !needsExpansion(q) {
		id := b.id(scope)
		b.node(id, label+": "+q.String(), "rectangle")
		return
	}

	// A branch whose whole content is one structured term merges with it, so
	// an options argument reads "options: Object { }" rather than nesting an
	// unlabelled container inside a labelled one.
	if stages := flattenPipe(q); len(stages) == 1 && len(stages[0].bindings) == 0 &&
		stages[0].query.Op == opNone && stages[0].query.Term != nil {
		b.emitTerm(stages[0].query.Term, scope, label, "")
		return
	}

	id := b.id(scope)
	close := b.container(id, label)
	b.emitPipeline(q, id)
	close()
}

// unwrapParens strips grouping parentheses that carry no suffix, since they
// bound a pipeline the caller is already labelling.
func unwrapParens(q *gojq.Query) *gojq.Query {
	for q != nil && q.Op == opNone && q.Term != nil &&
		q.Term.Type == gojq.TermTypeQuery && len(q.Term.SuffixList) == 0 {
		q = q.Term.Query
	}
	return q
}

// withPrefix joins a branch label to the label of the thing it contains.
func withPrefix(prefix, label string) string {
	if prefix == "" {
		return label
	}
	return prefix + ": " + label
}

func patternLabel(p *gojq.Pattern) string {
	if p == nil {
		return "?"
	}
	return p.String()
}

// suffixLabel renders `.a[0]?` style suffixes onto the node's own label, since
// they qualify the term rather than being separate steps.
func suffixLabel(suffixes []*gojq.Suffix) string {
	if len(suffixes) == 0 {
		return ""
	}
	var s strings.Builder
	for _, suffix := range suffixes {
		s.WriteString(suffix.String())
	}
	return " " + s.String()
}

func operatorLabel(op gojq.Operator) string {
	names := map[gojq.Operator]string{
		gojq.OpComma: "comma  ,  (both outputs)",
		gojq.OpAdd:   "add  +",
		gojq.OpSub:   "subtract  -",
		gojq.OpMul:   "multiply  *",
		gojq.OpDiv:   "divide  /",
		gojq.OpMod:   "modulo  %",
		gojq.OpEq:    "equals  ==",
		gojq.OpNe:    "not equal  !=",
		gojq.OpGt:    "greater than  >",
		gojq.OpLt:    "less than  <",
		gojq.OpGe:    "at least  >=",
		gojq.OpLe:    "at most  <=",
		gojq.OpAnd:   "and",
		gojq.OpOr:    "or",
		gojq.OpAlt:   "alternative  //",
	}
	if name, ok := names[op]; ok {
		return name
	}
	return op.String()
}

// needsExpansion decides whether a subquery earns a container of its own.
//
// The test is whether it contains a step: a pipe, a comma, a call with
// arguments, a constructed object or array, or a control-flow form. Everything
// else - a path, a literal, a comparison of two paths - says more as one line
// of text than as a box tree.
func needsExpansion(q *gojq.Query) bool {
	if q == nil {
		return false
	}
	if q.Op == gojq.OpPipe || q.Op == gojq.OpComma {
		return true
	}
	if q.Op != opNone {
		return needsExpansion(q.Left) || needsExpansion(q.Right)
	}
	return termNeedsExpansion(q.Term)
}

func termNeedsExpansion(t *gojq.Term) bool {
	if t == nil {
		return false
	}
	switch t.Type {
	case gojq.TermTypeIf, gojq.TermTypeTry, gojq.TermTypeReduce, gojq.TermTypeForeach,
		gojq.TermTypeLabel:
		return true
	case gojq.TermTypeFunc:
		return anyNeedsExpansion(t.Func.Args)
	case gojq.TermTypeObject:
		if t.Object == nil {
			return false
		}
		// A constructed object is worth showing whenever it has more than a
		// key or two, since its keys are the shape of the output.
		if len(t.Object.KeyVals) > 1 {
			return true
		}
		for _, kv := range t.Object.KeyVals {
			if needsExpansion(kv.Val) {
				return true
			}
		}
		return false
	case gojq.TermTypeArray:
		return t.Array != nil && needsExpansion(t.Array.Query)
	case gojq.TermTypeQuery:
		return needsExpansion(t.Query)
	case gojq.TermTypeUnary:
		return t.Unary != nil && termNeedsExpansion(t.Unary.Term)
	}
	return false
}

func anyNeedsExpansion(queries []*gojq.Query) bool {
	for _, q := range queries {
		if needsExpansion(q) {
			return true
		}
	}
	return false
}

// quote renders a label as a D2 string, escaping what D2 would otherwise read
// as syntax.
func quote(label string) string {
	label = strings.ReplaceAll(label, "\\", "\\\\")
	label = strings.ReplaceAll(label, `"`, `\"`)
	// D2 reads ${...} as a substitution even inside a quoted string, and jq
	// variables are spelled with a dollar, so every query naming one would
	// otherwise produce a script D2 refuses to compile.
	label = strings.ReplaceAll(label, "$", `\$`)
	label = strings.ReplaceAll(label, "\n", " ")
	return `"` + label + `"`
}
