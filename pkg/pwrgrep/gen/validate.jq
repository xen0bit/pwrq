# Checking a translated rule against the fixture its original was tested with.
#
# The rule files in the set this corpus was translated from sit beside a source
# file annotated with a convention: a comment reading `ruleid: <id>` says the
# next line must produce a finding, and `ok: <id>` says it must not. That
# annotated file is the only statement anywhere of what a rule is supposed to
# do, so it is what a translation has to be measured against - and measuring
# against it is the difference between "the translator produced a rule" and
# "the translator produced the rule".
#
# Nothing from those fixtures is copied here. They are read where they are
# checked out; see generate.sh for where that is.

# marks reads the annotations out of a fixture, whatever the language spells a
# comment as. An annotation describes the next line of code, so a run of
# annotations stacked above one statement all point at that statement.
def marks:
  split("\n") as $lines
  | ($lines | length) as $n
  | "\\b(ruleid|ok|todoruleid|todook|deepruleid):\\s*([A-Za-z0-9_.-]+(\\s*,\\s*[A-Za-z0-9_.-]+)*)" as $re
  | [ range(0; $n)
      | . as $i
      | [$lines[$i] | match($re)][0]
      | select(. != null)
      | { at: $i,
          kind: .captures[0].string,
          ids: (.captures[1].string | split(",") | map(sub("^\\s+"; "") | sub("\\s+$"; ""))) } ]
  | map( . as $a
         | [range($a.at + 1; $n) | select(($lines[.] | test($re)) | not)][0]
         | select(. != null)
         | { kind: $a.kind, ids: $a.ids, line: (. + 1) } );

# score says what one rule did against what its fixture asked for.
#
# `todoruleid` marks a line the original does not find either, and `deepruleid`
# one only an interprocedural analysis finds; neither is a line this port is
# measured on, so both are left out of the count in both directions.
def score($id; $marks; $got):
  ([$marks[] | select(.kind == "ruleid" and (.ids | index($id))) | .line] | unique) as $want
  | ([$marks[] | select(.kind == "ok" and (.ids | index($id))) | .line] | unique) as $forbid
  | ([$marks[] | select(.kind | IN("todoruleid", "todook", "deepruleid"))
               | select(.ids | index($id)) | .line] | unique) as $ignored
  | ($got | unique) as $found
  | { want: $want, forbidden: $forbid,
      got: $found,
      found: [$found[] | select(IN($want[]))],
      missed: [$want[] | select(IN($found[]) | not)],
      wrong: [$found[] | select(IN($forbid[]))],
      extra: [$found[] | select((IN($want[]) or IN($forbid[]) or IN($ignored[])) | not)] }
  | . + { exact: ((.missed | length) == 0 and (.wrong | length) == 0 and (.extra | length) == 0),
          marked: (((.want | length) + (.forbidden | length)) > 0) };
