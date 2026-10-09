# pwrq design

This is the durable design record: the constraints, the decisions and the
reasons behind them. It is a distillation of five plans that drove the project
(`PLAN.md`, `CLEANUP-PLAN.md`, `SHAPE-PLAN.md`, `LLM-PLAN.md`, `TUI-PLAN.md`),
kept here as the *why*; the *how* for a contributor is [AGENTS.md](../AGENTS.md),
and usage is [README.md](../README.md) / [EXAMPLES.md](../EXAMPLES.md).

## The one constraint

pwrq is a strict gojq superset: any valid jq program produces byte-identical
output. That is enforced, not aspirational — the CLI test corpus (`cli/test.yaml`)
runs gojq's own 831 cases plus 9 pwrq-specific ones, so pwrq cannot drift from
jq without a test failing. The pwrq-specific cases cover query-file module
resolution, the `select_object` pipeline forms, `_val`/`_meta` anti-regression,
number types and `get_childitem` objects.

Two facts about gojq shape everything downstream:

- gojq's compiler checks custom functions **last**, after builtins. A custom
  function structurally cannot shadow a jq builtin, which makes the superset
  property nearly free.
- That same ordering means a *definition* (an alias) does win over a builtin.
  A badly chosen alias silently changes what existing jq programs mean, so the
  alias table needs a collision guard, not just wiring.

The original fork broke the superset guarantee in one place: `cli/encoder.go`
unwrapped any map carrying `_val` + `_meta` at print time, rewriting ordinary
user JSON. Removing that unwrap is the root of the object model below.

## The object model

**The wire format of a typed object is ordinary JSON:** a flat object whose keys
are its property names, plus a `PwrqType` property. No `_val`, no `_meta`, no Go
structs in the output stream. This is what `ConvertTo-Json` emits, and it makes
cmdlet output navigable with plain jq.

Three kinds of function reach the encoder, and nothing else:

| Kind | Returns | Examples |
|---|---|---|
| Transform | the transformed value | `sha256`, `base64_encode`, `find` |
| Object producer | flat object + `PwrqType` | `get_childitem`, `get_process`, `http` |
| Formatter | a string | `format_table`, `format_list` |

A Go value that is not a JSON type is a query error, not just an encoding one: a
`time.Time` in the pipeline is a value no jq builtin can act on. `typed.NormalizeJSON`
converts at the boundary (`time.Time` → RFC3339, `os.FileMode` → its string,
sized integers → `int`). Binary is hex- or base64-encoded per cmdlet, JSON having
no byte type; decoders accept the same representation so round-trips work, and
each cmdlet declares which encoding it emits (see "Declare facts where you
register them").

Failures return an `error`, which gojq puts on its error channel, so `try`/`catch`,
`//` and the process exit status behave as they do for jq's own failures. A value
that merely *looks* like a failure is the anti-pattern: the pipeline carries on
with it and the caller finds out much later.

## The type space is pwrq's own

Every type name used to be borrowed: `System.IO.FileInfo`,
`Microsoft.PowerShell.Commands.GroupInfo`, `System.String`. That is a promise
pwrq cannot keep — the name says .NET, the flat object says otherwise, and the
name is what a reader (or a model) trusts first. So:

| kind | name |
|---|---|
| envelope keys | `PwrqType`, `PwrqValue` |
| cmdlet output | `Pwrq.FileSystem.File`, `Pwrq.Group`, `Pwrq.Sqlite.Row`, … |
| a value with no pwrq type | `string`, `number`, `object`, `array` |
| the Go model | `typed.Object` |

`PwrqType`/`PwrqValue` are prefixed rather than shortened (`Type`, `TypeName`
both collide with real data), and a bare identifier, so `.PwrqType` reads in a
path expression without quoting. An untyped value answers in jq's own
vocabulary — there is no catalogue entry for a bare string, so inventing a
`Pwrq.*` name for it would be a key that resolves to nothing.

PowerShell's *property* names stay (`invoke_web_request` still returns
`StatusCode`, `RequestMethod`, `BaseResponse`); only the name of the thing
changed, not the names inside it.

## Output shapes

`PSTypeName` looked like a type system but was decoration applied through three
unrelated idioms with no registry. The replacement records, for every object
producer, whether its keys are:

| Kind | Means | Rendered as |
|---|---|---|
| `Fixed` | the cmdlet decides the keys | the type name and its field list |
| `Derived` | the input decides the keys | the rule, e.g. "keys are the input's leaf paths" |
| `Dynamic` | an external source decides | what the source is, e.g. "one key per selected column" |
| `Unspecified` | not an object producer | nothing |

The declaration lives at registration (`pkg/core/shape`, read back through
`ShapeOf`), because registration is the chokepoint that already works: choosing
`WithIterFunction` over `WithFunction` *is* the streaming declaration, so
documentation cannot disagree with behaviour. Declared and emitted are reconciled
at construction: a shape is also the constructor, and a key that is undeclared or
a declared key that is missing is recorded as a discrepancy — never an error to
the caller, so a documentation bug cannot break a user's query. A test asserts
the discrepancy table is empty after the suite has run, which reconciles against
real cmdlet output rather than a second hand-written list.

Nothing is declared for the ~434 scalar transforms: inventing a field list for a
cmdlet with no fields is the drift this exists to remove. Because Derived and
Dynamic shapes exist, a declared catalogue can never be complete, so `run_query`
also reports the shape it *observed* in the values it just produced — free,
cannot drift, and covers precisely the cases declaration cannot reach.

The same treatment applies to the input side: `common.SplitInput` decides whether
the input arrives from the pipeline or as the leading argument, and that
declaration is hoisted to registration so `get_childitem/1-2` says which argument
is the input.

Censys and LLM producers stay `Unspecified`: their examples cannot run in CI, so a
declaration could not be reconciled against real output, and an unreconciled
declaration is the drift this is built to avoid.

## Registration is the single source of truth

Cmdlet names and arities are not declared by hand. `Registry.Signatures()`
discovers registered name/arity pairs by asking gojq for `builtins` with and
without the registry applied and taking the difference. Aliases are compiled into
the query as jq `FuncDef`s generated from the arities the registry reports
(`pkg/udf/alias.go`), so an alias cannot fall out of step with its target.

**Declare facts where you register them.** Streaming (`WithIterFunction` vs
`WithFunction`), output shape (`WithFunctionOf`), input form (`DeclareInput`),
output encoding (`DeclareEncoding`) and accepted input encoding
(`DeclareConsumes`) are all read back by `get_help`, `get_command`,
`list_functions`, the MCP observed-shape report and the encoding-mismatch
warnings, so documentation cannot drift from behaviour.

Standing guards fail the build rather than relying on anyone remembering:

- `TestCliRun` — gojq's corpus, byte-identical.
- `TestNoBuiltinShadowing` — no UDF or alias shares a builtin's signature.
- `TestUDFListMatchesRegistry` — documented set equals registered set.
- `TestMetadataArityMatches` — documented arities equal registered arities.
- `TestAliasesResolve` — every alias names something that exists.
- `TestEveryPublishedExampleRuns` / `TestMetadataExamplesCompile` — every
  published example compiles and runs (with an explicit exemption list).
- `TestCallingFormsAgree` — piped and explicit forms of a cmdlet agree.

## The cmdlet catalogue: what earns its place

The catalogue grew to 488 functions across four batch-generated rounds, then was
cut back to ~441 by two tests applied in order:

1. **Cut test.** A function goes if it reproduces a jq builtin, or if it is a
   one-off novelty with no place in a general-purpose library.
2. **Keep test.** A function stays if a mainstream standard library covers it —
   Python's `statistics`, `math`, `datetime`, `zipfile`, `tarfile`. This is the
   permissive reading, and it deliberately protects the statistics category and
   the core of number theory.

Notable cuts: `upper`/`lower` beside `ascii_upcase`/`ascii_downcase`,
`regex_split` beside `splits`, `take`/`drop` beside `.[:n]`/`.[n:]`, the whole
`regex_*` family beside jq's `scan`/`match`/`capture`/`sub`, novelty number
theory (`roman_numeral`, `to_words`, `collatz_steps`), and 22 pairwise unit
converters collapsed into one table-driven `convert_unit`. Keeping a private
`regex_*` dialect teaches newcomers the wrong vocabulary. `sort_keys` was deleted
as a no-op: gojq has no object key order and the encoder already sorts keys.

The cut exposed a bug class the guards had missed: binding by guessing. The
collection helpers returned whichever argument happened to be an array, so
`count_by(rows; "key")` and `count_by("key"; rows)` both succeeded and
`zip_arrays(a; b)` paired its operands in the opposite order from
`a | zip_arrays(b)`. The rule is now positional and documented — see
[AGENTS.md](../AGENTS.md#binding) — and `TestCallingFormsAgree` enforces it.

Writing the gallery (204 → 389 cmdlets covered) was the best
API review in the exercise: it found that `csv_parse` yields rows rather than
objects, that the JSON-path cmdlets take a dot-and-bracket string rather than an
array, that `template` uses `{{name}}`, and that `percentage`, `gcd`, `lerp`,
`days_between`, `subnet_of` and the ciphers all bind differently than a reader
would guess.

## Language models and agents

A jq pipeline has nobody to ask, so OpenCode's interactive defaults invert:
deny by default, bound everything, and never let a wrong query spend money or
change a remote system.

- **Structured output is the feature.** `{Schema: ...}` returns a decoded,
  validated value, because homogeneous rows are what `group_by`/`sort_by` need
  and prose is what they cannot use. Validation is enforced on the way back, with
  one bounded repair turn.
- **No prompt-templating DSL, ever.** jq string interpolation already is one.
- **Cost is bounded by a ceiling, not a limit.** One process makes 100 model
  calls before it refuses, because `map(invoke_llm(...))` over a large input
  looks exactly like the query that meant to do it. Temperature defaults to 0 so
  a query can be re-run, and the response cache is opt-in.
- **Prices are not compiled in.** A price table in a binary is stale the week
  after it ships; `.Cost` is reported only when the caller supplies rates.
- **The agent's tool surface is pwrq itself, and the allowlist is structural.**
  The agent's sub-queries compile against a registry built from the allowed
  cmdlets alone, so a denied cmdlet does not exist to the compiler. `sh`, `rm`,
  the write cmdlets and the network cmdlets are not in the default set, and the
  LLM cmdlets can never be allowed — that is what stops a billing loop. A run is
  bounded by `MaxSteps`, `MaxSeconds` and a per-query result cap, and the agent's
  queries get no environment loader.
- **Writing to a remote is opt-in at registration, not at call time.** The Censys
  write cmdlets are absent unless `PWRQ_CENSYS_WRITE=1`, and withheld from the
  catalogue to match, so naming one is a compile error rather than a request. A
  wrong search can be rewritten; a wrong `remove_censys_tag` has already deleted
  a tag.
- **System One answers typed questions as probabilities.** A model labels each
  option with a single token and one forward pass reads how likely each is, so
  nothing is generated and nothing can come back outside the options.

### What running a model actually taught

These are protocol lessons, not model-quality excuses; the same server runs the
agent loop cleanly on a 12B model.

1. An optional field is a field a small model omits. Requiring an `action` enum
   fixed a model that narrated every step and wrote no query.
2. Two fields for one choice is one field too many: one `content` field read
   according to `action` beats separate `query` and `answer` fields.
3. Models wrap things in markdown whatever the schema says. Stripping it is
   unambiguous, because a fenced block is never valid jq.
4. Handing a model the data teaches it to paste the data. The prompt now
   describes the shape (length, field names, one sample) and says to refer to
   `.`; it is cheaper per step as well.
5. A placeholder that looks like syntax is syntax. "Collect with `[...]`" made a
   model write `[...] | select(...)`; "wrap the call in square brackets" did not.
6. `invoke_llm_batch` ran its whole pool before checking for failure, so a batch
   that failed on the first prompt still billed for the rest. Fail-fast now
   cancels the remainder. Only cost makes this visible; every behavioural test
   passed either way.
7. A failed loop is not an unknowable finding: the evidence window is seeded
   before the first turn, so a model that ran out of tokens mid-thought has said
   nothing, not that the question cannot be judged.
8. A second model needs a second endpoint variable — `PWRQ_SYSTEMONE_MODEL` is
   separate from `PWRQ_LLM_MODEL` so one shell can hold both.
9. Two models disagree, and the probability is where you see it. A threshold is
   the caller's to set; an enum hides the disagreement.
10. A probability is not bit-reproducible (llama.cpp slot batching). Thresholds
    are safe; equality on a probability is not.

## The editors: one engine, three clients

`pkg/ideengine` is the one typed engine; `pkg/webapi` (WASM, in the tab),
`pkg/webnative` (native, on the machine) and the terminal UI are configurations
of it. `graph.RenderOptions` carries the cmdlet vocabulary so the diagram can
tell a cmdlet from a jq builtin, and the legend is generated from the same palette
the renderer used.

- **The WASM page is capability-free.** It evaluates against the pure in-memory
  cmdlets in a worker thread; the server behind it is static files with no API,
  so rebuilding it can never expose anything and it is safe to deploy statically.
- **The native page runs as the user**, in its working directory. Any non-loopback
  bind is refused unless `PWRQ_IDE_TOKEN` is set. It is a separate page, flag and
  build (`ide_native`), so the WASM deployment is unaffected.
- **The terminal UI validates as you type and runs only when you ask.** Validation
  compiles against the full vocabulary; nothing runs until Ctrl-R, because a
  half-typed `new_item("f")` is already a complete query. Ctrl-X prints the query
  (or, with `--emit=output`, the output) to stdout, and the screen is drawn on the
  terminal rather than stdout, so it composes in a pipe. It ships in plain `pwrq`.
- **A run cannot hang.** Three bounds, because each catches what the others miss:
  a result limit for unbounded streams, a deadline inside the engine for a query
  that spins without emitting, and terminating the worker for anything that
  survives both — the only thing that can interrupt WebAssembly mid-instruction.

## MCP server

`pwrq --mcp` exposes `run_query`, `list_functions` and `validate_query` so an
agent can evaluate queries directly. Three things are decided by software that is
not this server and are catered for rather than assumed away: a result is read as
text (so every tool returns its whole answer as text too), the input schema
outlives the client (so nothing advertised uses a type union, and loosened
argument shapes are read as intended), and the HTTP transport's security model is
*who can reach the port* — a loopback bind needs nothing, any other bind is
refused unless `PWRQ_MCP_TOKEN` is set.

## Known remaining work

- The object cmdlets (`select_object`, `where_object`, `sort_object`) overlap
  jq's own `select`/`sort_by`/`group_by`. Correct now, but whether they earn
  their place is worth revisiting with real usage.
- `pkg/mcpserver` still has an engine of its own; moving it onto `ideengine` is
  worth doing separately.
- The native web page still runs as you type. The TUI's validate-don't-run rule
  may be right for it too.
- `invoke_systemone_batch` is not built; if a run's wall clock turns out to be
  the `map` rather than the model, `runBatch` is the pool to reuse.
- Phase-5 LLM work deliberately unbuilt: an MCP client, multi-turn sessions,
  streaming tokens, multimodal input, conversation compaction.
- The old "call every function at its minimum arity and watch for a crash" sweep
  is still the right way to find the next `deep_merge`-style panic.

## Historical bug lessons

Each of these was found by a test or an audit and is worth remembering because
the shape recurs.

- A type switch that only knew `float64` was the cause of eight separate cmdlet
  bugs, because the CLI decodes with `UseNumber()`: piped numbers are
  `json.Number`, query literals are `int`/`float64`. `common.ToFloat64`/`ToInt`
  now hold the numeric cases once.
- Binding ByPropertyName through `Name` collapsed *any* object with a `Name`
  property to a string, breaking the object cmdlets.
- `json.Number` is a `fmt.Stringer`; the encoder was stringifying cmdlet output.
- `get_childitem` pruned any directory whose own name did not match `-Filter`, so
  `-Recurse -Filter *.go` returned nothing.
- `get_service` declared JSON struct tags then hand-parsed by splitting on
  newlines, and reported 1 service on a machine with 164.
- `test_path` returned an object, so `if test_path(x)` was always true.
- `format_table` narrowed every column to its header's width, and its column
  order came from ranging a Go map, so the same query printed columns in a
  different order each run.
- `set_content` writes no trailing newline; appending to a file it produced
  spliced two values onto one line.
- `compare_object` built occurrence counts and compared them only to zero, so a
  value twice on the left against once on the right reported nothing.
