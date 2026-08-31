// Package pwrgrep exposes pwrq's structural rules as cmdlets.
package pwrgrep

import (
	"context"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/pwrgrep"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterInvokePwrgrep registers invoke_pwrgrep: run structural rules over a
// tree and report what they find.
//
// A rule is what select_ast is not. select_ast answers one question - where
// does this piece of syntax occur - and that is rarely a finding on its own:
// "MD5, but only in a file that imports crypto/md5", "assigning to innerHTML,
// but not a string literal". A rule carries the combining, the message and the
// files it is about, so running one is naming it.
//
//	"src" | invoke_pwrgrep("go-weak-hash")
//	[invoke_pwrgrep("src"; ["go-*", "python-*"])] | group_by(.RuleId)
//	[invoke_pwrgrep("src"; "go/lang/security")] | map(.Path) | unique
//
// The rules named may be finding ids, globs over them, or paths into the
// catalogue; get_pwrgrep_rule lists what there is. Naming nothing that exists
// is an error rather than an empty result, because "no rule called that" and
// "your code is clean" must not look alike.
//
// A rule is an ordinary pwrq query in a file - scan_ast, of, within, not_at,
// finding and report are cmdlets like any other - and the catalogue is a
// directory, so writing one is copying one: put it in $PWRQ_RULES or in
// ~/.config/pwrq/rules and it is found beside the rules that shipped. A file
// there with the same path as a shipped rule replaces it.
//
// It streams, and a finding is an ordinary value: filtering vendored code is
// the next stage of the pipeline rather than an option here.
//
//	[invoke_pwrgrep("."; "javascript-*")] | map(select(.Path | test("min\\.js") | not))
func RegisterInvokePwrgrep() gojq.CompilerOption {
	common.DeclareInput("invoke_pwrgrep", common.InputPipeline)
	return common.WithIterFunctionOf("invoke_pwrgrep", 1, 2, Finding, func(v any, args []any) gojq.Iter {
		in, rest := common.SplitInput(v, args, 1)
		root, ok := common.BindPath(in)
		if !ok {
			return gojq.NewIter(fmt.Errorf("invoke_pwrgrep: expected a path, got %T", common.BindValue(in)))
		}
		selectors, err := bindSelectors(rest[0])
		if err != nil {
			return gojq.NewIter(fmt.Errorf("invoke_pwrgrep: %v", err))
		}
		rules, err := pwrgrep.Select(selectors)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("invoke_pwrgrep: %v", err))
		}
		var out []any
		for _, rule := range rules {
			found, err := rule.Run(context.Background(), root)
			if err != nil {
				return gojq.NewIter(fmt.Errorf("invoke_pwrgrep: %v", err))
			}
			out = append(out, found...)
		}
		return gojq.NewIter(out...)
	})
}

// bindSelectors reads the rules argument, which is one name or several.
func bindSelectors(arg any) ([]string, error) {
	list, ok := common.BindValue(arg).([]any)
	if !ok {
		list = []any{common.BindValue(arg)}
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no rules named, so there is nothing to run")
	}
	selectors := make([]string, len(list))
	for i, item := range list {
		name, err := common.BindString(item, "rule")
		if err != nil {
			return nil, fmt.Errorf("rule %d: %v", i+1, err)
		}
		selectors[i] = name
	}
	return selectors, nil
}

// RegisterGetPwrgrepRule registers get_pwrgrep_rule: the rules pwrq can see.
//
// Which rules those are is a property of this machine as much as of this
// binary - the catalogue is a search path, and a directory of your own comes
// first - so the only honest answer comes from walking it rather than from a
// list written down beside it.
//
//	get_pwrgrep_rule("go-weak-hash") | .Query
//	[get_pwrgrep_rule] | length
//	[get_pwrgrep_rule("python-*")] | map(.Id)
func RegisterGetPwrgrepRule() gojq.CompilerOption {
	common.DeclareInput("get_pwrgrep_rule", common.InputPipeline)
	return common.WithIterFunctionOf("get_pwrgrep_rule", 0, 1, Rule, func(v any, args []any) gojq.Iter {
		in, _ := common.SplitInput(v, args, 0)
		var selectors []string
		if bound := common.BindValue(in); bound != nil {
			s, ok := bound.(string)
			if !ok {
				return gojq.NewIter(fmt.Errorf("get_pwrgrep_rule: expected a rule to name, got %T", bound))
			}
			selectors = []string{s}
		}
		rules, err := pwrgrep.Select(selectors)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("get_pwrgrep_rule: %v", err))
		}
		out := make([]any, len(rules))
		for i, rule := range rules {
			ids := make([]any, len(rule.Ids))
			for j, id := range rule.Ids {
				ids[j] = id
			}
			out[i] = Rule.Build(map[string]any{
				"Id":          rule.Id(),
				"Ids":         ids,
				"Path":        rule.Path,
				"From":        rule.From,
				"Fixture":     rule.Fixture,
				"Description": rule.Description,
				"Query":       strings.TrimSpace(rule.Query),
				"Origin":      rule.Origin,
				"PwrqValue":   rule.Id(),
			})
		}
		return gojq.NewIter(out...)
	})
}

// RegisterAll registers the rule cmdlets and the vocabulary a rule is written
// with.
func RegisterAll() []gojq.CompilerOption {
	return append([]gojq.CompilerOption{
		RegisterInvokePwrgrep(),
		RegisterGetPwrgrepRule(),
	}, RegisterVocabulary()...)
}
