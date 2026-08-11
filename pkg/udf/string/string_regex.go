package string

import (
	"fmt"
	"regexp"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// strFromPipeline resolves a string from the pipeline value.
func strFromPipeline(v any, name string) (string, error) {
	input, err := stringInput(common.BindValue(v))
	if err != nil {
		return "", fmt.Errorf("%s: %v", name, err)
	}
	return input, nil
}

// compilePattern compiles the first argument as a regular expression.
func compilePattern(args []any, name string) (*regexp.Regexp, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("%s: a pattern argument is required", name)
	}
	pattern, ok := common.BindValue(args[0]).(string)
	if !ok {
		return nil, fmt.Errorf("%s: pattern must be a string, got %T", name, args[0])
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", name, err)
	}
	return re, nil
}

// RegisterRegexFindAll registers regex_find_all, every non-overlapping match of
// a pattern in a string, as an array. An optional second argument caps the
// count.
func RegisterRegexFindAll() gojq.CompilerOption {
	return gojq.WithFunction("regex_find_all", 1, 2, func(v any, args []any) any {
		input, err := strFromPipeline(v, "regex_find_all")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		re, err := compilePattern(args, "regex_find_all")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		limit := -1
		if len(args) > 1 {
			if n, ok := common.ToInt(args[1]); ok && n >= 0 {
				limit = n
			}
		}
		matches := re.FindAllString(input, limit)
		out := make([]any, len(matches))
		for i, m := range matches {
			out[i] = m
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterRegexExtractFirst registers regex_extract_first, the first match of
// a pattern, or the named/ numbered capture group given as the optional second
// argument (0 for the whole match).
func RegisterRegexExtractFirst() gojq.CompilerOption {
	return gojq.WithFunction("regex_extract_first", 1, 2, func(v any, args []any) any {
		input, err := strFromPipeline(v, "regex_extract_first")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		re, err := compilePattern(args, "regex_extract_first")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		group := 0
		if len(args) > 1 {
			if n, ok := common.ToInt(args[1]); ok {
				group = n
			}
		}
		match := re.FindStringSubmatch(input)
		if match == nil || group >= len(match) {
			return common.MakeUDFSuccessResult(nil, nil)
		}
		return common.MakeUDFSuccessResult(match[group], nil)
	})
}

// RegisterRegexReplaceFirst registers regex_replace_first, replacing the first
// match of a pattern with a replacement string. Backreferences like $1 work.
func RegisterRegexReplaceFirst() gojq.CompilerOption {
	return gojq.WithFunction("regex_replace_first", 2, 2, func(v any, args []any) any {
		input, err := strFromPipeline(v, "regex_replace_first")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		re, err := compilePattern(args, "regex_replace_first")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		repl, ok := common.BindValue(args[1]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("regex_replace_first: replacement must be a string, got %T", args[1]), nil)
		}
		loc := re.FindStringSubmatchIndex(input)
		if loc == nil {
			return common.MakeUDFSuccessResult(input, nil)
		}
		var out []byte
		out = append(out, input[:loc[0]]...)
		out = re.ExpandString(out, repl, input, loc)
		out = append(out, input[loc[1]:]...)
		return common.MakeUDFSuccessResult(string(out), nil)
	})
}

// RegisterRegexSplit registers regex_split, splitting a string on every match
// of a pattern. jq's split takes a literal string; this one takes a regex.
func RegisterRegexSplit() gojq.CompilerOption {
	return gojq.WithFunction("regex_split", 1, 1, func(v any, args []any) any {
		input, err := strFromPipeline(v, "regex_split")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		re, err := compilePattern(args, "regex_split")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		parts := re.Split(input, -1)
		out := make([]any, len(parts))
		for i, p := range parts {
			out[i] = p
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterRegexCount registers regex_count, how many non-overlapping matches a
// pattern has in a string.
func RegisterRegexCount() gojq.CompilerOption {
	return gojq.WithFunction("regex_count", 1, 1, func(v any, args []any) any {
		input, err := strFromPipeline(v, "regex_count")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		re, err := compilePattern(args, "regex_count")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(len(re.FindAllString(input, -1)), nil)
	})
}
