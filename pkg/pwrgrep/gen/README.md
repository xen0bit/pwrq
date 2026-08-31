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

2095 rules read, 763 translated.

| why the rest were not | rules |
| --- | ---: |
| taint mode — tracking a value from a source to a sink | 246 |
| a pattern over text rather than syntax (`regex`, `generic`) | 359 |
| an operator with no equivalent (`pattern-regex`, `focus-metavariable`, …) | 179 |
| the pattern is not code in the language it claims | ~330 |
| a hole's regex needs lookaround, which RE2 has none of | 16 |

The largest single block of the fourth row is HCL: 215 Terraform rules whose
patterns are written against a resource shape rather than against HCL syntax.

## What they do

665 of the 763 have a fixture upstream that marks a line for them, and those
are the ones worth quoting:

| | rules |
| --- | ---: |
| find exactly what the fixture marks | 231 |
| find some of it | 138 |
| find none of it | 296 |

Line by line, over those 665: 933 of the 1964 lines marked as findings are
found, 126 of the 1160 lines marked as fine are reported anyway, and 70 lines
nobody marked either way are reported.

Two things account for most of the gap, and both are recorded per rule in
`VALIDATION.json` rather than papered over.

**The grammar is matched exactly where the original normalises.** `import x`
and `import x as y` are one thing upstream and two nodes here, so a pattern
written for the first does not find the second. Same for a fully qualified
class name against an imported one, and for a method reference against a call.

**A file-scope guard is wider than the span it stands for.** Half the corpus
writes a guard meaning "in a file that imports this", which is `in_files_with`;
the original means "later in this block". Where the two differ, this reports
more, and that is where most of the 126 false positives are.
