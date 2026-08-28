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
//	select_string("src"; "TODO") | .Path
//	[select_string("src"; "panic"; {Include: "*.go", Context: 2})]
//	find("."; "dir") | select_string("TODO")
//
// It emits one object per match, as a stream. That matches get_childitem, find
// and every other cmdlet that enumerates something, so a caller collects with
// [...] or does not, by the same rule everywhere.
//
// The stream is also lazy: the walk advances only as far as the next match the
// caller reads, so `first(select_string("."; "needle"))` stops at the first hit
// instead of grepping the whole tree first.
//
// The options object is only available in the explicit-path form: the path is
// one operand, so at two arguments the leading one is always the path. Piping a
// path and passing options at the same time is an error rather than a guess.
func RegisterSelectString() gojq.CompilerOption {
	common.DeclareInput("select_string", common.InputPipeline)
	return common.WithIterFunction("select_string", 1, 3, func(v any, args []any) gojq.Iter {
		in, rest := common.SplitInput(v, args, 1)
		root, ok := common.BindPath(in)
		if !ok {
			return gojq.NewIter(fmt.Errorf("select_string: expected a path, got %T", common.BindValue(in)))
		}
		pattern, err := common.BindString(rest[0], "pattern")
		if err != nil {
			return gojq.NewIter(fmt.Errorf("select_string: %v", err))
		}
		opts, err := selectStringOptions(rest[1:])
		if err != nil {
			return gojq.NewIter(fmt.Errorf("select_string: %v", err))
		}
		if !opts.caseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("select_string: %v", err))
		}
		iter, err := newMatchIter(root, re, opts)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("select_string: %v", err))
		}
		return iter
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
				return o, fmt.Errorf("expected a string for Include")
			}
			o.include = s
		case "context":
			f, ok := common.ToInt(val)
			if !ok || f < 0 {
				return o, fmt.Errorf("expected a non-negative integer for Context")
			}
			o.context = f
		case "casesensitive":
			b, ok := val.(bool)
			if !ok {
				return o, fmt.Errorf("expected a boolean for CaseSensitive")
			}
			o.caseSensitive = b
		case "list":
			b, ok := val.(bool)
			if !ok {
				return o, fmt.Errorf("expected a boolean for List")
			}
			o.listOnly = b
		default:
			return o, fmt.Errorf("unknown option %q", k)
		}
	}
	return o, nil
}

// matchIter walks the tree and scans files as the caller reads matches.
//
// The walk is an explicit stack of directories rather than filepath.WalkDir,
// because WalkDir pushes: it calls back for every file before returning, which
// is the whole tree's work done up front. A stack can stop between two files
// and resume, which is what a lazy stream needs. Entry order is the same either
// way - os.ReadDir sorts by name, and a directory is descended into where its
// name falls - so the matches arrive in the order they always did.
type matchIter struct {
	re    *regexp.Regexp
	opts  selectOpts
	stack []*dirFrame // directories still being read, deepest last
	file  string      // the root, when it is a single file rather than a tree
	ready []any       // matches from the file most recently scanned
	done  bool
}

// dirFrame is one directory part-way through being read.
type dirFrame struct {
	path    string
	entries []fs.DirEntry
	next    int
}

func newMatchIter(root string, re *regexp.Regexp, o selectOpts) (*matchIter, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	it := &matchIter{re: re, opts: o}
	if !info.IsDir() {
		it.file = root
		return it, nil
	}
	frame, err := readDirFrame(root)
	if err != nil {
		return nil, err
	}
	it.stack = []*dirFrame{frame}
	return it, nil
}

func readDirFrame(path string) (*dirFrame, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	return &dirFrame{path: path, entries: entries}, nil
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
		path, ok, err := it.nextFile()
		if err != nil {
			it.done = true
			return fmt.Errorf("select_string: %v", err), true
		}
		if !ok {
			it.done = true
			return nil, false
		}
		matches, err := searchFile(path, it.re, it.opts)
		if err != nil {
			it.done = true
			return fmt.Errorf("select_string: %v", err), true
		}
		it.ready = matches
	}
}

// nextFile advances the walk to the next file worth scanning.
func (it *matchIter) nextFile() (string, bool, error) {
	if it.file != "" {
		path := it.file
		it.file = ""
		return path, true, nil
	}
	for len(it.stack) > 0 {
		frame := it.stack[len(it.stack)-1]
		if frame.next >= len(frame.entries) {
			it.stack = it.stack[:len(it.stack)-1]
			continue
		}
		entry := frame.entries[frame.next]
		frame.next++
		path := filepath.Join(frame.path, entry.Name())
		if entry.IsDir() {
			// Skip the directories nobody means to grep.
			if name := entry.Name(); name == ".git" || name == "node_modules" || name == "vendor" {
				continue
			}
			child, err := readDirFrame(path)
			if err != nil {
				return "", false, err
			}
			it.stack = append(it.stack, child)
			continue
		}
		if it.opts.include != "" {
			ok, err := filepath.Match(it.opts.include, entry.Name())
			if err != nil || !ok {
				continue
			}
		}
		return path, true, nil
	}
	return "", false, nil
}

// pendingMatch is a match that has been recorded but is still collecting the
// context lines that follow it.
type pendingMatch struct {
	match map[string]any
	after []any
}

// searchFile returns every match in one file.
//
// The file is read a line at a time and only the context window is kept, so
// grepping a multi-gigabyte log costs the window rather than the log. What is
// held is the matches themselves, which is the answer the caller asked for.
func searchFile(path string, re *regexp.Regexp, o selectOpts) ([]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	out := []any{}
	before := make([]string, 0, o.context) // the o.context lines just read, oldest first
	var awaiting []*pendingMatch
	lineNumber := 0

	for sc.Scan() {
		line := sc.Text()
		lineNumber++

		// Matches already recorded are still owed the lines that follow them.
		kept := awaiting[:0]
		for _, p := range awaiting {
			p.after = append(p.after, line)
			p.match["After"] = p.after
			if len(p.after) < o.context {
				kept = append(kept, p)
			}
		}
		awaiting = kept

		if re.MatchString(line) {
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
				"LineNumber":           lineNumber,
				"Line":                 line,
				"Match":                re.FindString(line),
				psobject.PSPathKey:     path,
			}
			if o.context > 0 {
				m["Before"] = asValues(before)
				m["After"] = []any{}
				awaiting = append(awaiting, &pendingMatch{match: m})
			}
			out = append(out, m)
		}

		if o.context > 0 {
			if len(before) == o.context {
				copy(before, before[1:])
				before[len(before)-1] = line
			} else {
				before = append(before, line)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	return out, nil
}

// asValues copies lines into the []any the object model uses. The copy matters:
// the caller's slice is a window that keeps moving.
func asValues(lines []string) []any {
	out := make([]any, len(lines))
	for i, l := range lines {
		out[i] = l
	}
	return out
}
