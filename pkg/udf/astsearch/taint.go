package astsearch

import (
	"os"
	"regexp"
	"strings"
	"sync"

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
//
// target/result is Swift's assignment. Every other grammar here calls the
// right-hand side a value or a right; Swift calls it the result, so `s = x`
// carried nothing while `let s = x` - which is `name`/`value` - carried
// everything. A rule following a value through anything reassigned found the
// first hop and lost the second.
//
// item/collection is Swift's `for`, and it is the only pair here that is not
// an assignment written with an `=`. It is one all the same: `for part in
// url.pathComponents` gives `part` each of the collection's values, which is
// exactly what the Java pass had to teach this engine about
// `for (ZipEntry entry : zip.entries())`. Swift labels that pair and nothing
// else does, so a Swift `for` carried no flow at all - in the language whose
// idiom for walking anything untrusted is a `for` over it.
var bindingFields = [][2]string{
	{"left", "right"},
	{"name", "value"},
	{"target", "value"},
	{"pattern", "value"},
	{"declarator", "value"},
	{"target", "result"},
	{"item", "collection"},
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
	f := &flow{lang: entry.Language(), language: entry.Name, source: source, root: root,
		sanitizers: sanitizers}
	return f.reached(sources, sinks), nil
}

// flow is one file being followed.
type flow struct {
	lang *gotreesitter.Language
	// language is the registry's name for it, which is the key the per-language
	// measurements below are cached under.
	language   string
	source     []byte
	root       *gotreesitter.Node
	sanitizers []Span
}

// a taint is a name that holds a value from a source, the place it was given
// that value, and the body it is a name in.
//
// `at` is the end of the value, not the end of the statement that assigned
// it, and the difference is the whole of loop binding. A `for` is an
// assignment whose node encloses the body it assigns for, so measured at the
// node every use of the loop variable happens *before* the variable was given
// its value, and none of them was ever reached:
//
//	for (ZipEntry entry : zip.entries()) {
//	        new File(dir, entry.getName());   // never reported
//	}
//
// Measured at the value there is nothing between the two but the tokens that
// close the binding, so no use of a name can fall in the gap and an ordinary
// assignment reads exactly as it did.
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
		if f.holds(tainted, name) {
			return true
		}
	}
	return false
}

// holds reports whether one mention of a name is a mention of a tainted value.
//
// Three questions, and a rule needs all three. Is it that name; was it given
// its value before this, rather than after; and is this the same name - the
// same body, so that a `path` in one method is not a `path` in the next.
//
// The third is the one that is easy to leave out and expensive to leave out.
// Local names in one file repeat: `name`, `path`, `url`, `value`, `result`.
// Without the scope test any file with two methods that both spell a local
// `name` has one method's taint answering for the other's, and the finding
// lands on a line that was careful.
func (f *flow) holds(tainted []taint, use name) bool {
	for _, t := range tainted {
		if t.name != use.text || t.at >= use.span.Start {
			continue
		}
		if t.scope != (Span{}) && !t.scope.contains(use.span) {
			continue
		}
		if f.sanitized(use.span) {
			continue
		}
		return true
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
	f.walk(f.root, func(node *gotreesitter.Node) {
		left, right := f.sides(node)
		if left == nil || right == nil {
			return
		}
		fed := false
		for _, use := range f.namesIn(f.span(right)) {
			if f.nestedInAFunction(use.span, f.span(right)) {
				continue
			}
			if f.holds(tainted, use) {
				fed = true
				break
			}
		}
		if !fed || f.sanitized(f.span(right)) {
			return
		}
		scope := f.scopeOf(node)
		for _, name := range f.namesIn(f.span(left)) {
			grown := taint{name: name.text, at: int(right.EndByte()), scope: scope}
			if !contains(out, grown) {
				out = append(out, grown)
			}
		}
	})
	return out
}

// contains reports whether this exact binding is already known, which is what
// stops a round of propagation from re-adding what the last one found and
// keeps the loop in reached from running until it hits its limit every time.
// One assignment yields one binding, so the set is bounded by the file.
func contains(tainted []taint, want taint) bool {
	for _, t := range tainted {
		if t == want {
			return true
		}
	}
	return false
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
	for n := node; n != nil; n = n.Parent() {
		if f.leavesFunction(n, src) {
			break
		}
		left, right := f.sides(n)
		if left == nil || right == nil || !f.span(right).contains(src) {
			continue
		}
		var out []taint
		for _, name := range f.namesIn(f.span(left)) {
			out = append(out, taint{name: name.text, at: int(right.EndByte()), scope: f.scopeOf(n)})
		}
		return out
	}
	return f.arrivesUnderItsOwnName(src, node)
}

// arrivesUnderItsOwnName is the taint a source carries when the source is a
// name rather than something a name was given.
//
// A rule points at a parameter when the parameter is the untrusted thing.
// ASP.NET binds a controller action's arguments from the query string, the
// route and the body, so
//
//	public IActionResult ByCity(string city)
//
// says `city` came from the caller, and there is no accessor anywhere in the
// method to point at instead - which is how every C# web framework written
// since about 2016 spells it. The walk above asks what the source was *given
// to* and a parameter was given to nothing, so it found no flow and the sink
// two lines later went unreported.
//
// The name is scoped to the body it belongs to and dated to where it is
// written, so it behaves exactly like a name that was assigned there: a use
// before it, or in another method that happens to spell a local the same way,
// is not a use of this.
//
// It is asked only of a source whose text is a single name, which is what
// keeps it from turning an expression the walk declined to follow - a read
// inside a closure, the case leavesFunction exists for - into a taint on
// whatever it was written next to.
func (f *flow) arrivesUnderItsOwnName(src Span, node *gotreesitter.Node) []taint {
	if src.Start < 0 || src.End > len(f.source) || src.Start >= src.End {
		return nil
	}
	text := strings.TrimSpace(string(f.source[src.Start:src.End]))
	if !identifier.MatchString(text) || node == nil {
		return nil
	}
	return []taint{{name: text, at: src.End, scope: f.scopeOf(node)}}
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
	params, body := f.functionParts(node)
	if params == nil || body == nil {
		return false
	}
	return f.span(body).contains(src)
}

// functionParts are the parameter list and the body of a function, or nils
// where the node is not one.
//
// The labelled reading is first and is every grammar here but one. Kotlin
// labels nothing on a function or on a lambda:
//
//	function_declaration  "fun f(a: String) { ... }"
//	  simple_identifier          "f"
//	  function_value_parameters  "(a: String)"
//	  function_body              "{ ... }"
//
// so no Kotlin taint was ever scoped to the method it was written in - a
// `name` in one method answered for a `name` in the next, which is the bug the
// Java pass fixed everywhere a grammar says `body` - and no closure was ever a
// boundary.
//
// A grammar with no fields still says which child is the parameter list: it
// names the node for it, and the plural is the whole of the tell.
// `function_value_parameters` and `lambda_parameters` are lists; `parameter`
// is one item and is not this, which is what keeps a parameter list from
// reading its own last parameter as a body. The body is then the last child,
// rather than a child named for it, because a Kotlin lambda's body is
// `statements` and a function's is `function_body` and there is nothing in
// common between the two but their position.
//
// A lambda written without parameters is not recognised, and in Kotlin that is
// `Thread { ... }` and every listener in Android. It is the same gap a Ruby
// block has and the same direction: a value read inside such a lambda still
// escapes it, which costs noise rather than findings, and a rule that reports
// something built from a listener rather than from an intent is a rule that
// should be reading a narrower source.
//
// Swift says half of it, and it is the third shape. A function declaration
// labels its body and says nothing about its parameters, because it writes
// each of them as a child of its own with no list around them:
//
//	function_declaration  "func f(a: Int, b: String) -> Int { ... }"
//	  simple_identifier [name]  "f"
//	  parameter                 "a: Int"
//	  parameter                 "b: String"
//	  user_type [name]          "Int"
//	  function_body [body]      "{ ... }"
//
// and a closure puts the list one level down, inside the node that holds the
// signature:
//
//	lambda_literal        "{ (r: URL) in sink(r) }"
//	  lambda_function_type [type]         "(r: URL)"
//	    lambda_function_type_parameters   "r: URL"
//	  statements                          "sink(r)"
//
// So the parameters are measured wherever the grammar did not label them: a
// child, or a grandchild, whose type is named for parameters. The singular is
// admitted for the run Swift writes, and what keeps a parameter list from
// reading its own last parameter as its body is that a body may not itself be
// parameter-shaped and must begin after the parameters end.
func (f *flow) functionParts(node *gotreesitter.Node) (*gotreesitter.Node, *gotreesitter.Node) {
	body := node.ChildByFieldName("body", f.lang)
	params := node.ChildByFieldName("parameters", f.lang)
	if params == nil {
		params = node.ChildByFieldName("parameter", f.lang)
	}
	if params == nil {
		params = f.parametersOf(node)
	}
	if body != nil {
		// A grammar that labelled the body has said what it calls it, and a
		// node with no parameters anywhere is not a function in it.
		if params == nil {
			return nil, nil
		}
		return params, body
	}
	if params == nil {
		return nil, nil
	}
	// Nothing labelled, so the body is the last child - a Kotlin lambda's is
	// `statements` and a function's is `function_body` and they have nothing
	// in common but their position.
	last := node.NamedChild(node.NamedChildCount() - 1)
	if last == nil || parameterNode.MatchString(last.Type(f.lang)) ||
		last.StartByte() < params.EndByte() {
		return nil, nil
	}
	return params, last
}

// parametersOf is the parameter list a grammar did not label: a child of the
// node, or a child of one of its children, whose type says it holds
// parameters. One level down is as far as it looks, because that is where a
// grammar that wraps the signature puts it and any further is a guess.
func (f *flow) parametersOf(node *gotreesitter.Node) *gotreesitter.Node {
	for i := 0; i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		if parameterNode.MatchString(child.Type(f.lang)) {
			return child
		}
		for j := 0; j < child.NamedChildCount(); j++ {
			if inner := child.NamedChild(j); inner != nil &&
				parameterNode.MatchString(inner.Type(f.lang)) {
				return inner
			}
		}
	}
	return nil
}

// parameterNode is what a grammar calls a function's parameters, whether it
// writes them as one list or as a run of single ones. The plural was the whole
// of the tell until Swift, which writes `parameter` repeated and no list at
// all; the singular is admitted for that, and functionParts carries the guard
// that keeps a list from reading its own last parameter as a body.
var parameterNode = regexp.MustCompile(`(?:^|_)(?:parameters|parameter_list|params|parameter)$`)

// sides are the two halves of an assignment, or nil when the node is not one.
func (f *flow) sides(node *gotreesitter.Node) (*gotreesitter.Node, *gotreesitter.Node) {
	for _, pair := range bindingFields {
		left := node.ChildByFieldName(pair[0], f.lang)
		right := node.ChildByFieldName(pair[1], f.lang)
		if left != nil && right != nil {
			return left, right
		}
	}
	if left, right := f.namedAndUnnamed(node); left != nil {
		return left, right
	}
	if left, right := f.boundIdentifier(node); left != nil {
		return left, right
	}
	return f.positional(node)
}

// boundIdentifier is the binding a grammar spells by labelling the name and
// nothing else.
//
// Swift is that grammar and `guard let` is where it matters. Every value worth
// following in Swift is an optional - `url.fragment`, `queryItems`,
// `UIPasteboard.general.string`, `textField.text` are all `String?` - and the
// language has exactly two idioms for unwrapping one:
//
//	guard_statement       "guard let t = url.fragment else { return }"
//	  value_binding_pattern [condition]  "let"
//	  simple_identifier [bound_identifier]  "t"
//	  navigation_expression                 "url.fragment"
//	  else                                  "else"
//	  statements                            "return"
//
// None of the pairs above matches that. `namedAndUnnamed` wants a `name`
// field, and there is none. `positional` wants the target wrapped in a node of
// its own, and it is a bare leaf - Swift wraps the target of a *property*
// declaration in a `pattern` and wraps nothing here. So `guard let` and
// `if let` carried no flow, in the language where reading an untrusted value
// and unwrapping it are the same statement.
//
// What the grammar does say is the whole of the fix: it puts a field name on
// the leaf, and the word it chose - `bound_identifier` - exists for nothing
// else. The word is measured rather than listed, from the same probes
// bindingWrappers uses, and a field the pairs above already know is refused:
// a grammar that calls it `name` has been read correctly by those, and
// treating every labelled `name` as a binding would make a function
// declaration one.
//
// The value is the child after the target, for the reason it is there: between
// the name and the value there is nothing a name can be used in, and in a
// `for` the body is the child after that.
func (f *flow) boundIdentifier(node *gotreesitter.Node) (*gotreesitter.Node, *gotreesitter.Node) {
	field := boundIdentifierFields(f.lang, f.language)
	if field == "" {
		return nil, nil
	}
	for i := 0; i+1 < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		// The target is a name, so a node with children of its own is not it.
		if child == nil || child.NamedChildCount() != 0 ||
			f.fieldOf(node, child) != field {
			continue
		}
		next := node.NamedChild(i + 1)
		// A target followed by another target is a list of them rather than a
		// binding, and neither is given the other.
		if next == nil || f.fieldOf(node, next) == field {
			return nil, nil
		}
		return child, next
	}
	return nil, nil
}

// fieldOf is the field name a node's parent gave it, or "" for a child a
// grammar left unlabelled.
func (f *flow) fieldOf(parent, child *gotreesitter.Node) string {
	for i := 0; i < parent.ChildCount(); i++ {
		at := parent.Child(i)
		if at == nil || at.StartByte() != child.StartByte() || at.EndByte() != child.EndByte() {
			continue
		}
		return parent.FieldNameForChild(i, f.lang)
	}
	return ""
}

// boundIdentifierFieldTypes is the answer per language, worked out once.
var boundIdentifierFieldTypes sync.Map

// boundIdentifierFields is the field name this grammar puts on the leaf that a
// binding gives a value to, and "" for every grammar that says nothing there
// or says something the pairs above already read.
func boundIdentifierFields(lang *gotreesitter.Language, language string) string {
	if cached, ok := boundIdentifierFieldTypes.Load(language); ok {
		return cached.(string)
	}
	found := ""
	for _, probe := range bindingProbes {
		if field := boundIdentifierField(lang, probe); field != "" {
			found = field
			break
		}
	}
	boundIdentifierFieldTypes.Store(language, found)
	return found
}

// knownBindingField is a field name one of the pairs above already reads, so a
// grammar answering with it is being read correctly and must not be read twice.
func knownBindingField(field string) bool {
	for _, pair := range bindingFields {
		if pair[0] == field || pair[1] == field {
			return true
		}
	}
	return field == "" || field == "body" || field == "parameters"
}

// boundIdentifierField is one probe measured: the leaf holding the probe's
// name, and the word its parent labelled it with.
//
// Three things are refused, and the last of them is the whole of what keeps
// this off the grammars it is not for.
//
// The probe has to be the whole file's one construct, the same refusal
// bindingWrapper makes and for the same reason - a grammar that could not read
// it would otherwise answer from what it made of the words.
//
// The word has to be one none of the pairs above already reads. A grammar that
// calls it `name` is read correctly by those, and taking every labelled `name`
// for a binding would make a function declaration one.
//
// And the child after the target has to be the value the probe wrote. This is
// the one that matters: bash reads `pwrqProbe_name = 1` as a command with two
// arguments and labels the leaf `argument`, and PowerShell does the same and
// labels it `command_name`. Both are real field names on a real node and
// neither is a binding - the child after the name is the `=`, not the `1`, and
// that is what says so.
func boundIdentifierField(lang *gotreesitter.Language, probe string) string {
	tree, err := parserFor(lang).Parse([]byte(probe))
	if err != nil || tree == nil {
		return ""
	}
	bound := gotreesitter.Bind(tree)
	defer bound.Release()
	root := bound.RootNode()
	if root == nil || root.HasErrorOrMissing() || root.NamedChildCount() != 1 {
		return ""
	}
	at := strings.Index(probe, probePrefix+"name")
	if at < 0 {
		return ""
	}
	leaf := root.NamedDescendantForByteRange(uint32(at), uint32(at+len(probePrefix)+4))
	if leaf == nil || leaf.NamedChildCount() != 0 {
		return ""
	}
	parent := leaf.Parent()
	if parent == nil {
		return ""
	}
	field := ""
	for i := 0; i < parent.ChildCount(); i++ {
		child := parent.Child(i)
		if child != nil && child.StartByte() == leaf.StartByte() &&
			child.EndByte() == leaf.EndByte() {
			field = parent.FieldNameForChild(i, lang)
			break
		}
	}
	if knownBindingField(field) {
		return ""
	}
	// Up from the name to the child of the node that binds it, the same walk
	// bindingWrapper makes: Swift labels the leaf `bound_identifier` in both
	// places the word appears, but in a property declaration the leaf sits
	// inside a `pattern` and in a `guard` it is a child of the binding itself.
	// The value is asked of the binding either way.
	target, holder := leaf, parent
	for holder != nil && holder.NamedChildCount() < 2 {
		target, holder = holder, holder.Parent()
	}
	if holder == nil {
		return ""
	}
	for i := 0; i+1 < holder.NamedChildCount(); i++ {
		at := holder.NamedChild(i)
		if at == nil || at.StartByte() != target.StartByte() ||
			at.EndByte() != target.EndByte() {
			continue
		}
		value := holder.NamedChild(i + 1)
		if value == nil || string(bytesOf(probe, value)) != probeValue {
			return ""
		}
		return strings.Clone(field)
	}
	return ""
}

// namedAndUnnamed is the binding a grammar spells with a name on one side and
// nothing at all on the other.
//
// C# is that grammar. Its `variable_declarator` gives the name a field and
// leaves the initialiser an ordinary child:
//
//	variable_declarator [name=identifier] "name = Request.Query[\"n\"]"
//	  identifier                          "name"
//	  element_access_expression           "Request.Query[\"n\"]"
//
// so none of the pairs above match, and a declaration - which is how nearly
// every value in a C# program is introduced - carried no flow. Every rule in
// the corpus that follows a value through C# found nothing, for the same
// reason `declarator`/`value` had to be added for C.
//
// Three things keep it from binding anything else. There must be exactly two
// named children, so a declaration with a type and a name and an initialiser -
// which is what C# calls a `variable_declaration`, one level up - is not this.
// The name must be the first of them, which is what a `parameter` fails: it is
// spelled type-then-name, and reading it this way would say a parameter's name
// is given the value of its type. And a node with a body or a parameter list
// is a function, whose name is not given the value of anything.
func (f *flow) namedAndUnnamed(node *gotreesitter.Node) (*gotreesitter.Node, *gotreesitter.Node) {
	left := node.ChildByFieldName("name", f.lang)
	if left == nil || node.NamedChildCount() != 2 ||
		node.ChildByFieldName("body", f.lang) != nil ||
		node.ChildByFieldName("parameters", f.lang) != nil {
		return nil, nil
	}
	first, second := node.NamedChild(0), node.NamedChild(1)
	if first == nil || second == nil || first.StartByte() != left.StartByte() {
		return nil, nil
	}
	return left, second
}

// scopeOf is the body an assignment happens in, so that a name in one function
// is not read as the same name in another. A grammar names that child `body`;
// an assignment with no such ancestor is at file scope and is not narrowed.
func (f *flow) scopeOf(node *gotreesitter.Node) Span {
	for n := node.Parent(); n != nil; n = n.Parent() {
		if body := n.ChildByFieldName("body", f.lang); body != nil {
			return f.span(body)
		}
		// A grammar that labels nothing is asked the narrower question, and
		// only a function answers it: `body` in a labelled grammar covers a
		// loop as well as a function, but the unlabelled reading has no way to
		// tell a loop's body from an `if`'s, and scoping a name to the branch
		// it was assigned in would lose every use after the branch.
		if _, body := f.functionParts(n); body != nil {
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

// positional is the binding a grammar spells with no field names at all.
//
// Kotlin is that grammar, and it is the one that has none of the pairs above
// and nothing for namedAndUnnamed to find either. A property, an assignment
// and a `for` are all written positionally:
//
//	property_declaration  "val name = intent.getStringExtra(\"n\")"
//	  binding_pattern_kind      "val"
//	  variable_declaration      "name"
//	  call_expression           "intent.getStringExtra(\"n\")"
//
//	assignment            "other = name"
//	  directly_assignable_expression  "other"
//	  simple_identifier               "name"
//
//	for_statement         "for (e in list) { use(e) }"
//	  variable_declaration      "e"
//	  simple_identifier         "list"
//	  control_structure_body    "{ use(e) }"
//
// so no Kotlin file carried any flow at all, and `reaching` - which is what a
// quarter of any corpus is built on - returned nothing for the language whose
// security literature is almost entirely "an intent's extra reaches a sink".
//
// What makes it readable anyway is that a grammar with no fields still says
// which child is the target: it wraps it in a node of its own.
// `variable_declaration` and `directly_assignable_expression` exist for no
// other reason, and an expression's operand is never wrapped that way - the
// left of `name + "x"` is a bare `simple_identifier`, and the receiver of
// `intent.getStringExtra` is a `navigation_expression` holding two names
// rather than one. So the wrapper is the mark, it is measured rather than
// listed, and a grammar that does not wrap contributes nothing here and keeps
// the reading it had.
//
// The child after the target is the value, rather than the last child, and
// that is what makes a `for` read correctly: the body is the third child and
// the thing iterated is the second, so "the next one" is the value and "the
// last one" would have been the body.
//
// A target followed by another target is a parameter list rather than a
// binding - Kotlin writes `{ a, b -> ... }` as two `variable_declaration`s
// side by side - and is refused, because neither of them is given the other.
func (f *flow) positional(node *gotreesitter.Node) (*gotreesitter.Node, *gotreesitter.Node) {
	wrappers := bindingWrappers(f.lang, f.language)
	if len(wrappers) == 0 {
		return nil, nil
	}
	for i := 0; i+1 < node.NamedChildCount(); i++ {
		child, next := node.NamedChild(i), node.NamedChild(i+1)
		if child == nil || next == nil || !wrappers[child.Type(f.lang)] {
			continue
		}
		if wrappers[next.Type(f.lang)] {
			return nil, nil
		}
		return child, next
	}
	return nil, nil
}

// bindingProbes are the spellings a name being given a value is written in.
//
// It is a list of syntaxes rather than a list of languages, in keeping with
// memberHolders: whichever of them a grammar reads without complaint is the
// one it is asked about, and a grammar that reads none is not asked. Two of
// them are wanted even where both parse, because a language may wrap a
// declaration's name and an assignment's target in two different nodes, which
// is exactly what Kotlin does.
var bindingProbes = []string{
	probePrefix + "name = 1",
	"val " + probePrefix + "name = 1",
	"var " + probePrefix + "name = 1",
	"let " + probePrefix + "name = 1",
}

// bindingWrapperTypes is the answer per language, worked out once.
var bindingWrapperTypes sync.Map

// bindingWrappers are the node types this grammar wraps a binding target in,
// and empty for every grammar that labels its children instead.
//
// The measurement is the probe parsed, the leaf holding the probe's name
// found, and the walk back up to the child of the binding stopped one short:
// where that child is the leaf itself the grammar wrapped nothing and there is
// nothing here to use, and where it is a node of its own that node's type is
// the mark. A tree with an error in it is not measured, which is how a
// grammar that cannot read a spelling declines it rather than answering from
// wreckage.
func bindingWrappers(lang *gotreesitter.Language, language string) map[string]bool {
	if cached, ok := bindingWrapperTypes.Load(language); ok {
		return cached.(map[string]bool)
	}
	found := map[string]bool{}
	for _, probe := range bindingProbes {
		if wrapper := bindingWrapper(lang, probe); wrapper != "" {
			found[wrapper] = true
		}
	}
	bindingWrapperTypes.Store(language, found)
	return found
}

// bindingWrapper is one probe measured, and the three things it refuses are
// what keep the reading off the grammars it is not for.
//
// The probe has to be the whole file's one construct, or a grammar that could
// not read it answers from what it made of the words instead: Clojure takes
// `pwrqProbe_name = 1` for three symbols in a row and would have offered
// `sym_lit`.
//
// The construct has to label none of its children, because a grammar that
// labels them has already said which side is which and the pairs above are
// reading it correctly. This is what keeps Go's `expression_list` out - `x = 1`
// there is an `assignment_statement` with `left` and `right` - and Bash's
// `command_name`, and it is the whole justification for the reading: it exists
// for the grammars that say nothing, and where anything is said it is not
// wanted.
//
// And the child after the target has to be the value the probe wrote. OCaml's
// `x = 1` is a comparison rather than a binding and its left side is wrapped in
// a `value_path`, so without this every application and every equality in an
// OCaml file would have read as a name being given a value.
func bindingWrapper(lang *gotreesitter.Language, probe string) string {
	tree, err := parserFor(lang).Parse([]byte(probe))
	if err != nil || tree == nil {
		return ""
	}
	bound := gotreesitter.Bind(tree)
	defer bound.Release()
	root := bound.RootNode()
	if root == nil || root.HasErrorOrMissing() || root.NamedChildCount() != 1 {
		return ""
	}
	at := strings.Index(probe, probePrefix+"name")
	if at < 0 {
		return ""
	}
	leaf := root.NamedDescendantForByteRange(uint32(at), uint32(at+len(probePrefix)+4))
	if leaf == nil || leaf.NamedChildCount() != 0 {
		return ""
	}
	// Up from the name to the child of the node that binds it. The parent of
	// that child is the binding, and one step short of it is the target: for
	// `val x = 1` the walk passes `simple_identifier` and stops on
	// `variable_declaration`, whose parent is the `property_declaration`.
	target, parent := leaf, leaf.Parent()
	for parent != nil && parent.NamedChildCount() < 2 {
		target, parent = parent, parent.Parent()
	}
	if parent == nil || target == leaf || labelsAChild(parent, lang) {
		// Either the name stands alone in the binding, which is what every
		// grammar that labels its children does, or the grammar labelled the
		// binding and is already read correctly, or there is no binding here at
		// all. All three are "nothing to measure".
		return ""
	}
	for i := 0; i+1 < parent.NamedChildCount(); i++ {
		if parent.NamedChild(i) != target {
			continue
		}
		value := parent.NamedChild(i + 1)
		if value == nil || string(bytesOf(probe, value)) != probeValue {
			return ""
		}
		return strings.Clone(target.Type(lang))
	}
	return ""
}

// probeValue is what every binding probe gives the name, so that the child
// after the target can be checked for being the value rather than a piece of
// punctuation the grammar happened to name.
const probeValue = "1"

// bytesOf is a node's own text out of the source it was parsed from.
func bytesOf(source string, node *gotreesitter.Node) []byte {
	start, end := int(node.StartByte()), int(node.EndByte())
	if start < 0 || end > len(source) || start > end {
		return nil
	}
	return []byte(source[start:end])
}

// labelsAChild reports whether the grammar gave any of this node's children a
// field name.
func labelsAChild(node *gotreesitter.Node, lang *gotreesitter.Language) bool {
	for i := 0; i < node.ChildCount(); i++ {
		if node.FieldNameForChild(i, lang) != "" {
			return true
		}
	}
	return false
}
