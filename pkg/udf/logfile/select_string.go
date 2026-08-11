// Select-String: searching a tree of files for a pattern, with context.
package logfile

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterSelectString registers select_string, PowerShell's Select-String:
// every line matching a pattern, as an object naming the file, the line number
// and the surrounding lines.
//
// grep_lines already reads one file and returns bare strings. This is the other
// half: it walks a directory, it reports where each hit came from, and it can
// carry context lines, none of which a list of strings can express.
//
//	select_string("src"; "TODO")
//	select_string("src"; "panic"; {Include: "*.go", Context: 2})
//	find("."; "dir") | select_string("TODO")
//
// The options object is only available in the explicit-path form: the path is
// one operand, so at two arguments the leading one is always the path. Piping a
// path and passing options at the same time is an error rather than a guess.
func RegisterSelectString() gojq.CompilerOption {
	return gojq.WithFunction("select_string", 1, 3, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		root, ok := common.BindPath(in)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("select_string: expected a path, got %T", common.BindValue(in)), nil)
		}
		pattern, err := common.BindString(rest[0], "pattern")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("select_string: %v", err), nil)
		}
		opts, err := selectStringOptions(rest[1:])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("select_string: %v", err), nil)
		}
		if !opts.caseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("select_string: %v", err), nil)
		}
		matches, err := searchTree(root, re, opts)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("select_string: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(matches, nil)
	})
}

type selectOpts struct {
	include       string
	context       int
	caseSensitive bool
	listOnly      bool
}

func selectStringOptions(args []any) (selectOpts, error) {
	o := selectOpts{}
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
				return o, fmt.Errorf("Include must be a string")
			}
			o.include = s
		case "context":
			f, ok := common.ToInt(val)
			if !ok || f < 0 {
				return o, fmt.Errorf("Context must be a non-negative integer")
			}
			o.context = f
		case "casesensitive":
			b, ok := val.(bool)
			if !ok {
				return o, fmt.Errorf("CaseSensitive must be a boolean")
			}
			o.caseSensitive = b
		case "list":
			b, ok := val.(bool)
			if !ok {
				return o, fmt.Errorf("List must be a boolean")
			}
			o.listOnly = b
		default:
			return o, fmt.Errorf("unknown option %q", k)
		}
	}
	return o, nil
}

func searchTree(root string, re *regexp.Regexp, o selectOpts) ([]any, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	out := []any{}
	search := func(path string) error {
		hits, err := searchFile(path, re, o)
		if err != nil {
			return err
		}
		out = append(out, hits...)
		return nil
	}
	if !info.IsDir() {
		return out, search(root)
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip the directories nobody means to grep.
			if name := d.Name(); name == ".git" || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if o.include != "" {
			ok, err := filepath.Match(o.include, d.Name())
			if err != nil || !ok {
				return nil
			}
		}
		return search(p)
	})
	return out, err
}

func searchFile(path string, re *regexp.Regexp, o selectOpts) ([]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}

	out := []any{}
	for i, line := range lines {
		if !re.MatchString(line) {
			continue
		}
		if o.listOnly {
			// List mode answers "which files", so the first hit is enough.
			return []any{map[string]any{
				psobject.PSTypeNameKey: "Microsoft.PowerShell.Commands.MatchInfo",
				"Path":                 path,
				psobject.PSPathKey:     path,
			}}, nil
		}
		m := map[string]any{
			psobject.PSTypeNameKey: "Microsoft.PowerShell.Commands.MatchInfo",
			"Path":                 path,
			"LineNumber":           i + 1,
			"Line":                 line,
			"Match":                re.FindString(line),
			psobject.PSPathKey:     path,
		}
		if o.context > 0 {
			lo := max(0, i-o.context)
			hi := min(len(lines), i+o.context+1)
			before := make([]any, 0, i-lo)
			for _, l := range lines[lo:i] {
				before = append(before, l)
			}
			after := make([]any, 0, hi-i-1)
			for _, l := range lines[i+1 : hi] {
				after = append(after, l)
			}
			m["Before"], m["After"] = before, after
		}
		out = append(out, m)
	}
	return out, nil
}
