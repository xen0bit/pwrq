package pwrgrep

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
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
func filterOp(name string, decide func(match map[string]any, other []any) bool) gojq.CompilerOption {
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
		return keepIf(list, func(m map[string]any) bool { return decide(m, other) })
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

// encloses reports whether one match's span covers another's.
func encloses(outer, inner map[string]any) bool {
	outerPath, outerStart, outerEnd, ok := span(outer)
	if !ok {
		return false
	}
	innerPath, innerStart, innerEnd, ok := span(inner)
	if !ok {
		return false
	}
	return outerPath == innerPath && outerStart <= innerStart && outerEnd >= innerEnd
}

// samePlace reports whether two matches describe exactly the same code.
func samePlace(a, b map[string]any) bool {
	aPath, aStart, aEnd, ok := span(a)
	if !ok {
		return false
	}
	bPath, bStart, bEnd, ok := span(b)
	if !ok {
		return false
	}
	return aPath == bPath && aStart == bStart && aEnd == bEnd
}

func anyOf(list []any, pred func(map[string]any) bool) bool {
	for _, v := range list {
		if pred(object(v)) {
			return true
		}
	}
	return false
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
	return filterOp("within", func(m map[string]any, other []any) bool {
		return anyOf(other, func(o map[string]any) bool { return encloses(o, m) })
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
	return filterOp("outside", func(m map[string]any, other []any) bool {
		return !anyOf(other, func(o map[string]any) bool { return encloses(o, m) })
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
	return filterOp("in_files_with", func(m map[string]any, other []any) bool {
		return pathsOf(other)[text(m, "Path")]
	})
}

// RegisterInFilesWithout registers in_files_without: the same question
// answered the other way.
//
//	$all | of($calls) | in_files_without($all | of($guards))
func RegisterInFilesWithout() gojq.CompilerOption {
	return filterOp("in_files_without", func(m map[string]any, other []any) bool {
		return !pathsOf(other)[text(m, "Path")]
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
	return filterOp("at_same_place", func(m map[string]any, other []any) bool {
		return anyOf(other, func(o map[string]any) bool { return samePlace(o, m) })
	})
}

// RegisterNotAt registers not_at: its negation, which is how a rule excludes a
// special case.
//
//	$all | of($writes) | not_at($all | of($constant))
func RegisterNotAt() gojq.CompilerOption {
	return filterOp("not_at", func(m map[string]any, other []any) bool {
		return !anyOf(other, func(o map[string]any) bool { return samePlace(o, m) })
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
		findings := append([]any(nil), list...)
		sort.SliceStable(findings, func(a, b int) bool {
			return sortKey(object(findings[a])) < sortKey(object(findings[b]))
		})
		out := make([]any, 0, len(findings))
		seen := map[string]bool{}
		for _, f := range findings {
			key := sortKey(object(f)) + "\x00" + text(object(f), "Match")
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, f)
		}
		return out
	})
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
		RegisterFinding(),
		RegisterReport(),
	}
}
