package astsearch

import (
	"os"
	"regexp"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Following a value from where it comes in to where it is used.
//
// A quarter of the rules in any corpus are not about a construct, they are
// about a journey: this value came from the request, and it ends up in a
// command. Neither end is a finding on its own - reading a query parameter is
// what web applications do, and running a command is what programs do - so a
// rule that could only match constructs would have to report both and be
// wrong about both.
//
// What follows is deliberately intraprocedural and deliberately syntactic. It
// does not resolve types, it does not cross function boundaries, and it does
// not know what a library does. It knows the one thing that carries a value
// from one line to another in every language in this build: assignment. That
// is enough for the rules this corpus is made of, and where it is not enough
// it under-reports rather than inventing a path.

// A Span is a byte range in one file.
type Span struct {
	Start int
	End   int
}

// contains reports whether the span covers another.
func (s Span) contains(other Span) bool { return s.Start <= other.Start && s.End >= other.End }

// bindingFields are the field-name pairs a grammar uses to say "this side is
// given the value of that side".
//
// Every grammar in this build spells assignment with one of these, because
// there are only so many ways: Python, Go, JavaScript and Ruby all say
// left/right; a declarator with an initialiser says name/value; a few say
// target/value. A grammar that says none of them contributes no flow, which
// costs findings rather than inventing them.
var bindingFields = [][2]string{
	{"left", "right"},
	{"name", "value"},
	{"target", "value"},
	{"pattern", "value"},
}

// identifier is what a name looks like in the languages here, PHP's sigil
// included.
var identifier = regexp.MustCompile(`^\$?[A-Za-z_][A-Za-z_0-9]*$`)

// Reaches returns the indexes of the sinks that a source flows into.
//
// A sink is reached when the source is written inside it - `run(request.args)`
// - or when the source was assigned to a name that the sink then uses, before
// the sink, within the same body. A sanitizer covering either end stops the
// flow, which is what a sanitizer is for.
func Reaches(path, language string, sources, sinks, sanitizers []Span) ([]int, error) {
	if len(sources) == 0 || len(sinks) == 0 {
		return nil, nil
	}
	entry := grammars.DetectLanguageByName(language)
	if entry == nil {
		return nil, nil
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tree, err := parserFor(entry.Language()).Parse(source)
	if err != nil || tree == nil {
		return nil, err
	}
	bound := gotreesitter.Bind(tree)
	defer bound.Release()
	root := bound.RootNode()
	if root == nil {
		return nil, nil
	}
	f := &flow{lang: entry.Language(), source: source, root: root, sanitizers: sanitizers}
	return f.reached(sources, sinks), nil
}

// flow is one file being followed.
type flow struct {
	lang       *gotreesitter.Language
	source     []byte
	root       *gotreesitter.Node
	sanitizers []Span
}

// a taint is a name that holds a value from a source, the place it was given
// that value, and the body it is a name in.
type taint struct {
	name  string
	at    int
	scope Span
}

func (f *flow) reached(sources, sinks []Span) []int {
	var tainted []taint
	for _, src := range sources {
		if f.sanitized(src) {
			continue
		}
		tainted = append(tainted, f.bindings(src)...)
	}
	// A name given the value of a tainted name is tainted too. The corpus
	// rarely needs more than a step or two of this, and a fixed number of
	// rounds is what keeps a file with a cycle in it from being followed
	// forever.
	for round := 0; round < 4; round++ {
		grown := f.propagate(tainted)
		if len(grown) == len(tainted) {
			break
		}
		tainted = grown
	}

	var out []int
	for i, sink := range sinks {
		if f.sanitized(sink) {
			continue
		}
		if f.reaches(tainted, sources, sink) {
			out = append(out, i)
		}
	}
	return out
}

// reaches reports whether one sink is fed by a source.
func (f *flow) reaches(tainted []taint, sources []Span, sink Span) bool {
	// The source written straight into the sink needs no name to travel by.
	for _, src := range sources {
		if sink.contains(src) {
			return true
		}
	}
	for _, name := range f.namesIn(sink) {
		for _, t := range tainted {
			if t.name != name.text || t.at >= name.span.Start {
				continue
			}
			if t.scope != (Span{}) && !t.scope.contains(name.span) {
				continue
			}
			if f.sanitized(name.span) {
				continue
			}
			return true
		}
	}
	return false
}

// propagate adds the names that a tainted name was assigned to.
func (f *flow) propagate(tainted []taint) []taint {
	out := append([]taint(nil), tainted...)
	known := map[string]bool{}
	for _, t := range tainted {
		known[t.name] = true
	}
	f.walk(f.root, func(node *gotreesitter.Node) {
		left, right := f.sides(node)
		if left == nil || right == nil {
			return
		}
		fed := false
		for _, name := range f.namesIn(f.span(right)) {
			if known[name.text] {
				fed = true
				break
			}
		}
		if !fed || f.sanitized(f.span(right)) {
			return
		}
		for _, name := range f.namesIn(f.span(left)) {
			if known[name.text] {
				continue
			}
			out = append(out, taint{name: name.text, at: int(node.EndByte()), scope: f.scopeOf(node)})
		}
	})
	return out
}

// bindings are the names a source is given to, or none when it is used where
// it is written.
func (f *flow) bindings(src Span) []taint {
	node := f.root.NamedDescendantForByteRange(uint32(src.Start), uint32(src.End))
	for ; node != nil; node = node.Parent() {
		left, right := f.sides(node)
		if left == nil || right == nil || !f.span(right).contains(src) {
			continue
		}
		var out []taint
		for _, name := range f.namesIn(f.span(left)) {
			out = append(out, taint{name: name.text, at: int(node.EndByte()), scope: f.scopeOf(node)})
		}
		return out
	}
	return nil
}

// sides are the two halves of an assignment, or nil when the node is not one.
func (f *flow) sides(node *gotreesitter.Node) (*gotreesitter.Node, *gotreesitter.Node) {
	for _, pair := range bindingFields {
		left := node.ChildByFieldName(pair[0], f.lang)
		right := node.ChildByFieldName(pair[1], f.lang)
		if left != nil && right != nil {
			return left, right
		}
	}
	return nil, nil
}

// scopeOf is the body an assignment happens in, so that a name in one function
// is not read as the same name in another. A grammar names that child `body`;
// an assignment with no such ancestor is at file scope and is not narrowed.
func (f *flow) scopeOf(node *gotreesitter.Node) Span {
	for n := node.Parent(); n != nil; n = n.Parent() {
		if body := n.ChildByFieldName("body", f.lang); body != nil {
			return f.span(body)
		}
	}
	return Span{}
}

// a name is an identifier and where it sits.
type name struct {
	text string
	span Span
}

// namesIn are the identifiers written inside a span.
func (f *flow) namesIn(within Span) []name {
	node := f.root.NamedDescendantForByteRange(uint32(within.Start), uint32(within.End))
	if node == nil {
		return nil
	}
	var out []name
	f.walk(node, func(n *gotreesitter.Node) {
		if n.NamedChildCount() != 0 {
			return
		}
		span := f.span(n)
		if !within.contains(span) {
			return
		}
		text := strings.TrimSpace(string(f.source[span.Start:min(span.End, len(f.source))]))
		if identifier.MatchString(text) {
			out = append(out, name{text: text, span: span})
		}
	})
	return out
}

// sanitized reports whether a span is inside something the rule called a
// sanitizer.
func (f *flow) sanitized(span Span) bool {
	for _, clean := range f.sanitizers {
		if clean.contains(span) {
			return true
		}
	}
	return false
}

func (f *flow) span(node *gotreesitter.Node) Span {
	return Span{Start: int(node.StartByte()), End: int(node.EndByte())}
}

// walk visits a node and everything under it.
func (f *flow) walk(node *gotreesitter.Node, visit func(*gotreesitter.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for i := 0; i < node.NamedChildCount(); i++ {
		f.walk(node.NamedChild(i), visit)
	}
}
