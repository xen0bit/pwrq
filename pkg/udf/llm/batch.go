package llm

import (
	"context"
	"fmt"
	"sync"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// defaultParallel is how many calls are in flight at once when the caller does
// not say. It is small on purpose: providers rate-limit, and a query that
// opened fifty connections by default would fail for a reason the caller never
// chose.
const defaultParallel = 4

// RegisterInvokeLLMBatch registers invoke_llm_batch.
//
// This cmdlet exists because gojq evaluates synchronously. `map(invoke_llm(...))`
// over five hundred rows is five hundred sequential round trips — at a second
// each that is eight minutes of waiting on a machine doing nothing. Batching
// runs a bounded pool instead.
//
// Results are emitted in *input* order, not completion order. A pipeline
// correlates a result with its row by position, so returning them as they
// finish would be faster to first value and useless for everything after it.
func RegisterInvokeLLMBatch() gojq.CompilerOption {
	const op = "invoke_llm_batch"
	return common.WithIterFunction(op, 0, 2, func(v any, args []any) gojq.Iter {
		c, err := parseCall(op, v, args, "batch")
		if err != nil {
			return gojq.NewIter(err)
		}
		prompts, err := common.BindArray(c.input, op)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("%w; invoke_llm_batch takes an array of prompts", err))
		}
		p, err := c.options.resolve(op)
		if err != nil {
			return gojq.NewIter(err)
		}

		texts := make([]string, len(prompts))
		for i, raw := range prompts {
			s, err := common.BindString(common.BindValue(raw), "Prompt")
			if err != nil {
				return gojq.NewIter(fmt.Errorf("%s: prompt %d: %w", op, i, err))
			}
			texts[i] = s
		}

		results := runBatch(op, texts, c.options, p)

		// A failure fails the whole call, as every other cmdlet's does, unless
		// the caller asked to see the failures in band. Half a batch silently
		// becoming null is the outcome worth refusing.
		values := make([]any, 0, len(results))
		for i, r := range results {
			if r.err != nil && !c.options.ContinueOnError {
				return gojq.NewIter(r.err)
			}
			obj := map[string]any{
				"Index": i,
				"Error": nil,
			}
			if r.err != nil {
				obj["Content"] = nil
				obj["Error"] = r.err.Error()
				obj[psobject.PSTypeNameKey] = "Pwrq.LLM.Response"
			} else {
				for k, val := range responseObject(r.resp, &c.options) {
					obj[k] = val
				}
				obj["Index"] = i
				obj["Error"] = nil
			}
			values = append(values, obj)
		}
		return common.SliceIter(values)
	})
}

type batchResult struct {
	resp *response
	err  error
}

// runBatch fans the prompts out over a bounded pool and collects them by index.
//
// A failure cancels the rest when the caller has not asked to continue past
// one. Running to completion first and reporting the error afterwards would be
// simpler and would bill the caller for four hundred and ninety-nine prompts
// whose answers are about to be thrown away.
func runBatch(op string, prompts []string, o options, p provider) []batchResult {
	parallel := o.Parallel
	if parallel <= 0 {
		parallel = defaultParallel
	}
	if parallel > len(prompts) {
		parallel = len(prompts)
	}

	results := make([]batchResult, len(prompts))
	if len(prompts) == 0 {
		return results
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	work := make(chan int)
	var wg sync.WaitGroup
	for range parallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				// A cancelled run still fills in the slot, so the caller sees
				// which prompts were abandoned rather than a silent nil.
				if ctx.Err() != nil {
					results[i] = batchResult{err: fmt.Errorf("%s: prompt %d was not sent: an earlier prompt failed", op, i)}
					continue
				}
				resp, err := complete(ctx, op, prompts[i], o, p)
				results[i] = batchResult{resp: resp, err: err}
				if err != nil && !o.ContinueOnError {
					cancel()
				}
			}
		}()
	}
	for i := range prompts {
		work <- i
	}
	close(work)
	wg.Wait()
	return results
}
