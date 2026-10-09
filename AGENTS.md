# AGENTS.md

Guidance for working in this repository. It covers the layout, the build and
test commands, and the patterns every change is expected to follow. For the
*why* behind the design, see [docs/DESIGN.md](docs/DESIGN.md); for usage, see
[README.md](README.md) and [EXAMPLES.md](EXAMPLES.md).

## What pwrq is

`pwrq` is gojq plus a library of PowerShell-style cmdlets that reach the
filesystem, the OS, the network and a pile of codecs. The one hard constraint:

> Any valid jq program produces byte-identical output.

The CLI corpus (`cli/test.yaml`) runs gojq's own 831 cases plus nine
pwrq-specific ones and enforces this. Never register a function that shares a
jq builtin's name and arity — gojq resolves builtins first, so it would silently
never run. `TestNoBuiltinShadowing` fails the build instead.

## Repository layout

| Path | What lives there |
|---|---|
| `cmd/pwrq`, `cmd/pwrq-viz`, `cmd/web` | entry points (CLI, diagram/IDE binary, WASM) |
| `cli/` | the CLI: flag parsing, encoder, `--udf-list`, the TUI entry point |
| `pkg/udf/` | the cmdlet vocabulary, one package per category |
| `pkg/udf/common/` | registration wrappers, value binding, the shape/encoding declarations |
| `pkg/udf/discovery/` | `get_command` / `get_help` over the catalogue |
| `pkg/core/typed/` | the object model (`typed.Object`, `NormalizeJSON`) |
| `pkg/core/shape/` | the `Fixed`/`Derived`/`Dynamic`/`Unspecified` shape system |
| `pkg/ideengine/` | the one engine the browser, native and terminal editors share |
| `pkg/webapi/`, `pkg/webnative/` | WASM and native front ends for that engine |
| `pkg/tui/` | the terminal UI |
| `pkg/mcpserver/` | the MCP server (its own engine today) |
| `pkg/pwrgrep/` | the rule corpus and its loader |
| `pkg/graph/`, `pkg/graph/graphsvg/` | query diagramming (behind the `viz` tag) |
| `docs/` | design record and other long-form docs |

## Build and test

```bash
make build            # pwrq
make build-viz        # pwrq-viz (diagrams + browser IDE)
make build-all
make test             # full suite + gojq corpus, both builds, browser tests
make test-short       # skips tests that touch system services
make lint             # go vet + golangci-lint for every build tag
make web.build        # the browser editor (needs bun)
make web.build-native # the native editor (needs bun)
make web.test         # the editor's browser-side tests (needs bun)
make help
```

`make test` needs `bun` for the browser half. The most useful focused runs:

```bash
go test ./pkg/udf/                       # registry guards + examples
go test ./pkg/pwrgrep/                   # every rule and fixture
go test ./pkg/tui/                       # the terminal UI model
go test ./pkg/tui -update                # rewrite golden frames
```

### Build tags

| Tag | Effect |
|---|---|
| `viz` | links the D2/SVG diagrammer and the browser IDE (`pkg/graph`, `cli/viz.go`) |
| `ide_native` | the native, server-backed editor (`pkg/webnative`, `cli/ide_native.go`) |
| `embed_web` / `embed_web_native` | bake the bundled page into the binary |
| `grammar_subset_<lang>` | embed only the named tree-sitter grammars (see `GRAMMARS`) |

`BUILD_TAGS` is `grammar_subset` plus one tag per language in the Makefile's
`GRAMMARS` list. The tag combinations are mutually constrained on purpose so a
WASM deployment can never smuggle in the native server. `make test` does not
pass the grammar tags; `make build`/`install` do. `pkg/udf/astsearch/buildtags_test.go`
asserts the release builds pass `grammar_subset` and that the tag set matches
`GRAMMARS`.

### CI

`.github/workflows/ci.yml` runs, on every PR: `go test -race ./...`, the `viz`
and `viz ide_native` variants, `go vet` for each tag set, gofmt (excluding
`testdata/`), `golangci-lint`, a cross-build of all eight Debian architectures,
`govulncheck`, the bun web tests and page builds, and a full release dry run
that builds and consumes the apt repository. A change that passes locally but
fails one of these is almost always a missing build tag or an untidy `go.mod`.

## Adding a cmdlet

1. **Implement** it under `pkg/udf/<category>/`. Register through the wrappers
   in `pkg/udf/common`, never `gojq.WithFunction` directly — the wrappers
   normalize every result into gojq's value space, and a test fails a direct
   registration.
   - `common.WithFunction` / `common.WithIterFunction` — a single value vs a
     stream. Choosing the wrapper *is* the streaming declaration.
   - `common.WithFunctionOf` / `common.WithIterFunctionOf` — an object producer,
     with its `shape.Shape`.
   - `common.DeclareInput(name, common.InputPipeline)` where `SplitInput`
     applies; `common.DeclareEncoding` / `common.DeclareConsumes` for byte-as-text
     cmdlets.
2. **Register** it in `DefaultRegistry()` (`pkg/udf/registry.go`). If it is
   browser-safe, also add it to `WebRegistry()` (`pkg/udf/registry_web.go`).
   CLI-only families — filesystem, process, service, network, sqlite, censys,
   llm, astsearch/pwrgrep, logfile, archive — stay out of the web registry.
3. **Document** it in `allFunctionMetadata()` (`pkg/udf/metadata.go`): name,
   arity range, category, description, examples. `GetFunctionMetadata()` filters
   the Censys write cmdlets by environment; leave that alone.
4. **Category order**: if you add a category, slot it into `categoryOrder` in
   `cli/udf_list.go`.
5. **Gallery**: add a worked example to `pkg/ideengine/examples.go`. It is
   re-exported by `pkg/webapi` and every entry is run by a test.
6. **Test** it: a table-driven test and a "through the registered function" test
   in the package. If it has both a piped and an explicit form, add it to
   `TestCallingFormsAgree` (`pkg/udf/binding_test.go`).

### Guards that will fail the build

- `TestCliRun` — gojq's corpus, byte-identical.
- `TestNoBuiltinShadowing` — no UDF or alias shares a builtin's signature.
- `TestUDFListMatchesRegistry` — the documented set equals the registered set.
- `TestMetadataArityMatches` — documented and registered arities agree both ways.
- `TestAliasesResolve` — every alias names something that exists.
- `TestMetadataExamplesCompile` — every published example parses and compiles.
- `TestEveryPublishedExampleRuns` — every non-exempt example runs; exemptions
  are listed with a reason in `pkg/udf/examples_test.go`.
- `TestEveryCmdletHasAnExample`, `TestEveryExemptionIsACmdlet`.
- `TestCallingFormsAgree` — the piped and explicit forms agree.
- `TestWebRegistryExcludesTheUnavailable` — the web registry's omissions are the
  documented ones.

## Adding a rule

A rule is an ordinary pwrq query in a file. Put it at
`pkg/pwrgrep/rules/<language>/<kind>/<name>.pwrq`, with its fixture at
`pkg/pwrgrep/testdata/fixtures/<language>/<file>`. Kinds are `inventory`,
`entrypoints`, `flow`, `hotspots`; the shipped languages are `go`, `python`,
`javascript`, `java`, `c`, `csharp` and `generic`.

The header block is the contract:

```
# rules: go-input-reaches-effect      # the finding ids a caller asks for
# languages: go                        # grammars the rule needs
# fixture: go/input-reaches-effect.go  # the file it is checked against
# from: <source>                        # optional provenance
```

Anything else in the header is prose and surfaces as `Description`. The body is
ordinary pwrq using `scan_ast`/`scan_regex`, `of`, `within`, `outside`,
`not_at`, `in_files_with(out)`, `where_capture*`, `focus`, `reaching`,
`finding`, `report`.

Annotate the fixture the way semgrep does: `// ruleid: <id>` marks the next line
as one that must fire, `// ok: <id>` one that must not. At least one `ok:` line
is required. The package tests enforce:

- `TestEveryRuleCompiles`;
- `TestEveryRuleLanguageIsInTheBuild` — every declared language is in `GRAMMARS`;
- `TestEveryShippedRuleHasAFixture`, `TestEveryFixtureBelongsToARule`;
- `TestEveryRuleWithAFixtureFindsExactlyWhatItMarks` — set equality between
  marked and fired lines.

Run `go test ./pkg/pwrgrep/`. `ast_pattern` shows what a pattern became when a
rule comes back empty; a pattern that is not valid code compiles to something
that matches nothing, so a typo and an honest absence look identical.

## Conventions

### Value space

gojq accepts only `nil`, `bool`, `int`, `float64`, `*big.Int`, `json.Number`,
`string`, `[]any`, `map[string]any`. It **panics** inside builtins on anything
else, not only at encoding. Register through the `common` wrappers, which
normalize each result. Use `common.ToFloat64` / `common.ToInt`, which cover every
numeric Go type — the CLI decodes with `UseNumber()`, so piped numbers are
`json.Number` while query literals are `int`/`float64`, and a type switch that
only knows `float64` is the single most common bug in this codebase.

### Errors

Return an `error`. It travels on jq's error channel, so `try`/`catch`, `//` and
the exit status behave as they do for jq. Use `common.MakeUDFErrorResult(err,
meta)`, which prefixes an `operation` name. Never return an in-band object that
merely looks like a failure — the pipeline carries on with it.

### Binding

`common.SplitInput(v, args, operands)`: a cmdlet with *n* operands registers the
arity range *n*..*n+1*. At the lower arity the input is the pipeline value and
every argument is an operand; at the upper arity — the explicit form — the
**first** argument is the input and the operands follow. Binding is strictly
positional; never inspect arguments to guess which was "meant" to be the input.
An optional trailing options object is available only in the explicit form.

```
[1,2,3,4] | chunks(2)      # input from the pipeline
chunks([1,2,3,4]; 2)       # input as the leading argument
```

`BindValue` binds a scalar or an object; `BindPath` follows PowerShell's
ByValue-then-ByPropertyName rule over `PwrqValue`/`FullName`/`Path`.

### Declare facts where you register them

Streaming, output shape, input form, output encoding and accepted input encoding
are all declared at registration and read back by `get_help`, `get_command`,
`list_functions`, the MCP observed-shape report and the encoding-mismatch
warnings. Declaring them anywhere else is how documentation drifts from
behaviour.

### Remote APIs and spending

- **Credentials** come from the vendor's documented environment variables, or
  from per-call options. They are never echoed into the pipeline.
- **Unknown option names are rejected** before the request. `pipeline.BindParameters`
  ignores undeclared names, which is fine for local flags and wrong for a
  network call; wrap it as `censys` and `llm` do.
- **Paging is opt-in.** A cursor followed to its end is unbounded billed
  requests; one page is the default.
- **Writes are opt-in at registration, not call time.** The Censys write cmdlets
  are absent unless `PWRQ_CENSYS_WRITE=1`, and withheld from the catalogue to
  match, so a wrong query fails to compile rather than reaching the API.
- **Model spending is bounded by a ceiling.** One process makes 100 calls before
  it refuses; `PWRQ_LLM_MAX_CALLS` raises or removes it. Temperature is 0 by
  default and the response cache is opt-in.
- **Prices are not compiled in.** `.Cost` is reported only when the caller
  supplies rates.

### Agents

`invoke_agent` compiles the model's sub-queries against a registry built from
the allowed cmdlets alone, so a denied cmdlet does not exist to the compiler —
the restriction is structural, not a runtime check. `defaultAllow` is read-only;
`forbiddenPrefixes` (`invoke_llm`, `invoke_agent`, `invoke_systemone`,
`get_llm_`) can never be allowed. A run is bounded by `MaxSteps`, `MaxSeconds`
and a per-query result cap, and the agent's queries get no environment loader.

### Comments and style

The house style is a comment that explains the *why* — the bug it prevents, the
invariant it holds, the measurement behind a choice. Do not restate the code.
`gofmt` is enforced in CI, and `testdata/` is deliberately excluded because some
fixtures must keep formatting `gofmt` would rewrite.

## The editors and the engine

`pkg/ideengine` is the one engine; `pkg/webapi` (WASM), `pkg/webnative`
(native) and `pkg/tui` are front ends over its seven operations: validate, run,
diagram, format, minify, inline, catalog. Host differences are `Config` fields.
When you add a capability to one editor, add it to the engine rather than
copying it into a second front end.

`pkg/mcpserver` still carries its own engine; moving it onto `ideengine` is
known remaining work, not a pattern to imitate.

## Documentation map

| Change | Update |
|---|---|
| a cmdlet | `pkg/udf/metadata.go` (examples are tested), `pkg/ideengine/examples.go`, and README/EXAMPLES if it is a headline feature |
| a rule | `pkg/pwrgrep/README.md` if the vocabulary changes |
| the registry, shapes or bindings | `pkg/udf/README.md` |
| architecture or a design decision | `docs/DESIGN.md` |
| the command surface or counts | README.md and EXAMPLES.md |

Counts in the docs are easy to get wrong. The current ones: 509 cmdlets (518
with `PWRQ_CENSYS_WRITE=1`), 692 published examples of which 590 run, 840 CLI
corpus cases, 67 rules, 34 grammars. Regenerate outputs rather than editing them
by hand, and run the guards above before committing.
