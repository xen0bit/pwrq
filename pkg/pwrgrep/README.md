# Rules

A rule is a pwrq query that searches code by its syntax and says what it found.
`select_ast` answers one question — where does this piece of syntax occur — and
that is rarely an answer on its own. "Every function, and the type a method
hangs off." "An environment variable, but only where it reaches
`exec.Command`." "A catch block, but only an empty one." The patterns are the
easy half; the combining is where a rule lives.

Because a rule is a query and nothing else, everything below is a query too.
Listing the catalogue, reading a rule, running one, checking it against a
fixture and writing a new one are all `pwrq -n '...'`, which means they are all
one `run_query` call over MCP as well. There is no rule format to learn, no
loader to configure, and nothing to rebuild.

    $ echo src | pwrq -R 'invoke_pwrgrep("go-functions")'
    $ pwrq -n '[invoke_pwrgrep("src"; "go/flow")] | group_by(.RuleId)'
    $ pwrq -n '[get_pwrgrep_rule("python")] | map(.Id)'

## What ships

The rules built into pwrq are for reading a codebase, not judging it. Each of
Go, Python, JavaScript/TypeScript, Java, C and C# has the same eleven, under
the prefix `go`, `python`, `js`, `java`, `c` or `cs`:

| Rule | Directory | What it reports |
| --- | --- | --- |
| `<lang>-functions` | `inventory/` | every function and method, by name |
| `<lang>-types` | `inventory/` | every class, struct, interface, enum and alias |
| `<lang>-variables` | `inventory/` | every variable and constant declared |
| `<lang>-imports` | `inventory/` | every module, package or header brought in |
| `<lang>-calls` | `inventory/` | every call, by what was called |
| `<lang>-entry-points` | `entrypoints/` | `main`, HTTP routes, CLI commands, tests, callbacks |
| `<lang>-external-input` | `flow/` | where values come in: env, args, stdin, files, requests |
| `<lang>-side-effects` | `flow/` | where the program acts: commands, files, network, SQL |
| `<lang>-input-reaches-effect` | `flow/` | an input that arrives, by assignment, at an effect |
| `<lang>-concurrency` | `hotspots/` | threads, goroutines, tasks, channels, locks |
| `<lang>-swallowed-errors` | `hotspots/` | errors caught or discarded and left there |

and `generic/todo-comments` reads the TODO, FIXME, HACK, XXX and BUG notes in a
file of any language.

The findings are meant to be piped, not read top to bottom. A package's table
of contents, the routes a service answers, what the outside world can steer:

```console
$ pwrq -n '[invoke_pwrgrep("src"; "go-functions")] | group_by(.Path) | map({(.[0].Path): map(.Message)}) | add'
$ pwrq -n '[invoke_pwrgrep("src"; "js-entry-points")] | map(select(.Message | startswith("HTTP")))'
$ pwrq -n '[invoke_pwrgrep("."; "python-input-reaches-effect")] | map({Path, LineNumber, Message})'
[
  {"Path":"python/input-reaches-effect.py","LineNumber":12,
   "Message":"external input reaches this call's arguments (full)"},
  {"Path":"python/input-reaches-effect.py","LineNumber":16,
   "Message":"external input reaches this call's arguments ([\"ls\", sys.argv[1]])"},
  ...
]
```

The `flow/` rules are one idea in three parts. `external-input` lists the
sources, `side-effects` lists the sinks, and `input-reaches-effect` joins them
with `reaching`, which follows a value through assignments inside one function.
It does not follow a value into a helper and back out, so an empty answer means
no path was found, not that none exists.

A security corpus — some two thousand rules, most translated from semgrep's —
lives apart in [pwrgrep-rules](https://github.com/xen0bit/pwrgrep-rules). It is
not built in; `PWRQ_RULES` puts it beside these (see below).

## Listing what is there

`get_pwrgrep_rule` is the catalogue: one object per rule, with no argument for
all of them and a selector for some.

```console
$ pwrq -n '[get_pwrgrep_rule] | length'
67
$ pwrq -n '[get_pwrgrep_rule("go")] | map(.Id)'
["go-entry-points","go-external-input","go-input-reaches-effect","go-side-effects",
 "go-concurrency","go-swallowed-errors","go-calls","go-functions","go-imports",
 "go-types","go-variables"]
$ pwrq -n '[get_pwrgrep_rule("go/flow")] | map(.Path)'
["go/flow/external-input","go/flow/input-reaches-effect","go/flow/side-effects"]
```

A selector is a finding id, a glob over ids, a path into the catalogue — a rule
file or a directory of them — or the name of a language. The language is what
a rule declares in its `# languages:` header rather than where it is filed, so
naming one finds rules of your own too, whatever you called them. The
JavaScript rules declare `javascript`, `typescript` and `tsx`, and answer to all
three.

A selector that matches nothing is an error rather than an empty list. "No rule
called that" is a typo, and reporting it as "nothing found" hands back a clean
bill of health nobody earned.

The catalogue is a value, so narrowing it further is the next stage of the
pipeline rather than an option on the call:

```console
$ pwrq -nc '[get_pwrgrep_rule] | map(select(.Origin != "<built in>")) | map(.Path)'
[]
```

That is the rules on this machine that did not ship with pwrq — yours, and
anything you have overridden. Empty, until you write one.

## Reading one

`Query` is the rule. It is published rather than summarised because reading it
is how you decide whether it asks what you wanted, and copying it is how you
write the rule nobody shipped.

```console
$ pwrq -nr 'get_pwrgrep_rule("go-swallowed-errors") | .Query'
# rules: go-swallowed-errors
# languages: go
# fixture: go/swallowed-errors.go
#
# Where an error is thrown away: assigned to the blank identifier, or checked
# and then ignored with an empty block. Not always wrong, but always a place
# where the code has decided a failure does not matter, and that decision is
# worth knowing about when reading it.

["$V, _ := $F($$$A)", "$V, _ = $F($$$A)"] as $beside
| ["_ = $F($$$A)"] as $blank
| ["if $ERR != nil { $$$B }"] as $checks
...
```

`Origin` says whether the copy you are looking at is one you can edit: a rule
from a directory can be changed where it sits, and `<built in>` has to be
copied out first. `Fixture` names the file it is checked against, `From` says
what it was translated from if it was, and `Description` is the prose in the
header — which is where a rule says what it does not cover and why a pattern is
written the way it is.

## Running them

`invoke_pwrgrep(root; rules)` takes the tree and the same selector. A finding
carries `RuleId`, `Path`, `LineNumber`, `Column`, `EndLineNumber`, `Message`
and `Match` — the source text it matched, spanning every line it covers. It is
an ordinary value, so summarising a run, or dropping vendored code, is the next
stage of the pipeline:

```console
$ pwrq -n '[invoke_pwrgrep("pkg/core"; "go-concurrency")] | map(.Message) | group_by(.) | map({(.[0]): length}) | add'
{"makes a chan int":3,"synchronises goroutines":28}
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
alternatives costs what a rule with one costs.

The eleven Go rules over this repository's `pkg/core` take about six seconds;
the JavaScript ones over the editor's source about twenty. **Over MCP this
matters**, because `run_query` defaults to a 30-second timeout: pass
`timeoutMs` for anything wider than a directory, or name a narrower selector.
The `calls` and `variables` rules are the loud ones — thousands of findings on
a tree of any size — so leave them out of a whole-language run unless that is
what you want.

## Where a rule of your own goes

The shipped rules live in [`rules/`](rules), beside this file, and are built
into the binary from there; the package installs the same files to
`/usr/share/pwrq/rules`. pwrq looks for rules in, in order:

1. every directory in `$PWRQ_RULES`, separated the way `PATH` is
2. `~/.config/pwrq/rules`
3. `/usr/share/pwrq/rules`
4. the copy inside the binary

A rule found earlier hides one with the same path found later, so changing a
shipped rule is copying it into your own directory under the same path and
editing it there. Adding one is dropping a file in, and adding a whole corpus
is adding a directory:

    PWRQ_RULES=../pwrgrep-rules/rules pwrq -n '[invoke_pwrgrep("."; "go")]'

`get_pwrgrep_rule` reports in `Origin` which of the four a rule came from.

The catalogue is re-read whenever those directories change, so a rule written
now is a rule this process can run now — which matters for the MCP server,
where one process answers for hours and there is no restart between writing a
file and asking for it.

## Writing one

A rule is a file, and this is the whole of it:

    # rules: go-input-reaches-effect
    # languages: go
    # fixture: go/input-reaches-effect.go

    ["os.Getenv($$$_)", "os.Args"] as $sources
    | ["exec.Command($$$A)", "os.WriteFile($$$A)"] as $sinks
    | scan_ast("*.go"; $sources + $sinks) as $all
    | $all | of($sinks)
    | reaching($all | of($sources); [])
    | finding("go-input-reaches-effect"; "external input reaches $A")
    | report

`# rules:` names the finding ids the file reports under, which is what a caller
asks for; a file may hold several, because rules that search the same files
share one walk of the tree. `# languages:` names the grammars the rule needs in
the build, which is what `TestEveryRuleLanguageIsInTheBuild` checks the
Makefile against: a rule written for a grammar the binary does not carry is a
rule that can never fire. `# fixture:` names a file it is checked against, and
`# from:` says what it was translated from, if it was. Anything else in the
header block is prose, and comes back as `Description`.

Everything below the header is ordinary pwrq.

### From a shell

Copy the nearest rule, change it, run it:

```console
$ mkdir -p ~/.config/pwrq/rules/mine
$ pwrq -nr 'get_pwrgrep_rule("go-side-effects") | .Query' > ~/.config/pwrq/rules/mine/no-timeout.pwrq
$ $EDITOR ~/.config/pwrq/rules/mine/no-timeout.pwrq
$ pwrq -n '[invoke_pwrgrep("src"; "mine/no-timeout")]'
```

Change the `# rules:` header while you are in there. It is the id the findings
carry and the name a caller asks for, and a copy that keeps the original's id
reports under it — two rules answering to `go-side-effects`, both running
whenever anybody names it.

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

`Query` in the same answer is the tree-sitter query the pattern became, and it
is worth reading when a valid pattern still finds too little. Several things
the shipped rules had to work around are visible there and nowhere else:

- **One match per node.** A pattern rooted at a construct binds its first
  child and no other: `class $C { $NAME($$$P) { $$$B } }` finds the first
  method of a class, a grouped Go `import ( ... )` reads one package, and a
  Python `except` hole binds whichever child comes first — often a comment.
  Where a rule needs each child, it finds them as text with `scan_regex` and
  keeps the ones `within` the construct: see `js-functions` and `go-imports`.
- **Arity in a parameter list differs by language.** In Go, Python and
  JavaScript `$$$P` binds the whole list; in Java and C# a named hole there
  binds exactly one parameter, and `$$$_` is the spelling for "any".
- **Some constructs have no pattern.** A Java or C# constructor, a C# property,
  a C# `catch` or `lock`, a Java record: the snippet parses as something else.
  Those rules say so in their headers and read text instead.

`scan_ast` on its own is the pattern without the rule around it — run it
against a file you know should match before wiring up the guards:

```console
$ pwrq -nc '"src" | scan_ast("*.go"; ["&tls.Config{$$$A}"]) | map({Path, LineNumber})'
```

Over MCP, `validate_query` takes the whole rule source and tells you whether it
compiles without running it, which is cheap and is where to catch a wrong
arity or a misspelled cmdlet.

### Checking a rule against a fixture

A fixture is a source file annotated the way semgrep's rules are tested:
`// ruleid: <id>` says the next line must produce a finding, and
`// ok: <id>` says it must not.

```go
func run() {
	cmd := os.Getenv("CMD")
	full := cmd + " --verbose"
	// ruleid: go-input-reaches-effect
	exec.Command(full).Run()
	// ok: go-input-reaches-effect
	exec.Command("ls").Run()
}
```

The annotation marks the line the finding lands on, which is where the match
*starts* — `focus` moves it onto a hole when that is not the interesting line.
For the shipped corpus `TestEveryRuleWithAFixtureFindsExactlyWhatItMarks` does
the comparing, as set equality: a rule that fires on a line nobody marked fails
as loudly as one that misses a line. For a rule of your own it is a query,
because the annotations are text and pwrq searches text:

```console
$ pwrq -nc '
  "pkg/pwrgrep/testdata/fixtures/go/input-reaches-effect.go" as $file
  | "go-input-reaches-effect" as $rule
  | ([$file | scan_regex("*.go"; ["ruleid:\\s*" + $rule])[] | .LineNumber + 1] | sort) as $marked
  | ([invoke_pwrgrep($file; $rule) | .LineNumber] | unique) as $fired
  | {marked: $marked, fired: $fired, missed: ($marked - $fired), extra: ($fired - $marked)}'
{"marked":[14,22,26],"fired":[14,22,26],"missed":[],"extra":[]}
```

`missed` is what the rule should have found and did not; `extra` is what it
found that nobody marked. Both empty is the rule passing.

## Adding a rule to the corpus

A rule here goes in `rules/<language>/<kind>/<name>.pwrq` with its fixture in
`testdata/fixtures/<language>/`, and the tests in this package hold it to four
things:

- it compiles against this binary's cmdlets (`TestEveryRuleCompiles`);
- its languages are in the Makefile's `GRAMMARS`
  (`TestEveryRuleLanguageIsInTheBuild`);
- it names a fixture (`TestEveryShippedRuleHasAFixture`), and every fixture
  belongs to a rule (`TestEveryFixtureBelongsToARule`);
- it fires on exactly the lines its fixture marks, and the fixture marks at
  least one `ok:` line so that firing everywhere cannot pass
  (`TestEveryRuleWithAFixtureFindsExactlyWhatItMarks`).

A fixture may only carry annotations for the first id its rule reports, and
the fixtures sit under `testdata/` so the Go tool neither builds nor formats
the Go ones. The whole check runs in seconds:

    go test ./pkg/pwrgrep/

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
means the same code twice, so `$X == $X` does not match `a == b`. A named group
in a `scan_regex` regex is a hole too: `$NAME` in a message is what it caught,
and `focus("NAME")` moves the finding onto it.

### The two readings of "inside"

A guard meaning "in a file that imports this" and a guard meaning "inside this
span" are different operators, and getting them the wrong way round produces no
findings and no error:

- `within` is a real span containing a real span — inside this function, inside
  this class body, inside this import block.
- `in_files_with` is "the same file also matched that", which is what an import
  guard means. A call is not inside an import statement, so `within` would find
  nothing.
