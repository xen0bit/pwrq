# pwrq: PowerShell-on-jq, on a strict gojq foundation

## Context

`pwrq` is a gojq fork that accreted three uncoordinated projects: a CyberChef-style
UDF library, a D2 query visualizer with a WASM page, and an agent-built PowerShell
port (~20k LOC). It has no users, so anything can change.

Direction:

- **Pitch:** PowerShell-on-jq. Users write PowerShell-shaped pipelines over JSON.
- **Hard constraint:** strict gojq superset — any valid jq program produces
  byte-identical output.
- **PowerShell layer:** keep all of it, fix in place.
- **Visualizer:** split into its own binary.

The first two goals were in direct conflict, and the conflict had one cause:
`cli/encoder.go` unwrapped any map carrying `_val` + `_meta` at print time. That
rewrote ordinary user JSON and discarded properties the cmdlets had already
computed. Everything below follows from removing it.

Two facts found while exploring, which shaped the sequencing:

- gojq's compiler checks custom functions **last** (`compiler.go:1003`), after
  builtins. Custom functions structurally cannot shadow jq builtins, so the
  superset property is nearly free.
- The same fact means an alias table can never override a builtin. Registering
  `select` → `select_object` would lose to jq's `select/1` *silently*, doing
  something plausible but wrong. Aliases need a collision guard, not just wiring.

## The one design decision

**A PSObject's wire format is ordinary JSON:** a flat object whose keys are its
PowerShell property names, plus a `PSTypeName` property. No `_val`, no `_meta`,
no Go structs in the output stream. This is what PowerShell's own
`ConvertTo-Json` emits, and it makes cmdlet output navigable with plain jq.

| Function class | Returns | Examples |
|---|---|---|
| Transforms | the transformed **scalar** | `base64_encode`, `sha256`, `upper`, `find` |
| Object producers | flat PascalCase object + `PSTypeName` | `get_childitem`, `get_process`, `get_date`, `http` |
| Formatters | a **string** | `format_table`, `format_list`, `out_string` |

Nothing else reaches the encoder.

---

## Phase 0 — Lock the superset guarantee ✅

- [x] Delete the `_val`/`_meta` unwrap in `cli/encoder.go`
- [x] Encoder returns an error instead of panicking on non-JSON Go values
- [x] Restore gojq's 831-case CLI corpus (the fork had kept 11) + `cli/testdata/`
- [x] Pin `TZ=Etc/GMT+7` in `TestMain` so the corpus is hermetic
- [x] Anti-regression cases for objects carrying `_val`/`_meta`
- [x] Fix `make test` — `cmd/web` is `js && wasm` only

## Phase 1 — One object model ✅

- [x] `psobject.ToJSON`/`ToMap` emit flat JSON + `PSTypeName`, evaluating
      ScriptProperty getters and resolving AliasProperties
- [x] A PSObject wrapping a map exposes that map's entries as properties
- [x] `NormalizeJSON` converts at the boundary (`time.Time`, `os.FileMode`,
      sized ints) so values are usable by jq builtins, not just printable
- [x] `_val`/`_meta`/`_err` retired from all non-test code
- [x] `MakeUDFErrorResult` returns a real `error` — failures use jq's error
      channel, so `try/catch` and exit status work
- [x] `BindValue` (value binding) split from `BindPath` (ByPropertyName path
      binding), keeping both `find(".") | cat` and `get_childitem(".") | cat`
- [x] Formatters return strings; `format_table`/`join_path` no longer crash
- [x] `http` returns an `Invoke-WebRequest`-shaped response object
- [x] ~176 test assertions migrated; fabricated tests rewired to the real UDFs

**Verified:** full suite green, 840 corpus cases.

Two bugs the tests caught, worth remembering:
- Binding ByPropertyName through `Name` collapsed *any* object with a `Name`
  property to a string, breaking the object cmdlets.
- `json.Number` is a `fmt.Stringer`; the CLI decodes with `UseNumber()`, so
  numbers in cmdlet output were being stringified.

## Phase 2 — Registry as the single source of truth ✅

- [x] `Registry.Signatures()` discovers registered name/arity pairs by asking
      gojq for `builtins` with and without the registry applied and taking the
      difference — nothing declares its own name, so it cannot drift
- [x] Aliases compiled into the query as jq `FuncDef`s, generated from the
      arities the registry reports (`pkg/udf/alias.go`)
- [x] Guards: no UDF or alias may share a builtin's signature; documented list
      must equal the registered set; documented arities must match
- [x] 20 missing metadata entries added, wrong arities corrected
- [x] `--udf-list` lists aliases, grouped by the cmdlet they name

**Note on the approach.** The plan called for the registry to carry
`Name/Arity/Fn/Description`, replacing the 119 `Register*() gojq.CompilerOption`
signatures. Discovering names from gojq instead achieves the same guarantee
without that sweep. Descriptions and examples cannot be derived from a function
pointer, so `metadata.go` remains the declaration — but it is now
machine-verified against reality rather than maintained by hand.

Found by the guards on first run:
- 20 registered functions were absent from `--udf-list`
- `split`, `join` and `trim` were registered but shadowed by jq builtins, so
  they had never executed. Removed; jq covers them.
- A function *fully* shadowed by a builtin is invisible to the set difference,
  so the guard also checks declared arities against the builtin set directly.

## Phase 3 — Make the cmdlets real

- [ ] Wire in `pkg/core/pipeline` (1,272 LOC, full test suite, zero importers)
- [ ] `where_object` script blocks use real `gojq.Parse`/`Compile`/`Run` instead
      of 751 lines hand-parsing jq expressions from strings; same for
      `sort_object`/`select_object` computed properties
- [ ] Fill out object producers with full property sets
- [ ] Put the 50s `service` test behind a short-test skip

## Phase 4 — Split the visualizer

- [ ] `pkg/graph` behind a build tag; add `cmd/pwrq-viz`
- [ ] Fix `-g` help text (advertises `.png`, which errors)
- [ ] Update `Makefile` and `Dockerfile` for two binaries
- [ ] Target ~8MB core binary, down from 43MB

## Phase 5 — Real tests and honest docs

- [ ] Replace the stub integration tests (`Run()` returns `"", "", 0`)
- [ ] Rewrite `README.md`, `EXAMPLES.md`, `pkg/udf/README.md` against actual
      behavior, generating examples by running them
- [ ] Add `get_command`/`get_help` over the registry

---

## Verification

```bash
make test                                          # full suite + gojq corpus
echo '{"_val":1,"_meta":{"a":2}}' | ./pwrq -c .    # => {"_meta":{"a":2},"_val":1}
./pwrq -c '[get_childitem(".") | select(.Length > 10000) | {Name, Length}]'
./pwrq 'gci' && ./pwrq 'gps | length'              # aliases resolve
go test ./pkg/udf/ -run TestNoBuiltinShadowing     # no name shadows a builtin
diff <(./pwrq -u | names) <(registry names)        # discovery matches reality
ls -la pwrq                                        # ~8MB target
```
