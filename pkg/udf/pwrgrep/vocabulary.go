package pwrgrep

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/filewalk"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/astsearch"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// Composing structural matches into rules.
//
// select_ast answers one question - where does this piece of syntax occur -
// and a real rule is several of those answers combined. "MD5, but only in a
// file that imports crypto/md5." "Taking the address of a variable, but only
// inside a range loop." "A call to Query, but not one whose argument is a
// constant." The patterns are the easy half; the combining is where a rule
// lives, and this file is that half.
//
// The operators are named after the ones the rule sets pwrq's corpus was
// translated from use, so a rule can be read beside its original. Each takes
// an array of matches and returns one, so a rule is a pipeline and nothing
// else - which is why a rule is a .pwrq file rather than a format.

// matches reads the array of matches an operator was given. Every operator in
// this file takes one and returns one, so the reading is the same everywhere
// and so is the complaint when it is the wrong thing.
func matches(op string, v any) ([]any, error) {
	list, ok := common.BindValue(v).([]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected a list of matches, got %T; "+
			"a rule pipes scan_ast into these, one after another", op, common.BindValue(v))
	}
	return list, nil
}

// object reads one match, whole.
//
// Not through BindValue, which is the trap here: a match carries a PwrqValue -
// its path, so that a match can be piped straight into cat - and BindValue
// collapses any object that has one down to that scalar. Every operator in
// this file wants the whole match, so every one of them would have quietly
// seen a string and matched nothing.
func object(v any) map[string]any {
	if o, ok := v.(*typed.Object); ok {
		v = typed.NormalizeJSON(o.Value)
	}
	m, _ := v.(map[string]any)
	return m
}

func text(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// span reads a match's byte range. Offsets are what let one match be tested
// for containing another; line numbers cannot, because two things on one line
// have the same line number whether one is inside the other or not.
func span(m map[string]any) (path string, start, end float64, ok bool) {
	if m == nil {
		return "", 0, 0, false
	}
	path = text(m, "Path")
	start, sok := number(m["Offset"])
	end, eok := number(m["EndOffset"])
	return path, start, end, sok && eok
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// keepIf is the shape every filtering operator has: walk the list, keep what
// the predicate admits.
func keepIf(list []any, pred func(map[string]any) bool) []any {
	out := make([]any, 0, len(list))
	for _, v := range list {
		if pred(object(v)) {
			out = append(out, v)
		}
	}
	return out
}

// filterOp registers an operator that narrows a list of matches by comparing
// it against another list.
//
// narrow is handed both lists whole rather than being asked about one match at
// a time. What these operators want from the other list is an index - the
// files it covers, the spans it encloses - and an index built inside the
// per-match question is an index built once per match: a rule combining two
// results of a thousand matches each did a million comparisons to answer a
// question worth two thousand.
func filterOp(name string, narrow func(list, other []any) []any) gojq.CompilerOption {
	common.DeclareInput(name, common.InputPipeline)
	return common.WithFunction(name, 1, 2, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		list, err := matches(name, in)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		other, err := matches(name, rest[0])
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return narrow(list, other)
	})
}

// RegisterScanAst registers scan_ast, the search a rule starts from.
//
// The input is the path to search; the first argument is the glob naming the
// files the rule is about, or several of them; the output is one array holding
// every match of every pattern, in file order.
//
//	"src" | scan_ast("*.go"; ["md5.New()", "sha1.New()"])
//	"src" | scan_ast(["*.js", "*.ts"]; $patterns) | of($patterns)
//
// One call rather than one per pattern is the point: the tree is walked once
// and each file parsed once however many patterns a rule has, so a rule with
// seven alternatives costs what a rule with one costs.
//
// Naming the files is not an optimisation. A pattern is code in more languages
// than the one it was written for - `$A != $A` is a useless comparison in Go
// and in JavaScript both - so a Go rule run over a repository with a vendored
// jquery.min.js in it reports findings in jquery.min.js, correctly and
// uselessly. The glob is how a rule says which language it is about, and it is
// also what makes a rule for one language runnable over a tree that has none
// of it: a glob that matches no file leaves an empty result rather than "no
// pattern was code in anything found here".
func RegisterScanAst() gojq.CompilerOption {
	const op = "scan_ast"
	common.DeclareInput(op, common.InputPipeline)
	return common.WithFunction(op, 2, 3, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 2)
		root, ok := common.BindPath(in)
		if !ok {
			return common.MakeUDFErrorResult(
				fmt.Errorf("%s: expected a path to search, got %T", op, common.BindValue(in)), nil)
		}
		globs, err := stringsOf(op, "file glob", rest[0])
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		patterns, err := stringsOf(op, "pattern", rest[1])
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if len(patterns) == 0 {
			return common.MakeUDFErrorResult(
				fmt.Errorf("%s: no patterns, so there is nothing to search for", op), nil)
		}
		out := []any{}
		for _, glob := range globs {
			found, err := astsearch.SearchTree(root, patterns, glob)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", op, err), nil)
			}
			out = append(out, found...)
		}
		return out
	})
}

// stringsOf reads an argument that is one string or a list of them, which is
// how every rule writes a thing it may want several of.
func stringsOf(op, what string, arg any) ([]string, error) {
	if list, ok := common.BindValue(arg).([]any); ok {
		out := make([]string, len(list))
		for i, item := range list {
			s, err := common.BindString(item, what)
			if err != nil {
				return nil, fmt.Errorf("%s: %s %d: %v", op, what, i+1, err)
			}
			out[i] = s
		}
		return out, nil
	}
	s, err := common.BindString(arg, what)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", op, err)
	}
	return []string{s}, nil
}

// RegisterScanRegex registers scan_regex, the search a rule starts from when
// what it is looking for is text rather than syntax.
//
// Not every rule is about a parse tree, and pretending otherwise is how a
// corpus loses a third of itself. A Django template with `{% autoescape off
// %}` in it is a real finding and there is no grammar in which that is a
// construct; the rule says so by being written as a regex, and this is what
// runs it.
//
//	"src" | scan_regex("*.html"; "{%\\s*autoescape\\s+off\\s*%}")
//	"." | scan_regex(["*.yml", "*.yaml"]; $patterns) | of($patterns)
//
// What comes back is the same kind of value scan_ast produces - a path, a
// span, the text - so every operator downstream works on it unchanged, and a
// rule can mix the two. A named group in the regex becomes a hole of that
// name, so `(?P<KEY>\\w+)` fills in $KEY in the message the way a pattern's
// hole does.
//
// The regexes are RE2: no backreferences and no lookaround, in exchange for
// running in time linear in the input.
func RegisterScanRegex() gojq.CompilerOption {
	const op = "scan_regex"
	common.DeclareInput(op, common.InputPipeline)
	return common.WithFunction(op, 2, 3, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 2)
		root, ok := common.BindPath(in)
		if !ok {
			return common.MakeUDFErrorResult(
				fmt.Errorf("%s: expected a path to search, got %T", op, common.BindValue(in)), nil)
		}
		globs, err := stringsOf(op, "file glob", rest[0])
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		exprs, err := stringsOf(op, "regex", rest[1])
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		compiled := make([]*regexp.Regexp, len(exprs))
		for i, expr := range exprs {
			re, err := regexp.Compile(expr)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", op, err), nil)
			}
			compiled[i] = re
		}
		out := []any{}
		for _, glob := range globs {
			found, err := scanTree(root, glob, exprs, compiled)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", op, err), nil)
			}
			out = append(out, found...)
		}
		return out
	})
}

// scanTree runs every regex over every file one glob names.
func scanTree(root, glob string, exprs []string, compiled []*regexp.Regexp) ([]any, error) {
	walk, err := filewalk.New(root, glob)
	if err != nil {
		return nil, err
	}
	var out []any
	for {
		path, ok, err := walk.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			return out, nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			// A file that cannot be read is not a finding and not a failure of
			// the rule; a tree has sockets and dangling links in it.
			continue
		}
		text := string(source)
		// The line starts are read once per file rather than once per number
		// reported, and the spans are sorted before the matches are rendered,
		// so the comparison is two integers rather than two decoded objects.
		lines := astsearch.Index(source)
		var found []textHit
		for i, re := range compiled {
			for _, span := range re.FindAllStringSubmatchIndex(text, -1) {
				found = append(found, textHit{expr: i, span: span})
			}
		}
		// A file reads top to bottom whatever order the regexes were written
		// in, which is what scan_ast promises too.
		sort.SliceStable(found, func(a, b int) bool { return found[a].span[0] < found[b].span[0] })
		for _, h := range found {
			out = append(out, textMatch(path, exprs[h.expr], compiled[h.expr], text, lines, h.span))
		}
	}
}

// textHit is one regex match before it is rendered: which regex found it, and
// where. Ordering these is cheaper than ordering finished match objects.
type textHit struct {
	expr int
	span []int
}

// textMatch renders one regex hit as the match object scan_ast would have
// produced, so that a rule cannot tell the two apart.
func textMatch(path, expr string, re *regexp.Regexp, text string, lines *astsearch.Lines, span []int) any {
	start, end := span[0], span[1]
	line, column := lines.At(start)
	endLine, endColumn := lines.At(end)
	captures := map[string]any{}
	for i, name := range re.SubexpNames() {
		if name == "" || 2*i+1 >= len(span) || span[2*i] < 0 {
			continue
		}
		captures[name] = text[span[2*i]:span[2*i+1]]
	}
	return astsearch.AstMatch.Build(map[string]any{
		"Path":          path,
		"Language":      "text",
		"Pattern":       expr,
		"LineNumber":    line,
		"Column":        column,
		"EndLineNumber": endLine,
		"EndColumn":     endColumn,
		"Offset":        start,
		"EndOffset":     end,
		"Text":          text[start:end],
		"Captures":      captures,
		"PwrqValue":     path,
	})
}

// RegisterWhereText registers where_text: keep the matches whose own text
// matches a regex.
//
// This is a regex used as a guard rather than as a search - "this call, but
// only where it is written like that" - which is what a rule means by putting
// a regex beside a pattern rather than instead of one.
//
//	$all | of($calls) | where_text("password|secret")
func RegisterWhereText() gojq.CompilerOption {
	return textOp("where_text", true)
}

// RegisterWhereTextNot registers where_text_not, its negation.
//
//	$all | of($calls) | where_text_not("^// ")
func RegisterWhereTextNot() gojq.CompilerOption {
	return textOp("where_text_not", false)
}

func textOp(name string, want bool) gojq.CompilerOption {
	common.DeclareInput(name, common.InputPipeline)
	return common.WithFunction(name, 1, 2, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		list, err := matches(name, in)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		expr, err := common.BindString(rest[0], "regex")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		return keepIf(list, func(m map[string]any) bool {
			return re.MatchString(text(m, "Text")) == want
		})
	})
}

// RegisterOf registers of: keep the matches that one pattern found, or that
// any pattern in a list found.
//
//	$all | of("md5.New()")
//	$all | of($calls)
func RegisterOf() gojq.CompilerOption {
	const op = "of"
	common.DeclareInput(op, common.InputPipeline)
	return common.WithFunction(op, 1, 2, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		list, err := matches(op, in)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		wanted, err := stringsOf(op, "pattern", rest[0])
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		want := make(map[string]bool, len(wanted))
		for _, p := range wanted {
			want[p] = true
		}
		return keepIf(list, func(m map[string]any) bool { return want[text(m, "Pattern")] })
	})
}

// place is one match's span: the file, and the bytes it covers.
type place struct {
	path       string
	start, end float64
}

// placeOf reads a match's place, or reports that it has none. A match without
// offsets is neither inside anything nor at the same place as anything, which
// is what the two indexes below do with it.
func placeOf(m map[string]any) (place, bool) {
	path, start, end, ok := span(m)
	return place{path: path, start: start, end: end}, ok
}

// enclosures answers "does some match in this list cover that span" without
// looking at every match in the list.
//
// Spans are grouped by file and sorted by where they begin, with a running
// maximum of where they end. One span covers another when it begins at or
// before it and ends at or after it - so among the spans that begin in time,
// the only question left is whether any of them reached far enough, and the
// running maximum is that answer. Building it is a sort; asking it is a binary
// search.
type enclosures map[string]*sortedSpans

type sortedSpans struct {
	starts []float64
	// reach[i] is the furthest any of the first i+1 spans ends.
	reach []float64
}

func enclosuresOf(list []any) enclosures {
	byPath := map[string][]place{}
	for _, v := range list {
		if p, ok := placeOf(object(v)); ok {
			byPath[p.path] = append(byPath[p.path], p)
		}
	}
	index := make(enclosures, len(byPath))
	for path, places := range byPath {
		sort.Slice(places, func(a, b int) bool { return places[a].start < places[b].start })
		f := &sortedSpans{starts: make([]float64, len(places)), reach: make([]float64, len(places))}
		furthest := math.Inf(-1)
		for i, p := range places {
			if p.end > furthest {
				furthest = p.end
			}
			f.starts[i], f.reach[i] = p.start, furthest
		}
		index[path] = f
	}
	return index
}

// covers reports whether some indexed span encloses this match.
func (e enclosures) covers(m map[string]any) bool {
	p, ok := placeOf(m)
	if !ok {
		return false
	}
	f := e[p.path]
	if f == nil {
		return false
	}
	// The last span that begins at or before this one; everything up to there
	// is a candidate, and reach says how far the best of them got.
	i := sort.Search(len(f.starts), func(i int) bool { return f.starts[i] > p.start })
	return i > 0 && f.reach[i-1] >= p.end
}

// places is the set of spans a list occupies, for the operators that ask
// whether two patterns describe exactly the same code.
type places map[place]bool

func placesOf(list []any) places {
	set := make(places, len(list))
	for _, v := range list {
		if p, ok := placeOf(object(v)); ok {
			set[p] = true
		}
	}
	return set
}

func (s places) holds(m map[string]any) bool {
	p, ok := placeOf(m)
	return ok && s[p]
}

func pathsOf(list []any) map[string]bool {
	paths := map[string]bool{}
	for _, v := range list {
		if m := object(v); m != nil {
			paths[text(m, "Path")] = true
		}
	}
	return paths
}

// RegisterWithin registers within: keep the matches some match in the other
// list encloses. This is the operator that means a real span inside a real
// span.
//
//	$all | of($calls) | within($all | of(["func $F($$$_) { $$$B }"]))
func RegisterWithin() gojq.CompilerOption {
	return filterOp("within", func(list, other []any) []any {
		covering := enclosuresOf(other)
		return keepIf(list, covering.covers)
	})
}

// RegisterOutside registers outside: keep the rest.
//
// An empty list to be outside of keeps everything, which is the right answer
// and worth saying out loud: not inside anything, because there was nothing to
// be inside.
//
//	$all | of($compare) | outside($all | of("assert($$$_)"))
func RegisterOutside() gojq.CompilerOption {
	return filterOp("outside", func(list, other []any) []any {
		covering := enclosuresOf(other)
		return keepIf(list, func(m map[string]any) bool { return !covering.covers(m) })
	})
}

// RegisterInFilesWith registers in_files_with: keep the matches in files the
// other list also matched.
//
// This is the other reading of "inside", and the one half a rule corpus means:
// an import at the top of the file, standing for "this file uses that package"
// rather than for a span the call sits in. Written as within it matches
// nothing, because a call is not inside an import statement.
//
//	$all | of($calls) | in_files_with($all | of($imports))
func RegisterInFilesWith() gojq.CompilerOption {
	return filterOp("in_files_with", func(list, other []any) []any {
		paths := pathsOf(other)
		return keepIf(list, func(m map[string]any) bool { return paths[text(m, "Path")] })
	})
}

// RegisterInFilesWithout registers in_files_without: the same question
// answered the other way.
//
//	$all | of($calls) | in_files_without($all | of($guards))
func RegisterInFilesWithout() gojq.CompilerOption {
	return filterOp("in_files_without", func(list, other []any) []any {
		paths := pathsOf(other)
		return keepIf(list, func(m map[string]any) bool { return !paths[text(m, "Path")] })
	})
}

// RegisterAtSamePlace registers at_same_place: keep the matches that start and
// end exactly where a match in the other list does.
//
// This is two patterns that must both describe the same code, as opposed to
// two patterns that must both appear somewhere.
//
//	$all | of($calls) | at_same_place($all | of($alsoTrue))
func RegisterAtSamePlace() gojq.CompilerOption {
	return filterOp("at_same_place", func(list, other []any) []any {
		taken := placesOf(other)
		return keepIf(list, taken.holds)
	})
}

// RegisterNotAt registers not_at: its negation, which is how a rule excludes a
// special case.
//
//	$all | of($writes) | not_at($all | of($constant))
func RegisterNotAt() gojq.CompilerOption {
	return filterOp("not_at", func(list, other []any) []any {
		taken := placesOf(other)
		return keepIf(list, func(m map[string]any) bool { return !taken.holds(m) })
	})
}

// capture is what a named hole in the pattern caught, or the empty string for
// a hole the pattern did not have.
func capture(m map[string]any, name string) string {
	if m == nil {
		return ""
	}
	captures, _ := m["Captures"].(map[string]any)
	s, _ := captures[name].(string)
	return s
}

// captureOp registers an operator that looks inside a match at a named hole.
func captureOp(name string, decide func(m map[string]any, hole, other string, re *regexp.Regexp) bool, needsRegex bool) gojq.CompilerOption {
	common.DeclareInput(name, common.InputPipeline)
	return common.WithFunction(name, 2, 3, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 2)
		list, err := matches(name, in)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		hole, err := common.BindString(rest[0], "hole name")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		second, err := common.BindString(rest[1], "second argument")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		var re *regexp.Regexp
		if needsRegex {
			re, err = regexp.Compile(second)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("%s($%s): %v", name, hole, err), nil)
			}
		}
		return keepIf(list, func(m map[string]any) bool { return decide(m, hole, second, re) })
	})
}

// RegisterWhereCapture registers where_capture: keep the matches whose named
// hole caught text matching a regex.
//
// A hole the pattern did not have caught nothing, and nothing matches no
// regex, so a misspelled name filters everything out rather than filtering
// nothing - the safe direction for a rule deciding what to report.
//
//	$all | of($writes) | where_capture("P"; "^\"/tmp/")
func RegisterWhereCapture() gojq.CompilerOption {
	return captureOp("where_capture", func(m map[string]any, hole, _ string, re *regexp.Regexp) bool {
		return re.MatchString(capture(m, hole))
	}, true)
}

// RegisterWhereCaptureNot registers where_capture_not, its negation.
//
//	$all | of($calls) | where_capture_not("ALGO"; "SHA-?256")
func RegisterWhereCaptureNot() gojq.CompilerOption {
	return captureOp("where_capture_not", func(m map[string]any, hole, _ string, re *regexp.Regexp) bool {
		return !re.MatchString(capture(m, hole))
	}, true)
}

// RegisterWhereSame registers where_same: keep the matches whose two named
// holes caught the same text.
//
// A pattern can write one hole twice - `$X == $X` is a comparison of something
// with itself, and select_ast reads it that way - so this is not how a rule
// says "the same code in both places" any more. It is for the other question:
// two holes that were written apart, and a rule that wants to know whether
// they landed on the same thing after all.
//
//	$all | of($assign) | where_same("A"; "B")
func RegisterWhereSame() gojq.CompilerOption {
	return captureOp("where_same", func(m map[string]any, hole, other string, _ *regexp.Regexp) bool {
		captures, _ := m["Captures"].(map[string]any)
		if _, ok := captures[hole]; !ok {
			return false
		}
		return capture(m, hole) == capture(m, other)
	}, false)
}

// RegisterWhereDifferent registers where_different, for a rule that needs the
// two holes to have caught different code.
//
//	$all | of($assign) | where_different("A"; "B")
func RegisterWhereDifferent() gojq.CompilerOption {
	return captureOp("where_different", func(m map[string]any, hole, other string, _ *regexp.Regexp) bool {
		return capture(m, hole) != capture(m, other)
	}, false)
}

// RegisterReaching registers reaching: keep the sinks that a source flows
// into.
//
// This is the operator a rule needs when neither end is a finding on its own.
// Reading a query parameter is what a web application does and running a
// command is what a program does; the finding is that the one reached the
// other.
//
//	$all | of($sinks) | reaching($all | of($sources); $all | of($sanitizers))
//
// The following is intraprocedural and syntactic - see astsearch.Reaches. It
// knows assignment, which is what carries a value from one line to the next in
// every language here, and it does not know what a library does with an
// argument. Where that is not enough it reports nothing rather than guessing,
// so a rule written this way finds less than one with a whole-program analysis
// behind it and does not find the wrong thing.
func RegisterReaching() gojq.CompilerOption {
	const op = "reaching"
	common.DeclareInput(op, common.InputPipeline)
	return common.WithFunction(op, 2, 3, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 2)
		sinks, err := matches(op, in)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		sources, err := matches(op, rest[0])
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		sanitizers, err := matches(op, rest[1])
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		// One file at a time: following a value means parsing the file it is
		// in, and a rule run over a tree has sinks in many. Each list is
		// grouped by file once, because picking one file's matches out of a
		// list is a pass over the list, and doing that per file is a pass per
		// file over all three.
		bySink, order := byFile(sinks)
		bySource, _ := byFile(sources)
		bySanitizer, _ := byFile(sanitizers)
		out := []any{}
		for _, path := range order {
			mine := bySink[path]
			reached, err := astsearch.Reaches(path, languageOf(mine),
				spansOf(bySource[path]), spansOf(mine), spansOf(bySanitizer[path]))
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("%s: %s: %v", op, path, err), nil)
			}
			for _, i := range reached {
				out = append(out, mine[i])
			}
		}
		return out
	})
}

// byFile groups a list of matches by the file they are in, and returns the
// files in the order they first appear, so that a run over one tree reports in
// the same order twice.
func byFile(list []any) (map[string][]any, []string) {
	grouped := map[string][]any{}
	var order []string
	for _, item := range list {
		path := text(object(item), "Path")
		if path == "" {
			continue
		}
		if _, seen := grouped[path]; !seen {
			order = append(order, path)
		}
		grouped[path] = append(grouped[path], item)
	}
	return grouped, order
}

func languageOf(list []any) string {
	for _, item := range list {
		if language := text(object(item), "Language"); language != "" {
			return language
		}
	}
	return ""
}

func spansOf(list []any) []astsearch.Span {
	out := make([]astsearch.Span, 0, len(list))
	for _, item := range list {
		m := object(item)
		start, ok := number(m["Offset"])
		end, done := number(m["EndOffset"])
		if !ok || !done {
			continue
		}
		out = append(out, astsearch.Span{Start: int(start), End: int(end)})
	}
	return out
}

// RegisterFocus registers focus: report the hole rather than the construct it
// was found in.
//
// A rule often matches a whole call in order to say something about one
// argument of it - `subprocess.run($CMD, shell=True)` is matched, and what the
// reader needs to look at is `shell=True`. The finding is the same finding
// either way; focus moves where it points.
//
//	$all | of($calls) | focus("TRUE") | finding("shell-true"; "...")
//
// A match whose pattern has no such hole is left where it was, because a rule
// that focuses on a hole one of its alternatives does not have still means the
// alternatives it does.
func RegisterFocus() gojq.CompilerOption {
	const op = "focus"
	common.DeclareInput(op, common.InputPipeline)
	return common.WithFunction(op, 1, 2, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		list, err := matches(op, in)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		hole, err := common.BindString(rest[0], "hole name")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", op, err), nil)
		}
		out := make([]any, 0, len(list))
		for _, item := range list {
			out = append(out, focused(object(item), hole))
		}
		return out
	})
}

// focused is one match moved onto one of its holes.
func focused(m map[string]any, hole string) any {
	if m == nil {
		return m
	}
	spans, _ := m["CaptureSpans"].(map[string]any)
	span, _ := spans[hole].(map[string]any)
	if span == nil {
		return m
	}
	moved := make(map[string]any, len(m))
	for k, value := range m {
		moved[k] = value
	}
	for _, key := range []string{"LineNumber", "Column", "EndLineNumber", "EndColumn", "Offset", "EndOffset"} {
		if at, ok := span[key]; ok {
			moved[key] = at
		}
	}
	if caught, ok := m["Captures"].(map[string]any); ok {
		if s, ok := caught[hole].(string); ok {
			moved["Text"] = s
		}
	}
	return moved
}

// RegisterWhereCaptureAst registers where_capture_ast: keep the matches whose
// named hole caught something that is itself a match for another pattern.
//
// This is a constraint on a hole written as syntax rather than as a regex -
// "the argument, but only when it is a call to getenv" - and it is what a rule
// reaches for when a regex over the text would be a guess.
//
//	$all | of($calls) | where_capture_ast("ARG"; "os.getenv($$$_)")
func RegisterWhereCaptureAst() gojq.CompilerOption {
	const op = "where_capture_ast"
	common.DeclareInput(op, common.InputPipeline)
	return common.WithFunction(op, 2, 3, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 2)
		list, err := matches(op, in)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		hole, err := common.BindString(rest[0], "hole name")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", op, err), nil)
		}
		pattern, err := common.BindString(rest[1], "pattern")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", op, err), nil)
		}
		// Answers are remembered for the length of this call. Deciding one
		// means parsing the fragment, and a rule's matches catch the same
		// fragment over and over - the same variable, the same call, once per
		// place it is used - so a list of a thousand matches usually asks a
		// few dozen distinct questions.
		decided := map[string]bool{}
		return keepIf(list, func(m map[string]any) bool {
			caught := capture(m, hole)
			if caught == "" {
				return false
			}
			language := text(m, "Language")
			key := language + "\x00" + caught
			if answer, asked := decided[key]; asked {
				return answer
			}
			// A pattern that is not code in this match's language cannot
			// constrain it, and saying so by keeping nothing is the safe
			// direction for a rule deciding what to report.
			ok, err := astsearch.MatchesText(pattern, language, caught)
			answer := err == nil && ok
			decided[key] = answer
			return answer
		})
	})
}

// comparison splits a rule's `$X > 5` into the three parts it is made of.
var comparison = regexp.MustCompile(`^\s*\$?([A-Za-z_][A-Za-z_0-9]*)\s*(<=|>=|==|!=|<|>)\s*(-?[0-9]+(?:\.[0-9]+)?|0[bBoOxX][0-9A-Fa-f]+)\s*$`)

// RegisterWhereCaptureCompare registers where_capture_compare: keep the
// matches whose named hole caught a number standing in some relation to
// another.
//
// File modes are what this is for. `chmod($PATH, $MODE)` finds every chmod;
// only `$MODE & 0o002` - or, as the corpus writes it, `$MODE >= 0o666` - says
// which ones are worth reporting.
//
//	$all | of($calls) | where_capture_compare("MODE"; "$MODE >= 0o666")
//
// The hole's text is read as a number the way the language wrote it, so 0o777,
// 0x1FF and 511 are the same value, and quotes around it are ignored - a mode
// passed as a string is the same mode.
func RegisterWhereCaptureCompare() gojq.CompilerOption {
	const op = "where_capture_compare"
	common.DeclareInput(op, common.InputPipeline)
	return common.WithFunction(op, 1, 2, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		list, err := matches(op, in)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		expr, err := common.BindString(rest[0], "comparison")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", op, err), nil)
		}
		parts := comparison.FindStringSubmatch(expr)
		if parts == nil {
			return common.MakeUDFErrorResult(fmt.Errorf(
				"%s: %q is not a comparison of a hole with a number, which is all this reads: "+
					"$MODE >= 0o666", op, expr), nil)
		}
		hole, operator := parts[1], parts[2]
		want, ok := asNumber(parts[3])
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %q is not a number", op, parts[3]), nil)
		}
		return keepIf(list, func(m map[string]any) bool {
			got, ok := asNumber(capture(m, hole))
			if !ok {
				return false
			}
			switch operator {
			case "<":
				return got < want
			case "<=":
				return got <= want
			case ">":
				return got > want
			case ">=":
				return got >= want
			case "==":
				return got == want
			default:
				return got != want
			}
		})
	})
}

// asNumber reads a hole's text as a number, in whatever base it was written
// in, with any quotes around it ignored.
func asNumber(s string) (float64, bool) {
	s = strings.TrimSpace(strings.Trim(strings.TrimSpace(s), `"'`))
	if s == "" {
		return 0, false
	}
	negative := strings.HasPrefix(s, "-")
	digits := strings.TrimPrefix(s, "-")
	// 0o777 and 0777 are the same mode, and a language writes it both ways.
	if len(digits) > 1 && digits[0] == '0' && !strings.ContainsAny(digits, ".eE") {
		rest := digits[1:]
		if rest[0] == 'o' || rest[0] == 'O' {
			rest = rest[1:]
		}
		if n, err := strconv.ParseInt(rest, 8, 64); err == nil && !strings.ContainsAny(rest, "xXbB89") {
			return sign(negative) * float64(n), true
		}
	}
	if n, err := strconv.ParseInt(digits, 0, 64); err == nil {
		return sign(negative) * float64(n), true
	}
	if f, err := strconv.ParseFloat(digits, 64); err == nil {
		return sign(negative) * f, true
	}
	return 0, false
}

func sign(negative bool) float64 {
	if negative {
		return -1
	}
	return 1
}

// metaVariable is a `$NAME` reference in a rule's message.
var metaVariable = regexp.MustCompile(`\$([A-Z_][A-Za-z_0-9]*)`)

// interpolate fills a message in from what the match caught. A name the
// pattern does not have is left as written, because a message that says
// `$1,000` means a thousand dollars.
func interpolate(message string, m map[string]any) string {
	if !strings.Contains(message, "$") {
		return message
	}
	captures, _ := m["Captures"].(map[string]any)
	return metaVariable.ReplaceAllStringFunc(message, func(ref string) string {
		if s, ok := captures[ref[1:]].(string); ok {
			return s
		}
		return ref
	})
}

// RegisterFinding registers finding: render matches as findings.
//
//	$all | of($calls) | finding("go-weak-hash"; "this hash is not collision resistant")
//	$all | of($chosen) | finding("java-weak-crypto"; "$ALGO has known breaks")
//
// The message is a template rather than a filter: `$NAME` in it is replaced by
// what that hole in the pattern caught, which is the convention the rules this
// vocabulary was built against are written in. A name no hole has is left
// alone.
func RegisterFinding() gojq.CompilerOption {
	const op = "finding"
	common.DeclareInput(op, common.InputPipeline)
	return common.WithFunctionOf(op, 2, 3, Finding, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 2)
		list, err := matches(op, in)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		id, err := common.BindString(rest[0], "rule id")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", op, err), nil)
		}
		message, err := common.BindString(rest[1], "message")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", op, err), nil)
		}
		out := make([]any, 0, len(list))
		for _, item := range list {
			m := object(item)
			if m == nil {
				continue
			}
			out = append(out, Finding.Build(map[string]any{
				"RuleId":        id,
				"Path":          text(m, "Path"),
				"LineNumber":    m["LineNumber"],
				"Column":        m["Column"],
				"EndLineNumber": m["EndLineNumber"],
				"Message":       interpolate(message, m),
				"Match":         text(m, "Text"),
				"PwrqValue":     text(m, "Path"),
			}))
		}
		return out
	})
}

// RegisterReport registers report: put findings in the order a person reads
// them, and drop the duplicates that alternatives produce when two of them
// describe the same code.
//
//	($all | of($a) | finding("x"; "...")) + ($all | of($b) | finding("x"; "...")) | report
func RegisterReport() gojq.CompilerOption {
	const op = "report"
	common.DeclareInput(op, common.InputPipeline)
	return common.WithFunction(op, 0, 1, func(v any, args []any) any {
		in, _ := common.SplitInput(v, args, 0)
		list, err := matches(op, in)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		// The key is built once per finding rather than inside the comparison,
		// which asks for it log n times, and again for the duplicate check.
		// Building it is a Sprintf, so a report of a thousand findings was
		// twenty thousand of them.
		findings := make([]keyed, len(list))
		for i, f := range list {
			m := object(f)
			findings[i] = keyed{value: f, order: sortKey(m), match: text(m, "Match")}
		}
		sort.SliceStable(findings, func(a, b int) bool { return findings[a].order < findings[b].order })
		out := make([]any, 0, len(findings))
		seen := make(map[string]bool, len(findings))
		for _, f := range findings {
			key := f.order + "\x00" + f.match
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, f.value)
		}
		return out
	})
}

// keyed is one finding with the strings the report is built from, so that
// neither the sort nor the duplicate check has to derive them again.
type keyed struct {
	value any
	order string
	match string
}

// sortKey orders findings the way a person reads a report: by file, then down
// the file, then by rule where two of them land on one line.
func sortKey(m map[string]any) string {
	line, _ := number(m["LineNumber"])
	column, _ := number(m["Column"])
	return fmt.Sprintf("%s\x00%09.0f\x00%09.0f\x00%s", text(m, "Path"), line, column, text(m, "RuleId"))
}

// RegisterVocabulary registers the operators a rule is written with.
func RegisterVocabulary() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterScanAst(),
		RegisterScanRegex(),
		RegisterOf(),
		RegisterWithin(),
		RegisterOutside(),
		RegisterInFilesWith(),
		RegisterInFilesWithout(),
		RegisterAtSamePlace(),
		RegisterNotAt(),
		RegisterWhereCapture(),
		RegisterWhereCaptureNot(),
		RegisterWhereSame(),
		RegisterWhereDifferent(),
		RegisterWhereText(),
		RegisterWhereTextNot(),
		RegisterWhereCaptureAst(),
		RegisterWhereCaptureCompare(),
		RegisterWhereCaptureEntropy(),
		RegisterWhereCaptureRedos(),
		RegisterFocus(),
		RegisterReaching(),
		RegisterFinding(),
		RegisterReport(),
	}
}
