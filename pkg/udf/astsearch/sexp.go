package astsearch

import (
	"fmt"
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
func anchorQuery(sexp string, folded map[string]int) string {
	forms, err := parseSexp(sexp)
	if err != nil || len(forms) == 0 {
		return sexp
	}
	for _, form := range forms {
		if strings.HasPrefix(form.head, "#") {
			continue
		}
		unfold(form, folded)
		anchorChildren(form)
	}
	// The first form is the pattern; the rest are (#eq? ...) predicates.
	if forms[0].capture == "" {
		forms[0].capture = rootCapture
	}
	parts := make([]string, len(forms))
	for i, form := range forms {
		parts[i] = form.render()
	}
	return strings.Join(parts, "\n")
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
func bubbleDepths(lang *gotreesitter.Language, pattern string, query *grep.CompiledPattern) map[string]int {
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

	out := map[string]int{}
	for _, mv := range mvars {
		if mv == nil || mv.Variadic || mv.Wildcard || mv.Name == "" {
			continue
		}
		here, ok := captured[mv.Name]
		if !ok {
			continue
		}
		there, ok := probed[strings.Replace(mv.Placeholder, "__GREP_", probePrefix, 1)]
		if !ok {
			continue
		}
		if d := there - here; d > 0 {
			out[mv.Name] = d
		}
	}
	return out
}

// captureDepths maps each capture name to how deep in the query it sits.
func captureDepths(sexp string) map[string]int {
	forms, err := parseSexp(sexp)
	if err != nil {
		return nil
	}
	out := map[string]int{}
	for _, form := range forms {
		if strings.HasPrefix(form.head, "#") {
			continue
		}
		walkDepth(form, 0, func(n *sexpr, depth int) {
			if n.capture != "" {
				out[n.capture] = depth
			}
		})
	}
	return out
}

// literalDepths maps the source text each literal capture stands for to how
// deep that capture sits, which is what the probe query is read for.
func literalDepths(sexp string) map[string]int {
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

	out := map[string]int{}
	for _, form := range forms {
		if strings.HasPrefix(form.head, "#") {
			continue
		}
		walkDepth(form, 0, func(n *sexpr, depth int) {
			if lit, ok := text[n.capture]; ok {
				out[lit] = depth
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
func unfold(s *sexpr, depth map[string]int) {
	for _, c := range s.children {
		unfold(c, depth)
	}
	for i, c := range s.children {
		// A hole compiles to a leaf - `(_)` for an untyped one, `(string)`
		// for `$S:string` - so a form with children of its own is structure
		// the pattern wrote and not a hole to put a level back around.
		d, ok := depth[c.capture]
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
