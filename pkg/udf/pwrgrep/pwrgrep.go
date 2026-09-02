// Package pwrgrep exposes pwrq's structural rules as cmdlets.
package pwrgrep

import (
	"context"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/runctx"
	"github.com/xen0bit/pwrq/pkg/pwrgrep"
	"github.com/xen0bit/pwrq/pkg/udf/astsearch"
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
//	[invoke_pwrgrep("src"; ["go", "python"])] | group_by(.RuleId)
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
// Each rule walks the tree itself, so nothing outside a rule knows which files
// it will ask for and nothing can share a walk between two of them. That is
// the price of a rule being a query rather than a description, and it is the
// right way round - a rule you can read and edit is worth more than one that
// is cheap to schedule. What can be shared is the reading: a parse tree does
// not depend on which rule is about to run against it, so the files one rule
// parses are kept for the next. See astsearch/treecache.go, which is also
// where the bound on that is.
//
// It streams, rule by rule, and a finding is an ordinary value: filtering
// vendored code is the next stage of the pipeline rather than an option here.
//
//	[invoke_pwrgrep("."; "javascript-*")] | map(select(.Path | test("min\\.js") | not))
//
// Streaming is not a detail of how the results are delivered. A corpus over a
// large repository is minutes of work, and a run that is stopped - by a
// deadline, or by a caller that wanted the first few - used to have nothing to
// show for the rules it had already finished, because every finding was held
// back until the last rule was done. They arrive as they are found now, so a
// run that does not finish still reports what it saw.
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
		return &findingIter{
			ctx:   runctx.Current(),
			root:  root,
			rules: rules,
			cache: astsearch.NewTreeCache(),
		}
	})
}

// findingIter runs the rules one at a time and hands back what each reported
// before starting the next.
type findingIter struct {
	ctx   context.Context
	root  string
	rules []*pwrgrep.Rule
	// cache is the files the rules have parsed so far, shared between them and
	// released when the last one is done.
	cache *astsearch.TreeCache

	next  int
	ready []any
	done  bool
}

func (it *findingIter) Next() (any, bool) {
	for {
		if len(it.ready) > 0 {
			// A finding is text and numbers copied out of the file it was
			// found in, so it outlives the tree it came from and the cache may
			// be released with findings still queued here.
			v := it.ready[0]
			it.ready = it.ready[1:]
			return v, true
		}
		if it.done {
			return nil, false
		}
		if it.next >= len(it.rules) {
			it.finish()
			return nil, false
		}
		// Between rules as well as inside one: a rule whose glob matches
		// nothing costs no time and reaches no deadline check of its own.
		if err := it.ctx.Err(); err != nil {
			it.finish()
			return fmt.Errorf("invoke_pwrgrep: %v after %d of %d rules", err, it.next, len(it.rules)), true
		}
		rule := it.rules[it.next]
		it.next++
		found, err := it.run(rule)
		if err != nil {
			it.finish()
			return fmt.Errorf("invoke_pwrgrep: %v", err), true
		}
		it.ready = found
	}
}

// run evaluates one rule with the shared cache in force. scan_ast is what
// reaches for it, and scan_ast is inside the rule's own query, so the cache is
// installed for the length of the call rather than passed down. It is restored
// straight afterwards, so nothing outside this run ever sees it.
func (it *findingIter) run(rule *pwrgrep.Rule) ([]any, error) {
	defer astsearch.InstallTreeCache(it.cache)()
	return rule.Run(it.ctx, it.root)
}

// finish gives back the trees. It is idempotent, because it is called both
// when the rules run out and when one of them fails.
//
// A caller that abandons the iterator - `first(invoke_pwrgrep(...))` - never
// reaches this, and the trees are then collected as ordinary garbage rather
// than returned to the parser's arena pool. That costs some recycling and
// leaks nothing: gojq has no way to tell an iterator it is finished with, and
// holding the arenas for the length of a run would be the worse trade.
func (it *findingIter) finish() {
	if it.done {
		return
	}
	it.done = true
	it.cache.Release()
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
//	[get_pwrgrep_rule("python")] | map(.Id)
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
			languages := make([]any, len(rule.Languages))
			for j, language := range rule.Languages {
				languages[j] = language
			}
			out[i] = Rule.Build(map[string]any{
				"Id":          rule.Id(),
				"Ids":         ids,
				"Languages":   languages,
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

// RegisterWritePwrgrepRule registers write_pwrgrep_rule: a rule of your own,
// saved where the catalogue will find it.
//
// A rule is a file, so writing one is writing a file, and for a session with a
// shell that is all it ever needed to be. The MCP server has no shell. What it
// has is this: a query that hands over the text and gets back the rule, from
// the same process that will run it a moment later.
//
// Three things have to be right for that to work and none of them are obvious
// from outside. The directory is one - it is $PWRQ_RULES or a path under the
// user's config directory, it differs by machine, and it usually does not
// exist yet. The other two are why this refuses more often than it writes: a
// file with no `# rules:` header, or one whose query does not compile, does
// not make a rule that never fires, it makes the catalogue itself unreadable,
// and every later invoke_pwrgrep for any rule at all fails with an error about
// this file. So the header is read and the query is compiled before anything
// lands, and a rule that would break the catalogue is refused instead.
//
// What comes back is visible immediately: the corpus notices its directories
// changing, so the next invoke_pwrgrep in the same process runs what was just
// written.
//
//	write_pwrgrep_rule("mine/no-timeout"; $source) | .File
//	"mine/no-timeout" | write_pwrgrep_rule($source) | .Ids
func RegisterWritePwrgrepRule() gojq.CompilerOption {
	common.DeclareInput("write_pwrgrep_rule", common.InputPipeline)
	return common.WithFunctionOf("write_pwrgrep_rule", 1, 2, WrittenRule, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		name, err := common.BindString(in, "rule name")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("write_pwrgrep_rule: %v", err), nil)
		}
		source, err := common.BindString(rest[0], "rule")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("write_pwrgrep_rule: %v", err), nil)
		}
		rule, file, err := pwrgrep.Write(name, source)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("write_pwrgrep_rule: %v", err),
				map[string]any{"rule": name})
		}
		ids := make([]any, len(rule.Ids))
		for i, id := range rule.Ids {
			ids[i] = id
		}
		languages := make([]any, len(rule.Languages))
		for i, language := range rule.Languages {
			languages[i] = language
		}
		return WrittenRule.Build(map[string]any{
			"Id":        rule.Id(),
			"Ids":       ids,
			"Languages": languages,
			"Path":      rule.Path,
			"File":      file,
			"Origin":    rule.Origin,
			"PwrqValue": file,
		})
	})
}

// RegisterAll registers the rule cmdlets and the vocabulary a rule is written
// with.
func RegisterAll() []gojq.CompilerOption {
	return append([]gojq.CompilerOption{
		RegisterInvokePwrgrep(),
		RegisterGetPwrgrepRule(),
		RegisterWritePwrgrepRule(),
	}, RegisterVocabulary()...)
}
