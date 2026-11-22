package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/d1"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// System One asks typed questions about a state. Each question lists its
// options, each option gets a single-token label, and one forward pass reads
// the probability of every label. Nothing is generated, so there is nothing
// to parse, nothing to repair and no answer outside the options — which is
// what a classification step in a pipeline wanted from a schema-constrained
// completion in the first place.
//
// Two backends serve it. The default is local: LiquidAI's d1-2B, downloaded
// and cached on first use, running in this process on the CPU
// or a CUDA GPU (systemone_local.go). It reads text, JSON, pictures and video
// frames, and answers every d1 question type: noul, choice (and multiple
// choice), score, text, number and date reads, estimates over a range, box
// and boxes. The remote backend is TypeSafe's HTTP API, served by TypeSafe
// itself or by llama.cpp's /v1/systemone, for noul, choice and score.

// maxSystemOneOptions is the most options a question can have: the labels are
// A-Z and a-z. A model whose vocabulary has fewer single-token labels says so
// with a 422, which apiErrorMessage turns into a readable error.
const maxSystemOneOptions = 52

// RegisterInvokeSystemOne registers invoke_systemone, which returns the
// answers keyed by question id.
//
// Like invoke_llm it returns the value rather than an envelope, so
// `map(. + (invoke_systemone($q) | {team: .team.choice}))` reads the way the
// pipeline thinks about it. invoke_systemone_request is for the tokens.
func RegisterInvokeSystemOne() gojq.CompilerOption {
	const op = "invoke_systemone"
	return common.WithFunction(op, 1, 3, func(v any, args []any) any {
		state, questions, opts, err := splitSystemOneCall(op, v, args)
		if err != nil {
			return err
		}
		r, err := systemOneCall(op, state, questions, opts)
		if err != nil {
			return err
		}
		return r.resp.Answers
	})
}

// RegisterInvokeSystemOneRequest registers invoke_systemone_request, the same
// call reported as an object.
func RegisterInvokeSystemOneRequest() gojq.CompilerOption {
	const op = "invoke_systemone_request"
	return common.WithFunction(op, 1, 3, func(v any, args []any) any {
		state, questions, opts, err := splitSystemOneCall(op, v, args)
		if err != nil {
			return err
		}
		r, err := systemOneCall(op, state, questions, opts)
		if err != nil {
			return err
		}
		if r.local {
			return localObject(r.resp, r.info)
		}
		return systemOneObject(r.resp, &r.o)
	})
}

// RegisterInvokeSystemOneBatch registers invoke_systemone_batch: many
// {State, Questions, Media} requests, answered in order with one set of
// options. Locally they share one loaded model, so a batch costs one load,
// not one per request.
//
//	[{State: "...", Questions: $q}, ...] | invoke_systemone_batch
//	invoke_systemone_batch($requests)
//	invoke_systemone_batch($requests; $options)
func RegisterInvokeSystemOneBatch() gojq.CompilerOption {
	const op = "invoke_systemone_batch"
	return common.WithFunction(op, 0, 2, func(v any, args []any) any {
		input, rawOpts := v, any(nil)
		switch len(args) {
		case 1:
			input = args[0]
		case 2:
			input, rawOpts = args[0], args[1]
		}
		items, ok := common.BindValue(input).([]any)
		if !ok {
			return fmt.Errorf("%s: input must be an array of {State, Questions, Media} requests, got %s", op, jsonType(common.BindValue(input)))
		}
		opts, err := optionsArg(op, rawOpts)
		if err != nil {
			return err
		}
		out := make([]any, 0, len(items))
		for i, item := range items {
			obj, ok := common.BindValue(item).(map[string]any)
			if !ok {
				return fmt.Errorf("%s: request %d must be an object with State, Questions and optional Media, got %s", op, i, jsonType(item))
			}
			for k := range obj {
				switch strings.ToLower(k) {
				case "state", "questions", "media":
				default:
					return fmt.Errorf("%s: request %d: unknown key %q; a request has State, Questions and Media", op, i, k)
				}
			}
			own := opts
			if m := field(obj, "Media"); m != nil {
				own = make(map[string]any, len(opts)+1)
				for k, val := range opts {
					if !strings.EqualFold(k, "Media") {
						own[k] = val
					}
				}
				own["Media"] = m
			}
			r, err := systemOneCall(fmt.Sprintf("%s: request %d", op, i), field(obj, "State"), field(obj, "Questions"), own)
			if err != nil {
				return err
			}
			out = append(out, r.resp.Answers)
		}
		return out
	})
}

// RegisterGetSystemOne registers get_systemone: the local model, loaded if
// it was not, and what it runs on - the device, why it is not the GPU, where
// its files came from. Calling it first warms the model up.
func RegisterGetSystemOne() gojq.CompilerOption {
	const op = "get_systemone"
	return common.WithFunction(op, 0, 1, func(v any, args []any) any {
		var rawOpts any
		if len(args) == 1 {
			rawOpts = args[0]
		}
		opts, err := optionsArg(op, rawOpts)
		if err != nil {
			return err
		}
		o := defaults()
		for k := range opts {
			switch strings.ToLower(k) {
			case "device", "modeldir", "threads":
			default:
				return fmt.Errorf("%s: unknown option %q; expected one of Device, ModelDir, Threads", op, k)
			}
		}
		if err := bindOptions(op, opts, &o, groupSystemOne); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), localTimeout)
		defer cancel()
		info, err := systemOneInfo(ctx, o)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		return info
	})
}

// systemOneResult is a call's outcome, from either backend.
type systemOneResult struct {
	resp  *response
	o     options
	local bool
	info  d1.Info
}

// systemOneCall binds the options, picks the backend, and answers.
func systemOneCall(op string, state, rawQuestions any, opts map[string]any) (*systemOneResult, error) {
	o := defaults()
	if err := bindOptions(op, opts, &o, groupSystemOne); err != nil {
		return nil, err
	}
	backend, err := chooseBackend(op, o, opts)
	if err != nil {
		return nil, err
	}
	state = normalizeNumbers(common.BindValue(state))
	rawQuestions = normalizeNumbers(common.BindValue(rawQuestions))
	if backend == backendLocal {
		req, err := parseLocal(op, state, rawQuestions, o)
		if err != nil {
			return nil, err
		}
		defer req.close()
		ctx, cancel := context.WithTimeout(context.Background(), localTimeout)
		defer cancel()
		resp, info, err := askLocal(ctx, op, req, o)
		if err != nil {
			return nil, err
		}
		return &systemOneResult{resp: resp, o: o, local: true, info: info}, nil
	}
	questions, ok := rawQuestions.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: questions must be an object of id to question, got %s", op, jsonType(rawQuestions))
	}
	if err := validateState(op, state); err != nil {
		return nil, err
	}
	if err := validateQuestions(op, questions); err != nil {
		return nil, err
	}
	p, err := o.resolveSystemOne(op)
	if err != nil {
		return nil, err
	}
	resp, err := askSystemOne(context.Background(), op, state, questions, o, p)
	if err != nil {
		return nil, err
	}
	return &systemOneResult{resp: resp, o: o}, nil
}

// splitSystemOneCall assigns the arguments by how many there are:
//
//	invoke_systemone($questions)                    state is the pipeline
//	invoke_systemone($questions; $options)          state is the pipeline
//	invoke_systemone($state; $questions; $options)
//
// parseCall tells a prompt from options by type, which works because a prompt
// is text. A state may be an object, so here the same trick would be guessing
// between two objects — which is what the arity rule forbids.
func splitSystemOneCall(op string, v any, args []any) (state, questions any, opts map[string]any, err error) {
	var rawOpts any
	switch len(args) {
	case 1:
		state, questions = v, args[0]
	case 2:
		state, questions, rawOpts = v, args[0], args[1]
	default:
		state, questions, rawOpts = args[0], args[1], args[2]
	}
	opts, err = optionsArg(op, rawOpts)
	return state, questions, opts, err
}

// validateState accepts what the API does: text, or a document to encode.
func validateState(op string, state any) error {
	switch s := state.(type) {
	case string:
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("%s: state is empty", op)
		}
	case map[string]any, []any:
	default:
		return fmt.Errorf("%s: state must be text, an object or an array, got %s; it comes from the pipeline, or the first of three arguments", op, jsonType(state))
	}
	return nil
}

// validateQuestions checks the questions before they cost a request.
//
// It mirrors the server's own rules, and is stricter in one way: a key the
// API does not define is an error here, where the server would ignore it. A
// question with "criterion" instead of "criteria" is a question whose options
// silently fell back to the default, which is the mistake worth catching.
func validateQuestions(op string, questions map[string]any) error {
	if len(questions) == 0 {
		return fmt.Errorf("%s: questions is empty; pass an object of id to {type, instructions, criteria}", op)
	}
	for _, id := range sortedKeys(questions) {
		if err := validateQuestion(questions[id]); err != nil {
			return fmt.Errorf("%s: question %q: %w", op, id, err)
		}
	}
	return nil
}

func validateQuestion(raw any) error {
	q, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("must be an object with type, instructions and criteria, got %s", jsonType(raw))
	}
	for _, key := range sortedKeys(q) {
		switch key {
		case "type", "instructions", "criteria":
		default:
			return fmt.Errorf("unknown key %q; a question has type, instructions and criteria", key)
		}
	}
	if !isDescription(q["instructions"]) {
		return fmt.Errorf("instructions must be text, an object, an array or null, got %s", jsonType(q["instructions"]))
	}

	criteria, hasCriteria := q["criteria"]
	switch q["type"] {
	case "noul":
		if !hasCriteria || criteria == nil {
			return nil
		}
		labels, ok := criteria.(map[string]any)
		if !ok {
			return fmt.Errorf("a noul's criteria must be an object with true and false, got %s", jsonType(criteria))
		}
		for _, key := range sortedKeys(labels) {
			desc := labels[key]
			if key != "true" && key != "false" {
				return fmt.Errorf("a noul's criteria has only true and false, not %q", key)
			}
			if !isDescription(desc) {
				return fmt.Errorf("criteria %q must be text, an object, an array or null, got %s", key, jsonType(desc))
			}
		}
	case "choice":
		options, ok := criteria.(map[string]any)
		if !ok {
			return fmt.Errorf("a choice needs criteria, an object of option name to description")
		}
		if err := countOptions(len(options)); err != nil {
			return err
		}
		for _, name := range sortedKeys(options) {
			if !isDescription(options[name]) {
				return fmt.Errorf("option %q must be described by text, an object, an array or null, got %s", name, jsonType(options[name]))
			}
		}
	case "score":
		levels, ok := criteria.([]any)
		if !ok {
			return fmt.Errorf("a score needs criteria, an array of levels from lowest to highest")
		}
		if err := countOptions(len(levels)); err != nil {
			return err
		}
		for i, level := range levels {
			if level == nil || !isDescription(level) {
				return fmt.Errorf("level %d must be text, an object or an array, got %s", i, jsonType(level))
			}
		}
	case nil:
		return fmt.Errorf("type is missing; it is one of noul, choice or score")
	default:
		return fmt.Errorf("type %v is not one of noul, choice or score", q["type"])
	}
	return nil
}

func countOptions(n int) error {
	if n < 2 {
		return fmt.Errorf("criteria has %d options; a question needs at least 2", n)
	}
	if n > maxSystemOneOptions {
		return fmt.Errorf("criteria has %d options; at most %d are supported", n, maxSystemOneOptions)
	}
	return nil
}

// isDescription reports whether v can describe an option: text or a
// document, or nothing at all.
func isDescription(v any) bool {
	switch v.(type) {
	case nil, string, map[string]any, []any:
		return true
	}
	return false
}

// askSystemOne performs one call with the cache, the budget and the retries
// around it. A request is one call however many questions it carries, since
// it is one round trip and the server bills it as one.
func askSystemOne(ctx context.Context, op string, state any, questions map[string]any, o options, p provider) (*response, error) {
	if err := o.requireKey(op, p); err != nil {
		return nil, err
	}
	cache, err := openCache(op, o)
	if err != nil {
		return nil, err
	}
	key := cache.systemOneKey(o, state, questions)
	if hit, ok := cache.get(key); ok && hit.Answers != nil {
		recordCacheHit()
		hit.Cached = true
		return hit, nil
	}

	body, err := json.Marshal(map[string]any{
		"state":     state,
		"model":     o.modelID(),
		"questions": questions,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: encoding request: %w", op, err)
	}

	if err := chargeCall(op, o); err != nil {
		return nil, err
	}
	url := systemOneURL(o.BaseUrl)
	resp, err := withRetries(ctx, op, o.Retries, func() (*response, time.Duration, error) {
		payload, wait, err := postJSON(ctx, op, o, p, url, body)
		if err != nil {
			return nil, wait, err
		}
		resp, err := systemOneResponse(payload, questions)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", op, err)
		}
		return resp, 0, nil
	})
	if err != nil {
		return nil, err
	}
	resp.Model = o.Model
	resp.Provider = p.name
	debugf("reply (%d in, %d out) %d answers", resp.InputTokens, resp.OutputTokens, len(resp.Answers))

	recordUsage(resp, o)
	cache.put(key, resp)
	return resp, nil
}

// systemOneURL is the endpoint under a base URL. The SDK's base is the host
// alone, and the chat dialects' habit is to end it in /v1; both are accepted,
// since there is no other thing either could mean.
func systemOneURL(base string) string {
	base = strings.TrimRight(base, "/")
	base = strings.TrimSuffix(base, "/v1")
	return base + "/v1/systemone"
}

// systemOneResponse decodes a reply, and insists every question was answered.
// A missing answer would otherwise surface as a null several stages later,
// looking like a model that said nothing.
func systemOneResponse(payload []byte, questions map[string]any) (*response, error) {
	var decoded struct {
		Model   string         `json:"model"`
		Answers map[string]any `json:"answers"`
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if decoded.Answers == nil {
		return nil, fmt.Errorf("the response has no answers: %s", truncateForDebug(string(payload)))
	}
	for _, id := range sortedKeys(questions) {
		if _, ok := decoded.Answers[id]; !ok {
			return nil, fmt.Errorf("the response has no answer to question %q", id)
		}
	}
	return &response{
		Answers:      decoded.Answers,
		ServedModel:  decoded.Model,
		InputTokens:  decoded.Usage.InputTokens,
		OutputTokens: decoded.Usage.OutputTokens,
	}, nil
}

// systemOneObject is the shape invoke_systemone_request reports a call as. It
// keeps responseObject's names, so a pipeline that sums TotalTokens or Cost
// over both kinds of call does not have to tell them apart.
func systemOneObject(resp *response, o *options) map[string]any {
	out := map[string]any{
		"Answers":      resp.Answers,
		"Model":        resp.Model,
		"ServedModel":  nil,
		"Provider":     resp.Provider,
		"InputTokens":  resp.InputTokens,
		"OutputTokens": resp.OutputTokens,
		"TotalTokens":  resp.InputTokens + resp.OutputTokens,
		"Cost":         nil,
		"Cached":       resp.Cached,

		typed.TypeKey: "Pwrq.LLM.SystemOne",
	}
	if resp.ServedModel != "" {
		out["ServedModel"] = resp.ServedModel
	}
	if o != nil && (o.PriceInput > 0 || o.PriceOutput > 0) {
		out["Cost"] = float64(resp.InputTokens)*o.PriceInput/1e6 + float64(resp.OutputTokens)*o.PriceOutput/1e6
	}
	return out
}

// jsonType names a value the way the query author wrote it.
func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "text"
	case bool:
		return "a boolean"
	case map[string]any:
		return "an object"
	case []any:
		return "an array"
	default:
		return "a number"
	}
}
