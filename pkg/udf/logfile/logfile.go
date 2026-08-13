// Package logfile provides line-oriented file readers for log analysis:
// head, tail, grep and line counting. They touch the filesystem, so they are
// registered in the CLI only and flagged unavailable in the browser.
package logfile

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every logfile cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterHead(),
		RegisterTail(),
		RegisterGrepLines(),
		RegisterWcLines(),
		RegisterSelectString(),
	}
}

// pathArg resolves the path from the first argument or the pipeline.
func pathArg(v any, args []any, name string) (string, error) {
	if len(args) > 0 {
		if s, ok := common.BindValue(args[0]).(string); ok {
			return s, nil
		}
	}
	switch val := common.BindValue(v).(type) {
	case string:
		return val, nil
	case *os.File:
		return val.Name(), nil
	default:
		return "", fmt.Errorf("%s: expected a path string, got %T", name, v)
	}
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

func toAny(lines []string) []any {
	out := make([]any, len(lines))
	for i, l := range lines {
		out[i] = l
	}
	return out
}

// RegisterHead registers head, the first n lines of a file (10 by default).
func RegisterHead() gojq.CompilerOption {
	return gojq.WithFunction("head", 0, 2, func(v any, args []any) any {
		path, err := pathArg(v, args, "head")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("head: %v", err), nil)
		}
		n := 10
		if len(args) > 0 {
			if m, ok := common.ToInt(args[len(args)-1]); ok {
				n = m
			}
		}
		if n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("head: n must not be negative, got %d", n), nil)
		}
		lines, err := readLines(path)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("head: %v", err), nil)
		}
		if n < len(lines) {
			lines = lines[:n]
		}
		return common.MakeUDFSuccessResult(toAny(lines), nil)
	})
}

// RegisterTail registers tail, the last n lines of a file (10 by default).
func RegisterTail() gojq.CompilerOption {
	return gojq.WithFunction("tail", 0, 2, func(v any, args []any) any {
		path, err := pathArg(v, args, "tail")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("tail: %v", err), nil)
		}
		n := 10
		if len(args) > 0 {
			if m, ok := common.ToInt(args[len(args)-1]); ok {
				n = m
			}
		}
		if n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("tail: n must not be negative, got %d", n), nil)
		}
		lines, err := readLines(path)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("tail: %v", err), nil)
		}
		if n < len(lines) {
			lines = lines[len(lines)-n:]
		}
		return common.MakeUDFSuccessResult(toAny(lines), nil)
	})
}

// RegisterGrepLines registers grep_lines, the lines of a file matching a
// regular expression. Usage: grep_lines(path; pattern) or "path" | grep_lines(pattern).
func RegisterGrepLines() gojq.CompilerOption {
	return gojq.WithFunction("grep_lines", 1, 2, func(v any, args []any) any {
		var path, pattern string
		switch len(args) {
		case 2:
			p, pOK := common.BindValue(args[0]).(string)
			r, rOK := common.BindValue(args[1]).(string)
			if !pOK || !rOK {
				return common.MakeUDFErrorResult(fmt.Errorf("grep_lines: expected (path; pattern), got %v and %v", args[0], args[1]), nil)
			}
			path, pattern = p, r
		case 1:
			r, rOK := common.BindValue(args[0]).(string)
			if !rOK {
				return common.MakeUDFErrorResult(fmt.Errorf("grep_lines: pattern must be a string, got %T", args[0]), nil)
			}
			pattern = r
			p, err := pathArg(v, nil, "grep_lines")
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("grep_lines: %v", err), nil)
			}
			path = p
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("grep_lines: %v", err), nil)
		}
		lines, err := readLines(path)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("grep_lines: %v", err), nil)
		}
		matches := make([]string, 0, len(lines))
		for _, line := range lines {
			if re.MatchString(line) {
				matches = append(matches, line)
			}
		}
		return common.MakeUDFSuccessResult(toAny(matches), nil)
	})
}

// RegisterWcLines registers wc_lines, the number of lines in a file.
func RegisterWcLines() gojq.CompilerOption {
	return gojq.WithFunction("wc_lines", 0, 1, func(v any, args []any) any {
		path, err := pathArg(v, args, "wc_lines")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("wc_lines: %v", err), nil)
		}
		lines, err := readLines(path)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("wc_lines: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(len(lines), nil)
	})
}
