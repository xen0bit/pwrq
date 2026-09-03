// Package astsearch searches source code by its syntax rather than its text.
package astsearch

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/grep"
)

// A pattern is code with holes in it. `func $NAME($$$ARGS) error` is a
// pattern, and it matches every Go function returning an error, however it is
// spaced, wrapped or commented - because the match is against the parse tree,
// not the bytes.
//
// This file is the part that decides whether a pattern means anything, which
// the engine underneath does not.

// errorNode is what tree-sitter calls the node it produces where the grammar
// could not follow the input.
const errorNode = "(ERROR"

// capturePrefix is what the engine rewrites `$NAME` into so a pattern with
// holes in it still parses as code. Finding it in the finished query means the
// rewrite was never undone. See patternProblem.
const capturePrefix = "__GREP_CAP_"

// literalPredicate matches the `(#eq? @_lit_1 "...")` clauses the compiler
// emits for the parts of a pattern that are text rather than holes.
var literalPredicate = regexp.MustCompile(`\(#eq\? @[A-Za-z0-9_]+ "((?:[^"\\]|\\.)*)"\)`)

// oneWord matches a pattern that is a single bare identifier. For those - and
// only those - a query that is one text comparison is the search the caller
// asked for rather than a symptom of one that failed. See patternProblem.
var oneWord = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// compiled is a pattern that has been turned into a tree-sitter query, along
// with the two facts a caller needs about it: whether it can match code at
// all, and what it became.
type compiled struct {
	pattern  string
	language string
	// queries are the readings of the pattern the grammar allows, most likely
	// first. There is usually one; see statementReading for the language where
	// the same characters mean two things.
	queries []*grep.CompiledPattern
	// problem is empty when the pattern is structural code in this language,
	// and otherwise says what went wrong, in the caller's terms.
	problem string
}

// query is the reading a caller is shown and asked about. A pattern that
// would not compile has none.
func (c *compiled) query() *grep.CompiledPattern {
	if len(c.queries) == 0 {
		return nil
	}
	return c.queries[0]
}

// valid reports whether the pattern can match code at all.
func (c *compiled) valid() bool { return c.problem == "" }

// compiledPatterns is every pattern this process has compiled, keyed by the
// pattern and the language it was compiled for.
//
// Compiling is expensive - the grammar is instantiated, the pattern parsed,
// and up to two readings of it checked against the tree they produce, which is
// several milliseconds and megabytes for one pattern - and the same handful of
// patterns is compiled over and over: a search compiles its patterns once per
// language it meets, a rule compiles the patterns it names on every run, and a
// corpus of rules shares patterns between them.
//
// A compiled pattern is read-only once it is returned, and grep executes a
// query without writing to it, so one copy answers for every caller. This is
// the same bargain the readings cache in tree.go already makes.
var compiledPatterns sync.Map // language + "\x00" + pattern -> compileResult

// compileResult is what one compilation produced, error included: a pattern
// that names a language this build does not have fails the same way every
// time, and re-deciding that is the cost the cache exists to avoid.
type compileResult struct {
	c   *compiled
	err error
}

// compilePattern turns a pattern into a query against a named language.
func compilePattern(pattern, language string) (*compiled, error) {
	key := language + "\x00" + pattern
	if cached, ok := compiledPatterns.Load(key); ok {
		r := cached.(compileResult)
		return r.c, r.err
	}
	c, err := compilePatternUncached(pattern, language)
	compiledPatterns.Store(key, compileResult{c: c, err: err})
	return c, err
}

func compilePatternUncached(pattern, language string) (*compiled, error) {
	entry := grammars.DetectLanguageByName(language)
	if entry == nil {
		return nil, fmt.Errorf("no grammar for language %q in this build; get_ast_language lists the %d it has",
			language, len(grammars.AllLanguages()))
	}
	query, err := grep.Compile(entry.Language(), pattern)
	// anchored is the plain reading, finished, so that it can be asked whether
	// it is the pattern before it is offered as the pattern.
	var anchored *grep.CompiledPattern
	// misreason is what to say if no other reading is found either. A reading
	// that parses but is not the pattern has to be refused with a reason of
	// its own, because patternProblem sees nothing wrong with it.
	var misreason string
	misread := err != nil || patternProblem(pattern, entry.Name, query.SExpr) != ""
	if !misread {
		anchored = query
		if a, aerr := anchorPattern(entry.Language(), pattern, query, nil); aerr == nil {
			anchored = a
		}
		// A reading that parses is not yet a reading that is right. An
		// ellipsis standing where an item goes says nothing in particular is
		// there, and a grammar that took the item below it into the ellipsis
		// produces a query with no error in it and the wrong shape. See
		// standsForNothing.
		if spots := bodyEllipses(pattern); len(spots) > 0 &&
			!(scaffold{}).standsForNothing(entry.Language(), pattern, spots, entry.Name, anchored) {
			misread = true
			misreason = fmt.Sprintf("pattern %q is read by the %s grammar with the item below an "+
				"ellipsis taken into it, so the query is a search for something the pattern does "+
				"not say; ast_pattern shows what it compiled to", pattern, entry.Name)
		}
		// A query that names no construct and compares no text matches every
		// node in every file, which no pattern means. `return $$$_\n$S` is one:
		// Python folds the two lines into a single hole and the query becomes
		// `(_) @S`. It is the silent-empty-result failure inverted, and it is
		// worse, because a rule built on it reports everything.
		if !misread && specificity(anchored.SExpr) == 0 {
			misread = true
			misreason = fmt.Sprintf("pattern %q compiled to a query that names no construct and "+
				"compares no text, so it would match every node in every file; ast_pattern shows "+
				"what it compiled to", pattern)
		}
	}
	// A pattern the grammar cannot follow gets a second reading before it is
	// refused, because a refusal here is a rule that will never run. See
	// scaffoldReading: the ellipsis standing where a list of items goes, and
	// the statement with nowhere to stand, are between them two thirds of what
	// a rule corpus writes that no grammar would take.
	if misread {
		if scaffolded := scaffoldReading(entry.Language(), pattern, entry.Name); scaffolded != nil {
			return finish(&compiled{
				pattern:  pattern,
				language: entry.Name,
				queries:  []*grep.CompiledPattern{scaffolded},
			}, entry.Language(), pattern), nil
		}
		if word := wordReading(entry.Language(), pattern); word != nil {
			return &compiled{
				pattern:  pattern,
				language: entry.Name,
				queries:  []*grep.CompiledPattern{word},
			}, nil
		}
	}
	if err != nil {
		// A pattern that will not compile for this language is not an error,
		// for the same reason a pattern that is not code in it is not: a
		// search runs over a tree, and a tree is written in several languages.
		// `$S:string` names a node type JavaScript has and Java does not, so
		// compiling it for Java fails - and a JavaScript rule must not die on
		// the Java beside it. Whether the pattern is code in anything at all
		// is decided once, at the end, by exhausted.
		// A pattern of several lines is the usual way to get here: where the
		// grammar cannot read one of the lines as a statement it folds the
		// rest of them into one piece of text, and a piece of text with a
		// newline in it is not a query the engine can even parse.
		lines := ""
		if strings.Contains(strings.TrimSpace(pattern), "\n") {
			lines = "; the pattern spans several lines, and a grammar that cannot follow one " +
				"of them reads the rest as text rather than as statements"
		}
		return &compiled{
			pattern:  pattern,
			language: entry.Name,
			problem: fmt.Sprintf("pattern %q will not compile for %s: %v%s", pattern,
				entry.Name, err, lines),
		}, nil
	}
	c := &compiled{
		pattern:  pattern,
		language: entry.Name,
		queries:  []*grep.CompiledPattern{query},
		problem:  patternProblem(pattern, entry.Name, query.SExpr),
	}
	if c.problem == "" && misreason != "" {
		c.problem = misreason
	}
	if !c.valid() {
		return c, nil
	}
	if name := siblingVariadic(query.SExpr, variadicNames(query.MetaVars)); name != "" {
		c.problem = fmt.Sprintf("pattern %q writes $$$%s beside other children, and there it "+
			"matches one node rather than the rest of them: a capture holds a single node, so "+
			"there is nothing for a run of them to be bound to. Write $$$_ where you mean "+
			"\"and anything else here\" - `f($A, $$$_)` - and name the children you need",
			pattern, name)
		return c, nil
	}
	c.queries[0] = anchored
	return finish(c, entry.Language(), pattern), nil
}

// finish is the part of compiling a pattern that is the same whichever reading
// of it the grammar accepted: the checks that a query says what the pattern
// said, and the second reading a statement terminator can add.
func finish(c *compiled, lang *gotreesitter.Language, pattern string) *compiled {
	// A hole written twice is bound to one thing by bindRepeats. If the name
	// is still on two nodes the rewrite did not happen - the query was a shape
	// this build could not parse - and the pattern would match wherever the
	// shape fits whatever the text is, which is not what it says.
	if name := repeatedCapture(c.query().SExpr); name != "" {
		c.problem = fmt.Sprintf("pattern %q uses $%s more than once and the query could not be "+
			"rewritten to require the two to be the same thing, so it would match wherever the "+
			"shape fits whatever the text is - `$X == $X` matching `a == b`. Give each hole its "+
			"own name and compare what they caught afterwards, with "+
			"select(.Captures.A == .Captures.B)", pattern, name)
		return c
	}
	if stmt := statementReading(lang, pattern, c.query()); stmt != nil {
		// The terminated reading is a reading like any other, so it answers
		// for the pattern's ellipses like any other. Java takes a whole method
		// declaration with a semicolon after it and reads the body's ellipsis
		// as part of the statement below it, which parses and is not what was
		// written.
		spots := bodyEllipses(pattern)
		if len(spots) == 0 ||
			(scaffold{closing: ";"}).standsForNothing(lang, pattern, spots, c.language, stmt) {
			c.queries = append([]*grep.CompiledPattern{stmt}, c.queries...)
		}
	}
	c.queries = append(c.queries, memberReadings(lang, pattern, c.language, c.query())...)
	c.queries = unwrapTopLevel(lang, pattern, c.language, c.queries)
	// Two readings that compiled to the same query are one reading. They
	// happen where a difference the grammar made before anchoring is gone
	// after it - Java reads `new C($X)` and `new C($X);` differently and
	// anchors them to the same thing - and matching under both costs a pass
	// over the tree to find what the first pass found. See select_ast, which
	// takes the same span twice as one finding.
	c.queries = distinctReadings(c.queries)
	return c
}

// distinctReadings drops the readings that are the same query as an earlier
// one, keeping the order they were offered in.
func distinctReadings(queries []*grep.CompiledPattern) []*grep.CompiledPattern {
	seen := map[string]bool{}
	out := queries[:0]
	for _, q := range queries {
		if q == nil || seen[q.SExpr] {
			continue
		}
		seen[q.SExpr] = true
		out = append(out, q)
	}
	return out
}

// memberReadings are the pattern read as a member of a type rather than as a
// statement - one reading per name a grammar has for it - and empty where a
// grammar spells them all the same way.
//
// Java spells them differently and gives no sign of it. `String $F = $E;`
// parses standing alone, because tree-sitter-java takes a bare statement at the
// top of a file, so it compiles cleanly on the first reading to a
// `local_variable_declaration` and no scaffold is ever reached for. The
// identical text inside a class body is a `field_declaration`: a different node
// type with the same children, and a query for one does not match the other.
//
// The cost of that was not a rule that fired in the wrong place, it was a rule
// that could not be written.
//
//	private static final String PASSWORD = "hunter2";
//
// is the shape every hardcoded-credential rule in the world is about, and no
// pattern in this engine could name it. Two rules in the Java corpus said so in
// their headers instead, which is the right thing to do about a limit and a bad
// thing to leave standing.
//
// The other names are measured rather than listed. The pattern is read again
// inside each of memberHolders, and where the same children come back under a
// different node type, that type is another spelling of the same construct. A grammar
// that calls it the same thing in both places produces the same head and gets
// no second reading, which is what every language here that is not shaped like
// Java does - and it is why this needs no table of the 206 grammars in this
// build, in keeping with unitTypes.
//
// Only the head is taken from the second reading. Everything after it - the
// children, the captures, the text comparisons - comes from the first, because
// the two are the same by construction and the first one's capture numbering is
// what its own predicates already refer to.
//
// A pattern of more than one statement is not read this way. A class body holds
// members and a member is one item, so the question does not arise, and asking
// it anyway would compile every multi-statement pattern in the corpus a second
// time for nothing.
func memberReadings(lang *gotreesitter.Language, pattern, language string,
	query *grep.CompiledPattern) []*grep.CompiledPattern {
	if query == nil || strings.Contains(strings.TrimSpace(pattern), "\n") {
		return nil
	}
	head, rest, ok := splitHead(query.SExpr)
	if !ok {
		return nil
	}
	// The language-level gate, and it is what keeps this off the hot path.
	// Compiling a corpus asks this of tens of thousands of patterns, and for
	// all but a handful the answer is "the same thing" - so the answer is
	// worked out once per language, from a declaration nobody wrote, and a
	// pattern that is not a declaration in that language never reaches the
	// parse below. Without it the whole test suite ran a tenth slower for
	// nothing.
	if !renamesDeclarations(lang, language, head) {
		return nil
	}
	var out []*grep.CompiledPattern
	seen := map[string]bool{head: true}
	for _, keyword := range memberHolders {
		// The cheap question first. Compiling a reading costs a parse, an
		// anchoring pass and half a dozen checks, and asking it of every
		// pattern in a corpus for the answer "the same thing" made a run of
		// the whole corpus a third slower. Parsing the pattern inside the
		// keyword answers "does this grammar call it something else in here"
		// on its own, in microseconds, and is the only question that decides
		// whether the rest of this is worth doing.
		if named := memberType(lang, keyword, pattern); named == "" || seen[named] {
			continue
		}
		inside := (scaffold{before: keyword + " " + scaffoldName + " {\n", after: "\n}"}).
			read(lang, pattern, pattern, language)
		if inside == nil {
			continue
		}
		other := otherHead(inside.SExpr, seen, rest)
		if other == "" {
			continue
		}
		seen[other] = true
		sexp := "(" + other + strings.TrimPrefix(query.SExpr, "("+head)
		q, err := gotreesitter.NewQuery(sexp, lang)
		if err != nil {
			continue
		}
		out = append(out, &grep.CompiledPattern{
			Query: q, MetaVars: query.MetaVars, Lang: lang, SExpr: sexp,
		})
	}
	return out
}

// memberHolders are the things that hold members rather than statements.
//
// They are not in the scaffolds table, and must not be: that table decides
// which reading a pattern that would not otherwise parse *becomes*, and adding
// to it would change patterns in every language. These are asked a narrower
// question - what does this grammar call the same text in here - and the answer
// is only ever used to swap a node type onto children that already matched.
//
// A grammar with no such keyword produces no reading for either and is
// unaffected. `interface` earns its place beside `class` because a constant in
// an interface is a third node type again - Java calls it a
// constant_declaration - and it is where a certain vintage of Java program
// keeps the strings this rule corpus most wants to find.
var memberHolders = []string{"class", "interface"}

// declarationProbe is a declaration with an initialiser, written in names no
// program uses, for asking a grammar what it calls one.
const declarationProbe = probePrefix + "type " + probePrefix + "name = 1;"

// renamed is what one language does to a declaration: what it calls one
// standing alone, and whether it calls it something else inside anything that
// holds members.
type renamed struct {
	local string
	elsew bool
}

// renamedIn is renamed per language, worked out once.
var renamedIn sync.Map

// renamesDeclarations reports whether this grammar gives a declaration a
// different name inside a class or an interface, and whether the pattern in
// hand is a declaration in the first place.
//
// It is a gate on cost rather than on correctness - everything it lets through
// is still checked child by child by otherHead. What it buys is that a
// language which names a declaration the same everywhere, or has no class at
// all, is asked once instead of once per pattern.
//
// The scope it fixes is worth being explicit about: the question is asked of a
// declaration, because a declaration is the construct grammars rename by
// position. A grammar that renames something else - a call, a block - inside a
// class body would not be noticed, and that is a deliberate limit rather than
// an oversight. Java renames a declaration three ways and nothing else, which
// is the case this exists for.
func renamesDeclarations(lang *gotreesitter.Language, language, head string) bool {
	cached, ok := renamedIn.Load(language)
	if !ok {
		r := renamed{local: memberType(lang, "", declarationProbe)}
		for _, keyword := range memberHolders {
			if named := memberType(lang, keyword, declarationProbe); named != "" && named != r.local {
				r.elsew = true
			}
		}
		renamedIn.Store(language, r)
		cached = r
	}
	r := cached.(renamed)
	return r.elsew && r.local != "" && head == r.local
}

// bodyScaffolds are the places a pattern is normally written that are not the
// top of a file: inside a function, and inside something that holds members.
// Each carries the skeleton of its own wrapper, for holderType.
//
// The terminated variant of the function body earns its place on its own: C#
// takes `new SqlCommand($Q, $C)` as a whole statement at file scope but not
// inside a block, where an expression needs its semicolon, so without it every
// bare-expression pattern in the corpus loses the only reading that could
// match.
//
// The class body is there for the patterns that are a member rather than a
// statement - `public $T $M($$$_, string $ARG, $$$_) { $$$_ }`, which a rule
// writes to say "a method taking a string" - because a method declaration has
// nowhere to stand inside a function.
var bodyScaffolds = []struct {
	sc       scaffold
	skeleton string
}{
	{scaffold{before: "void " + scaffoldFunc + "() {\n", after: "\n}"},
		"void " + scaffoldFunc + "() {}"},
	{scaffold{before: "void " + scaffoldFunc + "() {\n", closing: ";", after: "\n}"},
		"void " + scaffoldFunc + "() {}"},
	{scaffold{before: "class " + scaffoldName + " {\n", after: "\n}"},
		"class " + scaffoldName + " {}"},
}

// startsWhereThePatternDoes reports whether a reading anchors to something that
// begins where the pattern begins.
//
// Anchoring descends past the nodes a grammar wraps a construct in, which is
// what makes `Console.WriteLine($X);` a query about the call rather than about
// a statement. It has no way to tell those apart from the nodes that *say*
// something, and in C# it descended past two that do:
//
//	throw new $E($$$_);          ->  (object_creation_expression ...)
//	using ($T $V = $E) { $$$_ }  ->  (variable_declaration ...)
//
// The first is a search for every `new` in the program and the second for every
// declaration, both reported as valid. That is worse than the reading they
// replaced, which merely matched nothing.
//
// What the two have in common is a keyword the reading dropped, and a keyword
// is text at the front of the pattern. So the test is where the node starts:
// `throw_statement` begins where the pattern does and `object_creation_expression`
// begins six characters later, while `expression_statement` and the
// `invocation_expression` inside it begin in the same place, which is why
// descending past the one is right and past the other is not.
func startsWhereThePatternDoes(lang *gotreesitter.Language, sc scaffold, pattern, head string) bool {
	source := pattern
	if pre, _, err := grep.Preprocess(pattern); err == nil {
		source = strings.ReplaceAll(pre, "__GREP_", probePrefix)
	}
	tree, err := parserFor(lang).Parse([]byte(sc.before + source + sc.closing + sc.after))
	if err != nil || tree == nil {
		return false
	}
	bound := gotreesitter.Bind(tree)
	defer bound.Release()
	// A tree with an error in it is still asked, deliberately. The reading has
	// already been vetted by scaffold.read; the only question left is where it
	// begins, and the terminated scaffold puts a stray `;` after a block that
	// the grammar flags while still parsing the construct correctly.
	root := bound.RootNode()
	if root == nil {
		return false
	}
	start, end := len(sc.before), len(sc.before)+len(source)
	node := root.NamedDescendantForByteRange(uint32(start), uint32(end))
	// The smallest node holding the whole pattern, the way memberType walks
	// back up when a grammar hands back something narrower.
	for node != nil && (int(node.StartByte()) > start || int(node.EndByte()) < end) {
		node = node.Parent()
	}
	// Then down the front of it. These are the nodes anchoring may descend to
	// without losing anything the pattern wrote: each begins where the pattern
	// begins, so nothing has been left in front of it. A zero-width lookup at
	// the pattern's first byte would be the shorter way to ask and is not
	// reliable - on a node boundary a grammar may answer with either side.
	for node != nil && int(node.StartByte()) == start {
		if node.Type(lang) == head {
			return true
		}
		node = node.NamedChild(0)
	}
	return false
}

// holderTypes is the node each body scaffold's own text makes, per language.
var holderTypes sync.Map

// holderType is what the grammar calls the wrapper a scaffold writes, so that a
// reading anchored to the wrapper can be told from one anchored to the pattern.
//
// A reading that anchors to the wrapper is one where stripping the scaffold
// failed and nothing said so. `Console.WriteLine($X);` inside a class body gave
// `(class_declaration (_) @_ body: (declaration_list . (method_declaration ...
// (expression_statement . INNER .) ...) .))` - a search for a class whose only
// member is the pattern, which no file has. The wrapper's own name is the tell,
// and it is the only one: everything else about that reading looks like a
// reading that worked.
func holderType(lang *gotreesitter.Language, language, skeleton string) string {
	key := language + "\x00" + skeleton
	if cached, ok := holderTypes.Load(key); ok {
		return cached.(string)
	}
	got := memberType(lang, "", skeleton)
	holderTypes.Store(key, got)
	return got
}

// unwrapTopLevel replaces a reading that anchors to a top-level wrapper with
// the same pattern read inside a function body, for a grammar that gives a
// statement standing at the top of a file a node of its own.
//
// C# is that grammar, and it is the only one of the twenty in this build. Since
// C# 9 a file may be nothing but statements, so `Console.WriteLine($X);`
// parses standing alone - cleanly, with no error and no complaint - and the
// least invasive scaffold is therefore the one that wins. But the grammar wraps
// each of those statements in a `global_statement`, and that node exists
// nowhere else. The query is `(global_statement . (expression_statement . ...))`
// and it can only ever match a file with no class in it:
//
//	class T { void f() { Console.WriteLine(q); } }   // no match
//	Console.WriteLine(q);                            // match
//
// Ninety-four of the hundred and sixty-nine C# patterns in this rule corpus -
// every call, every assignment, every `using` - compiled to exactly that and
// found nothing in twenty-two thousand files. Nothing said so. `ast_pattern`
// reported `Valid: true`, the rule ran, and the answer was a clean bill of
// health.
//
// This is the same failure statementReading exists for and the opposite shape.
// There the plain reading was wrong because the grammar had no context for the
// pattern; here it is wrong because the grammar had too much - it understood
// the pattern as a whole program, which is a thing the pattern was not.
//
// The wrapped reading is replaced rather than kept beside the new one, which is
// the part worth being careful about. It looks like a loss - a file of nothing
// but statements is still C# - and it is not, because the inner reading matches
// there too: a query matches anywhere in a tree, and the statement is still
// inside the wrapper. Keeping both instead reported such a file twice, once per
// reading, since the two anchor to spans that differ by a semicolon and so
// survive the deduplication in select_ast.
//
// A pattern that produced no body reading keeps the one it had. That is a
// refusal rather than a wrong answer, and it is what multi-statement patterns
// get: C# reads `$A = 1;\n$$$_\n$B = 2;` inside a block with `$B` for a type
// and the assignment for a declarator, which is not what it says.
func unwrapTopLevel(lang *gotreesitter.Language, pattern, language string,
	queries []*grep.CompiledPattern) []*grep.CompiledPattern {
	wrapper := wrapsTopLevelStatements(lang, language)
	if wrapper == "" {
		return queries
	}
	// The whole gate, and it costs nothing: the head is already computed. A
	// pattern the grammar did not wrap is a pattern this has nothing to say
	// about - a C# `class $C { $$$B }` is a declaration wherever it stands -
	// and asking anyway would put two parses on every pattern in the language
	// for the answer "the same thing".
	wrapped := false
	for _, q := range queries {
		if q == nil {
			continue
		}
		if head, _, ok := splitHead(q.SExpr); ok && head == wrapper {
			wrapped = true
		}
	}
	if !wrapped {
		return queries
	}
	seen := map[string]bool{}
	var inside []*grep.CompiledPattern
	for _, body := range bodyScaffolds {
		read := body.sc.read(lang, pattern, pattern, language)
		if read == nil || seen[read.SExpr] {
			continue
		}
		// A reading with no single head is a reading of several units - a
		// try/catch is one - and it anchors to no wrapper, so it is kept.
		if head := readingHead(read); head != "" &&
			(head == holderType(lang, language, body.skeleton) ||
				!startsWhereThePatternDoes(lang, body.sc, pattern, head)) {
			continue
		}
		seen[read.SExpr] = true
		inside = append(inside, read)
	}
	if len(inside) == 0 {
		return queries
	}
	out := inside
	for _, q := range queries {
		if q == nil {
			continue
		}
		if head, _, ok := splitHead(q.SExpr); ok && head == wrapper {
			continue
		}
		if seen[q.SExpr] {
			continue
		}
		seen[q.SExpr] = true
		out = append(out, q)
	}
	return out
}

// statementProbe is a call statement written in names no program uses, for
// asking a grammar what it makes of one standing at the top of a file.
const statementProbe = probePrefix + "name(1);"

// wrappedIn is the wrapper node one language puts around a top-level
// statement, worked out once. An empty string means it puts none.
var wrappedIn sync.Map

// wrapsTopLevelStatements is what this grammar calls a statement that stands
// at the top of a file, when that is not what it calls the same statement
// inside a body - and "" when the two are the same thing, which is every
// grammar here but C#.
//
// The question is asked of a call, because a call is the one construct every
// language in this corpus has and spells as a statement. A grammar that wraps
// only some other kind of top-level item would not be noticed; that is a limit
// rather than an oversight, and the same one memberReadings draws.
//
// It is asked of the compiled readings rather than of the parse tree, and that
// is the whole of why it works. A wrapper covers exactly the bytes it wraps, so
// "the smallest node holding all of it" - the question memberType asks - walks
// straight past a `global_statement` to the `expression_statement` inside it
// and reports that the two are the same. What tells them apart is what the
// query anchors to, which is the thing that decides whether a rule matches.
//
// A grammar that cannot read the probe in one of the two places answers "" and
// never reaches the readings below: for Go and Rust, which insist on a
// declaration at file scope, the plain reading was already the scaffolded one.
func wrapsTopLevelStatements(lang *gotreesitter.Language, language string) string {
	if cached, ok := wrappedIn.Load(language); ok {
		return cached.(string)
	}
	wrapper := ""
	alone := (scaffold{}).read(lang, statementProbe, statementProbe, language)
	inside := bodyScaffolds[0].sc.read(lang, statementProbe, statementProbe, language)
	if alone != nil && inside != nil && wrapsExactly(alone.SExpr, inside.SExpr) {
		wrapper = readingHead(alone)
	}
	wrappedIn.Store(language, wrapper)
	return wrapper
}

// wrapsExactly reports whether the first reading is the second one with a
// wrapper around it and nothing else added.
//
// Containment on its own is not the test, and getting that wrong is what this
// exists to say. JavaScript reads the probe as a `call_expression` standing
// alone and as a `call_expression` inside the scaffold too, and the shorter of
// the two turns up inside the longer, so a `strings.Contains` said yes and
// would have put two extra readings on every JavaScript pattern in the corpus.
// What tells a wrapper from a coincidence is that there is nothing *after* the
// inner reading but the punctuation that closes the wrapper: C# has
//
//	(global_statement . (expression_statement . INNER .) .)
//
// where the tail is ` .) .)` and nothing else. Anything with a sibling, a
// field name or another node after the inner reading is a different shape
// rather than the same shape wrapped.
func wrapsExactly(alone, inside string) bool {
	a, b := litless(headLine(alone)), litless(headLine(inside))
	at := strings.Index(a, b)
	// Strictly wrapped: at 0 the two readings begin alike, which is the same
	// reading rather than one inside the other.
	if b == "" || at <= 0 {
		return false
	}
	return strings.Trim(a[at+len(b):], " .)") == ""
}

// headLine is a reading's shape without its predicates: the query itself, with
// the capture that names the whole match taken off the end so that it can be
// looked for inside another reading, where it is not the whole match.
func headLine(sexp string) string {
	if i := strings.IndexByte(sexp, '\n'); i >= 0 {
		sexp = sexp[:i]
	}
	return strings.TrimSuffix(sexp, " @"+rootCapture)
}

// litless is a reading with its literal-comparison capture names taken out, so
// that two readings of the same text compare equal wherever they differ only in
// how many numbers the wrapper around one of them used up. Renumbering would do
// as well when the wrapper contributes none of its own, which is not something
// this can assume of a grammar it has not met.
func litless(sexp string) string {
	return litNumber.ReplaceAllString(sexp, "")
}

// readingHead is the node type a compiled reading anchors to, or "" when there
// is no reading or it anchors to no named type.
func readingHead(query *grep.CompiledPattern) string {
	if query == nil {
		return ""
	}
	head, _, ok := splitHead(query.SExpr)
	if !ok {
		return ""
	}
	return head
}

// memberType is what a grammar calls this text inside something that holds
// members, or "" when it cannot read it in there at all.
//
// It is the fast half of memberReadings and it decides the whole question: a
// grammar that gives the text the same node type in both places has nothing to
// add, and that is almost every pattern in this corpus. The compiled reading is
// only ever built for the few that come back with a different name, and it is
// still built, because a name is not a promise that the children are the same.
//
// An empty keyword asks the same question of the text standing on its own,
// which is what the language-level gate compares against.
//
// A wrapper the grammar cannot follow is not an answer. Java has nowhere to put
// a bare call inside a class body, so `foo($X);` in there is an ERROR parse,
// and an ERROR node's type is a name like any other - it would compare unequal
// to the plain reading's head and send every call in the corpus down the slow
// path to be refused. So a tree with an error in it is no reading at all.
func memberType(lang *gotreesitter.Language, keyword, pattern string) string {
	source := pattern
	if pre, _, err := grep.Preprocess(pattern); err == nil {
		source = strings.ReplaceAll(pre, "__GREP_", probePrefix)
	}
	before, after := "", ""
	if keyword != "" {
		before, after = keyword+" "+scaffoldName+" {\n", "\n}"
	}
	tree, err := parserFor(lang).Parse([]byte(before + source + after))
	if err != nil || tree == nil {
		return ""
	}
	bound := gotreesitter.Bind(tree)
	defer bound.Release()
	root := bound.RootNode()
	if root == nil || root.HasErrorOrMissing() {
		return ""
	}
	start, end := len(before), len(before)+len(source)
	node := root.NamedDescendantForByteRange(uint32(start), uint32(end))
	// The smallest node holding the whole of it, the way scaffoldUnits walks
	// back up when a grammar hands back something narrower.
	for node != nil && (int(node.StartByte()) > start || int(node.EndByte()) < end) {
		node = node.Parent()
	}
	if node == nil {
		return ""
	}
	return strings.Clone(node.Type(lang))
}

// splitHead takes a query apart into the name of its outermost node and
// everything after that name. A query rooted at a wildcard has no name to
// swap - `(_ ...)` is already "some node holds this" - and is left alone.
func splitHead(sexp string) (head, rest string, ok bool) {
	line := sexp
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if !strings.HasPrefix(line, "(") {
		return "", "", false
	}
	end := 1
	for end < len(line) && (line[end] == '_' || line[end] >= 'a' && line[end] <= 'z' ||
		line[end] >= 'A' && line[end] <= 'Z' || line[end] >= '0' && line[end] <= '9') {
		end++
	}
	head = line[1:end]
	if head == "" || !(head[0] >= 'a' && head[0] <= 'z' || head[0] >= 'A' && head[0] <= 'Z') {
		return "", "", false
	}
	return head, line[end:], true
}

// otherHead is the node type the second reading gave the same children, or ""
// when it gave them the one they already had.
//
// The comparison is the first reading's children as text, matched against what
// follows each node type in the second. It is deliberately exact: two readings
// that differ anywhere but the head are two different readings, and swapping a
// head onto children that are not the same children would produce a query for a
// shape no file has - which is the silent-empty-result failure this package
// exists to prevent, arrived at from the inside.
//
// The trailing capture comes off the first reading's children because the
// second reading has the pattern nested inside a class, so what follows there
// is the class closing rather than the end of the query. Nothing else is
// trimmed: the space in front of the first child is part of what is being
// compared, and taking it off makes the two never line up. And `_lit` captures
// are renumbered in both, because the second reading counted its own text
// comparisons from wherever the wrapper left off. Renumbering keeps them
// distinct rather than collapsing them to one name, so two different literals
// still have to be two different literals.
func otherHead(sexp string, seen map[string]bool, rest string) string {
	want := renumberLits(strings.TrimSuffix(rest, " @"+rootCapture))
	for _, at := range namedHead.FindAllStringIndex(sexp, -1) {
		name := sexp[at[0]+1 : at[1]]
		if seen[name] {
			continue
		}
		if strings.HasPrefix(renumberLits(sexp[at[1]:]), want) {
			return name
		}
	}
	return ""
}

// litNumber matches the capture a text comparison is written on.
var litNumber = regexp.MustCompile(`@_lit_[0-9]+`)

// renumberLits counts a query's text comparisons from one, so that two readings
// of the same pattern can be compared when one of them was compiled inside a
// wrapper that used up some of the numbers.
func renumberLits(sexp string) string {
	seen := map[string]string{}
	return litNumber.ReplaceAllStringFunc(sexp, func(name string) string {
		if to, ok := seen[name]; ok {
			return to
		}
		to := fmt.Sprintf("@_lit_%d", len(seen)+1)
		seen[name] = to
		return to
	})
}

// statementReading is the pattern read as a statement rather than as whatever
// the grammar makes of it standing alone, or nil when that is the same thing.
//
// In C it is not the same thing, and the difference is total. `gets($BUF)`
// parses as a declaration - `gets` a type, `($BUF)` a declarator - because
// that is what those characters mean at the top of a file. The query compiles,
// reports no problem, and matches nothing in any C ever written. `gets($BUF);`
// parses as the call. Every C pattern in a rule corpus is a call, so every one
// of them silently found nothing.
//
// Adding the terminator changes nothing in Go, Java, Python or JavaScript,
// where a semicolon is optional or a separator, so the second reading is kept
// only where it is genuinely a second reading. A pattern that already parses
// badly is refused before this is reached, which is what stops `$A + $B` in C
// - a fragment C has no context for at all - from being rescued into the
// nonsense the terminator makes of it.
func statementReading(lang *gotreesitter.Language, pattern string, query *grep.CompiledPattern) *grep.CompiledPattern {
	if strings.HasSuffix(strings.TrimSpace(pattern), ";") {
		return nil
	}
	stmt, err := grep.Compile(lang, pattern+";")
	if err != nil || stmt.SExpr == query.SExpr || patternProblem(pattern, "", stmt.SExpr) != "" {
		return nil
	}
	// A reading that needed a token tree-sitter had to invent is not a second
	// reading of the pattern, it is a repair of it: `$A + $B;` becomes a C
	// update expression whose `++` is missing. See emptyToken.
	if emptyToken.MatchString(stmt.SExpr) {
		return nil
	}
	anchored, err := anchorPattern(lang, pattern, stmt, nil)
	if err != nil {
		return nil
	}
	return anchored
}

// scaffoldReading is the pattern read inside the text a grammar needs around
// it before it will read it as code at all, and with its standalone ellipses
// spelled as something that can stand where the ellipsis stood. It is nil when
// neither helps.
//
// Both halves exist for the same reason: a rule is written in the vocabulary
// of the thing it describes, and a grammar has to be able to parse it before
// any of that matters. Two thirds of the patterns a rule corpus writes that no
// grammar would accept are one of these two shapes.
//
// The ellipsis half:
//
//	resource "aws_lambda_function" $ANYTHING {
//	  ...
//	}
//
// `...` becomes an identifier, and an HCL block body holds attributes and
// blocks - an identifier on its own is neither, so the whole pattern is an
// ERROR parse. Worse, the error node then carries HCL's significant whitespace
// as literals, so the query text ends up with a raw newline in a string and
// will not even parse as a query. The caller is told about the newline, which
// is true and useless. What does parse there is an item, so the ellipsis is
// written as one - a placeholder nobody would type - and dropMarked takes it
// back out, leaving the gap the caller meant.
//
// The wrapper half:
//
//	$COOKIE = new Cookie(...);
//	...
//	$COOKIE.setSecure(false);
//
// Three statements, and Java has nowhere to put a statement outside a method.
// The pattern is compiled inside one, and the method is then walked past: see
// scaffoldUnits, which measures which nodes the wrapper contributed, and
// anchorQuery, which descends them.
//
// Which spelling and which wrapper a grammar wants is not looked up, it is
// tried. A candidate counts only if it compiles to code - patternProblem -
// leaves nothing of the scaffolding behind, and says something about the code
// rather than nothing - see specificity. The first that does is the reading,
// because the list is ordered least invasive first: a pattern that needs only
// a semicolon must not end up with a class around it.
func scaffoldReading(lang *gotreesitter.Language, pattern, language string) *grep.CompiledPattern {
	spots := bodyEllipses(pattern)
	for _, sc := range scaffolds {
		for _, spell := range append([]string{""}, itemSpellings...) {
			text := pattern
			if spell != "" {
				if len(spots) == 0 {
					continue
				}
				text = substituteEllipses(pattern, spots, spell)
			}
			reading := sc.read(lang, pattern, text, language)
			if reading == nil {
				continue
			}
			if len(spots) > 0 && !sc.standsForNothing(lang, pattern, spots, language, reading) {
				continue
			}
			return reading
		}
	}
	return nil
}

// standsForNothing checks that a spelling put the placeholder where the
// ellipsis was and nowhere else.
//
// A spelling that parses is not yet a spelling that is right. Java takes
// `__pwrq_any_1__ = __pwrq_any_1__` on a line of its own, and then reads the
// statement below it as the rest of that same declaration - the pattern's own
// `HttpServletRequest $REQ = ...` becomes the placeholder's type - so the
// query compiles, reports no problem, and is a search for something the rule
// never wrote.
//
// What the ellipsis says is that nothing in particular is there. So the
// pattern with those lines struck out has to compile to the same query, up to
// the adjacency the ellipsis is there to loosen: if it does not, the
// placeholder took something with it.
func (sc scaffold) standsForNothing(lang *gotreesitter.Language, pattern string,
	spots [][2]int, language string, got *grep.CompiledPattern) bool {
	bare := cutEllipses(pattern, spots)
	plain := sc.read(lang, bare, bare, language)
	if plain == nil {
		// Nothing to compare against - a body with every item struck out is
		// often not a construct at all - so the reading stands.
		return true
	}
	return sameShape(plain.SExpr) == sameShape(got.SExpr)
}

// cutEllipses removes the whole line each standalone ellipsis stands on.
func cutEllipses(pattern string, spots [][2]int) string {
	var b strings.Builder
	last := 0
	for _, spot := range spots {
		start, end := spot[0], spot[1]
		for start > 0 && pattern[start-1] != '\n' {
			start--
		}
		for end < len(pattern) && pattern[end] != '\n' {
			end++
		}
		if end < len(pattern) {
			end++
		}
		if start < last {
			continue
		}
		b.WriteString(pattern[last:start])
		last = end
	}
	b.WriteString(pattern[last:])
	return b.String()
}

// read compiles one candidate reading of a pattern, or returns nil if this
// scaffold is not the one this grammar wanted.
func (sc scaffold) read(lang *gotreesitter.Language, pattern, text, language string) *grep.CompiledPattern {
	if sc.sigil {
		text = sigilHoles(text)
	}
	candidate := sc.before + text + sc.closing + sc.after
	query, err := grep.Compile(lang, candidate)
	if err != nil || patternProblem(candidate, language, query.SExpr) != "" {
		return nil
	}
	// A token with no text in it is one tree-sitter inserted to recover from a
	// parse it could not finish. The pattern did not write it, so a reading
	// that needed it did not read the pattern: `$A + $B` wrapped in braces
	// becomes a C update expression whose `++` is missing.
	if emptyToken.MatchString(query.SExpr) {
		return nil
	}
	tags := sc.tags()
	units := scaffoldUnits(lang, sc.before, sc.closing, sc.after, text)
	anchored, err := anchorPattern(lang, candidate, query, units, tags...)
	if err != nil {
		return nil
	}
	// Nothing may be left of the scaffolding: a query still naming a
	// placeholder would be a search for an attribute called __pwrq_any_1__,
	// which no file has, and one still naming the opener would be a search for
	// a file whose first statement is the one being looked for.
	if strings.Contains(anchored.SExpr, markerPrefix) {
		return nil
	}
	for _, tag := range tags {
		if strings.Contains(anchored.SExpr, `"`+tag+`"`) {
			return nil
		}
	}
	// A pattern written on one line is one construct, so a reading that left
	// the file node open read it as several things and did not understand it.
	// `$X = md5($Y)` does that where a bare name cannot be assigned to: the
	// hole becomes one top-level item and the call becomes another.
	if !strings.Contains(strings.TrimSpace(pattern), "\n") &&
		strings.HasPrefix(anchored.SExpr, "(_ ") {
		return nil
	}
	// A reading that named no node type and compared no text understood none
	// of the pattern. `echo $X` read without PHP's sigil is one of those: the
	// keyword goes and the query is the hole alone, which matches every node
	// in every file. See specificity.
	if specificity(anchored.SExpr) == 0 {
		return nil
	}
	return anchored
}

// namedHead matches a node type in a query, which is every `(` not followed by
// a wildcard or a predicate.
var namedHead = regexp.MustCompile(`\((?:[A-Za-z][A-Za-z0-9_]*)`)

// specificity is how much of a pattern a reading understood: the node types it
// named, plus the pieces of text it compared.
//
// It is what picks between two readings that both compile. `echo $X` read
// without a sigil becomes `(_ (_ . (_) @X .))` - a node holding a node,
// matching every construct in every file - and read with one becomes an echo
// statement holding a variable. Both are well-formed queries and only one is
// the pattern. A reading that named nothing scores zero and is refused
// outright, because a query of nothing but wildcards is the silent-empty-result
// failure this package exists to prevent, inverted: it matches everything.
func specificity(sexp string) int {
	score := 0
	for _, form := range strings.Split(sexp, "\n") {
		if strings.HasPrefix(strings.TrimSpace(form), "(#") {
			score++
			continue
		}
		score += len(namedHead.FindAllString(form, -1))
	}
	return score
}

// A scaffold is text a grammar may need around a pattern before it will read
// the pattern as code, together with the tags it leaves in the query for
// dropMarked to take back out.
//
// before and after go around the pattern; closing ends the pattern's own item
// and so is repeated with it when the scaffolding is measured. A grammar that
// needs no scaffold is unaffected: its patterns compile, so a scaffolded
// reading is never reached for them.
type scaffold struct {
	before  string
	closing string
	after   string
	// sigil says to write each hole the way the language writes a variable,
	// which is the second thing PHP needs. `$X = md5($Y)` preprocesses to
	// `__GREP_CAP_X__ = ...`, and PHP cannot assign to a bare name, so the
	// pattern that every PHP rule in the corpus is written as does not parse.
	// `$__GREP_CAP_X__` does. The plain spellings are tried first, because a
	// hole that binds to any expression is the more faithful reading where the
	// grammar allows it.
	sigil bool
}

// emptyToken matches an anonymous token with no text in it, as opposed to the
// escaped quote inside a literal comparison, which spells the same characters.
var emptyToken = regexp.MustCompile(`[\s(:]""[\s)]`)

// scaffoldWord is one word or one run of punctuation.
var scaffoldWord = regexp.MustCompile(`[A-Za-z_][A-Za-z_0-9]*|[^\sA-Za-z_0-9]+`)

// tags are the literal texts the scaffolding puts in the query, read off the
// scaffolding itself rather than listed: whatever a wrapper writes, a grammar
// may keep as a node of its own, and dropMarked has to be told to take it back
// out or the descent past the wrapper stops at it.
//
// A word the pattern happens to use as well is dropped from the pattern too.
// That loosens the reading rather than breaking it, and it only ever happens
// on a pattern no grammar would take as written.
func (sc scaffold) tags() []string {
	var out []string
	for _, word := range strings.Fields(sc.before + " " + sc.closing + " " + sc.after) {
		// Both readings of the wrapper's text: the word as written, because
		// `<?php` is one token to a PHP grammar, and its parts, because
		// `__pwrq_any_func__()` is two to every other one.
		out = append(out, word)
		if parts := scaffoldWord.FindAllString(word, -1); len(parts) > 1 {
			out = append(out, parts...)
		}
	}
	return out
}

// scaffoldName and scaffoldFunc are what a wrapper calls the class and the
// function it invents. They are spelled with markerPrefix so that dropMarked
// takes them out of the query without being told about them.
const (
	scaffoldName = markerPrefix + "type__"
	scaffoldFunc = markerPrefix + "func__"
)

// scaffolds are the wrappers tried, least invasive first, so that a pattern
// that needs only a semicolon does not get a class around it. Every one of
// them earns its place in the corpus this was ported against; a grammar none
// of them fits keeps the reading it had, which is a refusal rather than a
// wrong answer.
var scaffolds = []scaffold{
	{},
	{before: "<?php\n", closing: ";"},
	{before: "<?php\n"},
	{before: "<?php\n", closing: ";", sigil: true},
	{before: "<?php\n", sigil: true},
	{before: "{\n", after: "\n}"},
	{before: "[\n", after: "\n]"},
	{before: "class " + scaffoldName + " {\n", after: "\n}"},
	{before: "contract " + scaffoldName + " {\n", after: "\n}"},
	{before: "function " + scaffoldFunc + "() {\n", after: "\n}"},
	{before: "void " + scaffoldFunc + "() {\n", after: "\n}"},
	{before: "def " + scaffoldFunc + "():\n", after: ""},
	{before: "def " + scaffoldFunc + "\n", after: "\nend"},
	{before: "FROM " + scaffoldName + "\n", after: ""},
	{before: "class " + scaffoldName + " {\nvoid " + scaffoldFunc + "() {\n", after: "\n}\n}"},
	{before: "contract " + scaffoldName + " {\nfunction " + scaffoldFunc + "() public {\n", after: "\n}\n}"},
	{before: "class " + scaffoldName + " {\ndef " + scaffoldFunc + "() = {\n", after: "\n}\n}"},
}

// sigilHoles writes a `$` in front of every single hole, leaving a variadic
// alone: `$X` becomes `$$X`, and `$$$ARGS` stays as it is.
func sigilHoles(pattern string) string {
	var b strings.Builder
	for i := 0; i < len(pattern); {
		if pattern[i] != '$' {
			b.WriteByte(pattern[i])
			i++
			continue
		}
		run := i
		for run < len(pattern) && pattern[run] == '$' {
			run++
		}
		if run-i == 1 && run < len(pattern) && (pattern[run] == '_' || (pattern[run] >= 'A' && pattern[run] <= 'Z')) {
			b.WriteByte('$')
		}
		b.WriteString(pattern[i:run])
		i = run
	}
	return b.String()
}

// wordReading is a pattern that is one bare word, for a grammar that cannot
// read a word standing alone.
//
// `resource` is a whole pattern in the corpus - a rule that wants to report
// the keyword and puts every real condition in its guards - and in HCL a bare
// identifier is not a config file, so it is an ERROR parse and the rule is
// refused. In Go and Python the same pattern compiles, because an identifier
// is an expression there, which is how the corpus came to be written this way
// in the first place.
//
// What it means is the same in every grammar: this word, wherever it occurs as
// a piece of syntax. So that is what it compiles to - any node whose whole
// text is the word - and no grammar has to be asked what it calls an
// identifier. A node and its parent can both span exactly the word; select_ast
// keeps one match per span, so that is one match, not three.
func wordReading(lang *gotreesitter.Language, pattern string) *grep.CompiledPattern {
	word := strings.TrimSpace(pattern)
	if !oneWord.MatchString(word) {
		return nil
	}
	sexp := fmt.Sprintf("(_) @%s\n(#eq? @%s %q)", rootCapture, rootCapture, word)
	query, err := gotreesitter.NewQuery(sexp, lang)
	if err != nil {
		return nil
	}
	return &grep.CompiledPattern{Query: query, Lang: lang, SExpr: sexp}
}

// itemSpellings are the ways a grammar might write one item of a list, most
// likely first. %[1]s is the placeholder's name.
//
// Five entries cover this corpus: an assignment, which is what HCL, and any
// configuration language shaped like it, calls a body item; a binding, for the
// ones that write a colon; and the three terminated forms, for the languages
// where a statement ends in a semicolon and a run of statements is what the
// ellipsis stood in. A grammar none of these fit keeps the reading it already
// had, which is a refusal rather than a wrong answer.
var itemSpellings = []string{
	"%[1]s = %[1]s", "%[1]s: %[1]s",
	"%[1]s = %[1]s;", "%[1]s();", "%[1]s;",
}

// bodyEllipses are the offsets of every `$$$_` that stands where a whole item
// goes: alone between braces, or alone on its own line.
//
// An ellipsis anywhere else is a hole in something the grammar can already
// parse - `f($A, $$$_)` is an argument list, `x = $$$_` is a value - and those
// have never needed help.
func bodyEllipses(pattern string) [][2]int {
	const ellipsis = "$$$_"
	var spots [][2]int
	for i := 0; i+len(ellipsis) <= len(pattern); {
		at := strings.Index(pattern[i:], ellipsis)
		if at < 0 {
			break
		}
		start := i + at
		end := start + len(ellipsis)
		i = end
		if isItemBoundary(pattern, start-1, -1) && isItemBoundary(pattern, end, 1) {
			spots = append(spots, [2]int{start, end})
		}
	}
	return spots
}

// isItemBoundary reports whether the edge of the pattern at from, walked in
// the given direction past spaces and tabs, is where one item ends and the
// next begins.
func isItemBoundary(pattern string, from, step int) bool {
	for i := from; i >= 0 && i < len(pattern); i += step {
		switch pattern[i] {
		case ' ', '\t':
			continue
		case '\n', '{', '}':
			return true
		default:
			return false
		}
	}
	// The start and the end of the pattern are boundaries too.
	return true
}

// substituteEllipses writes one placeholder over each spot, numbered so that
// two of them in one pattern stay two things.
func substituteEllipses(pattern string, spots [][2]int, spell string) string {
	var b strings.Builder
	last := 0
	for n, spot := range spots {
		b.WriteString(pattern[last:spot[0]])
		fmt.Fprintf(&b, spell, fmt.Sprintf("%s%d__", markerPrefix, n+1))
		last = spot[1]
	}
	b.WriteString(pattern[last:])
	return b.String()
}

// variadicNames lists the holes a pattern wrote as $$$NAME, by the name the
// caller gave them. The map is keyed by placeholder, so the names are in the
// values.
func variadicNames(mvars map[string]*grep.MetaVar) map[string]bool {
	names := map[string]bool{}
	for _, mv := range mvars {
		if mv != nil && mv.Variadic && mv.Name != "" && mv.Name != ellipsisName {
			names[mv.Name] = true
		}
	}
	return names
}

// anchorPattern recompiles a query with its sibling lists anchored, so that a
// pattern naming two arguments matches a call that has two. See sexp.go for
// what the engine does without it.
func anchorPattern(lang *gotreesitter.Language, pattern string, query *grep.CompiledPattern,
	units map[string]bool, drop ...string) (*grep.CompiledPattern, error) {
	if units == nil {
		units = unitTypes(lang, pattern)
	}
	dropped := map[string]bool{}
	for _, text := range drop {
		dropped[text] = true
	}
	sexp := anchorQuery(query.SExpr, bubbleDepths(lang, pattern, query), units, dropped)
	if sexp == query.SExpr {
		return query, nil
	}
	q, err := gotreesitter.NewQuery(sexp, lang)
	if err != nil {
		return nil, err
	}
	return &grep.CompiledPattern{Query: q, MetaVars: query.MetaVars, Lang: lang, SExpr: sexp}, nil
}

// patternProblem is the check the engine underneath does not make: whether the
// query a pattern compiled to searches for code or for text.
//
// It is the whole reason a pattern goes through this package rather than
// straight to grep.Match. A grammar that cannot follow a pattern does not say
// so. It produces a query that is well-formed, runs against every file and
// matches none of them, so a typo and an honest absence come back identical
// and the caller reads the wrong one.
//
// One idea covers the three ways it happens: a query that still contains the
// pattern's own text did not understand the pattern.
//
//   - An ERROR node, where the grammar gave up. `func $$$(` compiles to
//     `(ERROR (ERROR (ERROR) @_lit_1))`.
//
//   - A leaked placeholder. `$NAME` is rewritten to __GREP_CAP_NAME__ so that
//     the pattern parses as code, then turned back into a capture. When it
//     survives into a text comparison the hole was read as part of a word.
//     PHP does this to every pattern, and reports no error at all: a PHP file
//     is HTML until `<?php`, so `md5($X)` parses as the literal text
//     "md5(__GREP_CAP_X__)".
//
//   - The whole pattern as a single literal. `foo(1)` against markdown or YAML
//     compiles to a query for a paragraph whose text is `foo(1)`. Nothing was
//     parsed; the pattern was quoted. The exception is a pattern that is one
//     bare word, where comparing text is exactly the search intended -
//     `select_ast("."; "md5")` looks for that identifier.
//
// What none of the three can catch is a pattern that parses as the wrong
// construct. Python's `except $E: $$$B` compiles cleanly, because "except"
// also reads as an identifier, into a query for an assignment. That is what
// ast_pattern's Query field is for.
func patternProblem(pattern, language, sexp string) string {
	if strings.Contains(sexp, errorNode) {
		return fmt.Sprintf("pattern %q does not parse as %s code, so it can never match; "+
			"write the pattern as code you could compile, with $NAME where a value varies "+
			"and $$$NAME where a list does", pattern, language)
	}

	literals := literalPredicate.FindAllStringSubmatch(sexp, -1)
	for _, m := range literals {
		if strings.Contains(m[1], capturePrefix) {
			return fmt.Sprintf("pattern %q is not code in %s: the grammar read its $ holes as "+
				"part of the surrounding text, so the query looks for the literal characters of "+
				"the pattern and can only ever match a file that contains them verbatim; "+
				"ast_pattern shows what it compiled to", pattern, language)
		}
	}

	// One literal holding the entire pattern means the grammar took the
	// pattern as a run of text and parsed nothing inside it.
	if len(literals) == 1 && literals[0][1] == strings.TrimSpace(pattern) && !oneWord.MatchString(literals[0][1]) {
		return fmt.Sprintf("pattern %q is not code in %s: the whole pattern compiled to a single "+
			"piece of text, so the query looks for those characters rather than for the construct "+
			"they spell; ast_pattern shows what it compiled to", pattern, language)
	}
	return ""
}

// validate is the error a caller sees for a pattern that cannot match code.
func (c *compiled) validate() error {
	if c.valid() {
		return nil
	}
	return fmt.Errorf("%s", c.problem)
}

// metaVariables lists the holes in a pattern, in a stable order.
//
// The map is keyed by the placeholder the engine rewrote each hole into -
// $NAME becomes __GREP_CAP_NAME__ so that the pattern still parses as code -
// and it is the Name inside that the caller wrote and that the matches come
// back under. Reading the keys instead reports the machinery.
//
// Anonymous wildcards are left out. $_ says "something goes here and I do not
// care what", so listing it as a name to look up would be answering a question
// the caller declined to ask.
func (c *compiled) metaVariables() []any {
	seen := map[string]bool{}
	if c.query() == nil {
		return nil
	}
	names := make([]string, 0, len(c.query().MetaVars))
	for _, meta := range c.query().MetaVars {
		if meta == nil || meta.Wildcard || meta.Name == "" || meta.Name == "_" || seen[meta.Name] {
			continue
		}
		seen[meta.Name] = true
		names = append(names, meta.Name)
	}
	sort.Strings(names)
	out := make([]any, len(names))
	for i, name := range names {
		out[i] = name
	}
	return out
}

// languageFor decides which grammar to parse a file with: the one the caller
// named, or the one the extension implies.
func languageFor(path, requested string) (*grammars.LangEntry, error) {
	if requested != "" {
		entry := grammars.DetectLanguageByName(requested)
		if entry == nil {
			return nil, fmt.Errorf("no grammar for language %q in this build; "+
				"get_ast_language lists the %d it has", requested, len(grammars.AllLanguages()))
		}
		return entry, nil
	}
	return grammars.DetectLanguage(path), nil
}
