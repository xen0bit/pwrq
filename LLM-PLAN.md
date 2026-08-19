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

## The examples are executed, not just written

`TestMetadataExamplesCompile` compiles every example `--udf-list` and `get_help`
print, and `TestAgentTriageExampleRuns` runs `examples/agent-triage.sh` end to
end against a stub provider — four stages, the agent loop included, in under a
second and with no API key. An example that needs a credential is an example
nothing runs, and an example nothing runs is documentation that rots: that
script had already shipped a jq scoping bug and a stage that printed zeros, both
past a careful read.

## Two bugs found in passing

Neither is about language models; both were found by insisting the examples run.

- **Nine documented examples could not work.** `--udf-list` and `get_help` print
  the examples in `metadata.go`, and nothing checked them: `format_table` and
  `measure_object` were shown at an arity they are not registered at,
  `where_object({ . > 10 })` is not valid jq, and `jsonl_parse`'s example was
  over-escaped. `TestMetadataExamplesCompile` now compiles every one of them.
- **`format_table` had no stable column order.** Columns came from ranging a Go
  map, so the same query printed `Gamma Alpha Beta` one run and
  `Alpha Beta Gamma` the next — which breaks diffs, golden files and anything
  reading a column by position. Sorted now, as the encoder already sorts keys.

## Standing guards

- LLM cmdlets are network calls, so like censys they are CLI-only and
  `WebRegistry` leaves them out.
- `metadata.go` carries every name; `TestUDFListMatchesRegistry` fails otherwise.
- The agent's allowlist is a compile-time registry, not a runtime check.
- No test performs real network I/O.
