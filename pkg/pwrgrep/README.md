# Rules

A rule is a pwrq query that searches code by its syntax and says what it found.
`select_ast` answers one question — where does this piece of syntax occur — and
that is rarely a finding on its own. "MD5, but only in a file that imports
crypto/md5." "Assigning to innerHTML, but not a string literal." "A call to
Query, but not one whose argument is a constant." The patterns are the easy
half; the combining is where a rule lives.

    $ echo src | pwrq -R 'invoke_pwrgrep("go-weak-hash")'
    $ pwrq -n '[invoke_pwrgrep("src"; "go/lang/security")] | group_by(.RuleId)'
    $ pwrq -n '[get_pwrgrep_rule("python-*")] | map(.Id)'

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

Everything below the header is ordinary pwrq. `scan_ast`, `of`, `within`,
`outside`, `not_at`, `at_same_place`, `in_files_with`, `in_files_without`,
`where_capture`, `finding` and `report` are cmdlets like any other — `pwrq
--udf-list` documents them, and a rule can use the rest of the language freely.

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

`rules/pwrq/` is the eighteen written by hand, one per language family, each
with a fixture of its own and prose saying where it departs from the rule it
came from. They came first and are what the translator was written against.

## What does not translate

Some of this is pwrq's and some is the source vocabulary's, and it is worth
knowing before writing a rule.

- **Taint mode.** Nothing here tracks a value from a source to a sink. Rules
  written as `pattern-sources`/`pattern-sinks` are skipped rather than
  approximated.

- **`focus-metavariable`** moves where a finding points, and a pwrq capture
  reports text rather than a place, so there is nothing to point at. Those
  rules fire on the same code; the caret sits further left.

- **Import resolution.** `new java.util.Random()` does not match `new Random()`
  here, because pwrq matches what is written. A rule names the spelling that
  appears in the file and will miss a class imported under an alias.

- **A method reference is not a call.** `Digest::SHA1.$FUNC` is a reference;
  the call has the arguments written out.

- **PHP.** Every PHP pattern compiles to a search for its own characters: a PHP
  file is HTML until `<?php`, so `md5($X)` parses as literal text. `select_ast`
  refuses those rather than returning nothing, and PHP is out of scope.

- **Grouped Go imports.** `import "crypto/md5"` does not match the same import
  inside an `import ( ... )` block, because the grouped form nests the spec one
  level deeper. Rules match the import path string instead, which finds both.

- **Lookaround.** A hole's regex is RE2, which has none. A rule needing one is
  not translated rather than translated with the constraint quietly dropped.

## Regenerating

    git clone --depth 1 https://github.com/opengrep/opengrep-rules /tmp/rules-src
    go build -o /tmp/pwrq ./cmd/pwrq
    SRC=/tmp/rules-src PWRQ=/tmp/pwrq pkg/pwrgrep/gen/generate.sh
    go build -o /tmp/pwrq ./cmd/pwrq   # the corpus is embedded, so rebuild
    SRC=/tmp/rules-src PWRQ=/tmp/pwrq pkg/pwrgrep/gen/validate.sh
