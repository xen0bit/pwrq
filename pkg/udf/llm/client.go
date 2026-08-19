// Package llm puts a language model inside the pipeline.
//
// pwrq already faces the model world in one direction: `pwrq --mcp` hands the
// whole vocabulary to an agent. These cmdlets are the other direction, so a
// query can call a model the way it calls http.
//
// Three things about that call are different from every other cmdlet, and they
// are what this package is mostly about:
//
//   - It is billed. `map(invoke_llm(...))` over a large input is an unbounded
//     bill, which is the same problem censys paging has and gets the same
//     answer: a ceiling that errors rather than a limit that silently stops.
//   - It is slow. gojq evaluates synchronously, so a mapped call is one round
//     trip per row. invoke_llm_batch exists because that is unusable at any
//     real scale.
//   - It is non-deterministic. Temperature defaults to 0 and the cache is
//     keyed on the exact request, so a pipeline re-run has some chance of
//     producing what it did the first time.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xen0bit/pwrq/pkg/core/pipeline"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// The environment variables each provider documents. Using the vendor's own
// names means a shell already set up for that provider's CLI or SDK works for
// pwrq unchanged.
const (
	EnvModel = "PWRQ_LLM_MODEL"

	EnvAnthropicKey  = "ANTHROPIC_API_KEY"
	EnvAnthropicBase = "ANTHROPIC_BASE_URL"
	EnvOpenAIKey     = "OPENAI_API_KEY"
	EnvOpenAIBase    = "OPENAI_BASE_URL"
	EnvOllamaBase    = "OLLAMA_BASE_URL"

	// EnvMaxCalls raises or removes the per-process call ceiling.
	EnvMaxCalls = "PWRQ_LLM_MAX_CALLS"
	// EnvCache turns the response cache on without editing the query.
	EnvCache = "PWRQ_LLM_CACHE"
	// EnvCacheDir moves it off the default location.
	EnvCacheDir = "PWRQ_LLM_CACHE_DIR"
	// EnvDebug traces every call to stderr.
	EnvDebug = "PWRQ_LLM_DEBUG"
)

// debugf traces a call to stderr when PWRQ_LLM_DEBUG is set.
//
// A model call is the one cmdlet whose failure is usually not in the code: the
// model said something unexpected, and no amount of reading pwrq explains
// what. This prints what went out and what came back. stderr is the only safe
// sink — stdout is the query's own output, and over --mcp it is the wire.
func debugf(format string, args ...any) {
	if os.Getenv(EnvDebug) == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "pwrq llm: "+format+"\n", args...)
}

// defaultTimeout matches the http cmdlet's, doubled: a model that is thinking
// is not a model that has hung, and 30s times out ordinary work.
const defaultTimeout = 120 * time.Second

// defaultMaxTokens bounds one reply. It is a cost control as much as a
// correctness one, and a caller who wants an essay says so.
const defaultMaxTokens = 4096

// provider is one API dialect. pwrq ships two, and the second is a family:
// anything speaking OpenAI's chat completions schema reaches it through a base
// URL, which is Ollama, vLLM, OpenRouter, Groq and LM Studio.
type provider struct {
	// name is the prefix in a "provider/model" string.
	name string
	// keyEnv and baseEnv are the vendor's documented variables.
	keyEnv  string
	baseEnv string
	// defaultBase is where a request goes with nothing configured.
	defaultBase string
	// keyOptional marks a provider that needs no credential, which local
	// servers do not.
	keyOptional bool
	// dialect selects the request and response encoding.
	dialect string
}

const (
	dialectAnthropic = "anthropic"
	dialectOpenAI    = "openai"
)

var providers = map[string]provider{
	"anthropic": {
		name: "anthropic", keyEnv: EnvAnthropicKey, baseEnv: EnvAnthropicBase,
		defaultBase: "https://api.anthropic.com", dialect: dialectAnthropic,
	},
	"openai": {
		name: "openai", keyEnv: EnvOpenAIKey, baseEnv: EnvOpenAIBase,
		defaultBase: "https://api.openai.com/v1", dialect: dialectOpenAI,
	},
	"ollama": {
		name: "ollama", keyEnv: "", baseEnv: EnvOllamaBase,
		defaultBase: "http://localhost:11434/v1", keyOptional: true, dialect: dialectOpenAI,
	},
	// openai-compatible is the escape hatch: any server speaking the chat
	// completions schema, named explicitly so the base URL is not a surprise.
	"openai-compatible": {
		name: "openai-compatible", keyEnv: EnvOpenAIKey, baseEnv: EnvOpenAIBase,
		defaultBase: "", keyOptional: true, dialect: dialectOpenAI,
	},
}

func providerNames() []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// options are everything a caller can say about a call.
//
// Every one of these changes what the remote API is asked for, or what it
// costs, which is why bindOptions rejects a name that is not here rather than
// ignoring it: a misspelled {MaxTokns: 8000} that silently used the default
// would leave the caller believing they had asked for something.
type options struct {
	Model       string   `param:"Model"`
	System      string   `param:"System"`
	Temperature float64  `param:"Temperature"`
	MaxTokens   int      `param:"MaxTokens"`
	TopP        float64  `param:"TopP"`
	StopAt      []string `param:"StopAt"`

	ApiKey  string `param:"ApiKey"`
	BaseUrl string `param:"BaseUrl"`
	Timeout int    `param:"Timeout"`
	Retries int    `param:"Retries"`

	Schema map[string]any `param:"Schema"`
	Repair int            `param:"Repair"`

	Cache    bool   `param:"Cache"`
	CacheDir string `param:"CacheDir"`

	MaxCalls    int     `param:"MaxCalls"`
	PriceInput  float64 `param:"PriceInput"`
	PriceOutput float64 `param:"PriceOutput"`

	// Parallel and ContinueOnError belong to invoke_llm_batch, and Allow,
	// MaxSteps, MaxSeconds and MaxResults to invoke_agent. They live in this
	// struct rather than in structs of their own because BindParameters does
	// not traverse embedded fields; the group tag is what keeps invoke_llm
	// rejecting {Allow: [...]}, since an option a cmdlet silently ignores is
	// one the caller believes they set.
	Parallel        int  `param:"Parallel" group:"batch"`
	ContinueOnError bool `param:"ContinueOnError" group:"batch"`

	Allow      []string `param:"Allow" group:"agent"`
	MaxSteps   int      `param:"MaxSteps" group:"agent"`
	MaxSeconds int      `param:"MaxSeconds" group:"agent"`
	MaxResults int      `param:"MaxResults" group:"agent"`

	// temperatureSet records that the caller asked for a temperature, so 0 —
	// the default, and also a meaningful request — is distinguishable from
	// unset.
	temperatureSet bool
	// modelSource and keySource record where the values came from, so
	// get_llm_context can explain a misconfiguration without printing the key.
	modelSource string
	keySource   string
	baseSource  string
}

// defaults returns the options a call runs under before anything is said.
func defaults() options {
	return options{
		Temperature: 0,
		MaxTokens:   defaultMaxTokens,
		Repair:      1,
		Retries:     2,
	}
}

// optionsArg checks that an options argument is an object before binding it.
func optionsArg(op string, v any) (map[string]any, error) {
	switch val := common.BindValue(v).(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return val, nil
	default:
		return nil, fmt.Errorf("%s: options must be an object, got %T", op, val)
	}
}

// bindOptions binds an options object, refusing a name the struct does not
// declare.
//
// pipeline.BindParameters ignores unknown names, which is right for a cmdlet
// whose options are local flags and wrong for one that talks to a paid API.
// This is the same rule pkg/udf/censys applies, for the same reason.
func bindOptions(op string, opts map[string]any, target *options, groups ...string) error {
	if len(opts) == 0 {
		return nil
	}
	known := paramNames(target, groups...)
	normalized := make(map[string]any, len(opts))
	for k, v := range opts {
		if _, ok := known[strings.ToLower(k)]; !ok {
			return fmt.Errorf("%s: unknown option %q; expected one of %s",
				op, k, strings.Join(sortedNames(known), ", "))
		}
		if strings.EqualFold(k, "Temperature") {
			target.temperatureSet = true
		}
		normalized[k] = normalizeNumbers(v)
	}
	if err := pipeline.BindParameters(normalized, target); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// paramNames maps a struct's option names, lowercased for the case-insensitive
// match PowerShell parameters use, to the spelling they were declared with —
// which is the one an error message should suggest.
func paramNames(target any, groups ...string) map[string]string {
	permitted := map[string]bool{"": true}
	for _, g := range groups {
		permitted[g] = true
	}
	names := make(map[string]string)
	t := reflect.TypeOf(target)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return names
	}
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("param")
		if tag == "" || !permitted[t.Field(i).Tag.Get("group")] {
			continue
		}
		declared := strings.TrimSpace(strings.Split(tag, ",")[0])
		names[strings.ToLower(declared)] = declared
	}
	return names
}

func sortedNames(known map[string]string) []string {
	out := make([]string, 0, len(known))
	for _, declared := range known {
		out = append(out, declared)
	}
	sort.Strings(out)
	return out
}

// normalizeNumbers puts json.Number into the Go numeric types BindParameters
// converts from. Input decoded by the CLI arrives as json.Number, and
// reflection-based binding cannot see through it.
func normalizeNumbers(v any) any {
	switch val := v.(type) {
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return int(i)
		}
		if f, err := val.Float64(); err == nil {
			return f
		}
		return val.String()
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = normalizeNumbers(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = normalizeNumbers(item)
		}
		return out
	}
	return v
}

// resolve fills in everything the caller did not say, from the environment and
// then from the defaults, and reports where each value came from.
func (o *options) resolve(op string) (provider, error) {
	o.modelSource = "Model option"
	if o.Model == "" {
		o.Model, o.modelSource = os.Getenv(EnvModel), EnvModel
	}
	if o.Model == "" {
		return provider{}, fmt.Errorf("%s: no model; set %s or pass {Model: \"provider/model\"}, e.g. {Model: \"anthropic/claude-sonnet-4-5\"}", op, EnvModel)
	}

	providerID, modelID, found := strings.Cut(o.Model, "/")
	if !found || modelID == "" {
		return provider{}, fmt.Errorf("%s: model %q must be spelled provider/model; known providers are %s",
			op, o.Model, strings.Join(providerNames(), ", "))
	}
	// An OpenRouter model is "openai/gpt-4o" behind a base URL, so a second
	// slash belongs to the model name, not the provider.
	p, ok := providers[providerID]
	if !ok {
		return provider{}, fmt.Errorf("%s: unknown provider %q in model %q; known providers are %s",
			op, providerID, o.Model, strings.Join(providerNames(), ", "))
	}

	o.keySource = "ApiKey option"
	if o.ApiKey == "" && p.keyEnv != "" {
		o.ApiKey, o.keySource = os.Getenv(p.keyEnv), p.keyEnv
	}
	if o.ApiKey == "" {
		o.keySource = ""
	}

	o.baseSource = "BaseUrl option"
	if o.BaseUrl == "" && p.baseEnv != "" {
		o.BaseUrl, o.baseSource = os.Getenv(p.baseEnv), p.baseEnv
	}
	if o.BaseUrl == "" {
		o.BaseUrl, o.baseSource = p.defaultBase, "default"
	}
	if o.BaseUrl == "" {
		return provider{}, fmt.Errorf("%s: provider %q has no default endpoint; pass {BaseUrl: \"https://...\"} or set %s",
			op, providerID, p.baseEnv)
	}
	o.BaseUrl = strings.TrimRight(o.BaseUrl, "/")

	if o.Timeout < 0 {
		return provider{}, fmt.Errorf("%s: Timeout must not be negative, got %d", op, o.Timeout)
	}
	if o.MaxTokens <= 0 {
		o.MaxTokens = defaultMaxTokens
	}
	if o.Retries < 0 {
		o.Retries = 0
	}
	if o.Repair < 0 {
		o.Repair = 0
	}
	return p, nil
}

// modelID is the model name with the provider prefix removed, which is what
// the API itself expects.
func (o options) modelID() string {
	_, id, _ := strings.Cut(o.Model, "/")
	return id
}

func (o options) timeout() time.Duration {
	if o.Timeout > 0 {
		return time.Duration(o.Timeout) * time.Second
	}
	return defaultTimeout
}

// requireKey fails a call that has nothing to authenticate with, before it
// becomes a 401 from the API. "ANTHROPIC_API_KEY is not set" is the actionable
// version of that message.
func (o options) requireKey(op string, p provider) error {
	if o.ApiKey != "" || p.keyOptional {
		return nil
	}
	return fmt.Errorf("%s: no API key for provider %q; set %s or pass {ApiKey: \"...\"}", op, p.name, p.keyEnv)
}

// response is one completed call, in the shape the object-producing cmdlets
// report and the cache stores.
type response struct {
	Content string
	// Reasoning is a thinking model's visible chain of thought, when the
	// provider sends one alongside the answer rather than inside it.
	Reasoning    string
	Model        string
	Provider     string
	StopReason   string
	InputTokens  int
	OutputTokens int
	// Structured is the decoded value when a Schema was asked for.
	Structured any
	// Cached records that this came back without a request being made.
	Cached bool
}

// complete performs one call, with the cache, the budget, the retries and the
// schema repair loop around it.
func complete(ctx context.Context, op string, prompt string, o options, p provider) (*response, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("%s: prompt is empty", op)
	}
	return completeMessages(ctx, op, []message{{Role: "user", Content: prompt}}, o, p)
}

// completeMessages is complete over a conversation rather than a single turn,
// which is what the agent loop needs: each step sends everything that has
// happened so far.
func completeMessages(ctx context.Context, op string, conversation []message, o options, p provider) (*response, error) {
	if err := o.requireKey(op, p); err != nil {
		return nil, err
	}

	var validator *schemaValidator
	if o.Schema != nil {
		var err error
		if validator, err = compileSchema(op, o.Schema); err != nil {
			return nil, err
		}
	}

	cache, err := openCache(op, o)
	if err != nil {
		return nil, err
	}

	// Copied, because the repair loop appends and the caller's slice is not
	// this function's to grow.
	messages := append([]message(nil), conversation...)
	var lastErr error
	// One attempt, plus Repair more if the model's structured answer does not
	// satisfy the schema. Sending the validation error back is the cheapest
	// repair there is, and bounding it is what stops a malformed schema from
	// becoming an unbounded bill.
	for attempt := 0; attempt <= o.Repair; attempt++ {
		if key := cache.key(o, messages); cache != nil && key != "" {
			if hit, ok := cache.get(key); ok {
				recordCacheHit()
				hit.Cached = true
				if validator != nil {
					decoded, err := validator.decode(op, hit.Content)
					if err == nil {
						hit.Structured = decoded
						return hit, nil
					}
					// A cached answer that no longer validates is a stale
					// entry, not a repair round; fall through and ask again.
				} else {
					return hit, nil
				}
			}
		}

		if err := chargeCall(op, o); err != nil {
			return nil, err
		}
		resp, err := send(ctx, op, messages, o, p, validator)
		if err != nil {
			return nil, err
		}
		recordUsage(resp, o)

		if validator == nil {
			cache.put(cache.key(o, messages), resp)
			return resp, nil
		}

		decoded, decodeErr := validator.decode(op, resp.Content)
		if decodeErr == nil {
			resp.Structured = decoded
			cache.put(cache.key(o, messages), resp)
			return resp, nil
		}
		lastErr = decodeErr
		if attempt == o.Repair {
			break
		}
		messages = append(messages,
			message{Role: "assistant", Content: resp.Content},
			message{Role: "user", Content: fmt.Sprintf(
				"That response did not satisfy the schema: %v. Reply with JSON that does, and nothing else.", decodeErr)},
		)
	}
	return nil, lastErr
}

// errTruncatedReply marks a reply that hit the token cap before producing
// anything. It is a sentinel rather than a message because only the caller of
// the decoder knows what the cap was set to, and the number is the whole point
// of the advice.
var errTruncatedReply = errors.New("the reply was cut off by the token cap")

// message is one turn, in the only shape both dialects share.
//
// The tags are the OpenAI wire spelling, because that dialect encodes these
// directly. The Messages API wraps them in its own content blocks, so it does
// not depend on them.
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// send performs one HTTP round trip, retrying what is worth retrying.
func send(ctx context.Context, op string, messages []message, o options, p provider, validator *schemaValidator) (*response, error) {
	var lastErr error
	for attempt := 0; ; attempt++ {
		resp, retryAfter, err := attemptSend(ctx, op, messages, o, p, validator)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		var retriable *retriableError
		if !asRetriable(err, &retriable) || attempt >= o.Retries {
			return nil, lastErr
		}
		// A server that said how long to wait knows better than a backoff
		// curve does.
		wait := retryAfter
		if wait <= 0 {
			wait = time.Duration(1<<attempt) * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%s: %w", op, ctx.Err())
		case <-time.After(wait):
		}
	}
}

// retriableError marks a failure that another attempt might not hit: a rate
// limit, a server fault, or a connection that did not survive.
type retriableError struct{ err error }

func (e *retriableError) Error() string { return e.err.Error() }
func (e *retriableError) Unwrap() error { return e.err }

func asRetriable(err error, target **retriableError) bool {
	for err != nil {
		if r, ok := err.(*retriableError); ok {
			*target = r
			return true
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapped.Unwrap()
	}
	return false
}

func attemptSend(ctx context.Context, op string, messages []message, o options, p provider, validator *schemaValidator) (*response, time.Duration, error) {
	var (
		url  string
		body []byte
		err  error
	)
	switch p.dialect {
	case dialectAnthropic:
		url, body, err = anthropicRequest(messages, o, validator)
	case dialectOpenAI:
		url, body, err = openAIRequest(messages, o, validator)
	default:
		return nil, 0, fmt.Errorf("%s: provider %q has no request encoding", op, p.name)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, o.timeout())
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, o.BaseUrl+url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}
	req.Header.Set("Content-Type", "application/json")
	switch p.dialect {
	case dialectAnthropic:
		req.Header.Set("x-api-key", o.ApiKey)
		req.Header.Set("anthropic-version", anthropicVersion)
	case dialectOpenAI:
		if o.ApiKey != "" {
			req.Header.Set("Authorization", "Bearer "+o.ApiKey)
		}
	}

	debugf("POST %s%s %s", o.BaseUrl, url, truncateForDebug(string(body)))

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		// A transport failure is worth another attempt; a deadline that has
		// already passed is not.
		if reqCtx.Err() != nil {
			return nil, 0, fmt.Errorf("%s: request timed out after %s", op, o.timeout())
		}
		return nil, 0, &retriableError{fmt.Errorf("%s: %w", op, err)}
	}
	defer func() { _ = httpResp.Body.Close() }()

	payload, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, 0, &retriableError{fmt.Errorf("%s: reading response: %w", op, err)}
	}

	if httpResp.StatusCode != http.StatusOK {
		wait := retryAfter(httpResp.Header.Get("Retry-After"))
		err := fmt.Errorf("%s: %s returned %s: %s", op, p.name, httpResp.Status, apiErrorMessage(payload))
		if httpResp.StatusCode == http.StatusTooManyRequests || httpResp.StatusCode >= 500 {
			return nil, wait, &retriableError{err}
		}
		return nil, 0, err
	}

	var resp *response
	switch p.dialect {
	case dialectAnthropic:
		resp, err = anthropicResponse(payload, validator != nil)
	case dialectOpenAI:
		resp, err = openAIResponse(payload)
	}
	if err != nil {
		if errors.Is(err, errTruncatedReply) {
			return nil, 0, fmt.Errorf("%s: the reply hit the token cap (MaxTokens: %d) before producing any content; raise it with {MaxTokens: n}", op, o.MaxTokens)
		}
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}
	resp.Model = o.Model
	resp.Provider = p.name
	debugf("reply (%d in, %d out, %s) %s", resp.InputTokens, resp.OutputTokens, resp.StopReason, truncateForDebug(resp.Content))
	return resp, 0, nil
}

// truncateForDebug keeps a trace line readable. A prompt can be a whole file.
func truncateForDebug(s string) string {
	const limit = 600
	if len(s) <= limit {
		return s
	}
	return s[:limit] + fmt.Sprintf("... (%d bytes)", len(s))
}

// retryAfter reads the header in either of the forms the RFC allows.
func retryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if when, err := http.ParseTime(header); err == nil {
		if wait := time.Until(when); wait > 0 {
			return wait
		}
	}
	return 0
}

// apiErrorMessage digs the human-readable part out of an error body, falling
// back to the body itself. Both dialects nest it under "error".
func apiErrorMessage(payload []byte) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &envelope); err == nil && envelope.Error.Message != "" {
		return envelope.Error.Message
	}
	trimmed := strings.TrimSpace(string(payload))
	if len(trimmed) > 400 {
		trimmed = trimmed[:400] + "..."
	}
	if trimmed == "" {
		return "(no response body)"
	}
	return trimmed
}
