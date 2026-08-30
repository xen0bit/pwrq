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

That holds for behaviour as well as for names. A cmdlet that merely renamed a jq
builtin would be a second vocabulary for the same idea, so there is no
`regex_split` beside `splits` and no `upper` beside `ascii_upcase`. Regular
expressions, slicing and case folding are jq's, and stay jq's.

## Installation

```bash
go install github.com/xen0bit/pwrq/cmd/pwrq@latest
```

### Debian / Ubuntu

A signed apt repository carries every release for all eight Debian release
architectures — amd64, arm64, armhf, armel, i386, ppc64el, riscv64, s390x.
pwrq is CGO-free, so they are all the same build, cross-compiled.

```bash
sudo curl -fsSL https://xen0bit.github.io/pwrq/pwrq-archive-keyring.gpg \
     -o /usr/share/keyrings/pwrq-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/pwrq-archive-keyring.gpg] https://xen0bit.github.io/pwrq stable main" \
     | sudo tee /etc/apt/sources.list.d/pwrq.list
sudo apt update
sudo apt install pwrq
```

`pwrq-viz` is the same tool plus `--graph` (renders a query as a diagram) and
`--ide` (serves the browser editor from inside the binary). It is a superset,
so install it instead of `pwrq` or alongside it:

```bash
sudo apt install pwrq-viz
```

The repository tracks the latest release only; older versions stay on
[GitHub Releases](https://github.com/xen0bit/pwrq/releases), which also carries
`.rpm` and `.apk` packages and plain tarballs.

## The object model

Everything a cmdlet emits is plain JSON. There is no envelope to unwrap.

| What it is | What it returns | Example |
|---|---|---|
| **Transforms** | the transformed value | `"hello" \| sha256` → `"2cf24d…"` |
| **Object producers** | an object of named properties, plus `PwrqType` | `get_childitem(".")` |
| **Formatters** | text | `format_table(.)` |

```console
$ pwrq -c 'get_childitem(".") | select(.Name == "go.mod")'
{"CreationTime":"2026-08-07T22:08:58-04:00","Extension":".mod",
 "FullName":"/home/you/pwrq/go.mod","IsHidden":false,"IsReadOnly":false,
 "LastWriteTime":"2026-08-07T22:08:58-04:00","Length":2928,"Mode":"-rw-rw-r--",
 "Name":"go.mod","PwrqType":"Pwrq.FileSystem.File","PwrqValue":"go.mod"}
```

Because it is JSON, everything jq knows how to do applies — `select`, `map`,
`group_by`, `to_entries`, string interpolation, all of it.

### One value or a stream

A cmdlet either emits a single value or streams one value per result, and which
it does decides whether you collect with `[...]`. The rule is that a cmdlet
enumerating something — files, processes, services, matching lines — streams,
and one computing a single answer does not:

```console
$ pwrq -c '[get_childitem(".")] | map(.Name) | length'          # stream: collect it
$ pwrq -c '[select_string("src"; "TODO")] | map(.Path)'         # stream: collect it
$ pwrq -c 'get_childitem(".") | select(.Length > 1000) | .Name' # or filter it as it goes
$ pwrq -nc 'sha256("go.mod")'                                   # one value: no brackets
```

Because the stream is lazy, filtering it costs only what it reads:
`first(select_string("."; "needle"))` stops at the first match rather than
grepping the whole tree and throwing the rest away.

Collecting a stream is the common case, and the mistake it guards against is
worth spelling out: without the brackets, `get_childitem(".") | map(...)` runs
`map` against *each* object rather than the list, so `.[]` iterates that
object's property values and you get `expected an object but got: string`.

You never have to guess which kind a cmdlet is. `get_help` prints it, and the
answer is read out of the registration itself rather than a hand-kept table, so
it cannot drift from the code:

```console
$ pwrq -r 'get_help("get_childitem")' | grep -A1 OUTPUT
OUTPUT
    a stream of values, one per result — collect with [...] to get an array
```

`get_command` carries the same fact as data, as `.Streaming` and `.Output`.

### And which keys come back

The same registration says what an object producer emits, so you do not have to
run a cmdlet once to find out what to select from it:

```console
$ pwrq -r 'get_help("get_service")' | sed -n '/^OUTPUT/,/^$/p'
OUTPUT
    a stream of values, one per result — collect with [...] to get an array
    object [Pwrq.Service] with 12 properties
        Name (string) — service name, as the manager knows it
        DisplayName (string) — human-readable name
        Status (string) — Running, Stopped, or another manager state
        ...
        ProcessId (number, optional) — pid of the running service; absent when it is not running
```

Three kinds of answer are possible, because three kinds of cmdlet exist. One
that decides its own keys lists them, as above. One whose keys come from *your*
data says so instead, since a property list would only ever be true of the
example that produced it:

```console
$ pwrq -rn 'get_command("flatten_keys") | .Shape'
object, keys from the input: one key per leaf of the input, named by its dot-and-bracket path
```

And a cmdlet that returns a string or a number says nothing at all. There is no
property list for `sha256`, and inventing one for the three hundred cmdlets in
that position is the drift this is built to avoid.

`PwrqType` is the key to the rest. A value carries it, `get_command` lists the
same name under `.TypeName`, and the properties are one lookup away. The names
are pwrq's own — `Pwrq.FileSystem.File`, `Pwrq.Process`, `Pwrq.Sqlite.Row` —
because what they resolve against is this catalogue and nothing else:

```console
$ pwrq -rn '[get_command | select(.TypeName == "Pwrq.FileSystem.File") | .Name]'
```

None of this is a table kept by hand. A cmdlet declares its shape where it is
registered, the shape is what builds the object, and the test suite runs the
cmdlets and compares the two — so a declaration that falls behind the code fails
the build rather than misleading you.

### Bytes survive the pipeline

A jq string is a byte string, so cmdlets pass binary content through unchanged —
`read_bytes`, `http`'s `.Content`, the codecs, the compressors and the write
cmdlets are all byte-exact:

```console
$ pwrq -n 'http("GET"; "https://example.com/a.tar.gz") | .Content | out_file("a.tar.gz")'
$ pwrq -n 'gzip_decompress("a.tar.gz"; true) | utf8bytelength'
```

Two things about such a value are worth knowing. Use `utf8bytelength` to measure
it — jq's `length` counts codepoints, so on non-text bytes it reports a smaller
number that is not the file size. And do not print it: bytes that are not valid
UTF-8 have no JSON spelling, so send the value on to a hash, a codec or a file
rather than to stdout.

### Reading a file: by value or by path

The codec, hash and compression cmdlets take their input *by value*, and read a
file only when told to with a trailing `true`:

```console
$ pwrq -n 'gzip_decompress("a.tar.gz"; true)'          # read the file
$ pwrq -n 'read_bytes("a.tar.gz") | gzip_decompress'   # already the bytes
```

Passing a path without the flag decompresses the path *string*, so if you see
`invalid header` on an archive you know is good, the error will tell you which
mistake you made.

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

Archives, searching a tree, and writing files back out:

```console
$ pwrq -nc 'read_archive("release.zip") | map(select(.Length > 100000) | .Name)'
$ pwrq -nc 'compress_archive("src"; "src.tar.gz") | .Length'
$ pwrq -nc 'expand_archive("release.zip"; "./out") | length'
$ pwrq -nc '[select_string("src"; "TODO"; {Include: "*.go", Context: 1})] | length'
$ pwrq -nc '"deploy finished" | add_content("run.log") | .Length'
```

`expand_archive` refuses any entry whose name would resolve outside the
destination directory: an archive's entry names are data, and `../../etc/cron.d`
is a valid one.

Which values resemble which, and which bytes they actually share:

```console
$ pwrq -nc '["the quick brown fox jumps over the lazy dog",
             "the quick brown cat jumps over the lazy dog",
             "lorem ipsum dolor sit amet consectetur adipiscing"]
            | [rncd_compare] | sort_by(.Hybrid) | .[0] | {IndexA, IndexB, Hybrid}'
{"Hybrid":0.136479,"IndexA":0,"IndexB":1}
```

`rncd_compare` scores every pair on a blend of compression distance and
entropy, so two encrypted blobs — incompressible, and so indistinguishable to
compression alone — still read as the same kind of thing. `shared_chunks` then
names the byte ranges behind a score:

```console
$ pwrq -nc 'shared_chunks("the quick brown fox"; "a quick brown fox indeed")
           | {Coverage, MatchedBytes, Spans}'
{"Coverage":0.842105,"MatchedBytes":16,"Spans":1}
```

Both take values, not paths, so anything that casts to bytes is a corpus:
strings, decoded blobs, response bodies, or files read into the pipeline with
`read_bytes`, which is `cat` without the text decoding. See
[EXAMPLES.md](EXAMPLES.md) for comparing files with them.

Time zones and date formatting, which jq's UTC-only `todate` cannot reach:

```console
$ pwrq -nc '"2026-08-11T12:00:00Z" | to_timezone("Asia/Tokyo") | {DateTime, Abbreviation}'
{"Abbreviation":"JST","DateTime":"2026-08-11T21:00:00+09:00"}

$ pwrq -nrc '"2026-08-11T12:00:00Z" | format_date("http")'
Tue, 11 Aug 2026 12:00:00 GMT

$ pwrq -nrc '"11/08/2026" | parse_date("02/01/2006")'
2026-08-11T00:00:00Z

$ pwrq -nc 'list_timezones("Europe/Lo")'
["Europe/London"]
```

Comparing two collections, where `deep_diff` compares two documents:

```console
$ pwrq -nc 'compare_object(["a","b"]; ["b","c"]) | map({v: .InputObject, s: .SideIndicator})'
[{"s":"<=","v":"a"},{"s":"=>","v":"c"}]
```

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

### SQLite

A database file is another source of objects. Queries stream one object per row
and are opened read-only; statements that change the database are a separate
cmdlet, so a typo in a SELECT cannot rewrite the file being read.

```console
$ pwrq -c '[invoke_sqlite_query("app.db"; "select * from users")] | length'
$ pwrq -c 'invoke_sqlite_query("app.db"; "select * from users where id = ?"; [42]) | .email'
$ pwrq -c 'invoke_sqlite_command("app.db"; "delete from users where id = ?"; [42]) | .RowsAffected'
$ pwrq -c '[get_sqlite_table("app.db")] | map(.Name)'
$ pwrq -c '[get_sqlite_schema("app.db"; "users") | select(.IsPrimaryKey) | .Name]'
```

Values are bound rather than interpolated - an array binds to `?`, an object
binds to `:name` - because a query built by pasting values into its own text is
one apostrophe away from meaning something else.

`out_sqlite` goes the other way, writing the piped objects into a table it
creates from their shape:

```console
$ pwrq -nc '[get_childitem("."; {Recurse: true})] | out_sqlite("files.db"; "files") | .RowCount'
```

Values map to SQLite's storage classes and back: NULL to null, INTEGER and REAL
to numbers, TEXT to a string, and BLOB to a jq byte string - so a blob can go
straight on to `sha256` or `out_file`, and a string that is not valid UTF-8 is
stored as a BLOB rather than corrupted into TEXT. A nested object or array is
stored as JSON text, which SQLite's own `json_extract` can read.

The driver is [modernc.org/sqlite](https://modernc.org/sqlite), which is SQLite
compiled to Go rather than linked through cgo, so `go install` still needs
nothing but a Go toolchain. It has no js/wasm target, so where the other
CLI-only cmdlets are simply left out of the browser registry, this package
registers nothing at all in the wasm build - the IDE lists the SQLite cmdlets
and marks them unavailable, as it does for anything that needs a filesystem.

### The Censys Platform

`pwrq` speaks the Censys Platform API through Censys' own Go SDK, with a
vocabulary that follows their CLI: view an asset, enrich a host, search,
aggregate, run CensEye, manage collections and tags, read the organization and
its credits.

```console
$ pwrq -c 'get_censys_host("1.1.1.1") | .resource | {ip, service_count}'
$ pwrq -c '[search_censys("host.services.protocol=SSH")] | length'
$ pwrq -c 'get_censys_aggregate("host.services.port=443"; "host.location.country")'
$ pwrq -c '[get_censys_tag] | map(.name)'
```

Credentials come from `CENSYS_PLATFORM_TOKEN` and `CENSYS_PLATFORM_ORGID` — the
names the Platform documents, so a shell set up for `censys` needs no change —
or from `{Token, OrganizationId}` on any cmdlet, which is what lets one query
reach two organizations. `get_censys_context` reports what was resolved, and
never the token.

Search emits one object per hit, so `select`, `map` and `group_by` work on
results directly. It stops after one page unless asked for more, because each
page costs credits:

```console
$ pwrq -c '[search_censys("host.location.country=\"Chile\""; {Pages: 3})
           | .host_v1.resource.ip]'
```

Objects keep the API's own field names, so what the Censys documentation and
CenQL call `autonomous_system` is what you write here. The payload is the SDK's
model of the response, so a field a newer API version adds arrives by upgrading
`censys-sdk-go`.

### Language models

`invoke_llm` sends a prompt and returns what the model said, so a call composes
like any other transform:

```console
$ pwrq -nr 'invoke_llm("One word for the colour of the sky")'
Blue
$ pwrq -c 'map(invoke_llm("Summarize in five words: \(.Body)"))'
```

The prompt is jq, which means there is no template language to learn: string
interpolation already is one.

Which model, and where it lives, is a `provider/model` name. Credentials come
from the variables each vendor documents — `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`
— or from `{ApiKey}` on the call. `PWRQ_LLM_MODEL` sets the default:

```console
$ export PWRQ_LLM_MODEL=anthropic/claude-sonnet-4-5
$ pwrq -nc 'get_llm_context | {Model, BaseUrl, HasApiKey, ApiKeySource}'
```

`get_llm_context` reports what a call would resolve to, and never the key.
Anything speaking OpenAI's chat completions API is reachable by base URL, which
covers Ollama, vLLM, OpenRouter, Groq and LM Studio:

```console
$ pwrq -nc '[get_llm_model({Model: "ollama/", BaseUrl: "http://localhost:11434/v1"})] | map(.Model)'
```

#### Typed answers

A pipeline cannot do much with prose. `{Schema: ...}` takes a JSON Schema, and
the cmdlet returns the decoded, validated value — so the answers are rows that
`group_by` and `sort_by` can act on:

```console
$ pwrq -c 'map(invoke_llm("Classify the sentiment: \(.text)";
    {Schema: {type: "object",
              properties: {sentiment: {enum: ["positive","negative","neutral"]},
                           confidence: {type: "number"}},
              required: ["sentiment"]}}))
  | group_by(.sentiment) | map({sentiment: .[0].sentiment, n: length})'
```

The schema is enforced on the way back, not just requested on the way out: an
answer that does not satisfy it goes back to the model once with the validation
error, and then fails as a jq error rather than reaching your pipeline as
something almost right.

#### Many prompts at once

gojq evaluates synchronously, so `map(invoke_llm(...))` over five hundred rows
is five hundred sequential round trips. `invoke_llm_batch` runs a bounded pool
and emits one response object per prompt, in input order:

```console
$ pwrq -c '[.[] | "Summarize: \(.)"] | [invoke_llm_batch({Parallel: 8})] | map(.Content)'
```

`{ContinueOnError: true}` reports a failed prompt as `.Error` on its own result
instead of failing the call.

#### What a call costs

Model calls are billed, and a `map` over a large input is an unbounded bill. So
one pwrq process makes at most 100 calls before it refuses; `PWRQ_LLM_MAX_CALLS`
or `{MaxCalls}` raises it, and `PWRQ_LLM_MAX_CALLS=0` removes it. `get_llm_usage`
reports what has been spent:

```console
$ pwrq -c '[invoke_llm_batch($prompts)] | length, get_llm_usage'
```

`.Cost` is null unless you supply `{PriceInput, PriceOutput}` in dollars per
million tokens. Prices are not compiled in: a table baked into a binary is stale
the week after it ships, and a confidently wrong cost is worse than none.

Temperature defaults to 0, so a query can be re-run. `{Cache: true}` — or
`PWRQ_LLM_CACHE=1` — stores answers on disk keyed by the exact request, which is
what makes the edit-and-rerun loop affordable while a pipeline is being built.
Entries never expire and nothing evicts them: it is a build-time convenience, so
`{CacheDir}` somewhere disposable is the way to keep it that way. Set
`PWRQ_LLM_DEBUG=1` to trace every request and reply to stderr.

#### Embeddings

`invoke_embedding` returns the vector a model represents text by, and
`cosine_similarity` ranks by it — semantic search as an ordinary pipeline:

```console
$ pwrq -nc '["how to bake sourdough", "naval warfare", "training a puppy"] as $docs
  | {Model: "openai/text-embedding-3-small"} as $m
  | invoke_embedding($docs; $m) as $vectors
  | invoke_embedding("my dog will not listen"; $m) as $q
  | [range(0; $docs|length) | {doc: $docs[.], score: cosine_similarity($vectors[.]; $q)}]
  | sort_by(-.score) | .[0].doc'
"training a puppy"
```

This is the comparison `levenshtein` and `jaccard` cannot make: they compare
spelling, and two sentences meaning the same thing in different words score
near zero on both.

### Agents

`invoke_agent` gives a model a task and lets it write pwrq queries until it can
answer:

```console
$ pwrq -nr 'invoke_agent("Which file here is largest, and how many bytes?")'
pwrq-viz 35913993
$ pwrq -c 'invoke_agent_request("Which row is the outlier?") | .Steps | map(.Query)'
```

The tool surface is pwrq itself, which is what `pwrq --mcp` already offers an
agent from the outside. `invoke_agent_request` returns the trace: an agent that
answered from three queries has made three claims, and `.Steps` is where a
reader checks them.

**What it may call is an allowlist, and the default is read-only.** `sh`, `rm`,
the write cmdlets and the network cmdlets are not in it. The restriction is
structural rather than a rule checked at call time — the agent's queries compile
against a registry built from the allowed cmdlets alone, so a denied cmdlet does
not exist to the compiler:

```console
$ pwrq -nc 'invoke_agent("count the TODOs"; {Allow: ["select_string", "get_childitem"], MaxSteps: 6})'
```

[`examples/agent-triage.sh`](examples/agent-triage.sh) runs the whole shape end
to end: cmdlets find the errors in a log corpus, `invoke_llm_batch` classifies
them against a schema, jq summarises the rows, and an agent answers a question
about the result. See [EXAMPLES.md](EXAMPLES.md) for its output.

Naming an object cmdlet that takes a script block — `where_object`,
`select_object` — hands the agent a whole query inside `{script: "..."}`; pwrq
narrows script blocks to the same allowlist while an agent runs, but the default
set leaves them out. The LLM cmdlets themselves can never be allowed, which is
what stops an agent spending in a loop no ceiling anticipates. A run is bounded
by `MaxSteps` (8), `MaxSeconds` (300) and a per-query result cap, and the agent's
queries get no environment loader, so `env` cannot hand a model the API keys of
the process running it.

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
- **Rewrites the query in place.** Format spreads it across lines, Minify puts
  it back on one, and Inline expands each `def` where it is called — a helper
  used three times is spelled out three times — so a pipeline reads straight
  through. None of the three changes what the query means: inlining renames a
  body's own bindings apart from the ones its arguments carry, and leaves a
  definition standing rather than move it somewhere a name it reads would mean
  something else. A definition that calls itself stays a definition, and the
  page says so.
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

## MCP server

pwrq can serve itself as a [Model Context Protocol](https://modelcontextprotocol.io)
server, so an agent can evaluate queries directly. The server exposes three tools:

- **run_query** — evaluate a pwrq/jq query against JSON or raw text input, with
  the full cmdlet vocabulary and the jq flags that matter (`-r`, `-c`, `-s`,
  `-n`), named arguments and per-call result limits and timeouts.
- **list_functions** — the cmdlet catalogue with arity, description, examples,
  what the cmdlet emits, how its output is encoded, and the keys it reads out
  of an options object.
- **validate_query** — check that a query parses *and compiles* before running
  it, so a query it accepts will run.

Runs are always bounded — a default timeout, a result limit, and an 8KB cap on
any one result — and each call gets a fresh session, so no state leaks between
calls. A result nested more than 10000 levels deep is refused as a query error
rather than encoded, because encoding it recurses deeply enough to exhaust the
goroutine stack, which no `recover` can catch.

### Answering the questions a caller actually asks

An agent driving this server cannot see what a terminal user sees, so anything
it is not told, it works out by experiment — and a wrong guess costs a round
trip each time. Seven things it used to have to guess:

- **What a cmdlet is called.** The catalogue filter is case-insensitive across
  names, aliases, categories, descriptions and option keys, so `hash` finds the
  eight cmdlets in the category spelled `Hash`, and `http` finds
  `invoke_web_request` — which is named for neither `http` nor its category, and
  is the one cmdlet with `Headers`, `Body` and `AllowAutoRedirect` on it. A
  search broad enough that listing every description match would cost the
  entries their examples lists the names it held back instead. A search matching
  nothing offers the nearest names rather than an empty list.
- **Whether the vocabulary is only the cmdlets.** It is not: pwrq is a strict
  superset of jq, so `ascii_upcase`, `split`, `fromjson`, `to_entries` and the
  rest are callable exactly like cmdlets. The catalogue documents only the 487
  cmdlets, which used to mean the tool denied the other half outright —
  `list_functions` filtered by `ascii_upcase` answered *no functions match, and
  nothing is named close to it*, about a function that runs. A filtered search
  now reports jq's matches too, in their own labelled section, and an
  unresolvable call is measured against both halves: `from_json` comes back with
  `did you mean fromjson?`, and `to_upper` with `ascii_upcase`. The list is
  derived from gojq's own `builtins`, so it cannot drift from what actually
  compiles.
- **Whether an example works.** Every one of the 652 published examples runs as
  written, and a test runs them to prove it. A hundred of them did not: `e.g.
  md5` was the whole example for `md5`, `aes_encrypt("data"; "key")` used a
  three-byte key for a cipher that needs sixteen, and `base64_encode(true)` left
  out the path the `true` refers to. The exemptions — the examples that reach a
  network, need a credential, or change the machine — are listed with a reason
  each.
- **How the output is written.** `zlib_compress` returns a hex string, not raw
  bytes, and `sha256` does too while `aes_encrypt` returns base64. Every cmdlet
  whose result is a text rendering of bytes declares which, and names the cmdlet
  that reverses it; the decoders declare what they take, so the catalogue
  answers both halves of the question.
- **Whether two stages fit together.** `zlib_compress | base64_encode` compiles,
  runs, and returns a plausible string that is twice the size it should be: the
  base64 covers the hex text rather than the compressed bytes. `md5 |
  base64_decode` is worse — hex is also valid base64, so it returns meaningless
  bytes and no error at all. `run_query` and `validate_query` compare what each
  stage produces against what the next one accepts and say which cmdlet
  reconciles them:

  ```
  -- warning: zlib_compress returns a hex string, but base64_encode expects raw
     bytes - base64_encode will encode the text rather than the bytes; pipe
     through hex_decode first
  ```

  It warns rather than refuses, and only where both sides have declared
  themselves, so a query it has nothing to say about is not thereby endorsed.
- **What goes in an options object.** The twenty-five cmdlets documented as
  taking `[options]` list their keys, their types and what each does — including
  where the casing is fussy, since an unknown key is ignored in silence rather
  than refused.
- **What went wrong.** A decoder that turns its input down shows the input:
  `json_parse: invalid JSON: invalid character 'h' … ; input was "hello world"`.
  A call that does not resolve says whether the name exists at another arity —
  `C/0 is not defined - you defined C taking 1 argument`, or `split is a jq
  builtin taking 1 to 2 arguments` — or offers the name you probably meant. A
  cmdlet handed its options down the pipe reads them there rather than reporting
  the option it was just given as missing.

Over stdio, which Claude Desktop, Cursor and friends launch directly:

```json
{
  "mcpServers": {
    "pwrq": {
      "command": "/path/to/pwrq",
      "args": ["--mcp"]
    }
  }
}
```

Or over streamable HTTP:

```bash
pwrq --mcp-http 127.0.0.1:8000
```

which is the transport a hosted client such as Open WebUI wants: add it under
*Admin Settings → External Tools* as an **MCP Streamable HTTP** server pointed
at that address, with `PWRQ_MCP_TOKEN` as the bearer key if you set one.

### Meeting clients where they are

Two things about a tool call are decided by software that is not this server,
and both are catered for here rather than assumed away:

- **A result is read as text.** The MCP spec has structured results, but a
  client is only obliged to pass the content blocks to the model, and several
  popular ones ignore the structured half entirely. So every tool returns its
  whole answer as text as well: `list_functions` renders the catalogue, not a
  count of it.
- **The input schema outlives the client.** It is handed to the model provider
  verbatim as the function's parameters, and the stricter providers reject a
  type union such as `["null", "array"]` outright. Nothing advertised here uses
  one. In the other direction, a model that sends an object where JSON text was
  asked for, a quoted number where an integer was, or a flag that does not
  exist, gets its arguments read as intended rather than a validation error.

### What the HTTP transport exposes

`run_query` evaluates against the whole CLI vocabulary, and that vocabulary
reads files, writes files, runs commands and makes network requests. Anyone who
can reach the port can therefore do anything the user running `pwrq` can do.
Over stdio that is the point — the client launched the process. Over HTTP, who
can reach the port *is* the security model:

- A loopback bind (`127.0.0.1:8000`, `[::1]:8000`) is reachable only from the
  machine itself, and needs nothing more.
- Any other address — including the bare `:8000`, which is every interface — is
  refused unless `PWRQ_MCP_TOKEN` is set. Clients then send it as
  `Authorization: Bearer <token>`.

```bash
export PWRQ_MCP_TOKEN=$(pwrq -rn 'random_string(32)')
pwrq --mcp-http :8000
```

The token lives in the environment rather than in a flag so it does not sit in
the process table. Setting it on a loopback bind works too, and is worth doing
on a machine with other users on it. The transport has no TLS of its own: put it
behind a reverse proxy if it needs to cross a network.

### What the server logs

The HTTP transport logs to stderr, and because the MCP protocol owns stdout,
stdio logs there too. At the default `info` level you see, in order, what the
server did:

```text
time=... level=INFO msg="mcp http server listening" addr=127.0.0.1:8000 auth="loopback, no token required"
time=... level=INFO msg="http request" method=POST path=/ status=200 remote=127.0.0.1:41080 session=4A3LFAS4... duration=7ms
time=... level=INFO msg="tool call" tool=run_query query="[1,2,3] | map(. * 2)" count=1 kind="" truncated=false duration=7ms
```

Every HTTP request gets a line — method, path, status, remote address, session
and duration, including requests rejected before they reach the session — and
each tool call gets one with the query (truncated), its result count and how
long it took. A rejected bearer token is logged at `warn` with the remote
address, since a burst of those from a non-loopback address is an attack rather
than a misconfigured client.

Set `PWRQ_LOG_LEVEL` to `debug`, `info`, `warn` or `error` to tune it; absent,
the HTTP transport logs at `info` (its behaviour is otherwise invisible) and
stdio at `warn`, so a single client's conversation stays quiet unless asked
for.

## Writing a cmdlet

Register with `common.WithFunction` or `common.WithIterFunction`, never with
`gojq.WithFunction` directly. The wrappers put every result into the value space
gojq operates on — `nil`, `bool`, `int`, `float64`, `*big.Int`, `json.Number`,
`string`, `[]any`, `map[string]any` — and gojq *panics* on anything else, not
only when encoding but inside any builtin that inspects the value. A cmdlet that
returns an `int32` in a map encodes fine and then crashes the first query that
calls `type` on that field, a long way from the mistake. A test fails if a
cmdlet registers directly.

Return errors as `error`; they travel on jq's error channel, where `try`/`catch`
and the exit status can see them, and they are deliberately left unnormalized.

## Development

```bash
make build       # pwrq (19MB)
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
