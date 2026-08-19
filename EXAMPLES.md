# pwrq Examples

Every command here was run against the current build; the outputs are what it
actually printed. Paths and counts naturally vary by machine.

## The shape of things

A cmdlet emits plain JSON, so jq's filters apply directly.

```console
$ pwrq -c 'get_childitem("cli") | select(.Name == "cli.go")'
{"CreationTime":"2026-08-07T22:08:58-04:00","Extension":".go",
 "FullName":"/home/you/pwrq/cli/cli.go","IsHidden":false,"IsReadOnly":false,
 "LastAccessTime":"2026-08-07T22:08:58-04:00","LastWriteTime":"2026-08-07T22:08:58-04:00",
 "Length":18644,"Mode":"-rw-rw-r--","Name":"cli.go","PSPath":"cli/cli.go",
 "PSTypeName":"System.IO.FileInfo"}
```

A transform returns its value, with nothing to unwrap:

```console
$ pwrq -r '"hello" | sha256'
2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
```

A formatter returns text:

```console
$ echo '[{"Name":"a","Age":30},{"Name":"bb","Age":4}]' | pwrq -r 'format_table(.)'
  Name Age
  ---- ---
  a    30
  bb   4
```

---

## Filesystem

```console
$ pwrq -c '[get_childitem("cli") | select(.Extension == ".go") | .Name] | .[0:3]'
["cli.go","cli_test.go","color.go"]

$ pwrq -c '[get_childitem("pkg"; {Recurse: true, Filter: "*.md"}) | .Name] | sort'
["README.md"]

$ pwrq -c 'test_path("go.mod")'
true

$ pwrq -c 'gl.Path'
"/home/you/pwrq"
```

Parameter names bind case-insensitively, so `{Recurse: true}` and
`{recurse: true}` are the same. `-Filter` and `-Include` choose what is
emitted, not what is descended into, so `-Recurse` reaches nested matches.

`find` yields paths rather than objects, which is what you want when feeding
another path-taking cmdlet:

```console
$ pwrq -c '[find("cli"; "file") | select(endswith(".yaml"))] | length'
5
```

Both forms bind to a cmdlet expecting a path:

```console
$ pwrq -c '[find("."; "file") | select(endswith("go.mod")) | cat] | length'
1
$ pwrq -c '[get_childitem(".") | select(.Name == "go.mod") | cat] | length'
1
```

## Processes, services, dates

```console
$ pwrq -c '[get_process | select(.CPU > 0)] | length'
31

$ pwrq -c '[get_process | select(.Name == "gopls") | .Id] | length'
2

$ pwrq -c '[get_service | select(.Status == "Running") | .Name] | length'
68

$ pwrq -c '[get_date | .Year, .Month]'
[2026,8]

$ pwrq -r 'new_timespan({Hours: 1, Minutes: 30}) | .Duration'
01:30:00.0000000
```

## Web

```console
$ pwrq -c 'invoke_web_request("https://example.com") | {StatusCode, ContentLength}'
{"ContentLength":559,"StatusCode":200}

$ pwrq -c 'http("GET"; "https://example.com") | .Headers["Content-Type"]'
"text/html"
```

Both return a response object, so the status code is available to branch on
rather than being discarded along with the rest of the response.

## Encoding and hashing

```console
$ pwrq -c '"hello world" | base64_encode'
"aGVsbG8gd29ybGQ="

$ pwrq -r '"aGVsbG8gd29ybGQ=" | base64_decode'
hello world

$ pwrq -c '"hello" | md5, sha1, sha256 | length'
32
40
64

$ pwrq -c '"the quick brown fox" | entropy'
3.8924071185928746

$ pwrq -c '"secret" | xor("key")'
"18001a19000d"
```

Binary results are hex-encoded, JSON having no byte type. Round-trips work
because the decoders accept the same representation:

```console
$ pwrq -c '"hello" | gzip_compress | gzip_decompress'
"hello"
```

## Data formats

```console
$ pwrq -c '"a,b\nc,d" | csv_parse'
[["a","b"],["c","d"]]

$ pwrq -c 'sh("echo hi")'
"hi"
```

`sh` returns stdout on success. A non-zero exit is an error, so it can be
caught or allowed to set the exit status:

```console
$ pwrq -c 'try sh("exit 3") catch "failed"'
"failed"
```

## Object cmdlets

Script blocks are jq — any expression, not a subset of one:

```console
$ echo '[{"Name":"Alice","Age":30},{"Name":"Bob","Age":25}]' \
    | pwrq -c 'where_object(.; {script: ".Age > 26 and (.Name | startswith(\"A\"))"}) | map(.Name)'
["Alice"]
```

Or the PowerShell property/operator/value form, which is where `-like` and
`-match` live:

```console
$ echo '[{"Name":"Alice"},{"Name":"Bob"}]' \
    | pwrq -c 'where_object(.; {property: "Name", operator: "like", value: "A*"}) | map(.Name)'
["Alice"]
```

For plain filtering jq's own `select` is shorter, and pwrq does not get in its
way: `map(select(.Age > 26))`.

## Composition

Find every Go file, hash it, and keep the largest three:

```console
$ pwrq -c '[get_childitem("cli"; {Filter: "*.go"})
            | {Name, Length, Hash: (.FullName | cat | sha256)}]
           | sort_by(-.Length) | .[0:3] | map(.Name)'
["cli.go","inputs.go","encoder.go"]
```

Nothing here needs a pwrq-specific idiom: `sort_by`, `map` and the object
constructor are jq's, working on cmdlet output because it is ordinary JSON.

## Grouping and summarising

Group-Object and Measure-Object as pure functions over arrays of JSON objects.
Keys are property names, matched case-insensitively:

```console
$ pwrq -n -c '[{"dept":"eng","pay":90},{"dept":"eng","pay":110},{"dept":"ops","pay":80}]
             | count_by("dept")'
{"eng":2,"ops":1}

$ pwrq -n -c '[{"dept":"eng","pay":90},{"dept":"eng","pay":110},{"dept":"ops","pay":80}]
             | summarize_by("dept"; "pay")'
[{"avg":100,"count":2,"key":"eng","max":110,"min":90,"sum":200},
 {"avg":80,"count":1,"key":"ops","max":80,"min":80,"sum":80}]
```

`sum_by("dept"; "pay")` and `avg_by("dept"; "pay")` give the object form;
`top_by("pay"; 2)` keeps the highest rows; `index_by("key")` keeps the first
row per value; `pivot` and `unpivot` reshape between wide and long:

```console
$ pwrq -n -c '[{"dept":"eng","year":2020,"salary":90},{"dept":"eng","year":2021,"salary":110}]
             | pivot({rows: "dept", cols: "year", values: "salary"})'
{"eng":{"2020":90,"2021":110}}
```

## Units, geo and finance

One Domain package for the everyday conversions:

```console
$ pwrq -n -c '20 | convert_unit("C"; "F")'
68

$ pwrq -n -c 'convert_unit(5; "mi"; "km")'
8.04672

$ pwrq -n -c '90 | convert_unit("min"; "h")'
1.5

$ pwrq -r '"1.5 MiB" | parse_size'
1572864

$ pwrq -n -c 'haversine_distance(51.5007; -0.1246; 40.7128; -74.0060)'
5570.674455985119

$ pwrq -n -c 'future_value(100; 0.05; 10)'
162.8894626777442

$ pwrq -n -c 'monthly_payment(20000; 0.06; 60)'
386.6560318950375
```

## Time series and statistics

Running totals, rolling extrema, smoothing, and relations between series:

```console
$ pwrq -n -c '[1,2,3,4] | cumsum'
[1,3,6,10]

$ pwrq -n -c '[1,2,3,4] | correlation([2,4,6,8])'
1

$ pwrq -n -c '[3,1,4,1,5] | moving_max(3)'
[4,4,5]

$ pwrq -n -c '[1,2,3,4,5,6,7,8] | quartiles'
[1,2.75,4.5,6.25,8]
```

## Regex and text

Regular expressions are jq's own `test`, `match`, `capture`, `scan`, `sub` and
`splits`. pwrq adds no second vocabulary for them, so what you know from jq is
what works here:

```console
$ pwrq -n -c '"a1 b22 c333" | [scan("[0-9]+")]'
["1","22","333"]

$ pwrq -n -c '"user=42" | capture("user=(?<id>\\d+)").id'
"42"

$ pwrq -n -c '"a,b;c" | [splits("[,;]")]'
["a","b","c"]
```

What pwrq does add is the text work jq has no answer for:

```console
$ pwrq -n -c '"café" | remove_accents'
"cafe"

$ pwrq -n -c '"orders.api (v2)" | escape_regex'
"orders\\.api \\(v2\\)"
```

## Versions, paths and sets

```console
$ pwrq -n -c 'semver_compare("1.0.0-rc.1"; "1.0.0")'
-1

$ pwrq -n -c '["file2","file10","file1"] | natural_sort'
["file1","file2","file10"]

$ pwrq -n -c '"2.1.3-rc.1+build5" | semver_parts'
{"build":"build5","major":2,"minor":1,"patch":3,"prerelease":"rc.1"}

$ pwrq -n -c '60 | prime_factors'
[2,2,3,5]
```

## Config formats

INI, .env / properties and logfmt are first-class string transforms:

```console
$ pwrq -n -r '"[server]\nhost = 10.0.0.1\nport = 8080" | ini_parse | .server.host'
10.0.0.1
```

Piped in as raw lines, since they read their input:

```console
$ echo 'level=info n=3 ok=true' | pwrq -R -c 'logfmt_parse'
{"level":"info","n":3,"ok":true}

$ echo 'APP_NAME=my-app' | pwrq -R -c 'properties_parse'
{"APP_NAME":"my-app"}

$ pwrq -n -c '{level: "warn", msg: "disk nearly full"} | logfmt_stringify'
"level=warn msg=\"disk nearly full\""
```

## Text and object helpers

```console
$ pwrq -n -c '"user@example.com" | {user: before_first("@"), domain: after_first("@")}'
{"domain":"example.com","user":"user"}

$ pwrq -n -c '"Robert" | soundex'
"R163"

$ pwrq -n -c '"a=1 b=22" | [match("(\\w+)=(\\d+)"; "g").captures | map(.string)]'
[["a","1"],["b","22"]]

$ pwrq -n -c '{"first_name":"ada","role":"admin"} | rename_keys({"first_name": "name"})'
{"name":"ada","role":"admin"}

$ pwrq -n -c '[{"metrics":{"cpu":12}},{"metrics":{"cpu":63}}] | map(.metrics.cpu)'
[12,63]

$ pwrq -n -c '"2026-08-13T12:00:00Z" | {doy: day_of_year, week: week_of_year}'
{"doy":225,"week":33}

$ pwrq -n -c '3.14159 | to_fixed(2)'
"3.14"
```

## Rolling windows, paths, words and money

```console
$ pwrq -n -c '[1,2,3,4,5] | windows(3)'
[[1,2,3],[2,3,4]]

$ pwrq -n -c '{a: {b: 1, c: 2}} | set_path("a.b"; 9) | get_path("a.b")'
9

$ pwrq -n -c '"2026-08-11T12:00:00Z" | to_timezone("Asia/Tokyo") | {DateTime, Abbreviation}'
{"Abbreviation":"JST","DateTime":"2026-08-11T21:00:00+09:00"}

$ pwrq -n -c '"11/08/2026" | parse_date("02/01/2006")'
"2026-08-11T00:00:00Z"

$ pwrq -n -c '1234567 | group_digits, 1234.5 | format_currency'
"1,234,567"
"$1,234.50"

$ pwrq -n -c '"a\tb\n1\t2" | tsv_parse'
[["a","b"],["1","2"]]

$ pwrq -n -c '93784 | iso_duration'
"P1DT2H3M4S"

$ pwrq -n -c 'net_present_value([-100, 50, 60]; 0.1)'
-4.95867768595042
```

## Archives

Reading an archive gives one object per entry, so the result flows into the
same filters as any other cmdlet output. Run from a directory holding `src/`:

```console
$ pwrq -nc 'compress_archive("src"; "release.zip") | .Name'
"release.zip"

$ pwrq -nc 'read_archive("release.zip") | map({Name, Length})'
[{"Length":0,"Name":"src/"},{"Length":40,"Name":"src/a.go"}]

$ pwrq -nc 'expand_archive("release.zip"; "./out") | map(split("/") | last)'
["a.go"]
```

`.tar`, `.tar.gz`/`.tgz` work the same way; `.tar.bz2` can be read but not
written. Extraction refuses an entry whose name would land outside the
destination directory.

## Searching a tree, and writing files

`select_string` reports where each match came from, which is what `grep_lines`
cannot:

```console
$ pwrq -nc 'select_string("src"; "TODO"; {Context: 1})
           | {LineNumber, Line, Before, After}'
{"After":["func main(){}"],"Before":["package main"],"Line":"// TODO: fix","LineNumber":2}
```

It streams one object per match, so `[...]` collects them and `first` stops
early — the walk only goes as far as what you read:

```console
$ pwrq -nc 'first(select_string("src"; "TODO")) | .Path'
"src/main.go"
```

`add_content` appends where `set_content` truncates:

```console
$ pwrq -nc '"first" | add_content("run.log") | .Length'
6
$ pwrq -nc '"second" | add_content("run.log") | .Length'
13
$ pwrq -nr 'cat("run.log")'
first
second
```

## Time zones

```console
$ pwrq -nc '"2026-08-11T12:00:00Z" | to_timezone("Asia/Tokyo")
           | {DateTime, Abbreviation, Offset}'
{"Abbreviation":"JST","DateTime":"2026-08-11T21:00:00+09:00","Offset":"+09:00"}

$ pwrq -nc '"2026-08-11T23:30:00Z" | format_date("date"; "Asia/Tokyo")'
"2026-08-12"

$ pwrq -nrc '"11/08/2026" | parse_date("02/01/2006")'
2026-08-11T00:00:00Z
```

The last one is why `parse_date` exists: jq can render an instant, but only the
layout you supply says whether `11/08` is August or November.

## Comparing two collections

```console
$ pwrq -nc 'compare_object(["a","b"]; ["b","c"])
           | map({v: .InputObject, s: .SideIndicator})'
[{"s":"<=","v":"a"},{"s":"=>","v":"c"}]

$ pwrq -nc 'compare_object([{id:1,v:"2.4.0"}]; [{id:1,v:"2.4.1"}];
                          {Property:"id", IncludeEqual:true})
           | map(.SideIndicator)'
["=="]
```

Matching on a property is what makes the second one report a match: the rows
differ, but they are the same row.

## Comparing byte strings

`rncd_compare` scores every pair in a corpus, so a collection of samples can be
sorted by what resembles what. Lower is more similar. The input is an array of
values — anything that casts to bytes — and each pair comes back as an object:

```console
$ pwrq -nc '["the quick brown fox jumps over the lazy dog",
             "the quick brown cat jumps over the lazy dog",
             "lorem ipsum dolor sit amet consectetur adipiscing"]
            | [rncd_compare] | sort_by(.Hybrid) | map({IndexA, IndexB, Hybrid})'
[{"Hybrid":0.136479,"IndexA":0,"IndexB":1},{"Hybrid":0.32082,"IndexA":1,"IndexB":2},
 {"Hybrid":0.340915,"IndexA":0,"IndexB":2}]
```

`IndexA` and `IndexB` are positions in the array you passed in. Give a value a
name and it is reported instead — an element can be an object carrying its
bytes under `Content` and its label under `Name`:

```console
$ pwrq -nc '[{Name: "cipher_a", Content: aes_encrypt(random_string(4000); "0123456789abcdef")},
             {Name: "cipher_b", Content: aes_encrypt(random_string(4000); "fedcba9876543210")},
             {Name: "prose",    Content: ("the cat sat on the mat. " * 170)}]
            | [rncd_compare] | sort_by(.Hybrid)
            | map({NameA, NameB, Ncd, EntropyGlobal, Hybrid})'
[{"EntropyGlobal":0.000042,"Hybrid":0.523936,"NameA":"cipher_a","NameB":"cipher_b","Ncd":0.987993},
 {"EntropyGlobal":0.362379,"Hybrid":0.654094,"NameA":"cipher_a","NameB":"prose","Ncd":1},
 {"EntropyGlobal":0.362337,"Hybrid":0.668635,"NameA":"cipher_b","NameB":"prose","Ncd":1}]
```

That is the whole argument for the blend. Two unrelated ciphertexts are both
incompressible, so `Ncd` alone (0.988 against 1.0) barely separates "two
encrypted blobs" from "an encrypted blob and a paragraph of English". The
entropy terms do: `EntropyGlobal` is 0.000042 for the two ciphertexts and 0.36
for either against the prose, and the `Hybrid` ordering follows.

`Hybrid` blends four distances, all in `[0, 1]` and all reported beside it:
`Ncd` — do the bytes compress well together — and `NcdFingerprint`,
`EntropyGlobal` and `EntropyProfile`, which ask whether these are the same
*kind* of thing.

```console
$ pwrq -nc 'rncd_compare([read_bytes("/usr/bin/cp"), read_bytes("/usr/bin/gzip")])
           | {Ncd, NcdFingerprint, EntropyGlobal, EntropyProfile, Hybrid}'
{"EntropyGlobal":0.019835,"EntropyProfile":0.133677,"Hybrid":0.710976,
 "Ncd":0.955143,"NcdFingerprint":0.856863}
```

`{Alpha: a, Beta: b}` moves the weight between them: `Alpha` is `Ncd`'s share,
`Beta` is `NcdFingerprint`'s, and what is left is split between the two entropy
terms. `{Alpha: 1, Beta: 0}` scores on bytes alone.

### Comparing files

Nothing here reads the disk, so a corpus of files is assembled in the query.
Read them with `read_bytes`, not with `cat`: `cat` decodes text, which is what
you want for logs and source but not for binaries, because it replaces every
byte that is not valid UTF-8. `read_bytes` does no decoding at all.

```console
$ pwrq -nc '[["cp", "mv", "cat", "gzip", "tar"][]
             | {Name: ., Content: read_bytes("/usr/bin/" + .)}]
           | [rncd_compare] | sort_by(.Hybrid) | .[0:3] | map({NameA, NameB, Ncd, Hybrid})'
[{"Hybrid":0.422155,"NameA":"cp","NameB":"mv","Ncd":0.42095},
 {"Hybrid":0.707604,"NameA":"mv","NameB":"gzip","Ncd":0.951845},
 {"Hybrid":0.710976,"NameA":"cp","NameB":"gzip","Ncd":0.955143}]
```

`cp` and `mv` are two builds of nearly the same program, and that is what the
score says. Any cmdlet that produces paths can feed the corpus, so filtering is
`get_childitem` and `select` rather than a second set of flags:

```console
$ pwrq -nc '[get_childitem("samples"; {Recurse: true, Filter: "*.bin"})
             | select(.Length > 4096) | {Name: .FullName, Content: read_bytes(.FullName)}]
           | [rncd_compare | select(.Hybrid < 0.4)] | length'
```

Pairs grow as N², so `rncd_compare` refuses a corpus above `MaxPairs` (100,000,
about 450 values) rather than filling memory with results. `{MaxPairs: 0}`
lifts the limit.

### Which bytes, exactly

`shared_chunks` answers the follow-up question — *which* bytes two values share:

```console
$ pwrq -nc 'shared_chunks("the quick brown fox"; "a quick brown fox indeed")'
{"Chunks":[{"End":3,"Length":3,"Matched":false,"PSTypeName":"Pwrq.SharedChunk",
            "RefOffset":null,"Start":0},
           {"End":19,"Length":16,"Matched":true,"PSTypeName":"Pwrq.SharedChunk",
            "RefOffset":1,"Start":3}],
 "Coverage":0.842105,"MatchedBytes":16,"MinMatch":16,"PSTypeName":"Pwrq.SharedChunks",
 "ReferenceLength":24,"Spans":1,"TargetLength":19}
```

Every matched chunk is a run of the target that occurs verbatim in the
reference at `RefOffset`; literal chunks have `RefOffset: null`. They tile the
target with no gaps, so `Coverage` is a fraction of the whole and a similarity
signal in its own right — and unlike the compression distances, an exact one.
`{MinMatch: n}` sets how long a run has to be to count: any two values share
four bytes somewhere, so the default of 16 is what separates structure from
coincidence.

```console
$ pwrq -nc 'read_bytes("/usr/bin/mv") | shared_chunks(read_bytes("/usr/bin/cp"))
           | {Coverage, MatchedBytes, Spans}'
{"Coverage":0.657682,"MatchedBytes":90597,"Spans":1593}

$ pwrq -nc 'read_bytes("/usr/bin/mv") | shared_chunks(read_bytes("/usr/bin/cp")).Chunks
           | map(select(.Matched)) | max_by(.Length)'
{"End":14615,"Length":2285,"Matched":true,"PSTypeName":"Pwrq.SharedChunk",
 "RefOffset":16426,"Start":12330}
```

Two thirds of `mv` occurs verbatim inside `cp`, in 1593 separate runs, the
longest of them 2285 bytes starting at offset 16426 of `cp` — and because the
offsets are exact, that claim is checkable with `dd` and `sha256sum`.

The input is the value being explained and the argument is what explains it, so
the piped form measures a stream of candidates against one reference:

```console
$ pwrq -nc '[["bzip2", "cat", "cp", "mv", "tar"][]
             | {Name: ., Coverage: (read_bytes("/usr/bin/" + .)
                                    | shared_chunks(read_bytes("/usr/bin/cp")).Coverage)}
             | select(.Coverage > 0.3)]'
[{"Coverage":0.371972,"Name":"bzip2"},{"Coverage":0.652372,"Name":"cat"},
 {"Coverage":1,"Name":"cp"},{"Coverage":0.657682,"Name":"mv"}]
```

## Censys Platform

These need credentials. `pwrq` reads the same environment variables the Censys
Platform documents, so a shell already set up for `censys` or for the Go SDK
works unchanged:

```console
$ export CENSYS_PLATFORM_TOKEN=... CENSYS_PLATFORM_ORGID=...
$ pwrq -c 'get_censys_context'
{"HasToken":true,"OrgIdSource":"CENSYS_PLATFORM_ORGID","OrganizationId":"…",
 "PSTypeName":"Censys.Platform.Context","ServerUrl":"https://api.platform.censys.io",
 "TimeoutSeconds":30,"TokenSource":"CENSYS_PLATFORM_TOKEN"}
```

`get_censys_context` never prints the token itself — query output ends up in
logs and scrollback.

Looking at one asset, the way `censys view` and `censys enrich` do:

```console
$ pwrq -c 'get_censys_host("1.1.1.1") | .resource | {ip, service_count}'
$ pwrq -c '"1.1.1.1" | get_censys_enrichment'
$ pwrq -c 'get_censys_host("1.1.1.1"; {AtTime: "2026-01-01T00:00:00Z"})'
$ pwrq -r 'get_censys_certificate($fp; {Raw: true})'   # the PEM text
```

Searching emits one object per hit, so jq's own verbs apply directly:

```console
$ pwrq -c '[search_censys("host.services.protocol=SSH")] | length'
$ pwrq -c '[search_censys("host.location.country=\"Chile\""; {Pages: 3})
           | .host_v1.resource.ip]'
$ pwrq -c 'get_censys_aggregate("host.services.port=443"; "host.location.country")
           | .buckets[0]'
```

A search costs credits per page, so it stops after one page unless you say
otherwise. `{Pages: 3}` fetches three, `{Pages: 0}` follows the cursor to the
end.

Cmdlets compose with the rest of pwrq, which is the point of having them here
rather than shelling out to `censys`:

```console
$ pwrq -c '[search_censys("host.services.port=8080")
           | .host_v1.resource.ip
           | select(is_public_ip)] | length'

$ pwrq -c '[search_censys("host.services.protocol=SSH")
           | .host_v1.resource.location.country]
          | value_counts'
```

Tagging what a search found, which is `censys tags assign` over a result set:

```console
$ pwrq -c '[search_censys("host.labels.value=\"c2\"")
           | .host_v1.resource.ip
           | add_censys_tag_assignment($tag)] | length'

$ pwrq -c '[get_censys_tag] | map({name, id})'
$ pwrq -c '[get_censys_tag_assignment($tag) | .asset_id]'
```

CensEye is asynchronous, so starting a job and reading it are two cmdlets
rather than one that blocks:

```console
$ pwrq -c 'new_censys_censeye_job("1.1.1.1") | .job_id'
$ pwrq -c 'get_censys_censeye_job($id) | .status'
$ pwrq -c '[get_censys_censeye_result($id)] | length'
```

Every cmdlet takes `{Token, OrganizationId, ServerUrl, Timeout}` as options, so
one query can reach two organizations:

```console
$ pwrq -c '[get_censys_credits({Scope: "user"}),
            get_censys_credits({OrganizationId: $other})]'
```

## Language models

These need a model and, for a hosted provider, a key. `PWRQ_LLM_MODEL` sets the
default, and the API key comes from the variable that vendor documents:

```console
$ export PWRQ_LLM_MODEL=anthropic/claude-sonnet-4-5 ANTHROPIC_API_KEY=...
$ pwrq -nc 'get_llm_context | {Model, Provider, HasApiKey, ApiKeySource, MaxCalls}'
{"ApiKeySource":"ANTHROPIC_API_KEY","HasApiKey":true,"MaxCalls":100,
 "Model":"anthropic/claude-sonnet-4-5","Provider":"anthropic"}
```

A local server works the same way, addressed by base URL. Anything speaking
OpenAI's chat completions API qualifies:

```console
$ export PWRQ_LLM_MODEL=openai-compatible/gemma-4-e2b-it-qat
$ export OPENAI_BASE_URL=http://127.0.0.1:1234/v1
$ pwrq -nc '[get_llm_model] | length'
13
$ pwrq -nr 'invoke_llm("Reply with exactly one word: blue")'
blue
```

The prompt is jq, so the pipeline is the template:

```console
$ pwrq -c 'map(invoke_llm("One word summary of: \(.)"))' <<< '["a book about bees","a film about cars"]'
["Pollination","Drive"]
```

### Typed answers

A schema turns prose into rows:

```console
$ pwrq -c '[.[] | invoke_llm("Classify the sentiment of this review: \(.text)";
    {Schema: {type: "object",
              properties: {sentiment: {type: "string", enum: ["positive","negative","neutral"]}},
              required: ["sentiment"]}})
   | .sentiment]' <<< '[{"text":"best thing I ever bought"},{"text":"broke after one day"},{"text":"it arrived Tuesday"}]'
["positive","negative","neutral"]
```

Because the answer is a value rather than text, the rest of the query is
ordinary jq:

```console
$ pwrq -c '[.[] | invoke_llm("Extract {name, org} from: \(.)"; {Schema: $S})]
           | group_by(.org) | map({org: .[0].org, people: map(.name)})'
```

### Many prompts at once

`map(invoke_llm(...))` is one round trip per row, in sequence. Batching runs a
bounded pool and keeps input order:

```console
$ pwrq -c '[.[] | "One word summary of: \(.)"] | [invoke_llm_batch({Parallel: 6})] | map(.Content)'
```

Against a local server that is roughly twice as fast for six prompts; against a
hosted API, where each call is a network round trip, the difference is larger.
A failure fails the whole call, unless you ask to see the failures in band:

```console
$ pwrq -c '[invoke_llm_batch($prompts; {Parallel: 8, ContinueOnError: true})]
           | map(select(.Error != null) | {Index, Error})'
```

### What it cost

```console
$ pwrq -nc '[invoke_llm_batch(["say a","say b"])] | length as $n | get_llm_usage'
{"CacheHits":0,"Calls":2,"Cost":null,"InputTokens":36,"OutputTokens":389,
 "PSTypeName":"Pwrq.LLM.Usage","TotalTokens":425}
```

`Cost` needs rates, in dollars per million tokens — pwrq does not ship a price
table that would go stale:

```console
$ pwrq -nc 'invoke_llm_request("hi"; {PriceInput: 3, PriceOutput: 15}) | {TotalTokens, Cost}'
```

While building a pipeline, cache the answers so re-running is free:

```console
$ pwrq -nc 'invoke_llm_request("expensive question"; {Cache: true}) | .Cached'
false
$ pwrq -nc 'invoke_llm_request("expensive question"; {Cache: true}) | .Cached'
true
```

### Semantic search

```console
$ pwrq -nc '["how to bake sourdough bread", "the history of naval warfare", "training a puppy to sit"] as $docs
  | {Model: "openai-compatible/text-embedding-nomic-embed-text-v1.5"} as $m
  | invoke_embedding($docs; $m) as $vectors
  | invoke_embedding("my dog wont listen to commands"; $m) as $q
  | [range(0; $docs|length) | {doc: $docs[.], score: (cosine_similarity($vectors[.]; $q) * 1000 | round / 1000)}]
  | sort_by(-.score)'
[{"doc":"training a puppy to sit","score":0.633},
 {"doc":"how to bake sourdough bread","score":0.385},
 {"doc":"the history of naval warfare","score":0.335}]
```

## Agents

`invoke_agent` answers a task by writing pwrq queries until it can:

```console
$ pwrq -nc 'invoke_agent_request("Which file in the current directory is largest, and how many bytes is it?")
           | {Content, Queries: [.Steps[] | .Query]}'
{"Content":"pwrq-viz 35913993",
 "Queries":["[get_childitem(\".\")] | map(select(.Name != \".\"))",
            "[get_childitem(\".\")] | map(select(.PSTypeName == \"System.IO.FileInfo\")) | top_by(\"Length\"; 1) | .[0] | {Name, Length}",
            null]}
```

The trace is the point: the second query above failed with `expected an object
but got: array`, the model read the error and fixed it, and `.Steps` is where a
reader sees that happen rather than taking the answer on faith.

Piping data in puts it under `.` in every query the agent writes:

```console
$ pwrq -c 'invoke_agent("Which record has the largest Bytes value? Answer with just its Name.")' \
      <<< '[{"Name":"alpha","Bytes":120},{"Name":"beta","Bytes":9800}]'
"beta"
```

What the agent may call is an allowlist, and the default is read-only:

```console
$ pwrq -nc 'invoke_agent("count the TODOs in this tree";
             {Allow: ["select_string", "get_childitem", "cat"], MaxSteps: 6})'
```

Asking for something outside it is refused before the run starts, and the model
cmdlets can never be in it:

```console
$ pwrq -nc 'invoke_agent("x"; {Allow: ["invoke_llm"]})'
pwrq: invoke_agent: "invoke_llm" cannot be in Allow; an agent that can call a
model can spend without limit
```

Small models need the room: `gemma-4-e2b` answers a one-query question, and a
12B model recovers from its own mistakes over several steps. When a run does not
converge, `PWRQ_LLM_DEBUG=1` prints every request and reply to stderr.

### A whole run, end to end

[`examples/agent-triage.sh`](examples/agent-triage.sh) is the four stages
together, over a corpus it writes itself so the run is reproducible:

1. **Find the errors** — `select_string` over the log files. No model involved;
   finding lines is a job the cmdlets already do.
2. **Classify them** — `invoke_llm_batch` with a `Schema`, one call per line, in
   parallel. This is the stage that turns text into rows.
3. **Summarise** — `group_by`, `map`, `sort_by`. Plain jq, because by now the
   model's answers are values.
4. **Ask about them** — `invoke_agent_request`, with the triaged rows piped in,
   writing its own queries against them.

```console
$ export PWRQ_LLM_MODEL=openai-compatible/gemma-4-e2b-it-qat
$ export OPENAI_BASE_URL=http://127.0.0.1:1234/v1
$ examples/agent-triage.sh
== 1. the errors on disk ==================================================
6 error lines

== 2. classified, one call per line, in parallel ==========================
  File       Line category severity
  ---------- ---- -------- --------
  api.log    2    data     high
  api.log    4    timeout  high
  auth.log   2    auth     high
  worker.log 2    crash    high
  worker.log 3    data     high
  worker.log 4    crash    high

== 3. summarised with plain jq ===========================================
  category count files
  -------- ----- -------------------
  crash    2     worker.log
  data     2     api.log, worker.log
  auth     1     auth.log
  timeout  1     api.log

== 4. the agent, asked about the same data ===============================
{"Answer":"The file with the most high-severity errors is worker.log, which
 contains panics due to index out of range and nil pointer dereferences, as well
 as JSON unmarshalling errors.",
 "Queries":["[.[] | select(.severity == \"high\")] | group_by(.File) | map({File: .[0].File, count: length}) | sort_by(-.count) | .[0] | .File",
            "[.[] | select(.File == \"worker.log\" and .severity == \"high\")] | .Text | unique",
            "[.[] | select(.File == \"worker.log\" and .severity == \"high\")] | map(.Text) | unique | join(\", \")"],
 "Tokens":13212}
```

The middle query is the loop earning its keep: `.Text` on an array is an error,
the agent read the message and rewrote it as `map(.Text)`. That is what `.Steps`
is for — the answer above rests on three queries, and all three are here to be
checked.

Stage 4 asks more of a model than stages 2 and 3 do: classifying one line is a
single judgement, while writing a query, reading its result and deciding what to
do next is a loop. `gemma-4-e2b` does the first well and the second badly — it
writes queries that paste the data in as a literal instead of using `.` — so the
script takes `PWRQ_AGENT_MODEL` to run that stage on a larger model. The output
above is the 12B one; the classification is the 2B one.

## Aliases

```console
$ pwrq -c '[gci(".")] | length'   # gci, dir, gi -> get_childitem
$ pwrq -c '[gps] | length'        # gps -> get_process
$ pwrq -c 'gd | .Year'            # gd  -> get_date
```

`pwrq --udf-list` prints every function and alias.
