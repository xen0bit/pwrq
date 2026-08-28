package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

const (
	defaultMaxSteps     = 8
	defaultMaxSeconds   = 300
	defaultQueryResults = 50
	defaultQueryBytes   = 20000
	agentQuerySeconds   = 30
)

// RegisterInvokeAgent registers invoke_agent, which returns the answer a model
// reached by writing pwrq queries.
//
// The tool surface is pwrq itself. That is not a shortcut: `pwrq --mcp` already
// hands the vocabulary to an agent and the loop works, so inverting it needs a
// prompt and a query runner rather than a tool protocol. What it does need is
// the restriction, and that is structural — the agent's queries compile against
// a registry built from the allowed cmdlets alone, so a denied cmdlet is a
// parse error rather than a rule a model can talk its way around.
func RegisterInvokeAgent() gojq.CompilerOption {
	const op = "invoke_agent"
	return common.WithFunction(op, 0, 2, func(v any, args []any) any {
		run, err := runAgent(op, v, args)
		if err != nil {
			return err
		}
		return run.answer
	})
}

// RegisterInvokeAgentRequest registers invoke_agent_request, the same run with
// its trace attached.
//
// The trace is the point. An agent that answered from three queries has made
// three claims about the filesystem, and .Steps is where a reader checks them.
func RegisterInvokeAgentRequest() gojq.CompilerOption {
	const op = "invoke_agent_request"
	return common.WithFunction(op, 0, 2, func(v any, args []any) any {
		run, err := runAgent(op, v, args)
		if err != nil {
			return err
		}
		return run.object()
	})
}

// agentRun is a finished run.
type agentRun struct {
	answer       any
	content      string
	steps        []any
	model        string
	provider     string
	inputTokens  int
	outputTokens int
	priced       bool
	cost         float64
}

func (r agentRun) object() map[string]any {
	out := map[string]any{
		"Content":      r.content,
		"Value":        r.answer,
		"Steps":        r.steps,
		"StepCount":    len(r.steps),
		"Model":        r.model,
		"Provider":     r.provider,
		"InputTokens":  r.inputTokens,
		"OutputTokens": r.outputTokens,
		"TotalTokens":  r.inputTokens + r.outputTokens,
		"Cost":         nil,

		typed.TypeKey: "Pwrq.Agent.Response",
	}
	if r.priced {
		out["Cost"] = r.cost
	}
	return out
}

func runAgent(op string, v any, args []any) (*agentRun, error) {
	c, err := parseCall(op, v, args, "agent")
	if err != nil {
		return nil, err
	}
	task, err := c.prompt(op)
	if err != nil {
		return nil, err
	}
	o := c.options
	p, err := o.resolve(op)
	if err != nil {
		return nil, err
	}

	allow, err := resolveAllow(op, o.Allow)
	if err != nil {
		return nil, err
	}
	build := getVocabulary()
	if build == nil {
		return nil, fmt.Errorf("%s: no cmdlet vocabulary is installed in this host", op)
	}
	vocab, err := build(allow)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// The data the agent works on is whatever was in the pipeline when the
	// task came from an argument. `$rows | invoke_agent("find the outliers")`
	// puts $rows under `.` in every query the agent writes; `"task" | invoke_agent`
	// gives it no data, only cmdlets.
	var inputJSON string
	if c.data != nil {
		encoded, err := gojq.Marshal(c.data)
		if err != nil {
			return nil, fmt.Errorf("%s: pipeline input is not encodable as JSON: %w", op, err)
		}
		inputJSON = string(encoded)
	}

	maxSteps := o.MaxSteps
	if maxSteps <= 0 {
		maxSteps = defaultMaxSteps
	}
	maxSeconds := o.MaxSeconds
	if maxSeconds <= 0 {
		maxSeconds = defaultMaxSeconds
	}
	maxResults := o.MaxResults
	if maxResults <= 0 {
		maxResults = defaultQueryResults
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(maxSeconds)*time.Second)
	defer cancel()

	// The step protocol rides on the same structured-output path as
	// {Schema: ...}: one JSON object per turn, validated before it is acted
	// on. Provider-native tool calling would need a second encoding per
	// dialect and would not be any more reliable than a schema both dialects
	// already enforce.
	//
	// Every field is required, and the choice is an enum rather than "which
	// key did it fill in". Constrained decoding emits the cheapest document
	// that satisfies the schema, so an optional query field is one a small
	// model simply omits: the first version of this asked for {thought} with
	// query and answer optional, and gemma-4-e2b spent every step narrating
	// the query it was about to write without ever writing one.
	step := o
	step.Schema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"thought": map[string]any{"type": "string", "description": "One sentence on what you are doing and why."},
			"action": map[string]any{
				"type": "string", "enum": []any{"query", "answer"},
				"description": "query to run a pwrq query, answer when you know the answer.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The pwrq query when action is query, or the final answer when action is answer.",
			},
		},
		"required": []any{"thought", "action", "content"},
	}
	step.System = agentSystemPrompt(vocab.Commands, inputJSON, o.System)

	run := &agentRun{model: o.Model, provider: p.name, steps: []any{}}
	messages := []message{{Role: "user", Content: task}}

	for n := 1; n <= maxSteps; n++ {
		resp, err := completeMessages(ctx, op, messages, step, p)
		if err != nil {
			return nil, err
		}
		run.inputTokens += resp.InputTokens
		run.outputTokens += resp.OutputTokens
		if o.PriceInput > 0 || o.PriceOutput > 0 {
			run.priced = true
			run.cost += float64(resp.InputTokens)*o.PriceInput/1e6 + float64(resp.OutputTokens)*o.PriceOutput/1e6
		}

		decoded, _ := resp.Structured.(map[string]any)
		if decoded == nil {
			return nil, fmt.Errorf("%s: step %d did not return an object", op, n)
		}
		thought, _ := decoded["thought"].(string)
		action, _ := decoded["action"].(string)
		// One content field rather than a query field beside an answer field.
		// The two-field version had gemma-4-e2b filling both — half a query in
		// one and half in the other — because nothing in a schema says which
		// field the action refers to. With one field there is nothing to
		// choose between.
		content := strings.TrimSpace(stripQueryFence(decoded["content"]))

		if content == "" {
			messages = append(messages,
				message{Role: "assistant", Content: resp.Content},
				message{Role: "user", Content: `"content" was empty. Reply again with action "query" and the query in content, or action "answer" and the answer in content.`})
			run.steps = append(run.steps, stepObject(n, thought, "", "", false, "empty content"))
			continue
		}
		if action == "answer" {
			run.steps = append(run.steps, stepObject(n, thought, "", "", false, ""))
			return finishAgent(ctx, op, run, task, content, o, p)
		}
		query := content

		output, truncated, queryErr := runAgentQuery(ctx, vocab, query, inputJSON, maxResults)
		run.steps = append(run.steps, stepObject(n, thought, query, output, truncated, queryErr))
		messages = append(messages,
			message{Role: "assistant", Content: resp.Content},
			message{Role: "user", Content: queryResultMessage(output, truncated, queryErr)})
	}

	return nil, fmt.Errorf("%s: no answer after %d steps; raise it with {MaxSteps: n}. Last query: %s",
		op, maxSteps, lastQuery(run.steps))
}

// finishAgent turns the agent's text answer into the value the caller asked
// for.
//
// When a Schema was supplied it is applied here, in one more call, rather than
// being folded into the step schema. A schema-shaped answer field would have to
// be satisfiable on every step — including the steps that are running a query
// and have no answer yet — so a caller's "required" would either be violated or
// would force an answer before the agent had one.
func finishAgent(ctx context.Context, op string, run *agentRun, task, answer string, o options, p provider) (*agentRun, error) {
	run.content = answer
	run.answer = answer
	if o.Schema == nil {
		return run, nil
	}

	structured := o
	structured.System = "Convert the answer into JSON matching the schema. Do not recompute it; use only what the answer says."
	resp, err := complete(ctx, op, fmt.Sprintf("Task: %s\n\nAnswer: %s", task, answer), structured, p)
	if err != nil {
		return nil, err
	}
	run.inputTokens += resp.InputTokens
	run.outputTokens += resp.OutputTokens
	if o.PriceInput > 0 || o.PriceOutput > 0 {
		run.priced = true
		run.cost += float64(resp.InputTokens)*o.PriceInput/1e6 + float64(resp.OutputTokens)*o.PriceOutput/1e6
	}
	run.answer = resp.Structured
	run.content = answerText(resp.Structured)
	return run, nil
}

// stripQueryFence recovers the query from whatever the model wrapped it in.
//
// Small models emit markdown around a field the schema already says is a
// query: a leading ```jq, a trailing ```, sometimes the start of a second
// object after it, and zero-width characters throughout. None of that is ever
// valid jq, so cutting at the fence is unambiguous — and the alternative is
// spending a step on a parse error the model cannot see the cause of.
func stripQueryFence(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	s = strings.Map(func(r rune) rune {
		// Zero-width space, zero-width non-joiner/joiner and the BOM. They are
		// invisible in the trace and a syntax error to the parser.
		switch r {
		case '\u200b', '\u200c', '\u200d', '\ufeff':
			return -1
		}
		return r
	}, s)

	if strings.HasPrefix(strings.TrimSpace(s), "```") {
		return strings.TrimSpace(stripFence(s))
	}
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[:i]
	}
	return trimUnmatchedClosers(strings.TrimSpace(s))
}

// trimUnmatchedClosers drops trailing brackets that close nothing.
//
// `[get_childitem(".")] | length}` is what a model produces when it starts
// closing the JSON object it is writing the query into, and the stray brace is
// a parse error the model cannot see: the field it wrote looked right. A
// closer with nothing open is not part of any valid program, so removing one
// at the very end cannot change what a valid query means.
func trimUnmatchedClosers(s string) string {
	pairs := map[rune]rune{')': '(', ']': '[', '}': '{'}
	var stack []rune
	inString, escaped := false, false
	// depths[i] is how many brackets were open just before byte i.
	unmatched := make(map[int]bool)
	for i, r := range s {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case inString:
			// Brackets inside a string literal are data.
		case r == '(' || r == '[' || r == '{':
			stack = append(stack, r)
		case r == ')' || r == ']' || r == '}':
			if len(stack) > 0 && stack[len(stack)-1] == pairs[r] {
				stack = stack[:len(stack)-1]
			} else {
				unmatched[i] = true
			}
		}
	}
	if len(unmatched) == 0 {
		return s
	}
	// Only trailing ones: an unmatched closer in the middle is a mistake this
	// cannot repair without guessing what was meant.
	trimmed := s
	for {
		end := strings.TrimRight(trimmed, " \t\n\r")
		if end == "" {
			return strings.TrimSpace(trimmed)
		}
		last := len(end) - 1
		if !unmatched[last] {
			return strings.TrimSpace(trimmed)
		}
		trimmed = end[:last]
	}
}

// runAgentQuery evaluates one of the agent's queries under the restricted
// vocabulary and every bound the host can impose.
func runAgentQuery(ctx context.Context, vocab *Vocabulary, query, input string, maxResults int) (output string, truncated bool, failure string) {
	req := &queryrun.Request{
		Query:          query,
		Input:          input,
		NullInput:      input == "",
		Compact:        true,
		MaxResults:     maxResults,
		MaxOutputBytes: defaultQueryBytes,
	}
	queryCtx, cancel := context.WithTimeout(ctx, agentQuerySeconds*time.Second)
	defer cancel()

	var res queryrun.Result
	// Script blocks compile against whatever the host installed at startup, so
	// without this an agent allowed to call an object cmdlet could reach the
	// whole vocabulary from inside {script: "..."}.
	common.WithScriptBlockOptions(vocab.compilerOptions(), func() {
		res = vocab.Runner.Run(queryCtx, req)
	})

	if res.Error != "" && res.Count == 0 {
		return "", false, res.Error
	}
	return strings.Join(res.Values, "\n"), res.Truncated, res.Error
}

// queryResultMessage is what the model is told about its query.
func queryResultMessage(output string, truncated bool, failure string) string {
	if failure != "" && output == "" {
		return fmt.Sprintf("That query failed: %s\n\nRemember a query is a jq program, not a shell command: no -name, -type or shell tools. "+
			"For example, counting the .md files here is `[get_childitem(\".\") | select(.Name | endswith(\".md\"))] | length`. "+
			"Fix the query, or try a different one.", failure)
	}
	var b strings.Builder
	b.WriteString("Result:\n")
	if output == "" {
		b.WriteString("(no output)")
	} else {
		b.WriteString(output)
	}
	if truncated {
		b.WriteString("\n(truncated; narrow the query if you need the rest)")
	}
	if failure != "" {
		b.WriteString("\n(the query also reported: " + failure + ")")
	}
	return b.String()
}

func stepObject(n int, thought, query, output string, truncated bool, failure string) map[string]any {
	step := map[string]any{
		"Step":      n,
		"Thought":   thought,
		"Query":     nil,
		"Output":    nil,
		"Truncated": truncated,
		"Error":     nil,

		typed.TypeKey: "Pwrq.Agent.Step",
	}
	if query != "" {
		step["Query"] = query
		step["Output"] = output
	}
	if failure != "" {
		step["Error"] = failure
	}
	return step
}

func lastQuery(steps []any) string {
	for i := len(steps) - 1; i >= 0; i-- {
		if step, ok := steps[i].(map[string]any); ok {
			if q, ok := step["Query"].(string); ok && q != "" {
				return q
			}
		}
	}
	return "(none)"
}

// answerText renders an answer for .Content, which is always a string so a
// caller reading it never has to check the type first.
func answerText(answer any) string {
	if s, ok := answer.(string); ok {
		return s
	}
	if encoded, err := json.Marshal(answer); err == nil {
		return string(encoded)
	}
	return fmt.Sprint(answer)
}

// describeInput summarises the pipeline input rather than reproducing it.
//
// The first version pasted the data itself into the system prompt, which was
// wrong twice over. It billed for the whole dataset on every step, and it
// taught the model that the data was *text it had been given* — gemma-4-e2b
// responded by inlining a six-element array into its query as a literal
// instead of writing `.`, every step until the step limit. A shape and one
// sample says what a query needs to know and nothing it can paste.
func describeInput(input string) string {
	var value any
	if err := json.Unmarshal([]byte(input), &value); err != nil {
		return truncateForPrompt(input, 400)
	}

	switch v := value.(type) {
	case []any:
		var b strings.Builder
		fmt.Fprintf(&b, "an array of %d element(s).", len(v))
		if len(v) > 0 {
			if first, err := json.Marshal(v[0]); err == nil {
				fmt.Fprintf(&b, " The first is:\n%s", truncateForPrompt(string(first), 600))
			}
			if obj, ok := v[0].(map[string]any); ok {
				fmt.Fprintf(&b, "\nEvery element has these fields: %s.", strings.Join(sortedKeys(obj), ", "))
			}
		}
		return b.String()
	case map[string]any:
		var b strings.Builder
		fmt.Fprintf(&b, "an object with these fields: %s.", strings.Join(sortedKeys(v), ", "))
		if encoded, err := json.Marshal(v); err == nil && len(encoded) <= 600 {
			fmt.Fprintf(&b, " It is:\n%s", string(encoded))
		}
		return b.String()
	default:
		return "a single value: " + truncateForPrompt(input, 400)
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func truncateForPrompt(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "... (truncated; query `.` to see the rest)"
}

// agentSystemPrompt describes the job, the vocabulary and the protocol.
//
// The examples are not decoration. A model asked to "write a query" against a
// list of cmdlet names writes shell — gemma-4-e2b answered every question with
// `find . -maxdepth 1 -name '*.md' | wc -l`, which is a reasonable guess from
// the names alone and is not a jq program. Showing what a query looks like,
// and giving each cmdlet the example the catalog already carries, is what
// moves it onto jq.
func agentSystemPrompt(commands []CommandDoc, input string, extra string) string {
	var b strings.Builder
	b.WriteString(`You answer questions by writing pwrq queries.

pwrq is the jq language plus extra cmdlets. A query is a jq program. It is NOT a shell command line: there are no shell tools, no flags like -name or -maxdepth, and no shell pipelines. The | in a query is jq's pipe, and every stage is a jq expression.

Queries look like this:

  [get_childitem(".")] | length
  [get_childitem(".") | select(.Name | endswith(".md"))] | length
  [get_childitem("."; {Recurse: true, Filter: "*.go"}) | .Name]
  get_childitem(".") | select(.Length > 10000) | {Name, Length}
  [select_string("."; "TODO")] | length
  cat("go.mod") | split("\n") | length

A cmdlet that enumerates things is a stream, so wrap the call in square brackets to collect it, as [get_childitem(".")] does above, before using length, map or sort_by. Cmdlet output is a plain JSON object: filter it with select, reshape it with {A, B}, and use any jq builtin you like — length, map, select, sort_by, group_by, add, keys, test, split, join, tostring, tonumber and the rest.
`)
	if input != "" {
		b.WriteString("\nThe pipeline input is `.` in every query you write. " +
			"Refer to it as `.` — do not paste the data into a query. Its shape is:\n")
		b.WriteString(describeInput(input) + "\n")
	} else {
		b.WriteString("\nThere is no pipeline input, so `.` is null and a query has to start from a cmdlet.\n")
	}

	b.WriteString("\nThese are the only cmdlets available. Calling anything else is an error:\n")
	for _, c := range commands {
		fmt.Fprintf(&b, "- %s", c.Name)
		if c.Description != "" {
			fmt.Fprintf(&b, ": %s", c.Description)
		}
		if c.Streaming {
			// Spelled out rather than shown as [...]: a placeholder that
			// looks like syntax is one a model will write down verbatim, and
			// gemma-4-e2b did exactly that — `[...] | select(...)`.
			b.WriteString(" (streams: wrap the call in square brackets to collect it)")
		}
		if c.Shape != "" {
			fmt.Fprintf(&b, "\n    emits %s", c.Shape)
		}
		if len(c.Examples) > 0 {
			fmt.Fprintf(&b, "\n    e.g. %s", c.Examples[0])
		}
		b.WriteString("\n")
	}

	// The protocol goes last on purpose. It is the instruction the model has
	// to follow on this turn, and a long cmdlet list between it and the reply
	// is exactly the distance a small model loses it over.
	b.WriteString(`
Reply with a JSON object every turn:

  {"thought": "one sentence", "action": "query", "content": "<the pwrq query to run>"}
  {"thought": "one sentence", "action": "answer", "content": "<the final answer>"}

Run one query at a time and read its result before deciding what is next. When a query fails, the error says why: usually a jq syntax mistake, or a cmdlet that is not on the list above. Answer as soon as you know the answer.
`)
	if extra != "" {
		b.WriteString("\n" + extra + "\n")
	}
	return b.String()
}
