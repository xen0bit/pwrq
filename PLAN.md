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

**A typed object's wire format is ordinary JSON:** a flat object whose keys are
its property names, plus a `PwrqType` property. No `_val`, no `_meta`, no Go
structs in the output stream. This is the shape `ConvertTo-Json` emits for a
PowerShell object too, and it makes cmdlet output navigable with plain jq.

| Function class | Returns | Examples |
|---|---|---|
| Transforms | the transformed **scalar** | `base64_encode`, `sha256`, `upper`, `find` |
| Object producers | flat PascalCase object + `PwrqType` | `get_childitem`, `get_process`, `get_date`, `http` |
| Formatters | a **string** | `format_table`, `format_list` |

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

- [x] `typed.Object.ToJSON`/`ToMap` emit flat JSON + `PwrqType`
- [x] An object wrapping a map exposes that map's entries as properties
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

**Verified:** full suite green, 839 corpus cases.

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

## Phase 3 — Make the cmdlets real ✅

- [x] `where_object` script blocks are real jq — `gojq.Parse`/`Compile`/`Run`,
      compiled once per call and against the query's own UDF vocabulary. The
      PowerShell `{property, operator, value}` form is untouched
- [x] `pkg/core/pipeline`'s parameter binder wired into `get_childitem`; two
      gaps closed first (case-insensitive names, `[]any` → `[]string`)
- [x] System-touching `service` tests behind `-short`

**Note on the approach.** Only the parameter binder was adopted. The rest of
`pkg/core/pipeline` — `CmdletBase`, the channel-based context, a second table
formatter — duplicates working code and does not fit gojq's synchronous
function model. Wiring it in would have meant rewriting ~40 cmdlets for no
user-visible gain.

Found while doing it:
- `get_childitem` pruned any directory whose own name did not match `-Filter`,
  so `-Recurse -Filter *.go` returned nothing at all.

## Phase 4 — Split the visualizer ✅

- [x] `pkg/graph` and the IDE behind a `viz` build tag; `cmd/pwrq-viz` added
- [x] `-g` help text no longer advertises `.png`, which errors
- [x] Makefile and Dockerfile build both; `make test` covers both builds
- [x] **43MB → 9.5MB** for the default binary

## Phase 5 — Real tests and honest docs ✅

- [x] `test/integration` builds and runs the binary instead of asserting
      against the empty strings its stub `Run()` returned
- [x] README, EXAMPLES.md, pkg/udf/README.md rewritten; every example taken
      from a real run
- [x] `get_command`/`get_help` over the registry, searchable by alias

Found while writing the docs:
- `test_path` returned an object, so `if test_path(x)` was always true
- `get_service` reported 1 service on a machine with 164: it declared JSON
  struct tags then hand-parsed by splitting on newlines
- `invoke_web_request` reported `ContentLength: -1` for chunked responses

---

## All phases complete

| | Before | After |
|---|---|---|
| gojq corpus cases | 11 | 839, all passing |
| Default binary | 43MB | 9.5MB |
| `--udf-list` accuracy | 20 missing, arities wrong | machine-verified |
| Aliases | none worked | 30, guarded against shadowing |
| Integration tests | asserted on `""` | run the real binary |

## Verification

```bash
make test          # full suite + gojq's corpus, for both builds
make test-short    # skips tests that touch system services
make build-all

echo '{"_val":1,"_meta":{"a":2}}' | ./pwrq -c .    # {"_meta":{"a":2},"_val":1}
./pwrq -c '[get_childitem(".") | select(.Length > 10000) | {Name, Length}]'
./pwrq -c '[gci(".")] | length'                    # aliases resolve
./pwrq -r 'get_help("get_childitem")'
go test ./pkg/udf/ -run TestNoBuiltinShadowing
```

## Standing guards

These fail the build rather than relying on anyone remembering:

- `TestCliRun` — gojq's 839-case corpus, byte-identical
- `TestNoBuiltinShadowing` — no UDF or alias shares a builtin's signature
- `TestUDFListMatchesRegistry` — documented set equals registered set
- `TestMetadataArityMatches` — documented arities equal registered arities
- `TestAliasesResolve` — every alias names something that exists

## Follow-up: the query visualizer

`pkg/graph` drew only an outline of any non-trivial query. Binary operators
rendered with unlabelled children, object shorthand keys were dropped, unary
operands vanished, and array construction was absent entirely — so
`[... ] | sort_by(...)` showed a sort with nothing to sort.

Rewritten to emit D2 source directly from the AST (1,691 → 614 lines, and the
`d2oracle`/`d2format`/`d2target` dependencies are gone). Every construct that
carries meaning expands; what does not stays inline as label text.

The old tests asserted on label spellings, which is how they passed while the
diagrams were wrong. They now assert that every part of a query appears in its
diagram — which caught two more bugs: dotted paths written inside a D2
container declare that path *under* it (stray `n1`/`n2` boxes), and D2 reads
`${...}` as a substitution even inside quotes, so any query naming a jq
variable produced an uncompilable script.

## Phase 6 — Close the four loose ends ✅

### Repo hygiene

- [x] Removed the `PowerShell/` submodule: 54MB of Microsoft's C# source,
      referenced by zero Go files, distinct from `pkg/udf/powershell/` which is
      the actual port
- [x] Deleted the unused two-thirds of `pkg/core/pipeline` — `cmdlet.go`,
      `context.go`, `output.go` and their tests. `parameter.go`'s
      `BindParameters` was the only export anything imported (Phase 3's note
      predicted this; now it is true rather than noted)

### Visualizer: `label`/`break`

- [x] `TermTypeLabel` expands like `if`/`try`/`reduce` do, instead of
      collapsing the whole labelled body into one opaque text node
- [x] `TermTypeBreak` needs no case: it carries no sub-query, so the leaf
      fallback already renders `break $found` correctly
- [x] The kitchen-sink fixture already exercised both; the test now asserts on
      them

### Integration tests, and the seven bugs they found

Real-binary tests for the object cmdlets, formatters, session variables, the
location stack, and the web cmdlets (against an `httptest.Server`, so hermetic
— no `-short` guard needed). `get_service` gets a `-short`-skipped smoke test.

Everything below was found by writing them, and every one is the same shape:
**a type switch that only knew `float64`.**

- `measure_object` reported `Sum: 0`, `Average: 0`, `Minimum: null` for any
  data piped in. The CLI decodes with `UseNumber()`, so property values are
  `json.Number`, and `convertToFloat64` skipped them as non-numeric.
- `sort_object` compared numbers as text for the same reason, so `9` sorted
  after `100`.
- `select_object`'s `first`/`last`/`skip` were never read: gojq represents an
  integral literal as `int`, not `float64`, so `{first: 2}` did nothing.
- `sort_object`'s `{property: "Age", descending: true}` silently sorted
  ascending — only the `"Age desc"` suffix form was parsed.
- `format_table` narrowed every column back to its header's width, so any
  value longer than its header overflowed and pushed every later column out of
  line. A two-column table did not line up.
- `set_variable("x"; {a: 1})` could only ever fail: an object second argument
  was read as an options map, so there was no way to store an object at all.
- `get_variable` and `invoke_web_request` returned objects with no
  `PwrqType`, the one property the object model says every producer carries.

`common.ToFloat64`/`ToInt` now hold the numeric cases once, so the next cmdlet
to need them cannot miss `json.Number` again.

### Web IDE: evaluate against sample input

- [x] `runQuery(query, inputJSON)` exported from `cmd/web`, compiling with the
      registry's options and aliases exactly as `cli/cli.go` does, decoding
      input with `UseNumber()`, and capping output at 10,000 values — a browser
      tab has no Ctrl-C, so `repeat(1)` would otherwise hang the page
- [x] `udf.WebRegistry()`: codecs, hashes, ciphers, compression, format
      conversion, and the object and formatting cmdlets. Everything that needs
      a filesystem, process table, service manager or shell is excluded,
      because in a browser it can only fail
- [x] `Registry.KnownAliases` filters the alias table to what a curated
      registry can resolve. `AliasFuncDefs` still errors on an unknown target —
      for the CLI that is a bug worth failing on
- [x] Input textarea and a results panel in `pkg/web/src`, sharing the existing
      300ms debounce. Results render as text, not markup
- [x] The whole UDF library now builds for `js/wasm`. Two files stood in the
      way: `set_date_unix.go` claimed `!windows` while using
      `syscall.Settimeofday`, and `http_serve` used `SO_REUSEADDR` inline.
      Both now have a portable fallback

### Web IDE: a real editor

The page could run a query and draw it. It could not do anything with the
result: no sharing, no vocabulary, no way to stop a runaway query, and a
diagram in one colour.

- [x] `pkg/webapi`: the whole engine as JSON-in, JSON-out functions - validate,
      run, diagram, format, catalog. `cmd/web` is now 50 lines of `syscall/js`
      glue, and the page's behaviour is covered by ordinary Go tests rather
      than by clicking around in a browser
- [x] Runs cannot hang the tab. Three bounds, because each catches what the
      others miss: a result limit for unbounded streams, a deadline inside the
      engine for a query that spins without emitting, and terminating the
      worker for anything that survives both. The deadline reads the clock in
      `Done()` rather than waiting on a timer - under `GOOS=js` there is one
      thread and no preemption, so a timer set by `context.WithTimeout` never
      fires while a query is running
- [x] The engine moved off the main thread. A worker keeps the editor
      responsive whatever the engine is doing, and terminating it is the only
      thing that can interrupt WebAssembly mid-instruction
- [x] Coloured diagrams. `graph.RenderOptions` carries the cmdlet vocabulary,
      so the renderer can tell a cmdlet from a jq builtin - which the diagram
      previously could not show at all - plus a theme, a layout engine, a
      direction and sketch mode. The legend in the page is generated from the
      same palette the SVG was drawn with
- [x] Sharing by URL fragment: query, input and arguments, deflate-compressed.
      A fragment is never sent to a server, so a link needs no backend and
      leaks nothing to one
- [x] The editor: syntax highlighting for pwrq's own vocabulary, completion
      driven by the registry, the parse error underlined where gojq says it is,
      bracket matching, comment toggling, a command palette, history, saved
      snippets, and jq's flags (`-c`, `-r`, `-s`, `-n`) plus `--argjson`-style
      arguments as controls
- [x] The gallery lives in Go (`webapi.Examples`), so every example is run and
      drawn by a test. An example that rots into a broken query fails the build
- [x] The server pre-compresses the 27MB module and serves the 7MB copy to any
      browser that will take it

## Utilitarian cmdlets — plan

A round of pure, single-purpose cmdlets for common data work, landed in five
reviewable phases. Everything is a pure transform, so each cmdlet is registered
in both `DefaultRegistry` (CLI) and `WebRegistry` (browser) and is
automatically flagged available in the IDE catalog. One exception: the
file-log helpers at the end are CLI-only, like `cat`/`find`.

Conventions, same as every package in `pkg/udf/`:
- One `Register*()` per cmdlet returning `gojq.CompilerOption`, a
  `RegisterAll()` per package, input via `common.BindValue`/`common.ToFloat64`,
  results via `common.MakeUDFSuccessResult`/`MakeUDFErrorResult`.
- `metadata.go` entry for every cmdlet — pinned by `TestUDFListMatchesRegistry`
  and `TestMetadataArityMatches` — plus a table-driven test and a
  "through the registered function" test per package.
- Names are checked against jq builtins (no shadowing) and against existing
  cmdlets. None of the names below collide.

No new dependencies: YAML uses the already-direct `itchyny/go-yaml`
(`Unmarshal`/`Marshal` produce `map[string]any`/`[]any`), bcrypt and blake2 use
the already-present `golang.org/x/crypto`, checksums/IP/radix are stdlib, and
UUIDs/random use `crypto/rand`, which works under `GOOS=js`.

### Phase 1 — Data-wrangling base

- `string` package extended (category **String**): `slugify`, `snake_case`,
  `kebab_case`, `camel_case`, `pascal_case`, `title_case`, `truncate(s; n; [suffix])`,
  `pad_left(s; n; [pad])`, `pad_right(s; n; [pad])`, `mask(s; [visible])`,
  `count_occurrences(s; sub)`
- `number` package (category **Numbers**): `to_base(n; base)`,
  `from_base(s; base)`, `to_hex_number`, `from_hex_number`, `clamp(n; lo; hi)`,
  `gcd`, `lcm`, `round_to(n; places)`, `human_bytes` (binary MiB units),
  `percentage(part; whole)`
- `path` package (category **Paths**): `basename`, `dirname`,
  `file_extension`, `is_absolute`

### Phase 2 — Numeric and temporal analysis

- `stats` package (category **Statistics**): `mean`, `median`, `mode`,
  `variance`, `stdev`, `percentile(arr; p)`, `summary(arr)`. `variance`/`stdev`
  are sample statistics (n-1)
- `duration` package (category **Duration**): `human_duration(seconds)`,
  `parse_duration("2h30m")`, `time_ago(ts)`, `weekday(date)`, `is_weekend(date)`
- `random` package (category **Random**): `random_int`, `random_float`,
  `random_string(n; [alphabet])`, `random_choice(arr)`, `shuffle(arr)`,
  `sample(arr; n)`

### Phase 3 — Analyst and forensics core

- `net` package (category **IP & Network**): `is_ip`, `is_ipv4`, `is_ipv6`,
  `ip_to_int`, `int_to_ip`, `in_cidr(ip; cidr)`, `cidr_size(cidr)`, `is_mac`,
  `mac_normalize`
- `token` package (category **IDs & Tokens**): `uuid4`, `is_uuid`,
  `uuid_version`, `jwt_decode` → `{header, payload, signature}`, `is_jwt`,
  `base64url_encode`, `base64url_decode`, `rot13`, `rot(s; n)`
- `validate` package (category **Validation**): `is_email`, `is_url`,
  `is_domain`, `is_json`, `extract_emails`, `extract_urls`, `extract_ips`,
  `strip_tags`

### Phase 4 — Clustering, config and integrity

- `similarity` package (category **Similarity**): `levenshtein(a; b)`,
  `hamming_distance(a; b)`, `jaccard(a; b)`, `deep_diff(a; b)` →
  `{added, removed, changed}` summary
- `yaml` package (category **YAML**): `yaml_parse`, `yaml_stringify`
- `checksum` package (category **Checksum**): `crc32`, `crc32c`, `crc64`,
  `fnv1a`, `adler32`, `blake2b_256`, `blake2b_512`, `bcrypt_hash(s; [cost])`,
  `bcrypt_verify(s; hash)`

### Phase 5 — CLI-only file logs

- `logfile` package (category **File Operations**): `head(path; [n])`,
  `tail(path; [n])`, `grep_lines(path; pattern)`, `wc_lines(path)`.
  Registered in `DefaultRegistry` only; added to
  `TestWebRegistryExcludesTheUnavailable` so they are documented but flagged
  unavailable in the browser.

### Wiring and docs

- Both registries get one `RegisterAll()` line per new package; `metadata.go`
  gains the entries; `cli/udf_list.go` `categoryOrder` slots the new
  categories into the CLI listing; the gallery (`webapi/examples.go`) gains a
  few representative examples per category, so `TestExamplesAllRun` and
  `TestExamplesDrawToo` cover them.

## More utilitarian cmdlets — plan

A second round of pure, browser-safe cmdlets to close the gaps the first round
left open. Same conventions and guards; nothing here touches a filesystem or
the network, so every cmdlet is registered in both registries and available in
the browser. No new dependencies: SHA-3/Keccak come from the Go 1.24+ stdlib
`crypto/sha3`, Argon2 and PBKDF2 from the already-present `x/crypto`, and
everything else is stdlib.

### Text predicates and inspection (extend `string`)

`is_blank` (empty or whitespace), `is_alphanumeric`, `is_alphabetic`,
`is_numeric_string`, `is_uppercase`, `is_lowercase`, `is_ascii`,
`word_count`, `normalize_whitespace`, `acronym`
("International Business Machines" → "IBM")

### Patterns and globs (extend `string`)

`escape_regex` (regex.QuoteMeta), `glob_to_regex` ("*.txt" → anchored regex),
`match_glob(s; glob)`, `is_regex_valid(pattern)`

### Collections (new package `collection`)

`chunks(arr; n)`, `dedupe` (preserve-order unique), `deep_merge(a; b)`,
`sort_keys` (recursive), `compact` (drop null/empty/false),
`prune` (recursively remove empties), `flatten_keys` / `unflatten_keys`
(dot-path keys), `zip_arrays(a; b)`

### JSON pointer and query strings (extend `json`)

`json_pointer(obj; "/a/b/0")` (RFC 6901, with `~0`/`~1` escapes),
`json_pointer_set(obj; pointer; value)`, `query_string_parse("a=1&b=two")`,
`query_string_build(obj)`

### Time and date (extend `duration`)

`duration_between(a; b)`, `add_seconds(ts; n)`, `add_days(ts; n)`,
`start_of_day(ts)`, `end_of_day(ts)`, `is_leap_year(y)`,
`days_in_month(y; m)`, `month_name(m)`

### IP and network (extend `net`)

`ip_version(s)` ("v4"/"v6"), `is_private_ip(s)`, `is_loopback(s)`,
`cidr_network(cidr)`, `cidr_broadcast(cidr)`, `ip_add(ip; n)`,
`ipv6_expand(ip)`, `reverse_ip(ip)` (PTR name)

### Hashes, key derivation and sniffing (extend `checksum`)

`sha3_256`, `sha3_512`, `keccak_256`, `crc16` (CRC-16/CCITT),
`pbkdf2_sha256(password; salt; [iterations]; [keyLen])`,
`argon2id_hash(password; salt; [time]; [memory]; [keyLen])`,
`random_hex(n)`

### IDs and tokens (extend `token`)

`uuid7` (time-ordered), `nanoid(n)`, `is_base64(s)` (incl. `is_base64url`),
`base58_encode` / `base58_decode`

### Numbers (extend `number`)

`factorial(n)`, `is_prime(n)`, `fibonacci(n)`, `combinations_count(n; k)`,
`permutations_count(n; k)`, `ordinal(n)` ("1st", "2nd"), `lerp(a; b; t)`,
`human_number(n)` (1.2k, 3.4M), `is_even(n)`, `is_odd(n)`

### Forensics sniffing (new package `sniff`)

`file_type(data)` — magic-byte detection (PNG, JPEG, GIF, PDF, ZIP, gzip, ELF,
PE, ...), `is_binary(data)`, `is_utf8(data)` — all pure string/byte checks

That is ~60 new cmdlets across ten categories, each with metadata, table-driven
tests, a through-the-registered-function test, and a few gallery examples, all
pinned by the same registry guards as the first round.

**Status: implemented.** ~70 new cmdlets landed across the ten categories above
(text predicates and patterns, collections, JSON pointers and query strings,
time and date, IP extras, SHA-3/Keccak/CRC-16/PBKDF2/Argon2/random hex, UUID v7
and nanoid, number theory and formatting, and file sniffing), all registered in
both the CLI and the browser.

## Fourth round — group, convert and explain

A ~130-cmdlet round on branch `fourth-round-domain` covering the areas the
first three rounds left open: PowerShell-style grouping over arrays, unit/geo/
finance conversions, time-series statistics, regex and text tools, version and
path helpers, set operations, and number theory. Everything is a pure transform
except `expand_home` and `home_dir`, which need the user database and are
CLI-only (they were split out of `path` into `path.RegisterWeb()` for the
browser).

New packages:
- `aggregate` (category **Collections**): `group_by_key`, `count_by`,
  `sum_by`/`avg_by` (key, [column]), `index_by`, `value_counts`,
  `summarize_by`, `pivot`, `unpivot`, `top_by`, `bottom_by`, `distinct_count`
- `domain` (category **Domain**, new): 23 unit converters incl. `parse_size`,
  7 geo cmdlets (`haversine_distance`, `bearing`, `geo_midpoint`,
  `within_radius`, `parse_coords`, `geohash_encode`/`decode`), 8 finance
  cmdlets (`cagr`, `future_value`, `present_value`, `monthly_payment`,
  `compound_interest`, `simple_interest`, `rule_of_72`, `annual_yield`)

Extended:
- `stats`: `cumsum`, `cumulative_max`/`min`, `deltas`, `lag`, `fill_forward`,
  `ema`, `moving_max`/`min`, `correlation`, `covariance`, `skewness`,
  `kurtosis`, `weighted_mean`, `harmonic_mean`, `quartiles`, `trimmed_mean`,
  `standardize`, `rms`, `product`, `midrange`
- `string`: `regex_find_all`, `regex_extract_first`, `regex_replace_first`,
  `regex_split`, `regex_count`, `is_palindrome`, `reverse_words`,
  `truncate_words`, `remove_accents`, `sentence_case`, `line_count`, `dedent`,
  `swap_case`, `char_frequencies`, `anagram`, `first_line`, `last_line`,
  `reverse_lines`, `unique_lines`, `sort_lines`, `strip_quotes`, `pad_center`
- `path`: `normalize_path`, `relative_path`, `stem`, `with_extension`,
  `has_extension`, `is_dir_path`, `path_sep`, `expand_home` (CLI), `home_dir`
  (CLI)
- `validate`: `semver_compare`, `semver_parts`, `is_hex`, `is_cidr`,
  `is_port`, `is_date`, `is_iso8601`, `is_slug`, `extract_dates`
- `collection`: `intersection`, `union`, `difference`, `symmetric_difference`,
  `all_equal`, `contains_duplicates`, `take`, `drop`, `cartesian`, `column`,
  `lookup`, `natural_sort`
- `number`: `sign`, `is_perfect_square`, `is_coprime`, `next_prime`,
  `prime_factors`, `proper_divisors`, `is_perfect_number`, `euler_totient`

Wiring: one `RegisterAll()` line per package in both registries (`path` uses
`RegisterWeb()` for the browser), ~130 `metadata.go` entries, the `Domain`
category added to `cli/udf_list.go`, 28 runnable/drawable gallery examples, and
a new EXAMPLES.md section. All registry guards unchanged and passing; `make
test` green.

**Intentionally loose.** This round was built "go all out" for later pruning.
Likely candidates for merging or removal: the per-unit converters
(`km_to_mi`/`mi_to_km` and friends could collapse into one table-driven
converter), `top_by`/`bottom_by` overlap `sort_by | .[0:n]`, `value_counts`
overlaps `group_by`, and `take`/`drop` are thin wrappers over slices. The
gallery and `--udf-list` make the full surface easy to review.

### Fourth round, part two — config formats and loose ends

A second ~37-cmdlet pass on the same branch filling the gaps the first pass
left open:

- `config` package (category **Config**, new): `ini_parse`/`ini_stringify`,
  `properties_parse`/`properties_stringify` (.env / Java properties, with
  escapes and line continuations), `logfmt_parse`/`logfmt_stringify` (numbers
  and booleans typed). All pure, so they appear in the browser too.
- `string`: `before_first`, `after_first`, `surround`, `soundex`, `is_isogram`,
  `count_vowels`, `count_consonants`, `capitalize_first`, `regex_groups`,
  `unicode_escape`/`unicode_unescape`, `diff_lines`
- `collection`: `rename_keys`, `invert_object`, `pluck` (map(.key) with a
  string key and dot paths)
- `stats`: `autocorrelation`, `iqr`, `mad`, `spread`, `moving_stdev`
- `duration`: `days_between`, `day_of_year`, `week_of_year`, `start_of_week`,
  `add_months`, `add_years`, `age_in_years` (optional "now" arg for
  deterministic tests)
- `number`: `to_fixed`, `is_power_of_two`

Wiring and guards identical to the rest of the round: both registries,
~37 metadata entries, the `Config` category added, 14 more gallery examples,
EXAMPLES.md extended. `make test` green.

### Fourth round, part three — windows, path writes, words, money

A final ~24-cmdlet pass rounding out the language:

- `string`: `quoted_printable_encode`/`decode` (MIME email encoding, byte-aware
  for UTF-8), `prefix_lines`, `first_lines`, `last_lines`, `is_balanced`
  (bracket matching), `regex_last_match`
- `csv`: `tsv_parse`/`tsv_stringify` (tab-separated, quoted fields honoured)
- `collection`: `windows` (rolling n-windows), `pairs` (adjacent pairs),
  `is_subset`
- `json`: `set_path`/`has_path`/`del_path`, the dot-and-bracket complements to
  `get_path`
- `number`: `to_words` (hyphenated English), `roman_numeral` (1-3999),
  `group_digits` (thousands separators), `format_currency`,
  `collatz_steps`
- `validate`: `is_numeric` (any number syntax, unlike `is_numeric_string`)
- `stats`: `percentile_rank`
- `domain`: `net_present_value`
- `duration`: `iso_duration` (ISO 8601 durations like `P1DT2H3M4S`)

Same wiring: both registries, ~24 metadata entries, 15 more gallery examples,
EXAMPLES.md extended. `make test` green.

## Known remaining work

- Object cmdlets (`select_object`, `where_object`, `sort_object`) still
  overlap jq's own `select`/`sort_by`/`group_by`. They are correct now, but
  whether they earn their place is worth revisiting with real usage.
- `EXAMPLES.md` outputs are verified by hand today; generating them in CI
  would stop them drifting again.
- The bug pattern above suggests an audit: any remaining `case float64:` in a
  cmdlet's argument parsing is a silent failure waiting for input that came
  from stdin.
