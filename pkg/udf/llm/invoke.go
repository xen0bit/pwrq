package llm

import (
	"context"
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// call is one cmdlet's arguments, after the input and the options have been
// told apart.
type call struct {
	input   any
	options options
	// data is the pipeline value when the input came from an argument, which
	// is what lets `$rows | invoke_agent("...")` give an agent something to
	// work on while the argument carries the task.
	data any
}

// parseCall splits a cmdlet's input from its trailing options object.
//
// The arity rule in pkg/udf/README.md says the operands are never inspected to
// work out which one was meant to be the input. This is the one place that
// bends it, and only because the two roles have disjoint types: a prompt is
// text and options are an object, so `invoke_llm("summarize this")` and
// `invoke_llm({Model: "..."})` cannot be confused with one another. Guessing
// between two operands of the *same* type is what that rule forbids, and this
// is not that.
func parseCall(op string, v any, args []any, groups ...string) (call, error) {
	var (
		input   any
		data    any
		rawOpts map[string]any
	)
	switch len(args) {
	case 0:
		input = v
	case 1:
		if opts, ok := common.BindValue(args[0]).(map[string]any); ok {
			input, rawOpts = v, opts
		} else {
			input, data = args[0], v
		}
	default:
		var err error
		input, data = args[0], v
		if rawOpts, err = optionsArg(op, args[1]); err != nil {
			return call{}, err
		}
	}

	o := defaults()
	if err := bindOptions(op, rawOpts, &o, groups...); err != nil {
		return call{}, err
	}
	return call{input: input, data: data, options: o}, nil
}

// prompt binds the input as the text to send.
func (c call) prompt(op string) (string, error) {
	s, err := common.BindString(common.BindValue(c.input), "Prompt")
	if err != nil {
		return "", fmt.Errorf("%s: %w; the prompt comes from the pipeline or the first argument", op, err)
	}
	return s, nil
}

// RegisterInvokeLLM registers invoke_llm, which returns what the model said.
//
// It returns the completion itself rather than an object wrapping it, because
// that is what makes it composable: `map(invoke_llm("summarize: \(.Body)"))`
// yields strings a later filter can act on. invoke_llm_request is for when the
// tokens, the model or the stop reason matter.
func RegisterInvokeLLM() gojq.CompilerOption {
	const op = "invoke_llm"
	return common.WithFunction(op, 0, 2, func(v any, args []any) any {
		resp, _, err := runInvoke(op, v, args)
		if err != nil {
			return err
		}
		// Under a schema the answer is the decoded value; the raw text was
		// only ever the transport.
		if resp.Structured != nil {
			return resp.Structured
		}
		return resp.Content
	})
}

// RegisterInvokeLLMRequest registers invoke_llm_request, the same call reported
// as an object.
func RegisterInvokeLLMRequest() gojq.CompilerOption {
	const op = "invoke_llm_request"
	return common.WithFunction(op, 0, 2, func(v any, args []any) any {
		resp, o, err := runInvoke(op, v, args)
		if err != nil {
			return err
		}
		return responseObject(resp, &o)
	})
}

func runInvoke(op string, v any, args []any) (*response, options, error) {
	c, err := parseCall(op, v, args)
	if err != nil {
		return nil, options{}, err
	}
	prompt, err := c.prompt(op)
	if err != nil {
		return nil, options{}, err
	}
	p, err := c.options.resolve(op)
	if err != nil {
		return nil, options{}, err
	}
	resp, err := complete(context.Background(), op, prompt, c.options, p)
	return resp, c.options, err
}

// responseObject is the shape every LLM cmdlet reports a call as.
//
// Content is always the text the model produced and Value the decoded document
// when a Schema was asked for, so a caller reading .Content never has to
// wonder whether it is a string this time.
func responseObject(resp *response, o *options) map[string]any {
	out := map[string]any{
		"Content":      resp.Content,
		"Reasoning":    nil,
		"Value":        resp.Structured,
		"Model":        resp.Model,
		"Provider":     resp.Provider,
		"StopReason":   resp.StopReason,
		"InputTokens":  resp.InputTokens,
		"OutputTokens": resp.OutputTokens,
		"TotalTokens":  resp.InputTokens + resp.OutputTokens,
		"Cost":         nil,
		"Cached":       resp.Cached,

		psobject.PSTypeNameKey: "Pwrq.LLM.Response",
	}
	if resp.Reasoning != "" {
		out["Reasoning"] = resp.Reasoning
	}
	if o != nil && (o.PriceInput > 0 || o.PriceOutput > 0) {
		out["Cost"] = float64(resp.InputTokens)*o.PriceInput/1e6 + float64(resp.OutputTokens)*o.PriceOutput/1e6
	}
	return out
}
