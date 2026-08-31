package astsearch

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/grep"
	"github.com/xen0bit/pwrq/pkg/core/filewalk"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterSelectAst registers select_ast: every place a piece of syntax occurs
// in a file or a tree, as an object naming the file, the span and what each
// hole in the pattern caught.
//
// It is select_string's structural sibling, and the difference is what a regex
// cannot express. `func $NAME($$$ARGS) error` finds a Go function returning an
// error whatever its spacing, whatever its line breaks, and never inside a
// comment or a string literal - because the match is against the parse tree.
// The same search written as a regex is a guess that gets the easy half.
//
//	select_ast("src"; "func $NAME($$$ARGS) error { $$$BODY }") | .Captures.NAME
//	[select_ast("."; "if $C { return $E }"; {Include: "*.go"})] | length
//	select_ast("app.py"; "except $E: $$$B"; {Language: "python"})
//
// A pattern says how many children a construct has, so `f($A, $B)` is a call
// with two arguments and not a call whose first two are these. $$$_ is how a
// pattern says "and anything else here":
//
//	select_ast("."; "exec.Command($NAME, $$$_)")
//	select_ast("."; "tls.Config{$$$_, InsecureSkipVerify: true, $$$_}")
//
// The pattern may also be a list of patterns, which is one search rather than
// several: the tree is walked once and each file parsed once however many
// patterns there are, and every match reports in Pattern which one found it.
// That is the difference between asking forty questions of a repository and
// asking one question forty times.
//
//	select_ast("."; ["md5.New()", "sha1.New()"]) | {.Pattern, .Path}
//
// It streams, like every cmdlet that enumerates something, and the walk is
// lazy: `first(select_ast("."; "..."))` parses files until the first hit
// rather than parsing the tree.
//
// Files whose extension names no grammar this build carries are skipped rather
// than reported. A tree is mostly not the language being searched, and an
// error per README would drown the answer.
//
// The same goes for a file whose language the pattern is not code in: a Go
// pattern searched over a repository must not fail on its Dockerfile. That
// leaves one gap, and it is worth naming. A grammar can parse a pattern as the
// wrong construct without complaining - see patternProblem for the three ways
// that is caught and the one that is not - so over a mixed tree a pattern can
// be code somewhere, match nothing, and come back as an ordinary empty result.
// Naming a Language, or narrowing with Include to one extension, removes the
// ambiguity and gets the pattern checked outright.
func RegisterSelectAst() gojq.CompilerOption {
	common.DeclareInput("select_ast", common.InputPipeline)
	return common.WithIterFunctionOf("select_ast", 1, 3, AstMatch, func(v any, args []any) gojq.Iter {
		in, rest := common.SplitInput(v, args, 1)
		root, ok := common.BindPath(in)
		if !ok {
			return gojq.NewIter(fmt.Errorf("select_ast: expected a path, got %T", common.BindValue(in)))
		}
		patterns, err := bindPatterns(rest[0])
		if err != nil {
			return gojq.NewIter(fmt.Errorf("select_ast: %v", err))
		}
		opts, err := selectAstOptions(rest[1:])
		if err != nil {
			return gojq.NewIter(fmt.Errorf("select_ast: %v", err))
		}
		// Compiled once here rather than per file, so that a pattern which
		// cannot match anything is reported as a failure of the pattern before
		// a single file is read - not as an empty result after all of them.
		if opts.language != "" {
			for _, pattern := range patterns {
				c, err := compilePattern(pattern, opts.language)
				if err != nil {
					return gojq.NewIter(fmt.Errorf("select_ast: %v", err))
				}
				if err := c.validate(); err != nil {
					return gojq.NewIter(fmt.Errorf("select_ast: %v", err))
				}
			}
		}
		walk, err := filewalk.New(root, opts.include)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("select_ast: %v", err))
		}
		return &matchIter{patterns: patterns, opts: opts, walk: walk}
	})
}

// bindPatterns reads the pattern argument, which is either one pattern or a
// list of them.
func bindPatterns(arg any) ([]string, error) {
	if list, ok := common.BindValue(arg).([]any); ok {
		if len(list) == 0 {
			return nil, fmt.Errorf("the pattern list is empty, so there is nothing to search for")
		}
		patterns := make([]string, len(list))
		for i, item := range list {
			pattern, err := common.BindString(item, "pattern")
			if err != nil {
				return nil, fmt.Errorf("pattern %d: %v", i+1, err)
			}
			patterns[i] = pattern
		}
		return patterns, nil
	}
	pattern, err := common.BindString(arg, "pattern")
	if err != nil {
		return nil, err
	}
	return []string{pattern}, nil
}

type selectAstOpts struct {
	include  string
	language string
}

func selectAstOptions(args []any) (selectAstOpts, error) {
	o := selectAstOpts{}
	if len(args) == 0 {
		return o, nil
	}
	m, ok := common.BindValue(args[0]).(map[string]any)
	if !ok {
		return o, fmt.Errorf("options must be an object, got %T", common.BindValue(args[0]))
	}
	for k, val := range m {
		switch strings.ToLower(k) {
		case "include":
			s, ok := val.(string)
			if !ok {
				return o, fmt.Errorf("expected a string for Include")
			}
			o.include = s
		case "language":
			s, ok := val.(string)
			if !ok {
				return o, fmt.Errorf("expected a string for Language")
			}
			o.language = s
		default:
			return o, fmt.Errorf("unknown option %q", k)
		}
	}
	return o, nil
}

// matchIter parses files as the caller reads matches.
type matchIter struct {
	patterns []string
	opts     selectAstOpts
	walk     *filewalk.Walker
	// compiled holds the patterns compiled for one language, in the order they
	// were written. A tree can hold five languages, and compiling per file
	// would parse every pattern once per file rather than once per language.
	compiled map[string][]*compiled
	ready    []any
	done     bool

	// skipped names the languages no pattern was code in, and matched counts
	// the hits found. Together they answer the question an empty result leaves
	// open. See exhausted.
	skipped map[string]bool
	matched int
}

func (it *matchIter) Next() (any, bool) {
	for {
		if len(it.ready) > 0 {
			v := it.ready[0]
			it.ready = it.ready[1:]
			return v, true
		}
		if it.done {
			return nil, false
		}
		path, ok, err := it.walk.Next()
		if err != nil {
			it.done = true
			return fmt.Errorf("select_ast: %v", err), true
		}
		if !ok {
			it.done = true
			return it.exhausted()
		}
		matches, err := it.searchFile(path)
		if err != nil {
			it.done = true
			return fmt.Errorf("select_ast: %v", err), true
		}
		it.matched += len(matches)
		it.ready = matches
	}
}

// exhausted ends the stream, turning one particular empty result into the
// error it really is.
//
// A pattern is written in one language, and a tree is written in several. So a
// file no pattern is code in is skipped rather than reported: searching a
// repository for `func $N() error { $$$B }` should not fail on its Dockerfile,
// and the first version of this cmdlet did exactly that.
//
// Skipping quietly has a cost, though, and it is the cost this whole package
// exists to avoid: a mistyped pattern is code in nothing, is skipped
// everywhere, and comes back as an empty result that reads like an answer
// about the code. So the two are separated at the end. If some file was
// searched, an empty result is an answer. If every file was skipped, it is
// not, and the caller is told which languages were met and that no pattern was
// code in any of them.
func (it *matchIter) exhausted() (any, bool) {
	if it.matched > 0 || len(it.skipped) == 0 {
		return nil, false
	}
	for _, forLanguage := range it.compiled {
		for _, c := range forLanguage {
			if c.valid() {
				// Some language took some pattern; nothing matching is a fact
				// about the tree.
				return nil, false
			}
		}
	}
	names := make([]string, 0, len(it.skipped))
	for name := range it.skipped {
		names = append(names, name)
	}
	sort.Strings(names)
	subject := fmt.Sprintf("pattern %q is not", it.patterns[0])
	if len(it.patterns) > 1 {
		subject = fmt.Sprintf("none of the %d patterns is", len(it.patterns))
	}
	return fmt.Errorf("select_ast: %s code in any language found under this path, so "+
		"every file was skipped (%s) and the empty result says nothing about the code; "+
		"ast_pattern shows what a pattern compiles to, and Include narrows the walk to "+
		"the files a pattern is for",
		subject, strings.Join(names, ", ")), true
}

// hit is one match together with the pattern that found it, so that results
// from several patterns can be ordered by where they are in the file rather
// than by which pattern was written first.
type hit struct {
	result  grep.Result
	pattern int
}

// searchFile returns every match in one file, or nothing when the file is not
// in a language this build can parse.
func (it *matchIter) searchFile(path string) ([]any, error) {
	entry, err := languageFor(path, it.opts.language)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	compiled, err := it.patternsFor(entry.Name)
	if err != nil {
		return nil, err
	}
	// A pattern that is not code in this file's language cannot match it, and
	// says nothing about whether it is code in the language the caller meant.
	// Recorded rather than raised; exhausted decides what the silence meant.
	usable := false
	for _, c := range compiled {
		if c.valid() {
			usable = true
			break
		}
	}
	if !usable {
		if it.skipped == nil {
			it.skipped = map[string]bool{}
		}
		it.skipped[entry.Name] = true
		return nil, nil
	}

	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var hits []hit
	for i, c := range compiled {
		if !c.valid() {
			continue
		}
		// A pattern with more than one reading is matched under each of
		// them, and the same code found twice is one finding. See
		// statementReading.
		seen := map[[2]uint32]bool{}
		for _, q := range c.queries {
			results, err := q.Match(source)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", path, err)
			}
			for _, r := range results {
				span := [2]uint32{r.StartByte, r.EndByte}
				if seen[span] {
					continue
				}
				seen[span] = true
				hits = append(hits, hit{result: r, pattern: i})
			}
		}
	}
	// A file reads top to bottom whatever order the patterns were written in.
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].result.StartByte != hits[b].result.StartByte {
			return hits[a].result.StartByte < hits[b].result.StartByte
		}
		return hits[a].pattern < hits[b].pattern
	})

	out := make([]any, 0, len(hits))
	for _, h := range hits {
		out = append(out, matchObject(path, entry.Name, it.patterns[h.pattern], source, h.result))
	}
	return out, nil
}

// patternsFor compiles every pattern for one language, once.
func (it *matchIter) patternsFor(language string) ([]*compiled, error) {
	if it.compiled == nil {
		it.compiled = map[string][]*compiled{}
	}
	if c, ok := it.compiled[language]; ok {
		return c, nil
	}
	compiled := make([]*compiled, len(it.patterns))
	for i, pattern := range it.patterns {
		c, err := compilePattern(pattern, language)
		if err != nil {
			return nil, err
		}
		compiled[i] = c
	}
	it.compiled[language] = compiled
	return compiled, nil
}

// matchObject renders one match.
func matchObject(path, language, pattern string, source []byte, r grep.Result) any {
	line, column := position(source, r.StartByte)
	endLine, endColumn := position(source, r.EndByte)
	end := min(int(r.EndByte), len(source))

	// The rewritten query carries captures of its own: the whole matched
	// construct, which is where the span comes from; the ellipsis, which is a
	// hole the caller said not to look at; and the second and later halves of
	// a hole written twice, which hold the same text as the first. None of
	// them is a name anybody wrote, so none is reported. See sexp.go.
	captures := map[string]any{}
	for name, c := range r.Captures {
		if name == rootCapture || name == ellipsisName || strings.Contains(name, againSuffix) {
			continue
		}
		captures[name] = string(c.Text)
	}

	return AstMatch.Build(map[string]any{
		"Path":          path,
		"Language":      language,
		"Pattern":       pattern,
		"LineNumber":    line,
		"Column":        column,
		"EndLineNumber": endLine,
		"EndColumn":     endColumn,
		"Offset":        int(r.StartByte),
		"EndOffset":     end,
		"Text":          string(source[min(int(r.StartByte), end):end]),
		"Captures":      captures,
		"PwrqValue":     path,
	})
}

// position converts a byte offset into the one-based line and column a caller
// would use to open the file at it.
//
// Column counts bytes rather than runes, which is what an editor's :line:col
// wants and what every other pwrq path-and-position pair reports.
func position(source []byte, offset uint32) (int, int) {
	if int(offset) > len(source) {
		offset = uint32(len(source))
	}
	before := source[:offset]
	line := bytes.Count(before, []byte{'\n'}) + 1
	column := offset - uint32(bytes.LastIndexByte(before, '\n')+1) + 1
	return line, int(column)
}

// RegisterAstPattern registers ast_pattern: what a pattern compiles to, and
// whether it can match anything at all.
//
// This is validate_query for code patterns, and it exists for the same reason.
// A pattern that is not code in a language still compiles - into a tree-sitter
// query that runs against every file and matches none of them - so without
// this a typo and an honest absence produce the same empty result and the
// caller reads the wrong one.
//
//	ast_pattern("func $N($$$A) error { $$$B }"; "go") | .Valid
//	ast_pattern("except $E: $$$B"; "python") | .Query
//	ast_pattern("md5($X)"; "php") | .Problem
func RegisterAstPattern() gojq.CompilerOption {
	common.DeclareInput("ast_pattern", common.InputPipeline)
	return common.WithFunctionOf("ast_pattern", 1, 2, PatternInfo, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		pattern, err := common.BindString(in, "pattern")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("ast_pattern: %v", err), nil)
		}
		language, err := common.BindString(rest[0], "language")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("ast_pattern: %v", err), nil)
		}
		c, err := compilePattern(pattern, language)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("ast_pattern: %v", err), nil)
		}
		return PatternInfo.Build(map[string]any{
			"Pattern":       c.pattern,
			"Language":      c.language,
			"Valid":         c.valid(),
			"Problem":       c.problem,
			"MetaVariables": c.metaVariables(),
			"Query":         queryText(c),
			"PwrqValue":     c.pattern,
		})
	})
}

// RegisterGetAstLanguage registers get_ast_language: the languages this binary
// can parse.
//
// Which grammars are present is decided when the binary is built, so the only
// honest answer comes from the registry the parser itself consults. A list
// written down beside it would be right until the day someone changed the
// build tags.
//
//	[get_ast_language] | length
//	get_ast_language("python") | .Extensions
func RegisterGetAstLanguage() gojq.CompilerOption {
	common.DeclareInput("get_ast_language", common.InputPipeline)
	return common.WithIterFunctionOf("get_ast_language", 0, 1, LanguageInfo, func(v any, args []any) gojq.Iter {
		in, _ := common.SplitInput(v, args, 0)
		needle := ""
		if bound := common.BindValue(in); bound != nil {
			s, ok := bound.(string)
			if !ok {
				return gojq.NewIter(fmt.Errorf("get_ast_language: expected a name to filter by, got %T", bound))
			}
			needle = strings.ToLower(s)
		}

		var out []any
		for _, entry := range grammars.AllLanguages() {
			if needle != "" && !strings.Contains(strings.ToLower(entry.Name), needle) {
				continue
			}
			extensions := make([]any, len(entry.Extensions))
			for i, ext := range entry.Extensions {
				extensions[i] = ext
			}
			out = append(out, LanguageInfo.Build(map[string]any{
				"Name":       entry.Name,
				"Extensions": extensions,
				"PwrqValue":  entry.Name,
			}))
		}
		return gojq.NewIter(out...)
	})
}

// RegisterAll registers every structural search cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterSelectAst(),
		RegisterAstPattern(),
		RegisterGetAstLanguage(),
	}
}

// queryText is what a pattern compiled to, or an empty string when it would
// not compile for the language at all. Problem says which.
func queryText(c *compiled) string {
	if c.query() == nil {
		return ""
	}
	return c.query().SExpr
}
