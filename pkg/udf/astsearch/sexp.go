package astsearch

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grep"
)

// A pattern says how many children a node has, and the query the engine
// compiles it to does not.
//
// `f($A, $B)` becomes `(argument_list (_) @A (_) @B)`, and a tree-sitter query
// written that way means "two children somewhere in this list, in this order"
// rather than "these two children and no others". Against `f(1, 2, 3, 4)` it
// matches six times - every ordered pair - and each match binds $A and $B to a
// different pair. On pwrq's own source, `fmt.Errorf($A, $B)` reported 1939
// matches at 1211 places, and the pattern asks for calls with two arguments.
//
// Nothing about that is visible to a caller. The matches are well-formed, the
// captures are populated, and the only sign that the pattern meant something
// else is the count.
//
// The fix is one character of query syntax. A `.` between two children of a
// query pattern means "immediately adjacent"; one at the start means "the
// first child"; one at the end means "the last". Putting them in turns the
// list back into the list the pattern wrote. This file parses the query the
// engine produced, puts them in, and hands the query back.
//
// The rest of a pattern needs a way to say "and anything else here", which is
// what Semgrep spells `...`, and $$$_ is that: an anonymous variadic among
// siblings is dropped from the query, and the anchor that would have sat where
// it was is dropped with it. So `f($A, $$$_)` is "A first, then whatever", and
// `T{$$$_, Field: true, $$$_}` is "has this field, wherever it sits".

// rootCapture is bound to the whole matched construct.
//
// The engine reports a match's span as the union of its captures, which is not
// the construct: `tls.Config{InsecureSkipVerify: true}` captures the type name
// and the field, so the span stops before the closing brace, and a pattern
// with no metavariables at all has no captures to take a span from. Capturing
// the outermost form gives the span the caller means by "the match", and it is
// what Offset and EndOffset - and so `within` - are built on.
const rootCapture = "__pwrq_match"

// ellipsisName is the capture the engine gives `$$$_`. `$_` compiles to a
// bare `(_)` with no capture at all, so a capture named `_` is unambiguously
// the anonymous variadic.
const ellipsisName = "_"

// againSuffix names the extra captures a hole written more than once compiles
// to. A caller never asked for them, so they are dropped from the captures a
// match reports; see matchObject.
const againSuffix = "__pwrq_again_"

// sexpr is one form in a tree-sitter query: a parenthesised node pattern, a
// quoted anonymous token, or a bare word inside a predicate.
type sexpr struct {
	// field is the `name:` label this form was given, without the colon.
	field string
	// head is the node type of a parenthesised form, and "" for a token.
	head string
	// token is the literal text of an unparenthesised child - a quoted string
	// or a bare word - reproduced as it was written.
	token string
	// capture is the @name bound to this form, without the @.
	capture  string
	children []*sexpr
	// anchored records that this child must immediately follow the one before
	// it, or be the first child when it is first.
	anchored bool
	// anchoredEnd records that this form's last child must be the last child.
	anchoredEnd bool
}

// isForm reports whether this is a parenthesised node pattern rather than a
// token.
func (s *sexpr) isForm() bool { return s.token == "" }

// isEllipsis reports whether this child is `$$$_` - the "and anything else
// here" marker, which is a placeholder in the query and not a node to match.
func (s *sexpr) isEllipsis() bool {
	return s.isForm() && s.head == "_" && s.capture == ellipsisName && len(s.children) == 0
}

// anchorQuery rewrites a compiled query so that a list of children means that
// list, a hole binds the node it was written for, and the whole construct is
// captured.
//
// A query it cannot parse is returned unchanged. This file exists to make
// matching stricter, and failing open is the direction that keeps a pattern
// working when the engine's output grows a shape this parser has not seen.
func anchorQuery(sexp string, folded map[string][]int, units map[string]bool, drop map[string]bool) string {
	forms, err := parseSexp(sexp)
	if err != nil || len(forms) == 0 {
		return sexp
	}
	forms = dropMarked(forms, drop)
	for _, form := range forms {
		if strings.HasPrefix(form.head, "#") {
			continue
		}
		unfold(form, folded)
		anchorChildren(form)
	}
	// The first form is the pattern; the rest are (#eq? ...) predicates.
	root := forms[0]
	if units[root.head] && !labelled(root) {
		// Walk down the scaffolding a reading added. Every node a wrapper
		// contributed is in units, so the descent stops of its own accord at
		// the first node the caller wrote - and everything the wrapper carried
		// on the way down goes with it.
		for {
			next := lone(root)
			if next == nil || !next.isForm() || !units[next.head] {
				break
			}
			root = next
		}
		root = openUnit(root)
	} else if root.capture == "" {
		root.capture = rootCapture
	}
	forms = append([]*sexpr{root}, live(forms[1:], captureNames(root))...)
	forms = append(forms, bindRepeats(root)...)
	parts := make([]string, len(forms))
	for i, form := range forms {
		parts[i] = form.render()
	}
	return strings.Join(parts, "\n")
}

// labelled reports whether a form's children carry `name:` labels, which says
// the form is a construct the caller wrote and not scaffolding around one.
//
// The two are otherwise hard to tell apart. Scaffolding is whatever holds a
// list of items - a file, a block - and its children are the items, unlabelled
// because a list does not name its places. A construct names them: a subscript
// has an `argument:` and an `index:`, an assignment a `left:` and a `right:`.
//
// Without this, a pattern whose type happens to appear in the scaffold chain -
// `table[$I]` does, where `$A[$I]` does not - is opened up as though it were a
// list of statements. The head becomes `_`, the whole form loses its capture,
// and the match's span is then the union of the holes: `table[3` for
// `table[3]`, one byte short, with nothing to say it went wrong. Everything
// downstream that compares spans - within, outside, not_at - compares that.
func labelled(s *sexpr) bool {
	for _, c := range s.children {
		if c.isForm() && !c.isEllipsis() && c.field != "" {
			return true
		}
	}
	return false
}

// lone is the one child of a form that is not an ellipsis, or nil when the
// form has none or several. Ellipses are skipped because a scaffold's own
// parts are pruned into them - a class the reading invented has a name, and
// the name is not one of the things the pattern is looking for.
func lone(s *sexpr) *sexpr {
	var found *sexpr
	for _, c := range s.children {
		if c.isEllipsis() {
			continue
		}
		if found != nil {
			return nil
		}
		found = c
	}
	return found
}

// captureNames lists every @name bound anywhere in a form.
func captureNames(s *sexpr) map[string]bool {
	names := map[string]bool{}
	var walk func(*sexpr)
	walk = func(n *sexpr) {
		if n.capture != "" {
			names[n.capture] = true
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(s)
	return names
}

// live drops the predicates that no longer say anything, which is what
// descending past a scaffold leaves behind: `(#eq? @_lit_2 "void")` is a claim
// about a method the reading invented, and the capture it names went with the
// method. A query naming a capture it does not bind will not compile at all,
// so this is what makes the descent legal as well as what makes it right.
func live(preds []*sexpr, bound map[string]bool) []*sexpr {
	kept := make([]*sexpr, 0, len(preds))
	for _, pred := range preds {
		ok := true
		for _, c := range pred.children {
			if !c.isForm() && strings.HasPrefix(c.token, "@") && !bound[strings.TrimPrefix(c.token, "@")] {
				ok = false
				break
			}
		}
		if ok {
			kept = append(kept, pred)
		}
	}
	return kept
}

// litCapture matches a capture name the compiler generated for a piece of the
// pattern that is text rather than a hole.
var litCapture = regexp.MustCompile(`_lit_\d+`)

// sameShape is a query with the parts a comparison should not turn on taken out:
// the numbers in generated capture names, which count the literals the reading
// happened to produce; the anchors, which an ellipsis is there to loosen; and
// the order of the predicates, which is the compiler's and not the pattern's.
//
// It is what lets two readings of the same pattern be asked whether they say
// the same thing. See standsForNothing.
func sameShape(sexp string) string {
	seen := map[string]string{}
	sexp = litCapture.ReplaceAllStringFunc(sexp, func(name string) string {
		if s, ok := seen[name]; ok {
			return s
		}
		s := fmt.Sprintf("_lit_%d", len(seen)+1)
		seen[name] = s
		return s
	})
	lines := strings.Split(sexp, "\n")
	form := strings.ReplaceAll(lines[0], " . ", " ")
	form = strings.ReplaceAll(form, " .)", ")")
	preds := append([]string(nil), lines[1:]...)
	sort.Strings(preds)
	return strings.Join(append([]string{form}, preds...), "\n")
}

// markerPrefix names the placeholder a body reading writes where the pattern
// wrote `$$$_`. See bodyReading: some grammars have no node a bare identifier
// can be where a list of items goes, so the ellipsis is spelled as something
// that does parse there and turned back into an ellipsis here.
const markerPrefix = "__pwrq_any_"

// dropMarked turns a reading's placeholders back into the ellipses they stood
// for. drop names the literal texts that are placeholders because a prefix
// reading put them there - `<?php` - as opposed to the ones spelled with
// markerPrefix.
//
// A placeholder compiles to a whole item - in HCL, `__pwrq_any_1__ =
// __pwrq_any_1__` becomes an (attribute ...) with two literal comparisons
// under it - and what the caller wrote was "and anything else here". So the
// item goes, and anchorChildren then reads its absence as the gap it is.
//
// Which node to replace is not named, it is found: the outermost form whose
// every capture is a placeholder is the item the placeholder spelled, because
// nothing else in the pattern reached inside it. That is why this needs no
// list of grammars and no knowledge of what HCL calls an attribute.
func dropMarked(forms []*sexpr, drop map[string]bool) []*sexpr {
	markers := map[string]bool{}
	for _, form := range forms {
		if form.head != "#eq?" || len(form.children) != 2 {
			continue
		}
		text := form.children[1].token
		if strings.Contains(text, markerPrefix) || drop[strings.Trim(text, `"`)] {
			markers[strings.TrimPrefix(form.children[0].token, "@")] = true
		}
	}
	if len(markers) == 0 {
		return forms
	}
	for _, form := range forms {
		prune(form, markers)
	}
	kept := forms[:0]
	for _, form := range forms {
		if form.head == "#eq?" && len(form.children) == 2 &&
			markers[strings.TrimPrefix(form.children[0].token, "@")] {
			continue
		}
		kept = append(kept, form)
	}
	return kept
}

// prune replaces every child that is nothing but placeholders with an
// ellipsis.
func prune(s *sexpr, markers map[string]bool) {
	for i, c := range s.children {
		if n, all := onlyMarked(c, markers); n > 0 && all {
			s.children[i] = &sexpr{head: "_", capture: ellipsisName}
			continue
		}
		prune(c, markers)
	}
}

// onlyMarked counts the captures in a subtree and says whether every one of
// them is a placeholder.
func onlyMarked(s *sexpr, markers map[string]bool) (int, bool) {
	count, all := 0, true
	if s.capture != "" {
		count++
		if !markers[s.capture] {
			all = false
		}
	}
	for _, c := range s.children {
		n, a := onlyMarked(c, markers)
		count += n
		if !a {
			all = false
		}
	}
	return count, all
}

// unitTypes are the nodes a grammar wraps a whole file in - `config_file` and
// `body` in HCL, `source_file` in Go, `module` in Python - as opposed to the
// nodes the pattern actually wrote.
//
// The two cannot be told apart by shape. `(body (block ...))` and `(list
// (identifier))` both have one child, and only one of them is scaffolding; the
// difference is that the caller typed the brackets and did not type the body.
//
// So it is measured rather than guessed, by parsing the pattern twice over.
// Doubling it forces whatever holds a file's items to hold two of them, while
// everything the caller wrote keeps the shape it had - a list of one element
// is still a list of one element. The chain of single-child nodes from the
// root down to the node that now has two is therefore exactly the scaffolding,
// and it is the same answer for all 206 grammars in this build without a table
// of any of them.
func unitTypes(lang *gotreesitter.Language, pattern string) map[string]bool {
	return scaffoldUnits(lang, "", "", "", pattern)
}

// scaffoldUnits is unitTypes for a reading that wrapped the pattern in text of
// its own. The wrapper goes around the doubled pattern rather than around each
// copy of it, so the node that ends up holding two copies is the one the
// wrapper was there to provide - a class body, a function body, a file - and
// every node from there up to the root is scaffolding by construction.
//
// before and after are the wrapper; close ends the pattern's own item, and so
// is written after each copy rather than once.
func scaffoldUnits(lang *gotreesitter.Language, before, closing, after, pattern string) map[string]bool {
	source := pattern
	if pre, _, err := grep.Preprocess(pattern); err == nil {
		source = strings.ReplaceAll(pre, "__GREP_", probePrefix)
	}
	doubled := before + source + closing + "\n" + source + closing + after
	start, end := len(before), len(doubled)-len(after)
	tree, err := parserFor(lang).Parse([]byte(doubled))
	if err != nil || tree == nil {
		return nil
	}
	bound := gotreesitter.Bind(tree)
	defer bound.Release()
	root := bound.RootNode()
	if root == nil {
		return nil
	}
	node := root.NamedDescendantForByteRange(uint32(start), uint32(end))
	// The smallest node holding both copies is the answer; a grammar that
	// hands back something narrower is walked back up until it holds them.
	for node != nil && (int(node.StartByte()) > start || int(node.EndByte()) < end) {
		node = node.Parent()
	}
	if node == nil {
		node = root
	}
	// Copied out as they are read: the nodes belong to an arena that Release
	// hands back.
	units := map[string]bool{}
	for n := node; n != nil; n = n.Parent() {
		units[strings.Clone(n.Type(lang))] = true
	}
	return units
}

// openUnit turns the whole-file node a sequence pattern compiled to back into
// the "somewhere" the caller meant by it.
//
// A pattern of more than one statement has no single node to compile to, so
// the engine gives it the node the grammar wraps a whole file in - `module`,
// `source_file`, `program`. The caller did not write that node, and every part
// of it is wrong as a claim about the code being searched:
//
//   - Its type says the statements are at the top of a file. They are usually
//     in a function body, which is a different node, so the query as compiled
//     matches nothing at all. The head becomes `_`: some node holds these.
//   - Its ends say the first statement is the file's first and the last is its
//     last. The pattern says neither, so the boundary anchors come off. The
//     anchors between the statements stay: two written with nothing between
//     them mean adjacent, and $$$_ is how a pattern says otherwise.
//   - Capturing it would report the whole enclosing block as the match. The
//     statements are captured instead, so the span runs from the first to the
//     last of them, which is the code the pattern picked out.
func openUnit(s *sexpr) *sexpr {
	// One child left is not a sequence. It is a single construct with the
	// grammar's wrapper around it, which is what a reading that had to add
	// something leaves behind once the addition is dropped: `<?php md5($X);`
	// compiles to a program holding a tag and a statement, and with the tag
	// gone the pattern is the call and nothing else. Keeping the wrapper would
	// make the pattern a claim about where in the file the call sits - a
	// statement of its own - so `$h = md5($n)` would not match it.
	if len(s.children) == 1 {
		node := s.children[0]
		// Descend through wrappers, and stop at the first thing the caller
		// wrote. A hole is something the caller wrote: `echo $X` is an echo
		// statement holding one child, and descending into the child would
		// leave a query for the hole alone, which matches every node in every
		// file.
		for len(node.children) == 1 {
			child := node.children[0]
			if !child.isForm() || child.capture != "" || child.field != "" || child.head == "_" {
				break
			}
			node = child
		}
		node.anchored = false
		node.anchoredEnd = false
		if node.capture == "" {
			node.capture = rootCapture
		}
		return node
	}
	s.head = "_"
	if len(s.children) > 0 {
		s.children[0].anchored = false
	}
	s.anchoredEnd = false
	for _, c := range s.children {
		if c.capture == "" {
			c.capture = rootCapture
		}
	}
	return s
}

// bindRepeats makes a hole the pattern wrote twice mean the same code twice,
// and returns the predicate forms that say so.
//
// `$X == $X` compiles to `(binary_expression left: (_) @X operator: "==" right:
// (_) @X)`, and one capture name on two nodes does not require the two to be
// the same: the engine binds whichever it saw last, so the pattern matches
// `a == b`. The pattern was refused for that reason, and a caller who wanted
// it had to name the holes apart and compare the captures afterwards.
//
// A tree-sitter query can say it directly. Renaming the later occurrences and
// adding `(#eq? @X @X__pwrq_again_2)` compares the text the two captured,
// which is what a repeated hole means - and it is what half the corpus this
// was ported against writes: `$D = request.args`, then `foo($D)`, the same
// variable in both places.
//
// The anonymous variadic is exempt. Every `$$$_` compiles to `@_`, and two of
// them were never a claim that the two runs are equal.
func bindRepeats(root *sexpr) []*sexpr {
	seen := map[string]int{}
	var preds []*sexpr
	var walk func(*sexpr)
	walk = func(n *sexpr) {
		if c := n.capture; c != "" && c != ellipsisName && c != rootCapture {
			seen[c]++
			if nth := seen[c]; nth > 1 {
				n.capture = fmt.Sprintf("%s%s%d", c, againSuffix, nth)
				preds = append(preds, &sexpr{
					head:     "#eq?",
					children: []*sexpr{{token: "@" + c}, {token: "@" + n.capture}},
				})
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
	return preds
}

// anchorChildren walks a form and anchors every list of siblings that the
// pattern spelled out in full.
//
// Only a form whose children are all unlabelled node patterns is touched. A
// label - `arguments:`, `left:` - already pins a child to one position, so
// anchoring adds nothing there; and a form that mixes labelled and unlabelled
// children is one this parser has no ordering story for, so it is left alone.
// Predicates are left alone for the same reason.
func anchorChildren(s *sexpr) {
	for _, c := range s.children {
		anchorChildren(c)
	}
	if strings.HasPrefix(s.head, "#") || len(s.children) == 0 {
		return
	}
	for _, c := range s.children {
		if c.field != "" || !c.isForm() {
			return
		}
	}

	kept := make([]*sexpr, 0, len(s.children))
	// gap records that an ellipsis stood where the next anchor would go.
	gap := s.children[0].isEllipsis()
	for _, c := range s.children {
		if c.isEllipsis() {
			gap = true
			continue
		}
		c.anchored = !gap
		gap = false
		kept = append(kept, c)
	}
	s.anchoredEnd = len(kept) > 0 && !gap
	s.children = kept
}

// render turns a form back into query text.
func (s *sexpr) render() string {
	var b strings.Builder
	if s.field != "" {
		b.WriteString(s.field)
		b.WriteString(": ")
	}
	if !s.isForm() {
		b.WriteString(s.token)
		return b.String()
	}
	b.WriteString("(")
	b.WriteString(s.head)
	for _, c := range s.children {
		b.WriteString(" ")
		if c.anchored {
			b.WriteString(". ")
		}
		b.WriteString(c.render())
	}
	if s.anchoredEnd {
		b.WriteString(" .")
	}
	b.WriteString(")")
	if s.capture != "" {
		b.WriteString(" @")
		b.WriteString(s.capture)
	}
	return b.String()
}

// parseSexp reads the query text the engine produced: one node pattern
// followed by any number of predicate forms.
func parseSexp(s string) ([]*sexpr, error) {
	p := &sexpParser{src: s}
	var forms []*sexpr
	for {
		p.skipSpace()
		if p.done() {
			return forms, nil
		}
		if p.src[p.i] != '(' {
			return nil, fmt.Errorf("expected a form at offset %d", p.i)
		}
		form, err := p.form()
		if err != nil {
			return nil, err
		}
		p.attachCapture(form)
		forms = append(forms, form)
	}
}

type sexpParser struct {
	src string
	i   int
}

func (p *sexpParser) done() bool { return p.i >= len(p.src) }

func (p *sexpParser) skipSpace() {
	for p.i < len(p.src) && (p.src[p.i] == ' ' || p.src[p.i] == '\n' || p.src[p.i] == '\t' || p.src[p.i] == '\r') {
		p.i++
	}
}

// attachCapture consumes a trailing `@name`, which follows the form it binds.
func (p *sexpParser) attachCapture(form *sexpr) {
	save := p.i
	p.skipSpace()
	if p.i < len(p.src) && p.src[p.i] == '@' {
		p.i++
		start := p.i
		for p.i < len(p.src) && isWordByte(p.src[p.i]) {
			p.i++
		}
		if p.i > start {
			form.capture = p.src[start:p.i]
			return
		}
	}
	p.i = save
}

// form reads one parenthesised node pattern.
func (p *sexpParser) form() (*sexpr, error) {
	open := p.i
	p.i++ // the '('
	s := &sexpr{}
	start := p.i
	for p.i < len(p.src) && !isDelimiter(p.src[p.i]) {
		p.i++
	}
	s.head = p.src[start:p.i]

	for {
		p.skipSpace()
		if p.done() {
			return nil, fmt.Errorf("unclosed form at offset %d", open)
		}
		if p.src[p.i] == ')' {
			p.i++
			return s, nil
		}
		if p.src[p.i] == '.' {
			// An anchor the engine already wrote. Keeping it as a token would
			// make this form look like one with children this parser cannot
			// order, so it is dropped and re-derived.
			p.i++
			continue
		}
		field := p.fieldLabel()
		p.skipSpace()
		if p.done() {
			return nil, fmt.Errorf("unclosed form at offset %d", open)
		}
		var child *sexpr
		if p.src[p.i] == '(' {
			var err error
			if child, err = p.form(); err != nil {
				return nil, err
			}
			p.attachCapture(child)
		} else {
			tok, err := p.token()
			if err != nil {
				return nil, err
			}
			child = &sexpr{token: tok}
		}
		child.field = field
		s.children = append(s.children, child)
	}
}

// fieldLabel consumes a `name:` label if one is next, and returns "" if not.
func (p *sexpParser) fieldLabel() string {
	start := p.i
	for p.i < len(p.src) && isWordByte(p.src[p.i]) {
		p.i++
	}
	if p.i > start && p.i < len(p.src) && p.src[p.i] == ':' {
		label := p.src[start:p.i]
		p.i++
		return label
	}
	p.i = start
	return ""
}

// token reads a quoted string or a bare word, keeping it as written.
func (p *sexpParser) token() (string, error) {
	start := p.i
	if p.src[p.i] == '"' {
		p.i++
		for p.i < len(p.src) {
			if p.src[p.i] == '\\' {
				p.i += 2
				continue
			}
			if p.src[p.i] == '"' {
				p.i++
				return p.src[start:p.i], nil
			}
			p.i++
		}
		return "", fmt.Errorf("unterminated string at offset %d", start)
	}
	for p.i < len(p.src) && !isDelimiter(p.src[p.i]) {
		p.i++
	}
	if p.i == start {
		return "", fmt.Errorf("nothing readable at offset %d", start)
	}
	return p.src[start:p.i], nil
}

// isDelimiter reports the bytes that end a head or a bare word.
func isDelimiter(c byte) bool {
	return c == ' ' || c == '\n' || c == '\t' || c == '\r' || c == '(' || c == ')'
}

func isWordByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// repeatedCapture returns the first capture name a query binds to more than
// one node, or "" when each name is on one. It is what bindRepeats is checked
// against: a name still on two nodes after the rewrite is a pattern whose
// repeated hole means nothing.
func repeatedCapture(sexp string) string {
	forms, err := parseSexp(sexp)
	if err != nil {
		return ""
	}
	seen := map[string]bool{}
	var found string
	var walk func(*sexpr)
	walk = func(n *sexpr) {
		if c := n.capture; c != "" && c != ellipsisName && c != rootCapture && found == "" {
			if seen[c] {
				found = c
			}
			seen[c] = true
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	for _, form := range forms {
		if strings.HasPrefix(form.head, "#") {
			continue
		}
		walk(form)
	}
	return found
}

// siblingVariadic returns the first named $$$NAME a pattern put beside other
// children, or "" if it put none there.
//
// Alone in a list - `f($$$ARGS)` - a named variadic is what the caller thinks
// it is: the engine binds it to the whole list. Beside a sibling the engine
// compiles it to a single node, and the pattern then means "one more argument"
// rather than "the rest of them", quietly. Refusing it is the only honest
// answer, because a capture is one node and a run of them has nothing to be
// bound to.
func siblingVariadic(sexp string, variadic map[string]bool) string {
	if len(variadic) == 0 {
		return ""
	}
	forms, err := parseSexp(sexp)
	if err != nil {
		return ""
	}
	for _, form := range forms {
		if name := findSiblingVariadic(form, variadic); name != "" {
			return name
		}
	}
	return ""
}

func findSiblingVariadic(s *sexpr, variadic map[string]bool) string {
	// Only children the grammar left unlabelled compete for a position. A
	// labelled one - `body:`, `condition:` - is pinned to its field however
	// many children sit beside it, so `for $$$H { $$$B }` is fine: the header
	// is one node in a slot of its own.
	var unlabelled []*sexpr
	for _, c := range s.children {
		if c.field == "" && c.isForm() {
			unlabelled = append(unlabelled, c)
		}
	}
	if len(unlabelled) > 1 {
		for _, c := range unlabelled {
			if variadic[c.capture] {
				return c.capture
			}
		}
	}
	for _, c := range s.children {
		if name := findSiblingVariadic(c, variadic); name != "" {
			return name
		}
	}
	return ""
}

// probePrefix replaces the engine's own reserved prefix in a probe pattern.
// The engine rewrites `$NAME` to `__GREP_CAP_NAME__` before parsing, and
// refuses a pattern that already contains that prefix, so a probe renames it
// to something it will accept. See bubbleDepths.
const probePrefix = "pwrqProbe_"

// bubbleDepths reports, for each hole the caller wrote as $NAME, how many
// levels of grammar the engine folded away when it decided what to capture.
//
// The engine binds a hole to the outermost node whose whole subtree is that
// hole. In `f($P)` the argument list's whole subtree is $P, so $P is bound to
// the argument list - parentheses included, and matching a call with any
// number of arguments rather than the one the pattern wrote. Both are silent:
// `f($P)` reports hits on `f(1, 2, 3)`, and .Captures.P reads `(1, 2, 3)`, so
// a rule that filters on what a hole caught filters on the wrong text.
//
// The fold is invisible in the finished query - `(_) @P` says nothing about
// what it replaced - so it is measured rather than guessed at. Compiling the
// pattern a second time with each hole written as an ordinary identifier
// produces the structure the grammar really has, and the extra depth down to
// that identifier is what the fold removed. Nothing about the grammar is
// assumed; the two queries are compared and the difference is the answer.
//
// It returns nil when the comparison cannot be made, which leaves the pattern
// as the engine compiled it.
func bubbleDepths(lang *gotreesitter.Language, pattern string, query *grep.CompiledPattern) map[string][]int {
	pre, mvars, err := grep.Preprocess(pattern)
	if err != nil || len(mvars) == 0 {
		return nil
	}
	probe, err := grep.Compile(lang, strings.ReplaceAll(pre, "__GREP_", probePrefix))
	if err != nil || strings.Contains(probe.SExpr, errorNode) {
		return nil
	}

	captured := captureDepths(query.SExpr)
	if captured == nil {
		return nil
	}
	probed := literalDepths(probe.SExpr)
	if probed == nil {
		return nil
	}

	out := map[string][]int{}
	for _, mv := range mvars {
		if mv == nil || mv.Variadic || mv.Wildcard || mv.Name == "" {
			continue
		}
		here := captured[mv.Name]
		there := probed[strings.Replace(mv.Placeholder, "__GREP_", probePrefix, 1)]
		// A hole written more than once is folded by however much the grammar
		// folded it at each place it was written, which is not the same number
		// at each: `$D = x` binds $D to an identifier and `foo($D)` binds it
		// to the argument list around one. The two queries name the
		// occurrences in the same order, so they are paired off in that order.
		n := min(len(here), len(there))
		if n == 0 {
			continue
		}
		depths := make([]int, n)
		for i := range depths {
			if d := there[i] - here[i]; d > 0 {
				depths[i] = d
			}
		}
		out[mv.Name] = depths
	}
	return out
}

// captureDepths maps each capture name to how deep each of its occurrences
// sits, in the order the query names them.
func captureDepths(sexp string) map[string][]int {
	forms, err := parseSexp(sexp)
	if err != nil {
		return nil
	}
	out := map[string][]int{}
	for _, form := range forms {
		if strings.HasPrefix(form.head, "#") {
			continue
		}
		walkDepth(form, 0, func(n *sexpr, depth int) {
			if n.capture != "" {
				out[n.capture] = append(out[n.capture], depth)
			}
		})
	}
	return out
}

// literalDepths maps the source text each literal capture stands for to how
// deep that capture sits, which is what the probe query is read for.
func literalDepths(sexp string) map[string][]int {
	forms, err := parseSexp(sexp)
	if err != nil {
		return nil
	}
	// The text a literal capture must equal is written in a predicate, apart
	// from the capture itself: (#eq? @_lit_1 "os").
	text := map[string]string{}
	for _, form := range forms {
		if form.head != "#eq?" || len(form.children) != 2 {
			continue
		}
		name := strings.TrimPrefix(form.children[0].token, "@")
		quoted := form.children[1].token
		if len(quoted) < 2 {
			continue
		}
		text[name] = strings.ReplaceAll(quoted[1:len(quoted)-1], `\"`, `"`)
	}

	out := map[string][]int{}
	for _, form := range forms {
		if strings.HasPrefix(form.head, "#") {
			continue
		}
		walkDepth(form, 0, func(n *sexpr, depth int) {
			if lit, ok := text[n.capture]; ok {
				out[lit] = append(out[lit], depth)
			}
		})
	}
	return out
}

func walkDepth(n *sexpr, depth int, visit func(*sexpr, int)) {
	visit(n, depth)
	for _, c := range n.children {
		if c.isForm() {
			walkDepth(c, depth+1, visit)
		}
	}
}

// unfold puts back the levels the engine folded away, so that a hole binds the
// node the pattern wrote rather than the list that held it.
//
// A level is `(_ . x .)`: some node, whatever the grammar calls it, holding
// exactly x and nothing else. That is the whole of what was lost - the fold
// dropped both the wrapper and the fact that the wrapper had one child - and
// putting it back is what makes `f($P)` mean a call with one argument and
// .Captures.P read that argument.
func unfold(s *sexpr, depth map[string][]int) {
	// The depths are per occurrence, so the nodes are numbered the way
	// captureDepths numbered them - by capture name, in query order - before
	// any of them is rewritten. Rewriting as we walk would renumber the rest.
	want := map[*sexpr]int{}
	seen := map[string]int{}
	var number func(n *sexpr)
	number = func(n *sexpr) {
		if n.capture != "" {
			nth := seen[n.capture]
			seen[n.capture]++
			if ds := depth[n.capture]; nth < len(ds) && ds[nth] > 0 {
				want[n] = ds[nth]
			}
		}
		for _, c := range n.children {
			if c.isForm() {
				number(c)
			}
		}
	}
	number(s)
	putBack(s, want)
}

// putBack wraps each hole in the levels the fold removed from it.
func putBack(s *sexpr, want map[*sexpr]int) {
	for _, c := range s.children {
		if c.isForm() {
			putBack(c, want)
		}
	}
	for i, c := range s.children {
		// A hole compiles to a leaf - `(_)` for an untyped one, `(string)`
		// for `$S:string` - so a form with children of its own is structure
		// the pattern wrote and not a hole to put a level back around.
		d, ok := want[c]
		if !ok || !c.isForm() || len(c.children) > 0 {
			continue
		}
		field := c.field
		c.field = ""
		c.anchored = true
		inner := c
		for ; d > 0; d-- {
			inner = &sexpr{head: "_", children: []*sexpr{inner}, anchoredEnd: true}
		}
		inner.field = field
		s.children[i] = inner
	}
}
