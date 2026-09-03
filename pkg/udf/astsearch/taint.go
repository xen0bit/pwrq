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
//
// declarator/value is C's, and C++'s, and it is the pair that carries almost
// everything in those languages: `char *p = getenv("X");` is a declaration
// with an initialiser and not an assignment expression, so without this pair
// a C file's flow began at its second mention of a value and a rule about
// tainted input found nothing in ordinary code.
var bindingFields = [][2]string{
	{"left", "right"},
	{"name", "value"},
	{"target", "value"},
	{"pattern", "value"},
	{"declarator", "value"},
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
//
// A tainted name mentioned inside a function on the right-hand side does not
// feed the assignment, because what the assignment gives away is the function
// and not anything the function reads.
//
// This is the other half of the closure leak, and it survives the fix to
// bindings on its own. The right-hand side of
// `srv := httptest.NewServer(http.HandlerFunc(func(w, r) { ... }))` contains
// every identifier in the handler, so a value the handler read from the
// request fed `srv`, and then everything built from `srv`. Fixing only the
// walk up from the source makes this worse rather than better: the bogus
// binding used to shadow the name, and removing it let propagation reach
// further than before.
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
			if !known[name.text] || f.nestedInAFunction(name.span, f.span(right)) {
				continue
			}
			fed = true
			break
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

// nestedInAFunction reports whether a name sits inside a function that is
// itself inside the span being read.
//
// The span is a right-hand side, so a function inside it is a value being
// assigned rather than code being run here, and the names it reads are its
// own. The walk stops at the span rather than at the root, so the function
// the assignment is written in does not count as nested inside it.
func (f *flow) nestedInAFunction(at Span, within Span) bool {
	node := f.root.NamedDescendantForByteRange(uint32(at.Start), uint32(at.End))
	for ; node != nil; node = node.Parent() {
		span := f.span(node)
		if !within.contains(span) {
			return false
		}
		if f.leavesFunction(node, at) {
			return true
		}
	}
	return false
}

// bindings are the names a source is given to, or none when it is used where
// it is written.
func (f *flow) bindings(src Span) []taint {
	node := f.root.NamedDescendantForByteRange(uint32(src.Start), uint32(src.End))
	for ; node != nil; node = node.Parent() {
		if f.leavesFunction(node, src) {
			return nil
		}
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

// leavesFunction reports whether walking past this node would carry a source
// out of the function it was written in.
//
// The walk up from a source asks what the source was given to, and it used to
// ask it of every ancestor. A closure assigned to a name is an ancestor with
// two halves, so a source read anywhere inside a handler was reported as
// having been given to whatever that handler was assigned to:
//
//	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        if r.Header.Get("X-Fail") != "" { ... }
//	})
//	ts := httptest.NewServer(inner)
//	http.Get(ts.URL)
//
// `inner` was tainted by the header, `ts` by `inner`, and an SSRF rule
// reported a test server's own address. What `inner` was given is a function,
// and the header's value never left the closure.
//
// A function is recognised as a node with both parameters and a body, which
// is what separates it from the other things a grammar gives a body to. A
// `for` has a body and no parameters, and so does a Python comprehension -
// and the comprehension is the reason this is not simply "any node with a
// body": `x = [request.args.get(n) for n in names]` is a real path from the
// source to `x`, and cutting it would cost findings rather than noise.
//
// The source has to be inside the body rather than be the whole node, so that
// a rule that names a closure as its source still binds the name the closure
// is given to.
//
// A Ruby block written without parameters has a body and no parameter list,
// so it is not recognised and the old behaviour survives there. That is a gap
// rather than a decision, and it is the direction that costs noise rather
// than findings.
func (f *flow) leavesFunction(node *gotreesitter.Node, src Span) bool {
	body := node.ChildByFieldName("body", f.lang)
	if body == nil {
		return false
	}
	params := node.ChildByFieldName("parameters", f.lang)
	if params == nil {
		params = node.ChildByFieldName("parameter", f.lang)
	}
	if params == nil {
		return false
	}
	return f.span(body).contains(src)
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
