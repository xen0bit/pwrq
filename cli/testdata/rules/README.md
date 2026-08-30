# Rules

Eighteen of opengrep's rules, ported to pwrq, in six languages. They are here
as an exercise rather than as a product: the point was to find out where pwrq's
structural search is unusable by using it, and most of what this directory is
worth is in the engine changes it caused.

## Reading a rule

    include "opengrep";

    # fixture: fixtures/go/weak-hash.go
    # ported-from: go/lang/security/audit/crypto/use_of_weak_crypto.yaml (use-of-md5, use-of-sha1)

    ["md5.New()", "md5.Sum($$$A)"] as $calls
    | ["\"crypto/md5\""] as $imports
    | scan("*.go"; $calls + $imports) as $all
    | ($all | of($calls) | in_files_with($all | of($imports)))
    | finding("go-weak-hash"; "\(.Text) is not collision resistant")
    | report

`opengrep.jq` is the vocabulary. Each operator in it names the Semgrep operator
it stands in for, so a port can be read against its original: `of` is `pattern`
and `pattern-either`, `within` is `pattern-inside`, `not_at` is `pattern-not`,
`where_capture` is `metavariable-regex`, and so on. Every rule is a pipeline
from one `scan` to one `report`.

One `scan` per rule is deliberate. It walks the tree once and parses each file
once however many patterns the rule has, so a rule with seven alternatives
costs what a rule with one costs.

## Running one

    echo /path/to/repo | pwrq --raw-input --from-file cli/testdata/rules/go-weak-hash.pwrq

The path goes in on standard input because with `--from-file` the remaining
arguments are input files, not arguments to the query. `include "opengrep"`
resolves because a query file's own directory is searched first.

## The test

`cli/rules_test.go` runs every rule against the fixture named in its header and
checks the lines it fired on against the lines the fixture marks - `ruleid:`
for a line that must be reported, `ok:` for one that must not, which are
Semgrep's own annotations. The check is set equality, so a rule that fires
somewhere nobody marked fails as loudly as one that misses a line.

## Provenance and licence

Nothing from opengrep is vendored here. No YAML, no fixtures, no message text.
opengrep-rules is LGPL 2.1 with a Commons Clause and pwrq carries no licence
file of its own, so the safe thing was to write the pattern text, the messages
and the fixtures fresh and keep only the facts: which rule was ported, under
its upstream id and path, in the `# ported-from:` header. That header is what
makes a port checkable against its original, and a test requires every rule to
carry one.

## What did not port

Some of this is pwrq's, some is Semgrep's, and it is all worth knowing before
writing a rule.

- **Taint mode.** Nothing in this corpus tracks a value from a source to a
  sink. `pattern-sources`/`pattern-sinks` rules were skipped rather than
  approximated.

- **`focus-metavariable`** (about 300 uses upstream) has no port. It moves
  where a finding points, and a pwrq capture reports text rather than a place,
  so there is nothing to point at. The rules that use it fire on the same code;
  the caret sits further left. `python-subprocess-shell-true` is one.

- **Statement sequences.** `$CONFIG = &tls.Config{...}` then, later,
  `$CONFIG.InsecureSkipVerify = true` is a claim about two statements in order,
  and pwrq has no way to say it. `go-tls-skip-verify` approximates it by file.

- **Import resolution.** Semgrep matches `new java.util.Random()` against `new
  Random()` because it resolves the import. pwrq matches what is written, so
  `java-weak-random` names the spelling that appears in the file and will miss
  a class imported under an alias.

- **A method reference is not a call.** Ruby's upstream rules write
  `Digest::SHA1.$FUNC` and mean the call; pwrq reads it as the reference, so
  `ruby-weak-hash` writes the arguments out.

- **PHP.** Every PHP pattern compiles to a search for its own characters: a PHP
  file is HTML until `<?php`, so `md5($X)` parses as the literal text
  `md5(__GREP_CAP_X__)`. `select_ast` now refuses those rather than returning
  nothing, and PHP is out of scope for this corpus. DVWA is in the validation
  set for its JavaScript.

- **Grouped Go imports.** `import "crypto/md5"` does not match the same import
  inside a `import ( ... )` block, because the grouped form nests the spec one
  level deeper. The rules match the import path string instead, which finds
  both.

- **A repeated metavariable.** `$X == $X` is refused rather than matched: a
  match keeps one capture per name, so the second use constrains nothing and
  the pattern would match `a == b`. Write the two holes separately and compare
  them with `where_same`, as `go-useless-comparison` does.

## Validation

Every rule was run over five real repositories - gosec, pygoat, NodeGoat, DVWA
and WebGoat - in addition to its fixture: 57 findings, no errors, and ten of
the eighteen rules fire on real code there.

Most of the noise in that run is vendored: `javascript-insecure-document-method`
reports 31 findings in WebGoat and most of them are inside jquery-1.10.2.min.js,
which really does assign to innerHTML. There is no Exclude option, and there
does not need to be one - a finding is a value, so the filter is the next stage
of the pipeline:

    | map(select(.Path | test("(min\\.js|/libs/|node_modules)") | not))

The run that is worth quoting is `go-weak-hash` over gosec: grep finds 40 lines containing `md5.New()` and
friends, and the rule reports 7. The other 33 are inside Go source embedded in
string literals, which is what gosec's test corpus is made of, and inside
comments. That gap is the whole argument for matching the tree rather than the
text.
