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
$ pwrq -n -c '100 | c_to_f'
212

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

The regex cmdlets are named to keep clear of jq's own `test`/`match`/`scan`:

```console
$ pwrq -n -c '"a1 b22 c333" | regex_find_all("[0-9]+")'
["1","22","333"]

$ pwrq -n -c '"user=42" | regex_extract_first("user=(\\d+)"; 1)'
"42"

$ pwrq -n -c '"a,b;c" | regex_split("[,;]")'
["a","b","c"]

$ pwrq -n -c '"café" | remove_accents'
"cafe"
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

$ pwrq -n -c '"a=1 b=22" | regex_groups("(\\w+)=(\\d+)")'
[["a","1"],["b","22"]]

$ pwrq -n -c '{"first_name":"ada","role":"admin"} | rename_keys({"first_name": "name"})'
{"name":"ada","role":"admin"}

$ pwrq -n -c '[{"metrics":{"cpu":12}},{"metrics":{"cpu":63}}] | pluck("metrics.cpu")'
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

$ pwrq -n -c '1234 | to_words'
"one thousand two hundred thirty-four"

$ pwrq -n -c '2026 | roman_numeral'
"MMXXVI"

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

## Aliases

```console
$ pwrq -c '[gci(".")] | length'   # gci, dir, gi -> get_childitem
$ pwrq -c '[gps] | length'        # gps -> get_process
$ pwrq -c 'gd | .Year'            # gd  -> get_date
```

`pwrq --udf-list` prints every function and alias.
