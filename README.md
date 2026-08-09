# pwrq

PowerShell-style cmdlets on top of jq.

`pwrq` is [gojq](https://github.com/itchyny/gojq) plus a library of cmdlets that
reach the filesystem, the OS, the network and a pile of codecs. Cmdlets emit
ordinary JSON objects, so jq's own filters work on them directly:

```console
$ pwrq -c '[get_childitem(".") | select(.Length > 10000) | {Name, Length}]'
[{"Length":15673,"Name":"EXAMPLES.md"},{"Length":26859,"Name":"task.md"}]

$ pwrq -c '[get_service | select(.Status == "Running") | .Name] | length'
68
```

## It is a strict superset of jq

Any valid jq program produces byte-identical output. That is enforced, not
aspirational: the test suite runs gojq's own 839-case CLI corpus unchanged, so
pwrq cannot drift from jq without a test failing.

Concretely, this means pwrq never quietly reinterprets your data:

```console
$ echo '{"_val":1,"_meta":{"a":2}}' | pwrq -c .
{"_meta":{"a":2},"_val":1}
```

It also means pwrq's own functions never shadow jq's. `split`, `join` and `sort`
are jq's; a UDF that collided with a builtin would never run, so the build fails
if one is added.

## Installation

```bash
go install github.com/xen0bit/pwrq/cmd/pwrq@latest
```

## The object model

Everything a cmdlet emits is plain JSON. There is no envelope to unwrap.

| What it is | What it returns | Example |
|---|---|---|
| **Transforms** | the transformed value | `"hello" \| sha256` → `"2cf24d…"` |
| **Object producers** | an object whose keys are PowerShell property names, plus `PSTypeName` | `get_childitem(".")` |
| **Formatters** | text | `format_table(.)` |

```console
$ pwrq -c 'get_childitem(".") | select(.Name == "go.mod")'
{"CreationTime":"2026-08-07T22:08:58-04:00","Extension":".mod",
 "FullName":"/home/you/pwrq/go.mod","IsHidden":false,"IsReadOnly":false,
 "LastWriteTime":"2026-08-07T22:08:58-04:00","Length":2928,"Mode":"-rw-rw-r--",
 "Name":"go.mod","PSPath":"go.mod","PSTypeName":"System.IO.FileInfo"}
```

Because it is JSON, everything jq knows how to do applies — `select`, `map`,
`group_by`, `to_entries`, string interpolation, all of it.

### Failures are jq errors

A cmdlet that fails raises an error rather than returning a value that looks
successful, so `try`/`catch` and the exit status behave as they do for jq:

```console
$ pwrq -c 'try cat("/nope") catch "missing"'
"missing"
```

## Aliases

The PowerShell short names are compiled into your query as jq definitions:

```console
$ pwrq -c '[gci(".")] | length'      # gci, dir, gi
$ pwrq -c '[gps | .Name] | length'   # gps
$ pwrq -c 'gl.Path'                  # gl
```

Aliases that would collide with a jq builtin are deliberately absent. PowerShell's
`select` and `sort` would shadow jq's own `select/1` and `sort/0` — and unlike a
function, a definition *does* take precedence, so such an alias would silently
change what existing jq programs mean. Use `select_object` and `sort_object`.

`pwrq --udf-list` prints every function and alias, grouped by category.

## Cmdlets

Filesystem, location, processes, services, web, date/time:

```console
$ pwrq -c '[get_childitem("src"; {Recurse: true, Filter: "*.go"})] | length'
$ pwrq -c '[get_process | select(.Name | test("^go")) | {Name, Id}]'
$ pwrq -c 'get_date | {Year, Month, DayOfWeek}'
$ pwrq -c 'invoke_web_request("https://example.com") | {StatusCode, ContentLength}'
$ pwrq -c 'test_path("go.mod")'
```

Parameter names bind case-insensitively, as PowerShell's do, so `{Recurse: true}`
and `{recurse: true}` are the same.

### Object cmdlets

`select_object`, `where_object`, `sort_object`, `group_object` and
`measure_object` take either a jq script block or the PowerShell
property/operator/value form:

```console
$ pwrq -c 'where_object(.; {script: ".Age > 26 and (.Name | startswith(\"A\"))"})'
$ pwrq -c 'where_object(.; {property: "Name", operator: "like", value: "A*"})'
```

A script block is jq — any expression, not a subset. Note that jq's own `select`
is usually shorter: `map(select(.Age > 26))`.

### Codecs, hashes and crypto

Encodings (base64/32/85, hex, binary, url, html), hashes (md5 through sha512,
hmac, ssdeep), ciphers (AES, DES, 3DES, Blowfish, RC4, ChaCha20, XOR),
compression (gzip, zlib, deflate), format conversion (csv, xml), entropy, and
`sh`, `http`, `find`, `cat`, `tee`.

```console
$ pwrq -r '"hello" | base64_encode'
aGVsbG8=
$ pwrq -r 'cat("go.mod") | sha256'
$ pwrq -c '[find("."; "file") | select(endswith(".go"))] | length'
```

See [EXAMPLES.md](EXAMPLES.md) and [pkg/udf/README.md](pkg/udf/README.md).

## pwrq-viz

Query diagramming and the browser IDE live in a separate binary. Rendering uses
d2, which brings a JavaScript engine, a syntax highlighter and a PDF writer with
it — about 35MB that everyday use has no need for.

```bash
make build-viz
./pwrq-viz -g query.svg '.a | .b'   # render the query's structure
```

The IDE is a full editor, and everything in it runs in the tab: pwrq itself is
compiled to WebAssembly and evaluated in a worker thread, so nothing you type is
uploaded anywhere.

```bash
make web.build          # build the page (needs bun)
./pwrq-viz -i           # then open http://localhost:8080/tools/pwrq/
make build-viz-with-ide # or bake the page into the binary
```

What it does:

- **Runs as you type**, against sample JSON you paste, drop or open. The input
  pane takes a stream of values the way the CLI reads a file, and the jq flags
  that matter are switches: `-c`, `-r`, `-s`, `-n`, a result limit and a
  timeout. Named arguments bind jq variables, as `--argjson` does.
- **Draws the query**, coloured by what each node *is*: your cmdlets in blue,
  jq's own builtins in teal, control flow in orange, constructed data in
  magenta, paths in indigo. The legend under the diagram is generated from the
  same palette the renderer used. Zoom, pan, switch the layout engine, and
  export the SVG or the D2 source.
- **Shares by link.** The `#` fragment carries the query, the input and the
  arguments, deflate-compressed. Browsers never send a fragment to a server, so
  a link is readable by whoever you send it to and by nobody in between.
- **Knows its own vocabulary.** Completion, highlighting and the catalogue are
  built from the registry the page actually evaluates against, so it can never
  offer a name it would then fail to run. There is a gallery of worked
  examples, a command palette on Ctrl+K, and a history of what you have run.
- **Cannot be hung.** A query that will not stop is ended by a deadline inside
  the engine, an unbounded stream by a result limit, and anything that survives
  both by terminating the worker — which is the only thing that can interrupt
  WebAssembly mid-instruction.

A browser tab has no filesystem, process table or service manager, so the
cmdlets that need one are not offered there: `get_childitem`, `get_process`,
`get_service`, `sh` and their aliases are absent, as are the network cmdlets,
which would work only against origins that allow CORS. Codecs, hashes,
ciphers, compression, format conversion, and the object and formatting cmdlets
are all available. `get_command` in the page lists exactly what the page has.

## Development

```bash
make build       # pwrq (9.5MB)
make build-viz   # pwrq-viz
make build-all
make web.build   # the browser editor (needs bun)
make test        # full suite, including gojq's corpus, for both builds
make test-short  # skips tests that touch system services
make web.test    # the editor's browser-side tests
make help
```

## License

MIT, as gojq is.
