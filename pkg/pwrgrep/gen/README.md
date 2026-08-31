# The translator

Most of `../rules` is generated. `port.jq` reads one YAML rule file and writes
the `.pwrq` query the rules in it become; `MANIFEST.json` accounts for every
rule it was given, translated or not, with the reason; `validate.jq` and
`validate.sh` run every translated rule against the annotated fixture its
original was tested with and write `VALIDATION.json`.

Nothing under `../rules` except `../rules/pwrq` is edited by hand. A fix goes
in `port.jq` and the corpus is regenerated.

    git clone --depth 1 https://github.com/opengrep/opengrep-rules /tmp/rules-src
    go build -o /tmp/pwrq ./cmd/pwrq
    SRC=/tmp/rules-src PWRQ=/tmp/pwrq pkg/pwrgrep/gen/generate.sh
    go build -o /tmp/pwrq ./cmd/pwrq   # the corpus is embedded, so rebuild
    SRC=/tmp/rules-src PWRQ=/tmp/pwrq pkg/pwrgrep/gen/validate.sh

`port.jq` is written in pwrq because the hard half of the job is deciding
whether a pattern is code at all, and `ast_pattern` is the thing that knows.
Every pattern is compiled before it is emitted, so a rule reaches the corpus
only if the search it asks for can actually be run.

## What came through

2006 rules read, 1891 translated. (The count is lower than it once was because
the 89 entries in `*.test.yaml` files are the fixtures a rule is tested with
rather than rules, and are no longer counted as rules that were refused.)

| why the rest were not | rules |
| --- | ---: |
| the pattern is not code in the language it claims, and not a line either | 82 |
| a hole's regex needs lookaround, or another feature RE2 has not | 18 |
| an operator with no equivalent (`not_conflicting`, `metavariable-type`, …) | 7 |
| join mode (`from`/`to`), which composes two rules' findings | 6 |
| a `metavariable-comparison` that is not a comparison this reads | 2 |

Everything in that table is a refusal, and a refusal is deliberate. A rule
whose constraint was quietly dropped reports more than it said; a rule emitted
with a step the operator cannot read fails when it is run. Neither is allowed
in the corpus, which is why the regexes a rule will actually run and the
comparisons it will actually make are checked here rather than left to be
discovered by whoever runs it.

Four things that used to be most of this table are not in it any more:

- **Taint mode** (246 rules) is `reaching`: an intraprocedural syntactic
  following of a value from a source to a sink, stopped by a sanitizer. A
  source is an operator tree like any other, not a list of patterns: `$A`
  inside a loop over `getopt` results is a source, and `$A` on its own is every
  identifier in the file.
- **Text rules** (`regex`, `generic`, 359 rules) are `scan_regex`, which
  reports the same kind of match `scan_ast` does, so the rest of the operator
  vocabulary works on them unchanged. A rule whose patterns are code in no
  grammar falls back to the same reading.
- **The pattern that is not code** (~330 rules) is mostly two shapes, and both
  are read where they could have been written: an ellipsis standing where a
  whole item goes, spelled as an item and taken back out afterwards; and a
  construct with nowhere to stand, compiled inside the class, method or opener
  its language needs and walked back out of. See `scaffolds` and
  `scaffoldUnits` in `pkg/udf/astsearch`.
- **Semgrep's deep expression operator**, `<... E ...>`, is a hole and a
  `where_capture_ast` on what it caught, because a tree-sitter query matches at
  any depth and "somewhere inside" is the question it already asks.

## What they do

1442 of the 1891 have a fixture upstream that marks a line for them, and those
are the ones worth quoting:

| | rules |
| --- | ---: |
| find exactly what the fixture marks | 622 |
| find some of it | 255 |
| find none of it | 568 |

Line by line, over those 1442: 2126 of the 4256 lines marked as findings are
found, 318 of the lines marked as fine are reported anyway, and 129 lines
nobody marked either way are reported.

Two things account for most of the gap, and both are recorded per rule in
`VALIDATION.json` rather than papered over.

**The grammar is matched exactly where the original normalises.** `import x`
and `import x as y` are one thing upstream and two nodes here, so a pattern
written for the first does not find the second. Same for a fully qualified
class name against an imported one, for a method reference against a call, and
for a constant the original propagates and this does not.

**A file-scope guard is wider than the span it stands for.** Half the corpus
writes a guard meaning "in a file that imports this", which is `in_files_with`;
the original means "later in this block". Where the two differ, this reports
more, and that is where most of the false positives are.
