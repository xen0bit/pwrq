# LLM and agentic cmdlets

## Context

pwrq is a query engine. It already faces the LLM world in one direction:
`pwrq --mcp` exposes the whole vocabulary as MCP tools, so an agent can write
pwrq queries and get JSON back. This plan is the other direction — putting a
model *inside* the pipeline, so a query can call one the way it calls `http`.

OpenCode (`github.com/anomalyco/opencode`) is the reference for what that
machinery looks like when it is done properly. The parts worth taking:

- `provider/provider.ts` — `providerID/modelID` addressing, credentials from
  documented environment variables with a stored-auth fallback.
- `agent/agent.ts` — an agent is *data*: model, prompt, tool set, permission
  ruleset, step cap. It uses `generateObject` for schema-constrained output.
- `permission/index.ts` — wildcard rules, `allow`/`ask`/`deny`, last match wins.
- `tool/task.ts` — subagents with a depth limit and derived permissions.

The parts to leave behind: sessions, compaction, retry/SSE plumbing, the TUI,
and — most importantly — the defaults. OpenCode's base ruleset is `"*": "allow"`
with interactive `ask`, because a human is watching. A jq pipeline has nobody to
ask. **Every default inverts.**

## The design decisions

**1. The pipeline-shaped call returns the completion, not an envelope.**
`invoke_llm` is a transform: `map(invoke_llm("summarize: \(.Body)"))` yields
strings, not objects to `.Content` out of. `invoke_llm_request` is the object
producer carrying model, tokens, cost and stop reason. This is the split pwrq
already draws between a transform and an object producer, and the same one
`http` draws against the PowerShell web cmdlets.

**2. Structured output is the feature, not an add-on.** `{Schema: {...}}` makes
the cmdlet return a *decoded, validated JSON value*. Homogeneous rows are what
`group_by`, `sort_by` and `select` need; prose is what they cannot use. pwrq
already vendors `google/jsonschema-go` for the MCP server, so the validator is
free. Everything else here is a party trick by comparison.

**3. No prompt-templating DSL, ever.** jq string interpolation already is one.
A `prompt_template` cmdlet would be exactly the "second vocabulary for the same
idea" the README rules out.

**4. Two provider adapters, hand-rolled over `net/http`.** Anthropic Messages
and OpenAI-compatible Chat Completions — the second buys OpenAI, Ollama, vLLM,
OpenRouter, Groq and LM Studio through a base URL. pwrq carries one vendor SDK
in total; that is the dependency taste to respect. A `{BaseUrl}` option is also
what keeps the tests hermetic, exactly as `censys_test.go` points the SDK at an
`httptest` server.

**5. Cost control is the censys paging lesson, restated.** A cursor followed to
its end is an unbounded number of billed requests; so is `map(invoke_llm(...))`
over a large input. So: a per-call token cap and timeout, `Temperature: 0` by
default so a pipeline is reproducible, a per-process call ceiling that *errors*
rather than silently stopping, and an opt-in response cache — jq's whole
workflow is edit-and-rerun, and without a cache every rerun re-bills.

Prices are deliberately **not** compiled in. A table of per-model prices in a Go
binary is stale the week after it ships, and a wrong cost is worse than none.
`.Cost` is reported when the caller supplies rates and is `null` otherwise.

**6. Bounded parallelism is a v1 requirement.** gojq is synchronous, so
`map(invoke_llm(...))` over 500 rows is 500 sequential round trips — the naive
design is unusable at any real scale. `invoke_llm_batch` runs a bounded pool and
emits **in input order**; out-of-order completions would be hostile to a
pipeline.

**7. The agent's tool surface is pwrq itself.** The MCP server already proves
this loop works. Inverting it reuses `queryrun` and `discovery` — `get_command`
*is* the tool catalog — with almost no new machinery. The critical part: the
allowlist is enforced **structurally**, by compiling the agent's sub-queries
against a registry built from the allowed cmdlets alone. A denied cmdlet does
not exist to the compiler, rather than being blocked by a runtime check a model
can probe around. Deny by default; `sh`, `rm` and the write cmdlets are never in
the default set, and the LLM cmdlets themselves can never be allowed — that is
what stops a billing loop.

## The vocabulary

| Cmdlet | Emits | What it is for |
|---|---|---|
| `invoke_llm` | one value | the completion — a string, or the decoded value under `Schema` |
| `invoke_llm_request` | one object | the same call with model, tokens, cost, stop reason |
| `invoke_llm_batch` | a stream | many prompts, bounded parallelism, input order |
| `invoke_systemone` | one object | typed questions about a state, answered as probabilities |
| `invoke_systemone_request` | one object | the same call with the served model, tokens and cost |
| `invoke_agent` | one value | a task solved by a model writing pwrq queries |
| `invoke_agent_request` | one object | the same run plus its full step trace |
| `invoke_embedding` | one value | the vector a model represents text by |
| `get_llm_model` | a stream | what a provider serves, as `.Model` names |
| `get_llm_context` | one object | where a call would go and which credential won |
| `get_llm_usage` | one object | what this process has spent |

`cosine_similarity` ships beside `levenshtein` and `jaccard` rather than here: it
is a pure transform over two arrays, so it belongs with the other similarity
measures and works in the browser as they do.

`llm` is the alias for `invoke_llm`.

Each takes its prompt from the pipeline or as the first argument, with an
optional trailing options object. At arity 1 the argument is read as the prompt
when it is a string and as options when it is an object — the two roles are
disjoint types, so this is a total rule rather than the operand-order guessing
`pkg/udf/README.md` warns against.

The System One pair is the exception, and reads its operands by **count**:
`invoke_systemone($questions)`, `($questions; $options)`,
`($state; $questions; $options)`. The trick above works because a prompt is
text; a state may be an object, so telling it from an options object would be
exactly the guess that rule forbids. Counting is not a guess.

## Phases

### Phase 0 — Foundation ✅

- [x] `pkg/udf/llm` package: connection resolution, options binding that
      **rejects unknown names** (the censys rule — an option that changes what a
      remote call asks for must not be silently ignored)
- [x] Anthropic and OpenAI-compatible adapters over `net/http`
- [x] Retry with backoff on 429 and 5xx, honouring `Retry-After`
- [x] Usage accounting and the per-process call ceiling
- [x] Opt-in on-disk response cache
- [x] `get_llm_context`, which never prints the key
- [x] `httptest` harness; no test may reach a real API

### Phase 1 — The call ✅

- [x] `invoke_llm`, `invoke_llm_request`, alias `llm`
- [x] `{Schema:}` structured output: forced tool-use on Anthropic,
      `response_format: json_schema` on OpenAI, validated either way
- [x] Bounded repair — the validation error goes back to the model once

### Phase 2 — Scale ✅

- [x] `invoke_llm_batch`: `{Parallel: n}`, ordered emission, `ContinueOnError`
- [x] `get_llm_usage`

### Phase 3 — Agentic ✅

- [x] `invoke_agent` / `invoke_agent_request` over a structurally restricted
      registry
- [x] Deny-by-default allowlist, step cap, wall-clock cap, per-query result and
      output bounds
- [x] The full trace on `.Steps`, so a run is auditable

### Phase 4 — Embeddings and discovery ✅

Brought forward from "later" because the two of them turn out to be one feature
and both are cheap:

- [x] `invoke_embedding`, and `cosine_similarity` beside `levenshtein` and
      `jaccard` — where those compare spelling, this compares meaning, and
      together they make semantic search an ordinary pipeline
- [x] `get_llm_model`, which matters most for a local server whose model names
      are whatever the person who downloaded them called the files
- [x] `.Reasoning` on the response object, since local runtimes send a thinking
      model's chain of thought beside the answer rather than inside it
- [x] `PWRQ_LLM_DEBUG`, which traces every request and reply to stderr

### Phase 5 — Not built

An MCP *client*, so external tools join the agent's vocabulary, and multi-turn
sessions. Each multiplies the trust or state surface, and neither blocks the
core. Also deliberately absent: a chat REPL, streaming tokens to stdout,
multimodal input, and conversation compaction.

### Phase 6 — System One ✅

TypeSafe's [System One](https://docs.typesafe.ai/api), served either by TypeSafe
or by llama.cpp's `/v1/systemone` in front of a GGUF. A question names its
options, the server labels each with a single token and one forward pass reads
the probability of each label.

It is here because it is what `{Schema: {enum: [...]}}` was being used for and is
strictly better at: no tokens are generated, so no answer can be outside the
options, nothing needs parsing or repair, and the reply carries how sure it is
rather than a word the model also had to choose. A threshold then belongs to the
query — `select(.page > 0.9)` — instead of to the prompt.

- [x] `invoke_systemone` / `invoke_systemone_request`, a third dialect beside
      Anthropic and OpenAI
- [x] Client-side validation of the question types, stricter than the server's
      in one way: an undefined key is an error, because `criterion` for
      `criteria` is a question whose options silently went missing
- [x] The 422 `detail` list rendered as `questions.q.criteria: Field required`
- [x] `PWRQ_SYSTEMONE_MODEL` beside `PWRQ_LLM_MODEL`, since a pipeline that
      classifies and also writes prose needs one of each; the wrong kind of
      model in either cmdlet is refused before the request
- [x] The options that shape generated text (`Temperature`, `MaxTokens`,
      `Schema`, ...) are rejected rather than ignored
- [x] One request is one call against the ceiling, however many questions it
      carries, and the cache has its own key space
- [x] Stage 3 of `examples/agent-triage.sh`, and the language and verdict
      decisions in `examples/pwrgrep-triage.jq`

Not built: `invoke_systemone_batch`. gojq is synchronous, so a `map` over rows
is one round trip each, exactly as `invoke_llm_batch` exists to fix — but a
question is one forward pass against a warm prefix, and a local server with
`-np 8` already runs the questions within a request in parallel. If a run's
wall clock turns out to be the `map` rather than the model, `runBatch` is the
pool to reuse.

## What validation found

The plan was written before any of it ran against a model. Three things only
showed up against a real one, all in `invoke_agent`, and all in the step
protocol rather than the loop:

1. **An optional field is a field a small model omits.** The first step schema
   asked for `{thought}` with `query` and `answer` optional. Constrained
   decoding emits the cheapest document that satisfies a schema, so
   `gemma-4-e2b` spent every step narrating the query it was about to write and
   never wrote one. Requiring an `action` enum fixed it.
2. **Two fields for one choice is one field too many.** With `query` beside
   `answer`, the same model filled in both — half a query in each. One
   `content` field, read according to `action`, has nothing to choose between.
3. **Models wrap things in markdown whatever the schema says.** A query field
   arrived as ```` ```jq …``` ````, sometimes with a trailing brace from the
   JSON object the model had started closing, and once with a zero-width space
   in it. None of that is ever valid jq, so stripping it is unambiguous — and
   the alternative is spending a step on a parse error whose cause the model
   cannot see.

Two more only showed up once the whole pipeline ran end to end, and both were
in the prompt rather than the protocol:

4. **Handing a model the data teaches it to paste the data.** The system prompt
   reproduced the pipeline input, so the model treated it as text it had been
   given: gemma-4-e2b inlined a six-element array into every query as a literal
   instead of writing `.`, until the step limit. The prompt now describes the
   shape — length, field names, one sample — and says to refer to `.`. It is
   cheaper per step as well.
5. **A placeholder that looks like syntax is syntax.** The prompt said "collect
   with `[...]`", and the model duly wrote `[...] | select(...)`. Spelled out as
   "wrap the call in square brackets" it stopped.

A sixth came out of reading the code rather than running it. `invoke_llm_batch`
ran its whole pool before checking for a failure, so a batch of five hundred
that failed on the first prompt still billed for the other four hundred and
ninety-nine. Fail-fast now cancels the rest. It passed every behavioural test
either way, which is the kind of bug only cost makes visible.

None of these are model-quality excuses: a 12B model on the same server runs the
loop cleanly, recovering from its own `top_by` mistake in three steps. They are
what the protocol has to survive to be worth shipping. What does remain a
model-quality question is stage 4 of `examples/agent-triage.sh`: a 2B model
classifies text well and writes queries badly, so the example takes a separate
`PWRQ_AGENT_MODEL` and says why.

What validating it against a local llama.cpp found, in the same spirit as the
list above:

7. **A failed loop is not an unknowable finding.** `pwrgrep-triage.jq` marked a
   row whose verification call had failed as `uncertain` and skipped the
   judgement, so six of eight findings in a run came back with no probability
   at all. The evidence window is seeded before the first turn, so there is
   always something to weigh: a model that ran out of tokens mid-thought has
   said nothing, not that the finding cannot be judged. The rows are judged now
   and only the rationale records the failure.
8. **A second model needs a second variable.** `PWRQ_AGENT_MODEL` in
   `agent-triage.sh` only changes the *model name*, and both stages resolve the
   same `OPENAI_BASE_URL` — so pointing it at a larger model on another port
   silently ran the agent on the small one, which then hit the token cap. That
   is why System One gets `PWRQ_SYSTEMONE_MODEL` rather than sharing
   `PWRQ_LLM_MODEL`: a pipeline that classifies and also writes prose needs to
   name one of each, endpoint included.
9. **The two models disagree, and the probability is where you see it.** On the
   same finding — a Python `open()` with no encoding — the 12B answered 0.96
   and `gemma-4-E2B` 0.17. A word from an enum hides that; a number makes it
   the caller's threshold to set, which is `tp_threshold` in the example.
10. **A probability is not bit-reproducible.** The same request through curl and
    through `invoke_systemone` agreed on every answer and differed in the fifth
    decimal, which is llama.cpp's slot batching rather than anything pwrq does.
    Thresholds are safe; equality on a probability is not.

Two things about the API itself are worth writing down. Server-side validation
errors (422) are the only ones a caller cannot reach through pwrq, because the
question shapes are checked before the request — which is the intent, and is why
the 422 renderer is unit-tested rather than demonstrated. And `/v1/models` on a
local server reports the GGUF's *path*, so `get_llm_model` yields
`systemone//mnt/...gguf`; llama.cpp's `--alias` is what makes that a name.

## The examples are executed, not just written

`TestMetadataExamplesCompile` compiles every example `--udf-list` and `get_help`
print, and `TestAgentTriageExampleRuns` runs `examples/agent-triage.sh` end to
end against a stub provider — four stages, the agent loop included, in under a
second and with no API key. An example that needs a credential is an example
nothing runs, and an example nothing runs is documentation that rots: that
script had already shipped a jq scoping bug and a stage that printed zeros, both
past a careful read.

## Three bugs found in passing

None is about language models; all three were found by insisting the examples
run.

- **Nine documented examples could not work.** `--udf-list` and `get_help` print
  the examples in `metadata.go`, and nothing checked them: `format_table` and
  `measure_object` were shown at an arity they are not registered at,
  `where_object({ . > 10 })` is not valid jq, and `jsonl_parse`'s example was
  over-escaped. `TestMetadataExamplesCompile` now compiles every one of them.
- **The last stage of `agent-triage.sh` printed nothing.** It asked for the
  run's usage with `pwrq -c` and nothing piped in, so pwrq waited on an empty
  stdin, had no input to run the query against, and emitted nothing at all —
  past every previous reading of that script, because a stage that prints
  nothing looks like a stage that had nothing to say. `-n` fixes it, and the
  integration test now asserts the stage's output rather than only the four
  above it.
- **`format_table` had no stable column order.** Columns came from ranging a Go
  map, so the same query printed `Gamma Alpha Beta` one run and
  `Alpha Beta Gamma` the next — which breaks diffs, golden files and anything
  reading a column by position. Sorted now, as the encoder already sorts keys.

## Validating System One against a local llama.cpp

No test reaches a real endpoint, so this is the manual pass. It runs against
[the fork](https://github.com/xen0bit/llama.cpp/pull/1) that serves
`/v1/systemone`; the endpoint needs no flag to enable it.

```bash
M=/path/to/models
S=~/Projects/llama.cpp/build-cuda/bin/llama-server
$S -m $M/gemma-4-12B-it-QAT-Q4_0.gguf -np 8 -c 32768 --port 8080
$S -m $M/gemma-4-E2B_q4_0-it.gguf -np 8 -c 32768 --port 8081 --systemone-permute

export TYPESAFE_BASE_URL=http://127.0.0.1:8080 PWRQ_SYSTEMONE_MODEL=systemone/gemma
```

1. `get_llm_context({Model: "systemone/gemma"})` — the endpoint is
   `.../v1/systemone` and `ApiKeyRequired` is false against a local server.
2. `[get_llm_model({Model: "systemone"})]` — the served model.
3. The three question types in one `invoke_systemone_request`: `OutputTokens` is
   one per question, or two with `--systemone-permute` on :8081.
4. A base URL ending in `/v1` reaches the same endpoint as one without.
5. Errors: a `choice` with no criteria fails with no POST in `PWRQ_LLM_DEBUG=1`;
   `--api-key secret` gives a 401 without it and an answer with it; a server
   started `--embedding` gives a 501, once, with no retry.
6. `{Cache: true}` three times over the same question: `[false, true, true]`,
   one call, two cache hits.
7. A System One model in `invoke_llm`, and a chat model in `invoke_systemone`,
   are both refused before anything is sent.
8. `examples/agent-triage.sh` with `PWRQ_SYSTEMONE_MODEL` set — stage 3 fills
   in, and with it unset the run still completes and says it skipped.
9. `examples/pwrgrep-triage.jq` over a small tree, once per judging model, so
   the language scores and the verdict tally can be compared.
10. The same body through `curl` and through `invoke_systemone`: the same
    answers, to about four decimal places.

## Standing guards

- LLM cmdlets are network calls, so like censys they are CLI-only and
  `WebRegistry` leaves them out.
- `metadata.go` carries every name; `TestUDFListMatchesRegistry` fails otherwise.
- The agent's allowlist is a compile-time registry, not a runtime check.
- No test performs real network I/O. The harness pins `TYPESAFE_*` at the fake
  server too, so a developer's own shell cannot turn a test into a real call.
- `invoke_systemone` is in `forbiddenPrefixes`, for the reason the other model
  cmdlets are: an agent that can call a model can spend without limit.
