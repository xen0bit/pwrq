# Cleanup plan: the four cmdlet rounds

Branch: `cleanup-cmdlet-catalog`, cut from `fourth-round-domain`.

Commits `4f893d7`, `6993c6a` and `1fcf476` grew the catalog from 108 to 488
functions (+380) across ~16k lines, and `b319c2a` grew the IDE gallery to 259
examples. The implementations are correct — finance, geodesy and statistics all
check out against known values — so this is not a correctness rescue. It is a
scope, shape and duplication cleanup, plus three genuine bugs.

## Governing rules

Two tests, applied in this order:

1. **Cut test.** A function goes if it reproduces a jq builtin, or if it is a
   one-off novelty with no place in a general-purpose library.
2. **Keep test.** A function stays if a mainstream standard library covers it —
   Python's `statistics`, `math`, `datetime`, `zipfile`, `tarfile`, Ruby's
   equivalents. This is the permissive reading, and it deliberately protects the
   statistics category and the core of number theory.

Note the interaction: broad stdlib parity keeps far more than a stricter "must
earn its place over plain jq" rule would. `mean`, `median`, `mode`, `stdev`,
`variance`, `quartiles`, `correlation`, `covariance`, `harmonic_mean`, `geomean`
are all `statistics`-module members; `gcd`, `lcm`, `factorial`,
`combinations_count`, `permutations_count` are all in `math`. They stay.

Expected landing point is **~440 functions**, not the ~320 a stricter rule would
give.

## Phase 1 — Bugs

These are real defects, they exist on `main`, and they should land first as
their own commit so they are cherry-pickable.

### 1a. `zip_arrays` and `interleave` reverse their operands

```
[1,2,3] | zip_arrays(["a","b","c"])   →  [[1,"a"],[2,"b"],[3,"c"]]   correct
zip_arrays([1,2,3]; ["a","b","c"])    →  [["a",1],["b",2],["c",3]]   wrong
```

Root cause is the `arrInput(v, args[1:])` idiom (`pkg/udf/collection/collection.go:54`):
`args[0]` is the *other* operand and the trailing argument is the *input*, so
the explicit form reads backwards. `cartesian` gets this right, in the same
package, which is what makes it a bug rather than a convention.

Fix: for the two-operand functions, the explicit form must be
`f(input; other)`. Add tests covering *both* calling forms — their absence is
why this shipped.

### 1b. `arrInput` does not enforce argument order

`arrInput` returns whichever argument happens to be an array, so
`count_by(arr; "key")` and `count_by("key"; arr)` both succeed. That is
do-what-I-mean guessing: the signature is effectively undefined, and a genuine
user mistake is silently accepted. Bind positionally and error otherwise.

While here: the collection package is inconsistent about whether the explicit
form is accepted at all. `lookup(arr; key)` rejects it; `count_by` accepts it
either way round. Pick one rule and apply it package-wide.

### 1c. `sort_keys` is a no-op

gojq has no object key order and the encoder already emits sorted keys, so
`{b:1,a:2} | sort_keys` and `{b:1,a:2} | .` are indistinguishable. Delete it.

## Phase 2 — Deletions

Roughly 57 functions. Each removal drops the implementation, its tests, its
`metadata.go` entry, its `registry.go` wiring and its gallery example.

### 2a. Reproductions of jq builtins (19)

Verified byte-identical in every case below.

| Cut | jq already does it |
|---|---|
| `upper`, `lower` | `ascii_upcase`, `ascii_downcase` |
| `reverse_string` | `explode \| reverse \| implode` |
| `regex_split` | `splits` |
| `regex_find_all` | `scan` |
| `regex_count` | `[scan(re)] \| length` |
| `regex_extract_first`, `regex_groups`, `regex_last_match` | `match`, `capture` |
| `regex_replace_first` | `sub` |
| `take`, `drop` | `.[:n]`, `.[n:]` |
| `compact` | `map(select(. != null))` |
| `is_subset` | `contains` |
| `distinct_count` | `unique \| length` |
| `pluck` | `map(.key)` |
| `pairs` | `to_entries` |
| `invert_object` | `with_entries({key: .value, value: .key})` |
| `sort_keys` | no-op (phase 1c) |

The README claims pwrq "never shadows jq". True by *name* — but these shadow
jq by *function*, which is the part a user actually experiences. Keeping the
whole `regex_*` family in particular teaches newcomers a private dialect
instead of jq's own regex builtins.

**Keep** `group_by_key` despite the name: it returns an object keyed by value,
where jq's `group_by` returns an array of arrays. Different shape, real value.
Also keep `escape_regex` and `is_regex_valid` — no jq equivalent.

### 2b. Novelty number theory (8)

Cut `collatz_steps`, `euler_totient`, `is_perfect_number`, `is_coprime`,
`proper_divisors`, `roman_numeral`, `to_words`, `is_perfect_square`.

Keep `gcd`, `lcm`, `factorial`, `combinations_count`, `permutations_count`
(all `math`), `hamming_weight` (`int.bit_count`), plus `is_prime`,
`next_prime`, `prime_factors` and `digit_sum` as genuinely reached-for.

### 2c. Word games (4)

Cut `anagram`, `is_isogram`, `is_palindrome`, `pluralize` from a String
category that currently holds 75 functions.

Move `soundex` into Similarity, where it belongs alongside `jaro_winkler` and
`levenshtein` as a fuzzy-matching tool rather than a party trick.

### 2d. Collapse the unit converters (21 → 1)

`c_to_f`, `f_to_c`, `c_to_k`, `k_to_c`, `f_to_k`, `k_to_f`, `km_to_mi`,
`mi_to_km`, `kg_to_lb`, `lb_to_kg`, `cm_to_in`, `in_to_cm`, `ft_to_m`,
`m_to_ft`, `g_to_oz`, `oz_to_g`, `gal_to_l`, `l_to_gal`, `kph_to_mph`,
`mph_to_kph`, `l100km_to_mpg`, `mpg_to_l100km` become:

```
convert_unit(value; from; to)      # 20 | convert_unit("C"; "F")  → 68
```

A table-driven implementation covers every pair, including ones the 21 hand-
written functions miss, and stops the category growing by two functions every
time a unit is added.

### 2e. Trim the margins (4)

Cut `midrange` and `spread` from Statistics (both trivial `max`/`min`
arithmetic), and `first_line`/`last_line` from String (`split("\n")[0]`).
Keep `first_lines`/`last_lines` — head/tail over text is worth a name.

### 2f. Finance — a flagged judgment call

Cut `rule_of_72` (a mental-arithmetic shortcut, not a library function) and
`annual_yield`.

**Keeping** `compound_interest`, `simple_interest`, `present_value`,
`future_value`, `net_present_value`, `monthly_payment` and `cagr`. Strictly,
no mainstream stdlib ships these — they are spreadsheet functions (`FV`, `PV`,
`NPV`, `PMT`), and under a literal reading of the keep test they would all go.
I am reading "broad stdlib parity" as the permissive option it was presented as,
and these seven form a coherent, correct, genuinely useful module. **Say the
word and they come out too** — it is a one-line change to this plan.

## Phase 3 — Reorganize

The filenames are the clearest fingerprint of batch-generated work: they record
*when* code was written, not *what* it does.

```
collection_round4b.go  collection_round4c.go  collection_sets.go  collection_ext.go
string_round4b.go      string_round4c.go      string_text.go      string_regex.go
number_round4b.go      number_round4c.go      number_theory.go    number_tools.go
duration_round4b.go    duration_round4c.go
stats_round4b.go       stats_round4c.go       stats_rel.go        stats_series.go
validate_ext2.go       validate_round4c.go    path_ext.go         json_path_ext.go
```

Rename to topic — `collection_sets.go`, `collection_reshape.go`,
`string_case.go`, `string_lines.go`, `stats_series.go`, `stats_descriptive.go` —
and merge the `_ext`/`_ext2`/`_tools` splinters into them. Pure moves, no
behaviour change, so this lands as its own reviewable commit.

## Phase 4 — Close the gaps

All three confirmed absent from the catalog.

**Archives.** No zip or tar support at all; compression is stream-only
(gzip/zlib/deflate), so there is no way to handle an actual archive file.
Add `compress_archive` / `expand_archive` (PowerShell's own verbs, backed by
`archive/zip` and `archive/tar`), plus a `read_archive` that lists entries as
objects so the result flows into the existing pipeline.

**Time zones and date formatting.** `get_date` returns fixed fields and jq's
`todate` is UTC-only; there is no way to convert zones or format a date. Add
`to_timezone(tz)`, `format_date(layout)` and `parse_date(layout)`, taking
IANA names via `time.LoadLocation`.

**PowerShell verbs.** Add `select_string` (grep with context across a tree —
`grep_lines` only does one file, no context lines) and `compare_object`
(the diffing verb; `deep_diff` covers objects but not the two-list case).

Also worth adding while in the area: `add_content` (append; `set_content`
truncates) and `out_file`.

## Phase 5 — Rewrite the examples

The gallery is 259 entries, and **175 of them (67%) are one or two pipeline
stages** — a single function applied to a literal. It is organized by function
category, mirroring the catalog, so it reads as a reference index rather than a
tour of what the tool is for. 71 entries teach plain jq rather than pwrq.

Per the decision, **coverage stays roughly 1:1** but every example becomes
non-trivial. The rule for a rewritten example:

> It shows the function doing the job someone would actually reach for it to do,
> inside a pipeline with a beginning and an end.

Concretely, this:

```
{f: (100 | c_to_f), c: (212 | f_to_c), k: (0 | c_to_k)}
```

becomes something that reads like work — a batch of sensor readings normalized
to one unit, filtered to the ones out of range, and formatted as a table.

The browser sandbox blocks the filesystem, the process table and the network,
so inputs must be literals. That forces *literal input*, not triviality: a
pasted log excerpt, CSV blob or API response body all support a realistic
multi-step flow. Several examples can share one such fixture, which also lets
neighbouring examples build on each other instead of each standing alone.

Sequencing:

1. Delete the examples for removed functions (phase 2).
2. Build ~10 shared fixtures — an nginx log, a CSV export, a Kubernetes-ish API
   response, a config file, a series of metrics.
3. Rewrite category by category, largest first: String (18), Collections (18),
   Encoding (12), Statistics (11), Numbers (11).
4. Reconsider the 71 plain-jq entries. They have real value for newcomers, but
   they should be a clearly marked "jq itself" section rather than sitting
   alongside the cmdlet examples as though they were the same thing.

`TestExamplesAllRun` and `TestExamplesDrawToo` keep this honest throughout.

## Phase 6 — Docs

- Regenerate `--udf-list`; `TestUDFListMatchesRegistry` enforces it.
- Update `EXAMPLES.md` and `README.md` for the removals and the new cmdlets.
- Soften the README's "never shadow jq" claim to what is actually true after
  phase 2: pwrq does not shadow jq by name *or* by function.
- Document the binding convention fixed in phase 1b in `pkg/udf/README.md` —
  it is currently unwritten, which is how the two forms drifted apart.

## Order of work

Phases 1 → 2 → 3 → 6 are mechanical and fast. Phase 4 is new code. Phase 5 is
the long pole by a wide margin: ~230 examples to rewrite by hand, and it is
worth landing incrementally, one category per commit, rather than as one
reviewable-by-nobody diff.
