# Rules

A rule is a pwrq query that searches code by its syntax and says what it found.
`select_ast` answers one question — where does this piece of syntax occur — and
that is rarely a finding on its own. "MD5, but only in a file that imports
crypto/md5." "Assigning to innerHTML, but not a string literal." "A call to
Query, but not one whose argument is a constant." The patterns are the easy
half; the combining is where a rule lives.

    $ echo src | pwrq -R 'invoke_pwrgrep("go-weak-hash")'
    $ pwrq -n '[invoke_pwrgrep("src"; "go/lang/security")] | group_by(.RuleId)'
    $ pwrq -n '[get_pwrgrep_rule("python")] | map(.Id)'

## Where rules come from

`rules/` is the corpus, built into the binary and installed to
`/usr/share/pwrq/rules` by the package. pwrq looks for rules in, in order:

1. every directory in `$PWRQ_RULES`, separated the way `PATH` is
2. `~/.config/pwrq/rules`
3. `/usr/share/pwrq/rules`
4. the copy inside the binary

A rule found earlier hides one with the same path found later, so changing a
shipped rule is copying it into your own directory and editing it there. Adding
one is dropping a file in. `get_pwrgrep_rule` reports in `Origin` which of the
four a rule came from.

## Writing one

A rule is a file, and this is the whole of it:

    # rules: go-weak-hash
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
share one walk of the tree. `# from:` says what it was translated from, if it
was. `# fixture:` names a file the rule is checked against.

Everything below the header is ordinary pwrq. `scan_ast`, `scan_regex`, `of`,
`within`, `outside`, `not_at`, `at_same_place`, `in_files_with`,
`in_files_without`, `where_capture`, `where_capture_not`, `where_capture_ast`,
`where_capture_compare`, `where_capture_entropy`, `where_capture_redos`,
`where_text`, `where_text_not`, `where_same`, `where_different`, `focus`,
`reaching`, `finding` and `report` are cmdlets like any other — `pwrq
--udf-list` documents them, and a rule can use the rest of the language freely.

`# languages:` names the grammars a rule needs in the build, which is what
`TestEveryRuleLanguageIsInTheBuild` checks the Makefile against: a rule written
for a grammar the binary does not carry is a rule that can never fire.

One `scan_ast` per rule is deliberate. It walks the tree once and parses each
file once however many patterns the rule has, so a rule with seven alternatives
costs what a rule with one costs.

Running many rules is many walks, and that is the price of a rule being a query
rather than a description: nothing outside the rule knows which files it will
ask for, so nothing can share a walk between two of them. Naming a directory of
the catalogue rather than all of it is what keeps a run honest —
`invoke_pwrgrep("."; "go/**")` over a Go repository, not the whole 700.

A finding is a value, so filtering is the next stage of the pipeline rather
than an option on the search:

    | map(select(.Path | test("(min\\.js|/libs/|node_modules)") | not))

## The two readings of "inside"

Half the corpus writes a guard meaning "in a file that imports this" and half
means "inside this span". They are different operators and getting them the
wrong way round produces no findings and no error:

- `within` is a real span containing a real span — inside this function, inside
  this loop.
- `in_files_with` is "the same file also matched that", which is what an import
  guard means. A call is not inside an import statement, so `within` would find
  nothing.

## Where the corpus came from

`gen/` holds the translator and its results. `gen/port.jq` reads a YAML rule
file and writes the .pwrq query; `gen/MANIFEST.json` accounts for every rule it
was given — translated, or listed with the reason it was not; and
`gen/VALIDATION.json` records how each translated rule scored against the
annotated fixture its original was tested with. Nothing under `rules/` except
`rules/pwrq/` is edited by hand: a fix goes in `port.jq` and the corpus is
regenerated.

1891 of the 2006 rules it was given are translated, in 26 languages, and 1442
of those have a fixture upstream that marks a line for them. Of those, 622 find
exactly what the fixture marks and 255 find some of it. The rest are in
`VALIDATION.json` with their score, which is the point of the file: a rule that
runs and finds nothing is worth knowing about, and a corpus that does not say
so is a list of claims.

`rules/pwrq/` is the eighteen written by hand, one per language family, each
with a fixture of its own and prose saying where it departs from the rule it
came from. They came first and are what the translator was written against.

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

A rule is one of three things, and `MANIFEST.json` says which:

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
refused, and `VALIDATION.json` is what says whether the rule still finds what
it was written to find.

## Regenerating

    git clone --depth 1 https://github.com/opengrep/opengrep-rules /tmp/rules-src
    go build -o /tmp/pwrq ./cmd/pwrq
    SRC=/tmp/rules-src PWRQ=/tmp/pwrq pkg/pwrgrep/gen/generate.sh
    go build -o /tmp/pwrq ./cmd/pwrq   # the corpus is embedded, so rebuild
    SRC=/tmp/rules-src PWRQ=/tmp/pwrq pkg/pwrgrep/gen/validate.sh
