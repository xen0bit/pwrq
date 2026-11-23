# Corpus feedback

Findings from building and exercising ~1,100 workflow examples across
the whole pwrq vocabulary — 961 single-domain entries and 127 chained
ones that cross domains (fs + search, fs + ai, log + net, and so on).
Everything below was discovered the hard way: a corpus entry that
failed, or an API that made an honest workflow impossible. Grouped by
kind, most surprising first. Line references are to the corpus entry
that hit them.

## Bugs

1. **`out_file` writes objects with Go's `%v`, not JSON.** Saving an
   object or array produces a file like
   `map[File:dup.txt Sha256:bce2...]` — invalid JSON that `fromjson`
   cannot read back. Every corpus entry that saves structured data has
   to pipe through `tojson` first
   (`file-io.pwrq:io-write-json`, `hash.pwrq:hash-manifest-save`,
   `net.pwrq:net-fetch-save-json`). A bare `out_file` should serialize
   values the way the encoder does.

2. **`builtinHelp` drifts from gojq.** `pkg/ideengine/suggest.go`'s
   builtinHelp advertises `leaf_paths` and the jq corpus knows
   `keys_unsorted`, but neither compiles in this gojq build
   (`json.pwrq:jq-leaf-values`, `formats.pwrq:formats-objects-to-csv`).
   A suggestion offered from that table would produce a query that
   cannot compile — the table should be generated from `builtins`, the
   way `builtinArities` already is.

## Ergonomics

3. **`invoke_agent` needs a remote LLM, not the local decision model.**
    With the d1-2B model downloaded and working, `invoke_agent` still
    refuses: "no model; set PWRQ_LLM_MODEL" (`ai.pwrq:ai-agent-count-files`).
    Everything `invoke_systemone` does runs locally on CPU, so the natural
    expectation is that the agent does too. If it is a deliberate boundary
    (a stepper needs a text model), `get_help` should say so; as it stands
    the most attractive agent story is unavailable offline.

4. **The cipher family has two calling conventions.** `aes_encrypt`,
   `des_encrypt`, `blowfish_encrypt` and friends take `(data; key)` as
   two arguments; `rc4`, `xor` and `chacha20` take the data on the
   pipeline and the key as their single argument. Both arities are
   documented, but nothing warns you when you switch families, and the
   failure is a compile error, not a hint. I hit this writing every
   single cipher entry the first time
   (`cipher.pwrq:cipher-aes-roundtrip`).

5. **`find` returns strings; `get_childitem` returns objects.** The two
   filesystem getters disagree on shape, so every workflow has to know
   which one it is using before it can pipe into anything
   (`fs.pwrq:fs-list-depth-limited` vs `fs.pwrq:fs-list-files`). It is
   the single most common decision in filesystem entries.

6. **`xml_parse` reads only the root element.** Child elements stay as
   raw XML text inside `_content`; there is no way to walk a document
   structurally. The honest workflows are "root tag, attributes, inner
   text" and regex-over-content
   (`formats.pwrq:formats-xml-file-titles`). Worth saying in `get_help`,
   because "parse XML" raises bigger expectations.

7. **`ssdeep` has a 4096-byte floor.** Small files error out, where
   `md5(true)` on the same file happily answers. Reasonable for fuzzy
   hashing, but it surprised every workflow that reached for it on
   ordinary files (`hash.pwrq:hash-ssdeep-file` needed an 8 KB fixture).

8. **`http_serve` needs a pipeline value to serve.** With null input it
   answers "no item available"; its metadata example
   (`http_serve("127.0.0.1"; 8080)`) reads as if it serves a directory.
   The working form is `{...} | http_serve(...)` — it serves the item,
   which is a different (and nicer) story than the example tells
   (`net.pwrq:net-serve`).

9. **Streaming is declared but not predictable.** `get_childitem`,
   `select_string`, `find` and `invoke_sqlite_query` stream;
   `grep_lines`, `scan_regex` and `test_connection` return arrays.
   `get_help` does say which — the surprise is how often the wrong
   guess produces a per-item error like "expected an object but got:
   string" rather than an obvious hint. The `[...]`-collect idiom is
   everywhere in the corpus for this reason.

10. **`invoke_sqlite_query` rows carry a `PwrqType` field.** Exporting a
   table with `tojson` includes the internal marker
   (`Pwrq.Sqlite.Row`) in every row. Any consumer of the export has to
   know to drop it (`db.pwrq:db-export-json`).

11. **`get_process` has no user field.** "My processes" — `ps -u` — has
    no pwrq form; `sys.pwrq` works around it, but the story is missing.

12. **`chacha20` decrypts only with the nonce you already know.** The
    cipher output embeds the nonce in the base64 blob, but the cmdlet
    never strips it, so a receiver cannot cleanly "open what was sent"
    in one call. Either the blob should be self-describing (decrypt
    reads the embedded nonce) or `get_help` should say the nonce must
    travel out of band (`cipher.pwrq:cipher-chacha20-fixed-nonce`).

## Suggestion-workflow format constraints

These matter to the session that will absorb the corpus into
`recipes.pwrq` and calibrate:

13. **The `template` cmdlet cannot appear in a recipe.** Its
    placeholder syntax is `{{name}}` — the same bytes the recipe parser
    reads as a slot reference. A query demonstrating `template` fails
    the library's own parser ("slot {{name}} is not declared"), and a
    declared slot would be substituted away before `template` ever saw
    it. No corpus entry uses `template`; it is the one cmdlet a
    recipe-based suggestion workflow cannot suggest.

14. **Hand recipes cannot declare `Optional` or `input` slots.** Only
    the example-recipe generator sets `Slot.Optional`; the text format
    has no way to say it. That is why the corpus keeps input-reading
    queries self-contained with a literal first stage instead.

15. **Slot defaults must be plain literals.** The corpus harness fills
    slots by direct substitution (the same `Fill` output for the
    defaults used), which holds only for defaults without quotes or
    backslashes. Any absorbed default like `"a b&c"` or `\\bword\\b`
    should be re-checked against `Fill`.

16. **The strict header grammar has no room for metadata.** Only
    `# task:`, `# also:` and `# slot` lines are legal in a block; there
    is nowhere to record a "needs the network" or "compile-only" mark
    that the library itself could read. The corpus keeps that in
    `tools/validate.sh`'s tables instead.

## Small things

17. `stop_process` answers `{"Failed":[],"Stopped":[pid]}` — a lovely
    shape; the equivalent `rm` result is a plain boolean. Consistency
    between the destructive cmdlets' result shapes would help pipelines
    that report what they did.

18. `select_string`'s description warns that a regex backslash must be
    doubled — good, and it generalizes: the corpus standard is that
    every pattern in a query text carries `\\b`, `\\d` and friends
    escaped for the jq string first. A `get_help` example showing a
    doubled-backslash pattern would save everyone one failed run.

19. `get_service` on a machine without systemd fails with
    `exit status 1` from the child process. A pwrq-shaped message
    ("no service manager present") would diagnose the environment
    rather than the command.

## Chaining findings

From building the 117-entry chained corpus (`chain-*.pwrq`), where the
seams between domains are the whole point:

20. **There is no Get-Item story.** `get_childitem` on a *file* path
    yields nothing (it lists children, and a file has none), and the
    bound form `"notes.txt" | get_childitem` lists the file's *parent
    directory* instead of the file. Stating one file — PowerShell's
    `Get-Item`, the most basic step of every "download then stat the
    copy" chain — requires listing the parent and selecting:
    `[get_childitem(".") | select(.Name == "f")]`
    (`chain-net.pwrq:chain-net-download-age`). A `get_item` cmdlet, or
    `get_childitem` returning the file itself for a file path, would
    close a gap every chain into the filesystem hits.

21. **`reverse_dns` raises on addresses with no PTR record** — every
    RFC1918 and documentation-range address, which is most of what logs
    contain. Chains over real logs need
    `try reverse_dns(.) catch ["no ptr record"]` everywhere
    (`chain-search.pwrq:chain-search-whois-log-ips`). An empty result
    with the failure noted would be the friendlier shape.

22. **`is_base64` accepts any short alphanumeric word** ("one", "the",
    "line") that happens to fill a multiple of four. Sweeping files for
    hidden payloads needs the decode-then-verify pattern —
    `try base64_decode catch empty | select(is_utf8 and length > 0)` —
    rather than trusting the validator
    (`chain-fs.pwrq:chain-fs-decode-blobs`). A stricter default (length
    threshold, or padding requirements) would make the check trustworthy.

23. **`out_file` passes its value through, which is right — but chains
    that need the *path written* alongside it must track the path
    themselves** (`as $path` before the call), because nothing in the
    result says where it went (`chain-cipher.pwrq` needed it three
    entries in a row). A `{Path, Value}` result would save the binding.

## What the corpus needed that does not exist (the GNU-tool gaps)

Recorded because the task asked for every workflow a user may ever
reach for; these are the ones pwrq cannot do today. None is a complaint
— they draw the honest boundary.

- `chmod` / `chown`: no permission *setter* (only the read-only `Mode`
  and `IsReadOnly` fields). `ls -l` stories work; `chmod +x` does not.
- `touch` (set mtime): `new_item` creates, but there is no way to set
  or fake a file's times, which the "backdate for testing" story
  needs.
- `ln -s` / `readlink`: symlinks are not first-class anywhere; nothing
  lists, creates or resolves them.
- `stat` beyond size and times: no inode, no link count, no device.
- `strings`: no "extract printable runs from a binary" cmdlet; entropy
  and `is_binary` exist, but the classic strings story does not.
- `split -b` / `split -l`: no file chunking.
- `dd`: no raw byte-copy with offset/limit; `read_bytes` reads whole
  files only.
- `xargs` parallelism: `map` is sequential; there is no per-item
  concurrency for slow cmdlets (each `invoke_web_request` in a `map`
  is one at a time).
- `watch`: no repeat-until; polling a status requires re-running.
- `find -newer`: no "files newer than this file" comparison.

## Validation summary

Every entry is parsed with the same grammar `pkg/ideengine` enforces,
its slots filled with defaults, compiled and run through the installed
`pwrq` (rev b49d729, the MCP version) in a scratch fixture tree rebuilt
under `/tmp/opencode` on each run, once per file, so files that mutate
the tree cannot affect each other. Re-run everything with:

```console
$ corpus/tools/validate.sh            # all 25 files
$ corpus/tools/validate.sh fs.pwrq    # one file
```

What runs live here: the filesystem, text, data, crypto and sqlite
entries against generated fixtures; the network entries against
example.com, api.github.com and httpbin.org; the d1-2B decision model
on CPU (downloaded and cached) for every `invoke_systemone` entry,
including images and video through ffmpeg-generated fixtures; the
shell and process table; and a custom pwrgrep rule written and
immediately run, isolated by `PWRQ_RULES` pointing into the scratch.

What is validated by its expected environmental error: services (this
container has no systemd), Censys (no token) and the LLM cmdlets (no
API key) — each must fail with the specific credential or
service-manager error, never a compile error.

Two cmdlets are deliberately uncovered: `set_date`, because setting the
machine clock is not a workflow to demonstrate on a shared machine, and
`template`, because its `{{...}}` placeholder syntax cannot survive the
recipe grammar at all (finding 13). `invoke_agent_request` and
`invoke_agent` appear with the LLM entries for the same reason as
finding 3.

The final counts are in the run summary below.

## The corpus at a glance

1,088 entries across 37 files, every one parsed, slot-filled, compiled
and run through the installed binary:

| result | count | meaning |
|---|---|---|
| pass | 1,046 | ran, exited 0, produced output |
| empty | 4 | ran, exited 0, found nothing — the scratch tree honestly has no stale files, read-only files, packed files or taint flows |
| env | 38 | met the expected credential or service-manager error (7 services, 20 Censys, 11 LLM) |
| fail | 0 | — |

Single-domain files (961 entries):

| file | entries | file | entries |
|---|---|---|---|
| fs.pwrq | 58 | hash.pwrq | 36 |
| file-io.pwrq | 43 | cipher.pwrq | 22 |
| search.pwrq | 37 | ids.pwrq | 25 |
| log.pwrq | 37 | time.pwrq | 41 |
| sys.pwrq | 51 | num.pwrq | 54 |
| net.pwrq | 48 | sim.pwrq | 19 |
| json.pwrq | 86 | db.pwrq | 19 |
| coll.pwrq | 47 | code.pwrq | 48 |
| stats.pwrq | 41 | validate.pwrq | 23 |
| text.pwrq | 69 | meta.pwrq | 10 |
| codecs.pwrq | 34 | censys.pwrq | 21 |
| formats.pwrq | 37 | ai.pwrq | 30 |
| flow.pwrq | 25 | | |

Chained files (127 entries), grouped by the anchor stage a half-typed
query is standing on:

| file | entries | anchor → partners |
|---|---|---|
| chain-fs.pwrq | 22 | listing → search, hash, d1 triage of images and video, net downloads, stats, text, formats, codecs, cipher, time, archives |
| chain-search.pwrq | 15 | matches → stats, d1 urgency, url fetches, dns, json, sqlite, gzip blobs, dates, reports |
| chain-net.pwrq | 13 | resolve→port-check, port→fetch, fetch → sqlite, d1, validation, stats, age, sealed POST round trips |
| chain-log.pwrq | 11 | log parse → whois, sqlite, d1, incident span, rates, rotated logs, port liveness |
| chain-db.pwrq | 8 | query → per-row files, dns, d1, validation, month buckets, yaml, stats, report |
| chain-ai.pwrq | 9 | model verdict → moves, lists, sqlite, tables, second questions, csv |
| chain-sys.pwrq | 8 | process table / shell → binary existence, sqlite, stats, thresholds, host health |
| chain-codecs.pwrq | 8 | decode → json, files, checksums, url fetches, dates, sqlite, text |
| chain-cipher.pwrq | 6 | seal → manifest, wrong-key proof, vault, archive, extract, key rotation |
| chain-time.pwrq | 8 | age/date → dated bundles, weekday tables, sqlite, durations, zones, deadlines |
| chain-formats.pwrq | 9 | parse → sqlite import, column stats, d1 rows, xml arithmetic, format conversion, query fetches |
| chain-triples.pwrq | 10 | three- and four-domain showpieces (find→search→ask, post→sentiment→tally, decode→fetch→save) |

Cmdlet coverage: **509 of 511**. The two missing are `set_date` and
`template`, both deliberately, per the note above. Every category of
the catalogue is represented; the suggestion prefilter can match a
comment against a working query for any cmdlet a user can name, and
`# also:` lines carry the GNU tool names (`du`, `grep -l`, `md5sum -c`,
`dig -x`) that comments reach for first.

The chain files deliberately reuse the canonical first stages the
single-domain files teach (`get_childitem(...) | select(...)`,
`cat("access.log") | split("\n") | map(...)`), so a half-typed query
meets a chain at a stage head the continuation engine can match — that
is the property that makes them suggestion material rather than just
more examples.
