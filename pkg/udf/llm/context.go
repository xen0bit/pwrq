package llm

import (
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterGetContext registers get_llm_context, the answer to "why is this
// failing".
//
// It reports which model a call would reach, where it would send the request
// and which credential source won — and never the key itself. A query's output
// ends up in logs and terminal scrollback, so a cmdlet that printed an API key
// would be a way to leak one.
func RegisterGetContext() gojq.CompilerOption {
	const op = "get_llm_context"
	return common.WithFunction(op, 0, 1, func(v any, args []any) any {
		var rawOpts map[string]any
		if len(args) == 1 {
			var err error
			if rawOpts, err = optionsArg(op, args[0]); err != nil {
				return err
			}
		}
		o := defaults()
		if err := bindOptions(op, rawOpts, &o, "agent", "batch"); err != nil {
			return err
		}

		// A missing model is the most common misconfiguration, and it is one
		// this cmdlet exists to explain rather than fail on.
		p, err := o.resolve(op)
		if err != nil {
			return map[string]any{
				"Model":        o.Model,
				"ModelSource":  o.modelSource,
				"Provider":     nil,
				"BaseUrl":      nil,
				"HasApiKey":    false,
				"ApiKeySource": "",
				"Problem":      err.Error(),

				typed.TypeKey: "Pwrq.LLM.Context",
			}
		}

		limit, limitErr := callLimit(op, o)
		context := map[string]any{
			"Model":          o.Model,
			"ModelSource":    o.modelSource,
			"Provider":       p.name,
			"BaseUrl":        o.BaseUrl,
			"BaseUrlSource":  o.baseSource,
			"HasApiKey":      o.ApiKey != "",
			"ApiKeySource":   o.keySource,
			"ApiKeyRequired": !p.keyOptional,
			"TimeoutSeconds": int(o.timeout().Seconds()),
			"MaxTokens":      o.MaxTokens,
			"Temperature":    o.Temperature,
			"MaxCalls":       limit,
			"CacheEnabled":   false,
			"Problem":        nil,

			typed.TypeKey: "Pwrq.LLM.Context",
		}
		if limitErr != nil {
			context["MaxCalls"] = nil
			context["Problem"] = limitErr.Error()
		}
		if c, err := openCache(op, o); err == nil && c != nil {
			context["CacheEnabled"] = true
			context["CacheDir"] = c.dir
		}
		if err := o.requireKey(op, p); err != nil {
			context["Problem"] = err.Error()
		}
		return context
	})
}

// RegisterGetUsage registers get_llm_usage, which reports what this process has
// spent so far.
//
// Usage is process-wide rather than per-call because that is the number a
// caller actually wants: `[.[] | invoke_llm(...)] | get_llm_usage` answers
// "what did that pipeline cost", which no individual response object can.
func RegisterGetUsage() gojq.CompilerOption {
	return common.WithFunction("get_llm_usage", 0, 0, func(any, []any) any {
		return usageObject()
	})
}
