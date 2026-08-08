# User-Defined Functions

This package registers pwrq's functions with gojq. `pwrq --udf-list` prints the
current set; that listing is generated from the registry and checked against it
by a test, so it cannot drift from what the binary actually provides.

## What a function returns

There is no envelope. A function returns a value in jq's own value space, and
which kind depends on what the function is for:

| Kind | Returns | Example |
|---|---|---|
| **Transform** | the transformed value | `sha256`, `base64_encode`, `find` |
| **Object producer** | a flat object keyed by PowerShell property names, plus `PSTypeName` | `get_childitem`, `get_process`, `http` |
| **Formatter** | a string | `format_table`, `format_list` |

Nothing else may reach the encoder. A Go value that is not a JSON type is a
query error, so anything a function computes has to be converted at the boundary
— `psobject.NormalizeJSON` does that, turning `time.Time` into RFC3339,
`os.FileMode` into its string, sized integers into `int`, and so on. This is not
only about printing: a `time.Time` in the pipeline is a value no jq builtin can
act on.

Binary output is hex-encoded, JSON having no byte type. The decoders accept the
same representation, so round-trips work.

## Failures

A function that fails returns an `error`. gojq puts that on its error channel,
which means `try`/`catch`, `//` and the process exit status all behave as they
do for jq's own failures:

```console
$ pwrq -c 'try cat("/nope") catch "missing"'
"missing"
```

Returning a value that merely looks like a failure is the thing to avoid: the
pipeline carries on with it, and the caller finds out much later.

## Input binding

Pipeline input reaches a function through one of two helpers in `common`:

- `BindValue` — the value to operate on. A scalar binds directly; a cmdlet's
  object output binds as itself, because the object cmdlets need the whole
  object and collapsing it to some property would discard the rest.
- `BindPath` — a filesystem path, following PowerShell's ByValue-then-
  ByPropertyName rule. This is what lets `find(".") | cat` (a stream of strings)
  and `get_childitem(".") | cat` (a stream of objects) both work.

## Adding a function

1. Create a package under `pkg/udf/`.
2. Write a `Register*() gojq.CompilerOption` returning `gojq.WithFunction`.
3. Register it in `DefaultRegistry()` in `registry.go`.
4. Add an entry to `GetFunctionMetadata()` in `metadata.go` — name, arity range,
   category, description, examples. `--udf-list` reads it, and
   `TestUDFListMatchesRegistry` fails if it disagrees with what is registered.

```go
func RegisterMyFunction() gojq.CompilerOption {
    return gojq.WithFunction("my_function", 0, 1, func(v any, args []any) any {
        input := v
        if len(args) > 0 {
            input = args[0]
        }

        s, err := common.BindString(input, "input")
        if err != nil {
            return common.MakeUDFErrorResult(err, map[string]any{"operation": "my_function"})
        }

        return transform(s) // a plain JSON value
    })
}
```

### Names are checked against jq's

gojq resolves builtins before custom functions, so a function sharing a
builtin's name and arity never runs — it does not error, it is simply never
reached. `TestNoBuiltinShadowing` fails the build rather than letting one ship.
This caught `split`, `join` and `trim`, which had been registered but never once
executed; they are gone, because jq provides them.

The same test covers aliases, and matters more there: an alias is compiled as a
jq `def`, and a definition *does* take precedence over a builtin, so a badly
chosen alias would silently change what existing jq programs mean.

## Aliases

`StandardAliases` in `alias.go` maps PowerShell short names to cmdlets. They are
compiled into the query as jq definitions, one per arity the target accepts,
generated from what the registry reports — so an alias cannot fall out of step
with the cmdlet it names.

Resolving aliases at runtime is not possible: gojq binds function names at
compile time and never consults session state.

## Parameter binding

Cmdlets taking an options object can bind it by reflection instead of unpacking
each field:

```go
type MyOptions struct {
    Path    string `param:"Path"`
    Recurse bool   `param:"Recurse"`
    Include []string `param:"Include"`
}

if err := pipeline.BindParameters(optsMap, &opts); err != nil { ... }
```

Names match case-insensitively, as PowerShell's parameters do, and JSON arrays
convert to typed slices.
