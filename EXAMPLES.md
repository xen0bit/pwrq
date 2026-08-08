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

## Aliases

```console
$ pwrq -c '[gci(".")] | length'   # gci, dir, gi -> get_childitem
$ pwrq -c '[gps] | length'        # gps -> get_process
$ pwrq -c 'gd | .Year'            # gd  -> get_date
```

`pwrq --udf-list` prints every function and alias.
