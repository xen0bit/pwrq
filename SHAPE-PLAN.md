# Output shapes

## Context

pwrq faces an LLM in two directions. `--mcp` exposes the vocabulary as tools so
an agent can write queries; the LLM cmdlets put a model inside the pipeline.
This plan is about the first direction, and about one complaint: a model using
the MCP server cannot work out what a query will *return*, so it writes the
follow-up stage against a shape it guessed.

The information exists. It is just not anywhere the model can reach.

## What is actually there today

`PSTypeName` looks like a type system. It is not one — it is a decoration
applied at construction, through three unrelated idioms:

```go
psobject.PSTypeNameKey: "System.IO.FileInfo"                      // map literal
psobj.TypeName = "System.IO.FileInfo"                             // field assignment
psobject.NewPSObjectWithTypeName(m, "…GenericMeasureInfo")        // constructor
```

There is no registry, so the type space cannot be enumerated: grepping the first
two idioms misses `GenericMeasureInfo` entirely. Running every documented
example and classifying the result gives the real picture:

| | count |
|---|---|
| examples that ran | 351 |
| of those, producing an object | 43 |
| objects carrying a `PSTypeName` | 9 |
| objects with no type at all | 34 |
| distinct type names observed | 8 |

`get_process` and `get_service` — the README's own headline examples — emit
untyped objects. So a catalogue built on `PSTypeName` would be silently partial
in exactly the places a model most needs it, and nothing would ever say so.

The string points at nothing. There is no table to look it up in. **That** is
the defect, not the idea of naming a type.

## The three-way split

The 34 untyped producers are not one group. Reading their keys splits them
cleanly, and the split is the whole design:

**Fixed.** The cmdlet decides the keys. `get_process` is always
`{CPU, Handles, Id, Name, …}`; `summary` is always
`{count, max, mean, median, min, stdev}`. These want a declared field list, and
they are the ones that simply never got a type.

**Derived.** The *input* decides the keys. `flatten_keys` returns
`{"a.b": …}` because the input had a nested `a.b`; `index_by`, `group_by_key`,
`prune` and `rename_keys` are the same. Declaring a static field list for these
would be a lie. What they can honestly declare is the *rule*: "an object whose
keys are the input's leaf paths".

**Dynamic.** An external source decides the keys — SQLite columns, CSV headers,
HTTP response headers. Not knowable until the query runs.

A shape system that only understands the first kind would either omit the other
two or misdescribe them. Both are worse than saying nothing, because a model
trusts a catalogue.

## The design decisions

**1. The declaration moves to registration, because that is the chokepoint that
already works.** `pkg/udf/common/register.go` says it plainly:

> So it is not written down anywhere. Choosing `WithIterFunction` over
> `WithFunction` *is* the declaration ... which means the documentation cannot
> disagree with the behaviour.

`Streaming` is correct for all 487 cmdlets because its declaration site is the
code path. `PSTypeName` is wrong for 34 of 43 because its declaration sites are
58 string literals scattered across the tree. So shapes are declared where
cmdlets are registered, recorded in the same table `recordEmission` writes, and
read back through `ShapeOf` beside `IsStreaming`.

**2. Declared and emitted are reconciled at construction, not by a hand-written
test.** A shape is also the constructor: `FileInfo.Build(...)` stamps
`PSTypeName` and is the only way a `System.IO.FileInfo` is made. A key that is
not declared, or a declared key that is missing, is a *discrepancy* — recorded
in a package-level table, never an error to the caller. A user's query is never
broken by a documentation bug. A test then asserts the table is empty after the
suite has run, so the reconciliation happens against real cmdlet output rather
than against a second hand-written list.

This is `recordEmission`'s trick applied to fields instead of cardinality.

**3. `PSTypeName` stays on the wire, demoted.** It is load-bearing:
`format_table` reads it, and `PreserveTypeName` carries provenance through a
projection. But it stops being *the* type system and becomes a foreign key into
a catalogue that now exists. That is exactly what makes it useful to a model —
it sees the name once and looks the fields up, instead of inferring PascalCase
keys from 143 samples.

**4. Nothing is declared for the ~300 scalar transforms.** `sha256` returns a
hex string; the description says so. Inventing a field list for a cmdlet that
has no fields is the drift risk this plan exists to remove, and there are 300
opportunities to get it wrong. They record `Unspecified`, and the runtime arm
below covers them.

**5. The runtime arm is structural, not a nicety.** Because Derived and Dynamic
shapes exist, a declared catalogue can never be complete — so `run_query`
reports the shape it *observed* in the values it just produced. This is free,
cannot drift, and covers precisely the cases declaration cannot reach. It also
pays off hardest where the model is blindest: a truncated run, or a first probe
query.

**6. The input side gets the same treatment, from a fact already encoded.**
`common.SplitInput(v, args, operands)` decides whether the input arrives from
the pipeline or as the leading argument:

```
[1,2,3,4] | chunks(2)      // pipeline
chunks([1,2,3,4]; 2)       // leading argument
```

That `operands` integer is an input-position declaration sitting inside 28
function bodies where no catalogue can see it. Hoisting it to registration makes
the arity range self-explaining: `get_childitem/1-2` currently tells a model
nothing about which argument is the input.

## The vocabulary

`pkg/core/shape`:

| Kind | Means | Rendered as |
|---|---|---|
| `Fixed` | the cmdlet decides the keys | the type name and its field list |
| `Derived` | the input decides the keys | the rule, e.g. "keys are the input's leaf paths" |
| `Dynamic` | an external source decides | what the source is, e.g. "one key per selected column" |
| `Unspecified` | not an object producer | nothing |

A `Field` is a name, a JSON type, and one line of prose.

## The surfaces

One declaration, four readers, no restatement:

- `get_help` grows an OUTPUT shape block under the cardinality line it prints.
- `get_command` carries `.Shape` as data, beside `.Streaming` and `.Output`.
- MCP `list_functions` carries streaming, aliases and shape — today it drops all
  three, because it reads `GetFunctionMetadata()` directly instead of the
  catalogue `discovery` already assembles.
- MCP `run_query` reports the observed shape of what it returned.

And a fifth, nearly free: the MCP result structs carry no `jsonschema` tags at
all, so the advertised `outputSchema` is typed but undocumented — a model reads
`"count": {"type": "integer"}` with no explanation, and `values` is declared
`array of string` when each element is JSON text needing a second parse.

## Scope

In: the shape package, the registration wrappers, every object producer
reachable without credentials or a network, the four surfaces, the enforcement
tests, and the README.

Out: declaring shapes for scalar transforms (decision 4), and the Censys and LLM
producers, whose examples cannot run in CI — they are declared where the
construction site is centralised enough to be safe, and left `Unspecified`
otherwise rather than guessed at.
