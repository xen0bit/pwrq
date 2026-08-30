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
// in a file or a tree, as an object naming the file, the line and what each
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
// leaves one gap, and it is worth naming. A permissive grammar accepts almost
// anything - markdown parses `func $$$(` as a paragraph - so over a mixed tree
// a mistyped pattern can be "valid" somewhere, match nothing, and come back as
// an ordinary empty result. Naming a Language, or narrowing with Include to
// one extension, removes the ambiguity and gets the pattern checked outright.
func RegisterSelectAst() gojq.CompilerOption {
	common.DeclareInput("select_ast", common.InputPipeline)
	return common.WithIterFunctionOf("select_ast", 1, 3, AstMatch, func(v any, args []any) gojq.Iter {
		in, rest := common.SplitInput(v, args, 1)
		root, ok := common.BindPath(in)
		if !ok {
			return gojq.NewIter(fmt.Errorf("select_ast: expected a path, got %T", common.BindValue(in)))
		}
		pattern, err := common.BindString(rest[0], "pattern")
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
			c, err := compilePattern(pattern, opts.language)
			if err != nil {
				return gojq.NewIter(fmt.Errorf("select_ast: %v", err))
			}
			if err := c.validate(); err != nil {
				return gojq.NewIter(fmt.Errorf("select_ast: %v", err))
			}
		}
		walk, err := filewalk.New(root, opts.include)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("select_ast: %v", err))
		}
		return &matchIter{pattern: pattern, opts: opts, walk: walk}
	})
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
	pattern string
	opts    selectAstOpts
	walk    *filewalk.Walker
	// patterns holds one compiled pattern per language met so far. A tree can
	// hold five languages, and recompiling per file would parse the pattern
	// once per file rather than once per language.
	patterns map[string]*compiled
	ready    []any
	done     bool

	// skipped names the languages the pattern could not parse as, and matched
	// counts the hits found. Together they answer the question an empty result
	// leaves open. See exhausted.
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
// file whose language the pattern does not parse as is skipped rather than
// reported: searching a repository for `func $N() error { $$$B }` should not
// fail on its Dockerfile, and the first version of this cmdlet did exactly
// that.
//
// Skipping quietly has a cost, though, and it is the cost this whole package
// exists to avoid: a mistyped pattern parses as nothing, is skipped
// everywhere, and comes back as an empty result that reads like an answer
// about the code. So the two are separated at the end. If some file was
// searched, an empty result is an answer. If every file was skipped, it is
// not, and the caller is told which languages were met and that the pattern
// parsed as none of them.
func (it *matchIter) exhausted() (any, bool) {
	if it.matched > 0 || len(it.skipped) == 0 {
		return nil, false
	}
	for _, c := range it.patterns {
		if c.valid {
			// Some language took the pattern; nothing matching it is a fact
			// about the tree.
			return nil, false
		}
	}
	names := make([]string, 0, len(it.skipped))
	for name := range it.skipped {
		names = append(names, name)
	}
	sort.Strings(names)
	return fmt.Errorf("select_ast: pattern %q does not parse as code in any language found "+
		"under this path (%s), so every file was skipped and the empty result says nothing "+
		"about the code; ast_pattern shows what a pattern compiles to",
		it.pattern, strings.Join(names, ", ")), true
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

	c, err := it.patternFor(entry.Name)
	if err != nil {
		return nil, err
	}
	// A pattern that is not code in this file's language cannot match it, and
	// says nothing about whether it is code in the language the caller meant.
	// Recorded rather than raised; exhausted decides what the silence meant.
	if !c.valid {
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
	results, err := c.query.Match(source)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}

	out := make([]any, 0, len(results))
	for _, r := range results {
		out = append(out, matchObject(path, entry.Name, source, r))
	}
	return out, nil
}

// patternFor compiles the pattern for one language, once.
func (it *matchIter) patternFor(language string) (*compiled, error) {
	if it.patterns == nil {
		it.patterns = map[string]*compiled{}
	}
	if c, ok := it.patterns[language]; ok {
		return c, nil
	}
	c, err := compilePattern(it.pattern, language)
	if err != nil {
		return nil, err
	}
	it.patterns[language] = c
	return c, nil
}

// matchObject renders one match.
func matchObject(path, language string, source []byte, r grep.Result) any {
	line, column := position(source, r.StartByte)
	endLine, _ := position(source, r.EndByte)

	captures := map[string]any{}
	for name, c := range r.Captures {
		captures[name] = string(c.Text)
	}

	return AstMatch.Build(map[string]any{
		"Path":          path,
		"Language":      language,
		"LineNumber":    line,
		"Column":        column,
		"EndLineNumber": endLine,
		"Text":          string(source[r.StartByte:min(r.EndByte, uint32(len(source)))]),
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
// A pattern that is not valid code still compiles - into a tree-sitter query
// full of ERROR nodes, which runs against every file and matches none of them.
// Without this, a typo and an honest absence produce the same empty result,
// and the caller reads the wrong one.
//
//	ast_pattern("func $N($$$A) error { $$$B }"; "go") | .Valid
//	ast_pattern("except $E: $$$B"; "python") | .Query
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
			"Valid":         c.valid,
			"MetaVariables": c.metaVariables(),
			"Query":         c.query.SExpr,
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
