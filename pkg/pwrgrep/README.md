# Rules

A rule is a pwrq query that searches code by its syntax and says what it found.
`select_ast` answers one question — where does this piece of syntax occur — and
that is rarely a finding on its own. "MD5, but only in a file that imports
crypto/md5." "Assigning to innerHTML, but not a string literal." "A call to
Query, but not one whose argument is a constant." The patterns are the easy
half; the combining is where a rule lives.

Because a rule is a query and nothing else, everything below is a query too.
Listing the catalogue, reading a rule, running one, checking it against a
fixture and writing a new one are all `pwrq -n '...'`, which means they are all
one `run_query` call over MCP as well. There is no rule format to learn, no
loader to configure, and nothing to rebuild.

    $ echo src | pwrq -R 'invoke_pwrgrep("go-weak-hash")'
    $ pwrq -n '[invoke_pwrgrep("src"; "go/lang/security")] | group_by(.RuleId)'
    $ pwrq -n '[get_pwrgrep_rule("python")] | map(.Id)'

## Listing what is there

`get_pwrgrep_rule` is the catalogue: one object per rule, with no argument for
all of them and a selector for some.

```console
$ pwrq -n '[get_pwrgrep_rule] | length'
1815
$ pwrq -n '[get_pwrgrep_rule] | map(.Languages[]?) | unique | length'
25
```

A selector is a finding id, a glob over ids, a path into the catalogue — a rule
file or a directory of them — or the name of a language. Naming a language is
usually what you want, and it is not the same as any of the others:

```console
$ pwrq -n '[get_pwrgrep_rule("go")] | length'
82
$ pwrq -n '[get_pwrgrep_rule("typescript")] | length'
176
$ pwrq -n '[get_pwrgrep_rule("go/lang/security/audit/crypto")] | map(.Path)'
```

Reach for the language rather than a glob over ids. Ids are not prefixed with
the language they are about, so `"python-*"` is a glob matching the handful
that happen to begin that way — it returns six rules of the 336 and no
complaint, which looks exactly like a clean answer. And the directory a rule
sits in is where it was ported from rather than what it is about: most of the
TypeScript rules are under `javascript/`, because that is the pack they came
from, so `"typescript/"` as a path finds 28 of the 176 and says nothing about
the rest. The language is what the rule declares in its header.

A selector that matches nothing is an error rather than an empty list. "No rule
called that" is a typo, and reporting it as "nothing found" hands back a clean
bill of health nobody earned.

The catalogue is a value, so narrowing it further is the next stage of the
pipeline rather than an option on the call:

```console
$ pwrq -nc '[get_pwrgrep_rule("go")] | map(select(.Fixture != "")) | map(.Path)'
["go/lang/correctness/go-hardcoded-if-condition",
 "go/lang/correctness/go-useless-comparison",
 "go/lang/security/audit/crypto/go-weak-cipher",
 "go/lang/security/audit/crypto/go-weak-hash",
 "go/lang/security/audit/sqli/go-sql-string-concat",
 "go/lang/security/go-tmpfile-predictable",
 "problem-based-packs/insecure-transport/go-stdlib/go-tls-skip-verify"]
$ pwrq -nc '[get_pwrgrep_rule] | map(select(.Origin != "<built in>")) | map(.Path)'
[]
```

The first is the Go rules that carry a fixture, which are the ones to read
first: each was written by hand against an annotated file, and the prose in the
header says where it departs from the rule it was modelled on. The second is
the rules on this machine that did not ship with pwrq — yours, and anything you
have overridden. Empty, until you write one.

## Reading one

`Query` is the rule. It is published rather than summarised because reading it
is how you decide whether it asks what you wanted, and copying it is how you
write the rule nobody shipped.

```console
$ pwrq -nr 'get_pwrgrep_rule("go-weak-hash") | .Query'
# rules: go-weak-hash
# languages: go
# fixture: go/weak-hash.go
# from: go/lang/security/audit/crypto/use_of_weak_crypto.yaml (use-of-md5, use-of-sha1)
#
# CWE-328, use of a hash that is not collision resistant.
#
# The original pairs each call with `pattern-inside: import "crypto/md5"`, and
# that is the interesting half of the port. Written as within() it finds
# nothing: a call is not inside an import statement, it is in the same file as
# one. So the import becomes a second pattern and the two are joined by file.
...
```

`Origin` says whether the copy you are looking at is one you can edit: a rule
from a directory can be changed where it sits, and `<built in>` has to be
copied out first. `From` says what it was translated from, `Fixture` names the
file it is checked against, and `Description` is the prose in the header —
which is where a rule says what it does not cover and why a pattern is written
the way it is.

One id can name several rules. It names a *finding*, and the same finding gets
ported once per language:

```console
$ pwrq -nc 'get_pwrgrep_rule("cookie-missing-httponly") | {Path, Languages}'
{"Path":"go/lang/security/audit/net/cookie-missing-httponly","Languages":["go"]}
{"Path":"java/lang/security/audit/cookie-missing-httponly","Languages":["java"]}
{"Path":"kotlin/lang/security/cookie-missing-httponly","Languages":["kotlin"]}
```

Naming the id runs all three, which is what a caller asking for it meant.
`Path` is what tells them apart afterwards.

## Running them

`invoke_pwrgrep(root; rules)` takes the tree and the same selector.

```console
$ pwrq -n '[invoke_pwrgrep("pkg/pwrgrep/testdata/fixtures/go"; "go-weak-hash")]
           | map({Path, LineNumber, Match})'
[
  {"Path":".../weak-hash.go","LineNumber":12,"Match":"md5.New()"},
  {"Path":".../weak-hash.go","LineNumber":15,"Match":"md5.Sum(data)"},
  {"Path":".../weak-hash.go","LineNumber":20,"Match":"sha1.Sum(data)"}
]
```

A finding carries `RuleId`, `Path`, `LineNumber`, `Column`, `EndLineNumber`,
`Message` and `Match` — the source text it matched, spanning every line it
covers. It is an ordinary value, so summarising a run, or dropping vendored
code, is the next stage of the pipeline:

```console
$ pwrq -n '[invoke_pwrgrep("/tmp/gin"; "go")]
           | group_by(.RuleId) | map({rule: .[0].RuleId, hits: length}) | sort_by(-.hits)'
[
  {"rule":"go-tls-skip-verify","hits":4},
  {"rule":"missing-ssl-minversion","hits":4},
  {"rule":"use-of-unsafe-block","hits":4},
  {"rule":"cookie-missing-httponly","hits":1},
  ...
]
$ pwrq -n '[invoke_pwrgrep("."; "javascript")]
           | map(select(.Path | test("(min\\.js|/libs/|node_modules)") | not))'
```

### What a run costs

Each rule is a separate walk of the tree. Nothing outside a rule knows which
files it will ask for, so nothing can share a walk between two of them — that
is the price of a rule being a query rather than a description, and it is the
right way round, because a rule you can read and edit is worth more than one
that is cheap to schedule. Within one rule the cost is flat: `scan_ast` parses
each file once however many patterns it is given, so a rule with seven
alternatives costs what a rule with one costs. That is why rules are written
with one `scan_ast` and several patterns rather than the other way round.

In practice the 82 Go rules over gin — about 30k lines — take some fifty
seconds. A whole-language run over a large Python or JavaScript tree runs to
minutes. **Over MCP this matters**, because `run_query` defaults to a 30-second
timeout: pass `timeoutMs` for anything wider than a directory, or name a
narrower selector.

Naming a directory of the catalogue rather than all of it is what keeps a run
honest. `invoke_pwrgrep("."; "go/lang/security")` over a Go repository, not the
whole 1815.

## Where a rule of your own goes

The corpus is [pwrgrep-rules](https://github.com/xen0bit/pwrgrep-rules), a
module this package depends on. It is built into the binary from there and
installed to `/usr/share/pwrq/rules` by the package. pwrq looks for rules in,
in order:

1. every directory in `$PWRQ_RULES`, separated the way `PATH` is
2. `~/.config/pwrq/rules`
3. `/usr/share/pwrq/rules`
4. the copy inside the binary

A rule found earlier hides one with the same path found later, so changing a
shipped rule is copying it into your own directory under the same path and
editing it there. Adding one is dropping a file in. `get_pwrgrep_rule` reports
in `Origin` which of the four a rule came from.

The catalogue is re-read whenever those directories change, so a rule written
now is a rule this process can run now — which matters for the MCP server,
where one process answers for hours and there is no restart between writing a
file and asking for it.

## Writing one

A rule is a file, and this is the whole of it:

    # rules: go-weak-hash
    # languages: go
    # from: go/lang/security/audit/crypto/use_of_weak_crypto.yaml

    ["md5.New()", "md5.Sum($$$A)"] as $calls
    | ["\"crypto/md5\""] as $imports
    | scan_ast("*.go"; $calls + $imports) as $all
    | ($all | of($calls) | in_files_with($all | of($imports)))
    | finding("go-weak-hash";
        "this hash is not collision resistant; SHA-256 or SHA-3 instead")
    | report

`# rules:` names the finding ids the file reports under, which is what a caller
asks for; a file may hold several, because rules that search the same files
share one walk of the tree. `# languages:` names the grammars the rule needs in
the build, which is what `TestEveryRuleLanguageIsInTheBuild` checks the
Makefile against: a rule written for a grammar the binary does not carry is a
rule that can never fire. `# from:` says what it was translated from, if it
was, and `# fixture:` names a file it is checked against. Anything else in the
header block is prose, and comes back as `Description`.

Everything below the header is ordinary pwrq.

### From a shell

Copy the nearest rule, change it, run it:

```console
$ mkdir -p ~/.config/pwrq/rules/mine
$ pwrq -nr 'get_pwrgrep_rule("go-weak-hash") | .Query' > ~/.config/pwrq/rules/mine/no-timeout.pwrq
$ $EDITOR ~/.config/pwrq/rules/mine/no-timeout.pwrq
$ pwrq -n '[invoke_pwrgrep("src"; "mine/no-timeout")]'
```

Change the `# rules:` header while you are in there. It is the id the findings
carry and the name a caller asks for, and a copy that keeps the original's id
reports under it — two rules answering to `go-weak-hash`, both running whenever
anybody names it.

### From the MCP server

There is no shell there, so `write_pwrgrep_rule(name; source)` is the same
three steps in one call. It works out the directory — `$PWRQ_RULES` or the one
under the config directory — creates it if it is not there, and reports where
the file went:

```console
$ pwrq -nc 'write_pwrgrep_rule("mine/no-timeout"; "# rules: mine-no-timeout\n# languages: go\n\nscan_ast(\"*.go\"; [\"&http.Client{}\"])\n| finding(\"mine-no-timeout\"; \"this client has no Timeout\")\n| report\n")'
{"File":"/home/you/.config/pwrq/rules/mine/no-timeout.pwrq","Id":"mine-no-timeout",
 "Ids":["mine-no-timeout"],"Languages":["go"],
 "Origin":"/home/you/.config/pwrq/rules","Path":"mine/no-timeout",
 "PwrqType":"Pwrq.PwrgrepWrittenRule",
 "PwrqValue":"/home/you/.config/pwrq/rules/mine/no-timeout.pwrq"}
```

`Path` is what `invoke_pwrgrep` and `get_pwrgrep_rule` name it by from then on;
`File` is what to edit next. The name is a place in the catalogue — `.pwrq` is
optional, and an absolute path or one that climbs out of the directory is
refused, because a rule name is not a filesystem path.

The rule is live in the same process the moment it lands, so writing and
running it is one pipeline:

```console
$ pwrq -nc 'write_pwrgrep_rule("mine/no-timeout"; $source)
            | [invoke_pwrgrep("src"; .Path)] | map({Path, LineNumber})'
```

Writing over an existing rule is allowed and is what iterating on one looks
like. What is refused is anything that is not a rule: the header is read and
the query is compiled before a byte is written.

```console
$ pwrq -n 'write_pwrgrep_rule("mine/broken"; "# rules: broke\n\nscan_ast(\"*.go\" [\"x\"]) | report\n")'
pwrq: write_pwrgrep_rule: mine/broken: function not defined: scan_ast/1
```

That check is not politeness. A file in a rules directory with no `# rules:`
header, or with a query that does not compile, does not make a rule that fails
to fire — it makes the catalogue unreadable, and the next `invoke_pwrgrep` for
*any* rule at all comes back with an error about a file you wrote an hour ago.
Refusing the write is how that stays a failure of the write. (Which is also why
writing a rule with `set_content` is worth avoiding: it will happily put
anything on disk.)

## Iterating

A pattern that is not code in the language it is for still compiles, and then
matches nothing in silence — a typo and an honest absence look identical from
the outside. `ast_pattern` is what tells them apart, and it is the first thing
to reach for when a rule comes back empty:

```console
$ pwrq -nc '"&http.Client{}" | ast_pattern("go") | {Valid, Problem, MetaVariables}'
{"Valid":true,"Problem":"","MetaVariables":[]}
$ pwrq -nc '"&http.Clientt{" | ast_pattern("go") | {Valid, Problem}'
{"Valid":false,"Problem":"pattern \"&http.Clientt{\" does not parse as go code, so it
 can never match; write the pattern as code you could compile, with $NAME where a value
 varies and $$$NAME where a list does"}
```

Below that, `scan_ast` on its own is the pattern without the rule around it —
run it against a file you know should match before wiring up the guards:

```console
$ pwrq -nc '"/tmp/gin" | scan_ast("*.go"; ["&tls.Config{$$$A}"]) | map({Path, LineNumber})'
```

Over MCP, `validate_query` takes the whole rule source and tells you whether it
compiles without running it, which is cheap and is where to catch a wrong
arity or a misspelled cmdlet.

### Checking a rule against a fixture

A fixture is a source file annotated the way the rules being translated are
tested: `// ruleid: <id>` says the next line must produce a finding, and
`// ok: <id>` says it must not.

```go
func fingerprint(data []byte) string {
	// ruleid: go-weak-hash
	h := md5.New()
	...
}

func digest(data []byte) string {
	// ok: go-weak-hash
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
```

For the shipped corpus `TestEveryRuleWithAFixtureFindsExactlyWhatItMarks` does
the comparing. For a rule of your own it is a query, because the annotations
are text and pwrq searches text:

```console
$ pwrq -nc '
  "pkg/pwrgrep/testdata/fixtures/go" as $dir | "go-weak-hash" as $rule
  | ([$dir | scan_regex("*.go"; ["ruleid:\\s*" + $rule])[] | .LineNumber + 1] | sort) as $marked
  | ([invoke_pwrgrep($dir; $rule) | .LineNumber] | unique) as $fired
  | {marked: $marked, fired: $fired, missed: ($marked - $fired), extra: ($fired - $marked)}'
{"marked":[12,15,20],"fired":[12,15,20],"missed":[],"extra":[]}
```

`missed` is what the rule should have found and did not; `extra` is what it
found that nobody marked. Both empty is the rule passing.

## The vocabulary

`scan_ast`, `scan_regex`, `of`, `within`, `outside`, `not_at`, `at_same_place`,
`in_files_with`, `in_files_without`, `where_capture`, `where_capture_not`,
`where_capture_ast`, `where_capture_compare`, `where_capture_entropy`,
`where_capture_redos`, `where_text`, `where_text_not`, `where_same`,
`where_different`, `focus`, `reaching`, `finding` and `report` are cmdlets like
any other — `pwrq --udf-list`, or `list_functions` with a filter of
`"Code Rules"`, documents them, and a rule can use the rest of the language
freely.

Patterns are code with holes in them. `$NAME` stands for one node, `$$$NAME`
for a list of them, and `$_` and `$$$_` are the anonymous versions for when a
pattern declines to name what it does not care about. A hole written twice
means the same code twice, so `$X == $X` does not match `a == b`.

### The two readings of "inside"

Half the corpus writes a guard meaning "in a file that imports this" and half
means "inside this span". They are different operators and getting them the
wrong way round produces no findings and no error:

- `within` is a real span containing a real span — inside this function, inside
  this loop.
- `in_files_with` is "the same file also matched that", which is what an import
  guard means. A call is not inside an import statement, so `within` would find
  nothing.

## Where the corpus lives

In [pwrgrep-rules](https://github.com/xen0bit/pwrgrep-rules), which this
package depends on as a Go module and embeds. A rule fix is a change there, not
a release of the engine.

It is a module rather than a git submodule for a concrete reason: a submodule's
contents are not in the zip the module proxy serves, so `go install pwrq@latest`
would fetch a tree with an empty rules directory and fail at the embed — as
would any clone that forgot `--recurse-submodules`. As a dependency it is
fetched like any other and pinned in `go.sum`.

Editing a rule against a checkout is what `PWRQ_RULES` is for:

    PWRQ_RULES=../pwrgrep-rules/rules pwrq -n '[invoke_pwrgrep("."; "go/lang/security")]'

That repository's `tools/validate.py` runs the whole corpus against every
fixture on every change, which is where a rule is shown to fire on the lines it
claims. This side keeps the two checks the corpus repository cannot make:
`TestEveryRuleCompiles`, because what compiles is decided by this binary's
cmdlet vocabulary, and `TestEveryRuleLanguageIsInTheBuild`, because which
grammars ship is decided by the Makefile here.

## Where the corpus came from

Most of it was translated from
[opengrep-rules](https://github.com/opengrep/opengrep-rules) by the translator
in that repository's `gen/`, which is kept as provenance and is no longer run.
`gen/MANIFEST.json` accounts for every rule it was given — translated, or
listed with the reason it was not — and `gen/VALIDATION.json` records how each
translated rule scored against the annotated fixture its original was tested
with.

1891 of the 2006 rules it was given were translated, in 26 languages, and 1442
of those had a fixture upstream marking a line for them. Of those, 622 found
exactly what the fixture marks and 255 found some of it. The rest are in
`VALIDATION.json` with their score, which is the point of the file: a rule that
runs and finds nothing is worth knowing about, and a corpus that does not say
so is a list of claims.

Eighteen were written by hand rather than translated, one per language family,
each with a fixture of its own and prose saying where it departs from the rule
it came from. They came first and are what the translator was written against.
They are no longer kept in a directory apart; they sit in the category they
search, named for the id they report under.

## What does not translate

Some of this is pwrq's and some is the source vocabulary's, and it is worth
knowing before writing a rule. `gen/MANIFEST.json` names every rule that was
refused and why; this is what the reasons mean.

- **Join mode.** `from`/`to` composes the findings of two rules by a shared
  value, which is a query over results rather than over code. Nothing here does
  it, and the rules written that way are refused rather than approximated.

- **Type inference.** `(HttpServletRequest $REQ)` says "a variable whose type
  is that", which needs the program resolved and not just parsed. pwrq matches
  what is written.

- **Import resolution.** `new java.util.Random()` does not match `new Random()`
  here, for the same reason. A rule names the spelling that appears in the file
  and will miss a class imported under an alias. Grouped Go imports are the
  same shape: `import "crypto/md5"` does not match the spec inside `import (
  ... )`, so guards match the import path string instead, which finds both.

- **A method reference is not a call.** `Digest::SHA1.$FUNC` is a reference;
  the call has the arguments written out.

- **Lookaround.** A hole's regex is RE2, which has none. A negative lookahead
  at the front is not really lookaround — it is a negation, and it becomes
  `where_capture_not` — but one in the middle of a regex is, and a rule needing
  that is refused rather than translated with the constraint quietly dropped.

Three things that used to be on this list are not any more, and the shape of
the fix is the same in all three: the pattern is read where it could have been
written.

- **Taint mode** is `reaching`, an intraprocedural syntactic following: a value
  is followed through assignments in its own scope from a source to a sink, and
  a sanitizer on the way stops it. It is not a dataflow analysis and does not
  cross a function boundary.

- **PHP** works because a pattern gets an opener. A PHP file is HTML until
  `<?php`, so `md5($X)` parses as literal text; compiled after the tag it is
  the call, and the tag is then taken back off the query. The same machinery
  puts a statement inside a method for Java and a member inside a contract for
  Solidity — see `scaffolds` in `pkg/udf/astsearch/pattern.go`.

- **`focus-metavariable`** is `focus`, which moves the finding to what a named
  hole caught.

## What a rule is searched with

A rule is one of three things, and the corpus repository's `gen/MANIFEST.json`
says which:

- a **search over syntax**, which is most of them: `scan_ast` over the files of
  the languages the rule declares.
- a **search over text**, for the rules whose language is `regex` or `generic`,
  and for the ones whose patterns are not code in any grammar. Dockerfile is
  the case that matters: `EXPOSE $PORT` is a line, `$` is a shell variable to
  the grammar rather than a hole, and there is no parse of that pattern that
  means what the rule means. As a regex over the same files it says exactly
  what it said.
- a **value followed**, for the taint rules.

A text reading is looser than a parse — a `...` there crosses a boundary a
parse would not — so it is only ever reached after the syntax reading has been
refused, and `gen/VALIDATION.json` is what says whether the rule still finds
what it was written to find.

## Changing a rule

In [pwrgrep-rules](https://github.com/xen0bit/pwrgrep-rules), and then here:

    # in ../pwrgrep-rules
    $EDITOR rules/go/lang/security/audit/crypto/go-weak-hash.pwrq
    PWRQ=../pwrq/pwrq tools/validate.py

    # in pwrq, once that is released
    go get -u github.com/xen0bit/pwrgrep-rules
    go test ./pkg/pwrgrep/
