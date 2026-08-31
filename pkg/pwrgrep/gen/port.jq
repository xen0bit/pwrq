# Translating a rule into a pwrgrep rule.
#
# This is the program that produced the corpus in ../rules. It reads one YAML
# rule file - `pwrq --yaml-input --from-file port.jq` - and emits the .pwrq
# query the rules in it become, together with a note on every rule in the file
# saying whether it could be translated and, if not, why not.
#
# It is written in pwrq because the hard half of the job is deciding whether a
# pattern is code at all, and ast_pattern is the thing that knows. Every
# pattern below is compiled before it is emitted, so a rule reaches the corpus
# only if the search it asks for can actually be run.
#
# What it translates is the search-mode vocabulary, one operator at a time; the
# cmdlets it emits calls to - of, within, not_at, in_files_with - are named
# after the same operators.
# What it declines is listed in README.md beside it, and every declined rule is
# in MANIFEST.json with its reason, so the corpus is a complete account of the
# set it was translated from rather than a selection from it.

# ---------------------------------------------------------------- languages

# glob is the file a language is written in, and grammar is what pwrq calls it.
# A language pwrq has no grammar for, or that names no files - patterns over
# text rather than over syntax - is absent, and a rule in one is not
# translated.
def languages:
  {
    "go":         {grammar: "go",         glob: "*.go"},
    "golang":     {grammar: "go",         glob: "*.go"},
    "python":     {grammar: "python",     glob: "*.py"},
    "py":         {grammar: "python",     glob: "*.py"},
    "javascript": {grammar: "javascript", glob: "*.js"},
    "js":         {grammar: "javascript", glob: "*.js"},
    "typescript": {grammar: "typescript", glob: "*.ts"},
    "ts":         {grammar: "typescript", glob: "*.ts"},
    "java":       {grammar: "java",       glob: "*.java"},
    "ruby":       {grammar: "ruby",       glob: "*.rb"},
    "rb":         {grammar: "ruby",       glob: "*.rb"},
    "php":        {grammar: "php",        glob: "*.php"},
    "c":          {grammar: "c",          glob: "*.c"},
    "cpp":        {grammar: "cpp",        glob: "*.cpp"},
    "csharp":     {grammar: "c_sharp",    glob: "*.cs"},
    "c#":         {grammar: "c_sharp",    glob: "*.cs"},
    "kotlin":     {grammar: "kotlin",     glob: "*.kt"},
    "kt":         {grammar: "kotlin",     glob: "*.kt"},
    "scala":      {grammar: "scala",      glob: "*.scala"},
    "rust":       {grammar: "rust",       glob: "*.rs"},
    "swift":      {grammar: "swift",      glob: "*.swift"},
    "solidity":   {grammar: "solidity",   glob: "*.sol"},
    "sol":        {grammar: "solidity",   glob: "*.sol"},
    "elixir":     {grammar: "elixir",     glob: "*.ex"},
    "ex":         {grammar: "elixir",     glob: "*.ex"},
    "clojure":    {grammar: "clojure",    glob: "*.clj"},
    "ocaml":      {grammar: "ocaml",      glob: "*.ml"},
    "bash":       {grammar: "bash",       glob: "*.sh"},
    "sh":         {grammar: "bash",       glob: "*.sh"},
    "dockerfile": {grammar: "dockerfile", glob: "Dockerfile*"},
    "docker":     {grammar: "dockerfile", glob: "Dockerfile*"},
    "json":       {grammar: "json",       glob: "*.json"},
    "yaml":       {grammar: "yaml",       glob: "*.yaml"},
    "hcl":        {grammar: "hcl",        glob: "*.tf"},
    "terraform":  {grammar: "hcl",        glob: "*.tf"},
    "tf":         {grammar: "hcl",        glob: "*.tf"},
    "apex":       {grammar: "apex",       glob: "*.cls"},
    "html":       {grammar: "html",       glob: "*.html"},
    "lua":        {grammar: "lua",        glob: "*.lua"},
    "dart":       {grammar: "dart",       glob: "*.dart"}
  };

def targets: [.[] | ascii_downcase | languages[.] // empty] | unique;

# ------------------------------------------------------------------ patterns

# rewriteHoles is the source hole syntax in pwrq's. The two agree on `$NAME`; they
# differ on the variadic, which is spelled `...` where it means "and anything
# else here" and `$...NAME` where it names the run. The named form goes first,
# because `$...ARGS` contains the anonymous one. What is left is every
# remaining `...`, with no attempt to spot the ones that are not holes: Ruby
# spells an exclusive range `1...5`, and a pattern that writes one is
# mistranslated. None in this corpus does, and a rule that did would fail to
# compile rather than match the wrong thing.
def rewriteHoles:
  gsub("\\$\\.\\.\\.(?<n>[A-Z_][A-Z_0-9]*)"; "$$$\(.n)")
  | gsub("\\.\\.\\."; "$$$_")
  | sub("^\\s+"; "") | sub("\\s+$"; "");

# deepSplit separates a pattern from the deep expressions in it.
#
# `<... E ...>` says "an expression with E somewhere inside it", and it is what
# a fifth of the patterns no grammar would accept are written with: `$LOG.warn(
# <... $REQ.getParameter(...) ...>)` is a log call whose argument mentions a
# request parameter anywhere down the tree, however it was wrapped or
# concatenated on the way. Written out it parses as nothing at all, because
# `<` is a comparison and the rest is not an expression.
#
# What it means is two questions, and pwrq has an operator for each: a hole
# where the deep expression stood, and a constraint that what the hole caught
# is itself somewhere a match for E. A tree-sitter query matches at any depth,
# so "somewhere inside" is what where_capture_ast already asks - see
# sourceReadings for the other half, which is that what a hole catches is a
# fragment and has to be read as one.
#
# The holes are numbered per pattern, which is enough: each one is emitted
# beside the `of` that introduced it.
def deepSplit:
  . as $p
  | [$p | match("<\\.\\.\\.(?<i>(?s:.*?))\\.\\.\\.>"; "g")] as $ms
  | reduce range(0; $ms | length) as $n
      ({text: "", at: 0, deep: []};
        $ms[$n] as $m
        | .text = (.text + $p[.at:$m.offset] + "$PWRQDEEP\($n + 1)")
        | .at = ($m.offset + $m.length)
        | .deep = (.deep + [{hole: "PWRQDEEP\($n + 1)",
                             inner: ($m.captures[0].string | rewriteHoles)}]))
  | {pattern: ((.text + $p[.at:]) | rewriteHoles), deep: .deep};

# rewrite is the source hole syntax in pwrq's, deep expressions and all.
def rewrite: deepSplit | .pattern;

# ------------------------------------------------------- the operator tree
#
# A rule's body is a tree of operators over patterns. These walk it twice: once
# to collect every pattern that has to be compiled, and once to build the match
# tree that combines them.

def leafOps: ["pattern", "pattern-not", "pattern-inside", "pattern-not-inside"];

# scanBody is the rule body minus the parts that are not searched for.
#
# A metavariable-pattern is matched against what a hole caught, not against the
# files, so its pattern must not join the scan: searching for it would be a
# second search for something the rule never asked to find, and every match of
# it would be in $all for the operators to trip over.
def scanBody:
  walk(if type == "object" and has("metavariable-pattern")
       then with_entries(select(.key != "metavariable-pattern")) else . end);

# patternsIn collects every leaf pattern in a rule body, as written.
def patternsIn:
  [ scanBody | .. | objects | to_entries[] | select(.key | IN(leafOps[])) | .value
        | select(type == "string") | rewrite ] | unique;

# knownOps are the operators this translator has a step for. Anything else -
# taint mode, focus-metavariable, metavariable-comparison - is a rule left
# untranslated rather than approximated.
def knownOps:
  ["pattern", "patterns", "pattern-either", "pattern-inside", "pattern-not",
   "pattern-not-inside", "metavariable-regex", "metavariable", "regex",
   "pattern-regex", "pattern-not-regex", "metavariable-pattern",
   "metavariable-comparison", "comparison", "base", "strip",
   "metavariable-analysis", "analyzer",
   "focus-metavariable", "language",
   "pattern-sources", "pattern-sinks", "pattern-sanitizers",
   "pattern-propagators", "label", "requires", "by-side-effect", "exact"];

def opsIn:
  [ .. | objects | keys[] ] | unique;

# labelKeys are what a taint rule writes beside a pattern to name it for its
# `requires` line. They say nothing about what the pattern looks for, so an
# operator tree reads past them.
def labelKeys: ["label", "requires", "by-side-effect", "exact"];

# opKeys are the keys of one node of an operator tree that are operators.
def opKeys: keys - labelKeys;

# fileScope reports a `pattern-inside` that means "this file", not "this span".
#
# Much of the corpus writes `pattern-inside: |` with an import and a trailing
# `...`, meaning "somewhere in a file that imports this". Read as a span it
# finds nothing, because a call is not inside an import statement.
#
# What says so is an ellipsis in the first column: it stands for the rest of
# the file, where an indented one stands for the rest of a block. `def
# $F($$$_):\n  $$$_` really is a span - inside that function - and reading it
# as a file would widen the rule to every function in the file.
def fileScope: test("\\n\\$\\$\\$_\\s*$");

# spellings widens a file-scope guard to the other ways a grammar writes the
# same fact.
#
# `import "crypto/md5"` does not match the same import inside Go's grouped
# `import ( ... )` form, because the grouped one nests the spec a level deeper
# than the pattern's own parse does, and most Go is written grouped. The path
# string is in both, and asking "is this string in this file" is the question a
# file-scope guard was asking anyway. A guard naming exactly one string is
# widened to it; one naming several is left alone, because which of them stands
# for the fact is not something this can know.
def spellings:
  . as $p
  | [$p] + ( [$p | match("\"[^\"\n]{2,}\""; "g").string]
             | if length == 1 then . else [] end );

# guardSpellings is what a rule's file-scope guards would also accept. They are
# offered rather than required: a spelling that is not code in one of the
# rule's languages is dropped, and the guard keeps the pattern it was written
# with.
def guardSpellings:
  [ .. | objects | to_entries[]
        | select(.key | IN("pattern-inside", "pattern-not-inside"))
        | select(.value | type == "string")
        | (.value | rewrite) | select(fileScope) | spellings[] ]
  | unique;

# ------------------------------------------------------------------ regexes
#
# A hole's regex is matched from the start of what the hole caught - the rules
# being translated are written against a matcher that anchors - and it is
# written in a Perl dialect where pwrq's is RE2. Both differences are handled
# here rather than left for a reader to trip over: an unanchored port reports
# more than the rule asked for, and a Perl construct RE2 has never heard of
# reports nothing at all.

# re2Fixups rewrites the spellings RE2 has a different name for.
def re2Fixups: gsub("\\\\A"; "^") | gsub("\\\\[Zz]"; "$");

# anchored is a regex read the way a hole's regex is read: from the start.
def anchored: if test("^\\^") then . else "^(?:" + . + ")" end;

# lookahead splits `(?!X)` - a whole regex that says "not this" - into the
# thing it says not to be. RE2 has no lookaround, but a negative lookahead
# spanning the whole regex is not really lookaround: it is a negation, and a
# negation is a different operator rather than a different regex.
def lookahead:
  if test("^\\(\\?!") and test("\\)$") and ((. | sub("^\\(\\?!"; "") | sub("\\)$"; "")) | test("\\)") | not)
  then (sub("^\\(\\?!"; "") | sub("\\)$"; "") | anchored)
  else null end;

# lookbehind splits a trailing `(?<!X)` off the end, which the corpus writes to
# mean "and does not end with X".
def lookbehind:
  if test("\\(\\?<!.*\\)$")
  then (capture("\\(\\?<!(?<b>[^()]*)\\)$") | "(?:" + .b + ")$")
  else null end;

# leadingLookahead splits `^(?!X)REST` into the two questions it asks: not X at
# the start, and REST at the start.
#
# `^(?!(int|float|str))` is how the corpus says "a name that is not a builtin
# type", and it is the commonest lookaround in it. Read from the front like
# that it is not really lookaround either: it is a negation of one regex and,
# where anything follows it, a match of another - and where what follows is
# `.*$`, which says nothing, there is only the negation.
def leadingLookahead:
  if test("^\\^?\\(\\?!")
  then ( capture("^(?<a>\\^?)\\(\\?!(?<x>(?:[^()]|\\([^()]*\\))*)\\)(?<rest>.*)$")
         | {no: ("^(?:" + .x + ")"),
            yes: (if (.rest | test("^(\\.\\*)?\\$?$")) then null
                  else ("^(?:" + .rest + ")") end)} )
  else null end;

# holeRegexes are the steps one metavariable-regex becomes: usually one, and
# two where a leading negative lookahead splits it. Both the verdict and the
# emitted query read it, so what a rule is checked against is what it runs.
def holeRegexes:
  re2Fixups as $re
  | if ($re | lookahead) != null then
      [{op: "where_capture_not", re: ($re | lookahead)}]
    elif ($re | lookbehind) != null then
      [{op: "where_capture_not", re: ($re | lookbehind)}]
    elif ($re | leadingLookahead) != null then
      ( ($re | leadingLookahead) as $split
        | [{op: "where_capture_not", re: $split.no}]
          + (if $split.yes != null then [{op: "where_capture", re: $split.yes}] else [] end) )
    else [{op: "where_capture", re: ($re | anchored)}]
    end;

# ------------------------------------------------------------- text patterns
#
# Not every rule is about a parse tree. A third of the corpus declares its
# language as `regex` or `generic`, which is a rule saying that what it looks
# for is not a construct in anything: a Django template with `{% autoescape off
# %}` in it, a blade form without a CSRF token. Those are real findings, and a
# translator that only knew syntax would drop every one of them.
#
# So they become searches over text, and because scan_regex reports the same
# kind of match scan_ast does - a path, a span, the holes - the rest of the
# operator vocabulary works on them unchanged.

# reEscape makes a piece of literal text safe to put in a regex.
def reEscape: gsub("(?<c>[.^$*+?()\\[\\]{}|\\\\/-])"; "\\\(.c)");

# genericRegex turns a generic-mode pattern into the regex that stands for it.
#
# Generic mode matches a token at a time and does not care how the source was
# spaced, so the translation is: holes become named groups, `...` becomes "and
# anything", and every run of whitespace in the pattern becomes "some
# whitespace". It is looser than matching tokens - a `...` here will cross a
# boundary that Semgrep's would not - and it is the difference between the rule
# running and the rule not existing.
def genericRegex:
  (sub("^\\s+"; "") | sub("\\s+$"; ""))
  | gsub("\\$\\.\\.\\.(?<n>[A-Z_][A-Z_0-9]*)"; "\u0001\(.n)\u0001")
  | gsub("\\$(?<n>[A-Z_][A-Z_0-9]*)"; "\u0002\(.n)\u0002")
  | gsub("\\.\\.\\."; "\u0003")
  | reEscape
  | gsub("[ \t]*\n[ \t]*"; "\\s+")
  | gsub("[ \t]+"; "\\s+")
  | gsub("\u0001(?<n>[A-Z_0-9]*)\u0001"; "(?P<\(.n)>[\\s\\S]*?)")
  | gsub("\u0002(?<n>[A-Z_0-9]*)\u0002"; "(?P<\(.n)>\\S+)")
  | gsub("\u0003"; "[\\s\\S]*?");

# numericCompare is `$X > 5`, which is the shape where_capture_compare reads.
# It is the same test the operator makes, written here so that a rule the
# operator could not read is refused rather than shipped.
def numericCompare:
  test("^\\s*\\$?[A-Za-z_][A-Za-z_0-9]*\\s*(<=|>=|==|!=|<|>)\\s*(-?[0-9]+(\\.[0-9]+)?|0[bBoOxX][0-9A-Fa-f]+)\\s*$");

# ------------------------------------------------------- comparing a hole
#
# `metavariable-comparison` is a Python expression over what the holes caught,
# and the corpus writes seven shapes of it. Each is a question one of the
# operators already answers - a number compared, a regex matched, two holes
# equal, a name in a list - so each is translated into that operator rather
# than into an expression evaluator.
#
# What is not one of the seven is refused, and the rule with it. A comparison
# quietly dropped is a rule that reports more than it said, and a comparison
# emitted that the operator cannot read is a rule that fails when it is run,
# which is worse: nothing in the corpus is allowed to be a query that errors.

# castsAway strips the `int(...)` and `float(...)` a rule writes round a hole
# it is about to compare as a number. The reading is numeric already - 0o777,
# 0x1FF and 511 are one value - so the cast says nothing this does not do.
def castsAway:
  gsub("\\b(?:int|float)\\(\\s*(?<h>\\$[A-Za-z_][A-Za-z_0-9]*)\\s*\\)"; "\(.h)")
  | gsub("\\bstr\\(\\s*(?<h>\\$[A-Za-z_.]*[A-Za-z_][A-Za-z_0-9]*)\\s*\\)"; "\(.h)")
  | sub("^\\s+"; "") | sub("\\s+$"; "");

# holeOf is the name a hole is bound under, with the sigil and the variadic
# dots taken off.
def holeOf: ltrimstr("$") | ltrimstr("...");

# listAlternatives turns `["health", "*"]` into a regex matching any of them,
# with the quotes a hole catches round a string allowed for.
def listAlternatives:
  [ match("\"(?<v>(?:[^\"\\\\]|\\\\.)*)\""; "g").captures[0].string | reEscape ]
  | "^\"?(?:" + join("|") + ")\"?$";

# oneCompare is the steps one comparison becomes, or null when it is not one
# this knows.
def oneCompare:
  . as $c
  | if ($c | test("^re\\.match\\(")) then
      ( $c | capture("^re\\.match\\(\\s*[\"'](?<re>(?:[^\"'\\\\]|\\\\.)*)[\"']\\s*,\\s*(?<h>\\$[A-Za-z_.]*[A-Za-z_][A-Za-z_0-9]*)\\s*\\)$")
        | ["  | where_capture(\(.h | holeOf | tojson); \(.re | re2Fixups | anchored | tojson))"] ) // null
    elif ($c | test("^not\\s+\\$[A-Za-z_.]*[A-Za-z_0-9]+\\s+in\\s+\\[")) then
      ( $c | capture("^not\\s+(?<h>\\$[A-Za-z_.]*[A-Za-z_][A-Za-z_0-9]*)\\s+in\\s+(?<l>\\[.*\\])$")
        | ["  | where_capture_not(\(.h | holeOf | tojson); \(.l | listAlternatives | tojson))"] ) // null
    elif ($c | test("^\\$[A-Za-z_.]*[A-Za-z_0-9]+\\s+in\\s+\\[")) then
      ( $c | capture("^(?<h>\\$[A-Za-z_.]*[A-Za-z_][A-Za-z_0-9]*)\\s+in\\s+(?<l>\\[.*\\])$")
        | ["  | where_capture(\(.h | holeOf | tojson); \(.l | listAlternatives | tojson))"] ) // null
    elif ($c | test("^\\$[A-Za-z_.]*[A-Za-z_0-9]+\\s*[=!]=\\s*\\$[A-Za-z_.]*[A-Za-z_0-9]+$")) then
      ( $c | capture("^(?<a>\\$[A-Za-z_.]*[A-Za-z_][A-Za-z_0-9]*)\\s*(?<op>[=!]=)\\s*(?<b>\\$[A-Za-z_.]*[A-Za-z_][A-Za-z_0-9]*)$")
        | ["  | \(if .op == "==" then "where_same" else "where_different" end)(\(.a | holeOf | tojson); \(.b | holeOf | tojson))"] ) // null
    elif ($c | test("^\\$[A-Za-z_.]*[A-Za-z_0-9]+\\s*[=!]=\\s*(True|False|None)$")) then
      ( $c | capture("^(?<h>\\$[A-Za-z_.]*[A-Za-z_][A-Za-z_0-9]*)\\s*(?<op>[=!]=)\\s*(?<v>True|False|None)$")
        | ["  | \(if .op == "==" then "where_capture" else "where_capture_not" end)(\(.h | holeOf | tojson); \("^(?i:" + .v + ")$" | tojson))"] ) // null
    elif ($c | numericCompare) then
      ["  | where_capture_compare(\($c | tojson))"]
    else null
    end;

# compareSteps reads a whole comparison, splitting a conjunction into the
# separate questions it is. A disjunction is not split: a pipeline of filters
# is an "and", and there is nothing in it that means "or".
def compareSteps:
  castsAway as $whole
  | [$whole | splits("\\s+and\\s+")] as $parts
  | if ($whole | test("\\s+or\\s+")) then null
    else ( [$parts[] | oneCompare] as $steps
           | if any($steps[]; . == null) then null else [$steps[][]] end )
    end;

# breakRuns writes a long run of one character as a regex that matches it
# rather than as the characters themselves.
#
# A secret-detection rule is made of examples of secrets. The one this is for
# is not even a secret - it is Slack's own placeholder, which the rule excludes
# so that documentation does not get reported - but a scanner reading the
# corpus cannot tell, and a repository with push protection on will refuse the
# whole branch over it.
#
# Bracketing the last character of the run leaves a regex that matches exactly
# the same text and is not that text. Eight is the threshold because that is
# long enough that no ordinary word reaches it, so the corpus stays readable.
def breakRuns:
  reduce ("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
          | explode[] | [.] | implode) as $c
    (.; gsub("(?<r>\($c){7,})\($c)"; "\(.r)[\($c)]"));

# leaf renders one pattern the way the rule's kind needs it: as pwrq's hole
# syntax, as a regex built from a generic pattern, or as the regex it already
# was.
def leaf($mode):
  if $mode == "ast" then rewrite
  elif $mode == "generic" then (genericRegex | breakRuns)
  else breakRuns end;

# ofLeaf is the step that finds one pattern: the search for it, and the
# constraints the deep expressions in it turn into. See deepSplit.
def ofLeaf($mode):
  if $mode != "ast" then "$all | of([\(leaf($mode) | tojson)])"
  else deepSplit as $d
    | "$all | of([\($d.pattern | tojson)])"
      + ([$d.deep[]
          | "\n  | where_capture_ast(\(.hole | tojson); \(.inner | tojson))"]
         | join(""))
  end;

# asTextBody renames a text rule's operators to the ones expr knows, so that
# one operator tree builder serves both kinds. `pattern-regex` is a text rule's
# `pattern`.
def asTextBody:
  walk(if type == "object"
       then with_entries(
              if .key == "pattern-regex" then .key = "pattern"
              elif .key == "pattern-not-regex" then .key = "pattern-not"
              else . end)
       else . end);

# textPatterns collects the regexes a text rule searches for.
def textPatterns($mode):
  [ scanBody | .. | objects | to_entries[] | select(.key | IN(leafOps[])) | .value
        | select(type == "string") | leaf($mode) ] | unique;

# ---------------------------------------------------------------- following
#
# A taint rule is not a search for a construct, it is a claim about a journey:
# this value arrived from outside, and it ended up somewhere that decides what
# the program does. Neither end is a finding on its own, which is why the rule
# is written as two halves and why a translator that could only match
# constructs had to refuse all of them.

def taintSections: ["pattern-sources", "pattern-sinks", "pattern-sanitizers"];

# taintPatterns are the patterns of one half of a taint rule, whether that half
# is written as a list of patterns or as a block of them.
def taintPatterns($section):
  [ (.[$section] // [])[]
    | if type == "string" then . else (.. | objects | to_entries[]
        | select(.key | IN(leafOps[])) | .value | select(type == "string")) end
    | rewrite ] | unique;

# expr builds the pipeline for one operator tree. $all is the name the emitted
# query binds every match to; each combinator narrows that list.
#
# The shapes are one for one:
#
#   pattern            of                 this piece of syntax
#   pattern-either     of, or a union     any of these
#   patterns           a pipeline         all of these, of the same code
#   pattern-inside     within             inside a span this matched
#   pattern-not-inside outside            not inside one
#   pattern-not        not_at             not itself this
#   metavariable-regex where_capture      a hole whose text matches
#
# A guard that is really about the file rather than about a span becomes
# in_files_with instead of within; see fileScope.
def expr($keep; $mode):
  # operand is the matches one side of an operator describes: a pattern
  # written inline, or a whole nested block of them. It sits inside expr
  # because the two call each other.
  # Every operand is a question about the whole search, not about what the
  # pipeline has narrowed to so far: "not inside a span this matched" means any
  # span in the file, not one among the matches left. So an operand starts from
  # $all again.
  def operand:
    if type == "string" then ofLeaf($mode)
    else "(" + expr($keep; $mode) + ")"
    end;
  if has("pattern") and (opKeys | length) == 1 then
    (.["pattern"] | ofLeaf($mode))
  elif has("pattern-either") then
    ( .["pattern-either"] as $alts
      | if ($alts | all(has("pattern") and (opKeys | length) == 1
                        and (($mode != "ast") or ((.["pattern"] | deepSplit | .deep | length) == 0)))) then
          "$all | of(\([$alts[].["pattern"] | leaf($mode)] | tojson))"
        else
          "(" + ([$alts[] | expr($keep; $mode)] | join(")\n  + (")) + ")"
        end )
  elif has("patterns") then
    ( .["patterns"] as $parts
      | [$parts[] | select(has("pattern") or has("pattern-either"))] as $positive
      | [$parts[] | select((has("pattern") or has("pattern-either")) | not)] as $guards
      | (if ($positive | length) == 0 then "$all" else ($positive[0] | expr($keep; $mode)) end) as $base
      | ( [ ($positive[1:][] | "  | at_same_place(\(. | expr($keep; $mode) | "(" + . + ")"))"),
            ( $guards[]
              | if has("pattern-inside") then
                  (.["pattern-inside"] as $in
                   | if ($in | type) == "string" and ($in | rewrite | fileScope)
                     then "  | in_files_with($all | of(\($in | leaf($mode) | spellings | map(select(IN($keep[]))) | tojson)))"
                     else "  | within(\($in | operand))" end)
                elif has("pattern-not-inside") then
                  (.["pattern-not-inside"] as $in
                   | if ($in | type) == "string" and ($in | rewrite | fileScope)
                     then "  | in_files_without($all | of(\($in | leaf($mode) | spellings | map(select(IN($keep[]))) | tojson)))"
                     else "  | outside(\($in | operand))" end)
                elif has("pattern-not") then
                  "  | not_at(\(.["pattern-not"] | operand))"
                elif has("metavariable-regex") then
                  ( .["metavariable-regex"] as $mv
                    | ($mv.metavariable | ltrimstr("$") | ltrimstr("...")) as $hole
                    | [ $mv.regex | holeRegexes[]
                        | "  | \(.op)(\($hole | tojson); \(.re | tojson))" ]
                    | join("\n") )
                elif has("pattern-regex") then
                  "  | where_text(\(.["pattern-regex"] | breakRuns | tojson))"
                elif has("pattern-not-regex") then
                  "  | where_text_not(\(.["pattern-not-regex"] | breakRuns | tojson))"
                elif has("metavariable-analysis") then
                  ( .["metavariable-analysis"] as $an
                    | ($an.metavariable | ltrimstr("$") | ltrimstr("...")) as $hole
                    | if $an.analyzer == "entropy" then
                        "  | where_capture_entropy(\($hole | tojson))"
                      elif $an.analyzer == "redos" then
                        "  | where_capture_redos(\($hole | tojson))"
                      else empty end )
                elif has("metavariable-comparison") then
                  ( .["metavariable-comparison"].comparison | compareSteps
                    | if . == null then empty else join("\n") end )
                elif has("metavariable-pattern") then
                  ( .["metavariable-pattern"] as $mv
                    | ($mv.metavariable | ltrimstr("$") | ltrimstr("...")) as $hole
                    | if ($mv | has("pattern")) and ($mv.pattern | type) == "string"
                      then "  | where_capture_ast(\($hole | tojson); \($mv.pattern | leaf($mode) | tojson))"
                      elif $mv | has("pattern-regex")
                      then "  | where_capture(\($hole | tojson); \($mv["pattern-regex"] | re2Fixups | anchored | tojson))"
                      elif $mv | has("pattern-not-regex")
                      then "  | where_capture_not(\($hole | tojson); \($mv["pattern-not-regex"] | re2Fixups | anchored | tojson))"
                      else empty end )
                elif has("focus-metavariable") then
                  ( .["focus-metavariable"]
                    | if type == "array" then .[0] else . end
                    | "  | focus(\(ltrimstr("$") | ltrimstr("...") | tojson))" )
                else empty end ) ]
          | join("\n") ) as $steps
      | if $steps == "" then $base else "\($base)\n\($steps)" end )
  else
    # A guard standing on its own - `pattern-inside` as one alternative of a
    # `pattern-either` - is a guard with nothing to guard, which is every match
    # in the file narrowed by it. Read as a one-element `patterns` it becomes
    # exactly that. Without this it was read as $all, and a source that is
    # every node is a taint rule that reports every line.
    #
    # Only a node that is nothing but guards is rewrapped. One that names a
    # pattern reached here by some other route, and rewrapping it would put it
    # straight back into this branch.
    ( if (opKeys | length) == 0
         or any(opKeys[]; IN("pattern", "pattern-either", "patterns"))
      then "$all"
      else ({patterns: [.]} | expr($keep; $mode)) end )
  end;

# ---------------------------------------------------------------- the rule

# notBody names the keys that describe a rule rather than say what it looks
# for. What is left is the operator tree, so an operator this translator has
# never heard of shows up as one rather than being skipped over.
def notBody:
  ["id", "message", "languages", "severity", "metadata", "mode", "fix",
   "fix-regex", "options", "paths", "min-version", "max-version"];

def bodyOf: with_entries(select(.key | IN(notBody[]) | not));

# firstSentence keeps a rule's message short enough to read in a terminal.
def firstSentence:
  (. // "") | gsub("\\s+"; " ") | sub("^ +"; "") | sub(" +$"; "")
  | (capture("^(?<s>.*?[.!?])(\\s|$)") | .s) // .;

# verdict decides whether a rule can be translated, and says why not when it
# cannot. The order is the order the reasons stop mattering in: a taint rule's
# patterns are not worth compiling.
# kindOf says which of the three shapes a rule has: a search over syntax, a
# search over text, or a value followed from one place to another.
def kindOf:
  if .mode == "taint" then "taint"
  elif (.raw | any(ascii_downcase == "generic")) then "generic"
  elif (.raw | any(ascii_downcase == "regex")) then "regex"
  else "ast" end;

# textGlobs are the files a text rule is about. A rule that names none is about
# every file, which is what a rule with no grammar to narrow it means.
def textGlobs:
  ((.paths.include // []) | map(tostring)) as $named
  | if ($named | length) > 0 then $named else ["*"] end;

# usableRegex reports whether RE2 will take a pattern, which is where a rule
# needing lookaround is turned away.
def usableRegex($re): (try ("" | test($re) | true) catch false);

# verdictAst decides a search over syntax, which is the case every other one is
# a variation on: the patterns have to be code in at least one of the languages
# the rule claims.
def verdictAst:
  . as $r
  | ($r.body | patternsIn) as $pats
  | if ($r.langs | length) == 0 then
      {ported: false, why: "no grammar for \($r.raw | join(", "))"}
    elif ($pats | length) == 0 then
      {ported: false, why: "no structural pattern; it matches text"}
    else
      ( [ $r.langs[] | . as $l
          | select(all($pats[]; ast_pattern($l.grammar) | .Valid)) ] ) as $ok
      | if ($ok | length) == 0 then
          { ported: false,
            why: ( [ $r.langs[0] as $l
                     | $pats[] | select((ast_pattern($l.grammar) | .Valid) | not)
                     | ast_pattern($l.grammar) | .Problem
                     | sub("^pattern .*?\" "; "") ]
                   | first // "no pattern compiles" ) }
        else
          # A widened spelling is kept only where it is code in every language
          # the rule survived for; the guard falls back to what was written.
          ( [ ($r.body | guardSpellings)[]
              | . as $extra
              | select(($pats | index($extra)) == null)
              | select(all($ok[]; . as $l | ($extra | ast_pattern($l.grammar)) | .Valid)) ] ) as $extras
          | {ported: true, langs: $ok, pats: ($pats + $extras)}
        end
    end;

# verdictText decides a search over text. There is no grammar to be code in;
# what has to hold is that RE2 will take every regex the rule ends up running.
def verdictText:
  . as $r
  | ($r.body | asTextBody) as $body
  | ($body | textPatterns($r.kind)) as $pats
  | if ($pats | length) == 0 then
      {ported: false, why: "no pattern to search for"}
    elif any($pats[]; usableRegex(.) | not) then
      {ported: false, why: "regex needs a feature RE2 does not have (lookaround)"}
    else {ported: true, langs: [], pats: $pats, body: $body, globs: ($r.globs)}
    end;

# verdictTaint decides a rule that follows a value. Both ends have to be code:
# a rule with sources it cannot find, or sinks it cannot find, reports nothing
# whatever the following does.
def verdictTaint:
  . as $r
  | ($r.body | taintPatterns("pattern-sources")) as $sources
  | ($r.body | taintPatterns("pattern-sinks")) as $sinks
  | ($r.body | taintPatterns("pattern-sanitizers")) as $clean
  | ($sources + $sinks + $clean) as $pats
  | if ($r.langs | length) == 0 then
      {ported: false, why: "no grammar for \($r.raw | join(", "))"}
    elif ($sources | length) == 0 or ($sinks | length) == 0 then
      {ported: false, why: "taint rule with no \(if ($sources|length) == 0 then "sources" else "sinks" end)"}
    else
      ( [ $r.langs[] | . as $l | select(all($pats[]; ast_pattern($l.grammar) | .Valid)) ] ) as $ok
      | if ($ok | length) == 0 then
          { ported: false,
            why: ( [ $r.langs[0] as $l
                     | $pats[] | select((ast_pattern($l.grammar) | .Valid) | not)
                     | ast_pattern($l.grammar) | .Problem
                     | sub("^pattern .*?\" "; "") ]
                   | first // "no pattern compiles" ) }
        else
          {ported: true, langs: $ok, pats: $pats,
           sources: $sources, sinks: $sinks, clean: $clean}
        end
    end;

# asText is a rule read as a search over text: the same patterns, over the same
# files, matched a token at a time rather than a construct at a time.
#
# It is what is left when no grammar will take a rule's patterns, and for a
# whole class of rules it is not a fallback at all but the right reading.
# Dockerfile is the case: `EXPOSE $PORT` is a line, `$` is a shell variable to
# the grammar rather than a hole, and there is no parse of the pattern that
# means what the rule means. As a regex over the same files it says exactly
# what it said.
#
# It is looser than a parse everywhere else - a `...` here crosses a boundary
# a parse would not - so it is only ever reached after the syntax reading has
# been refused, and VALIDATION.json is what says whether the rule still finds
# what it was written to find.
def asText:
  . as $r
  | $r + {kind: "generic",
          globs: (if (($r.paths.include // []) | length) > 0 then $r.globs
                  else ([($r.langs // [])[] | .glob] | unique) end)};

def verdict:
  . as $r
  | ($r.body | opsIn) as $ops
  # Every regex the rule will actually run, wherever it is written: on a hole,
  # as a guard over the file's text, or inside a metavariable-pattern. A rule
  # is refused for one RE2 will not take, because a query that fails to compile
  # when it is run is worse than a rule that was never shipped.
  | [ $r.body | .. | objects
      | ( (.["metavariable-regex"]? | select(. != null) | .regex | holeRegexes[] | .re),
          (.["pattern-regex"]? | select(type == "string")),
          (.["pattern-not-regex"]? | select(type == "string") ) ) ] as $regexes
  | [ $r.body | .. | objects | .["metavariable-analysis"]? | select(. != null)
      | .analyzer | select(IN("entropy", "redos") | not) ] as $unknownAnalyzers
  | [ $r.body | .. | objects | .["metavariable-comparison"]? | select(. != null)
      | .comparison | select(compareSteps == null) ] as $unreadable
  | if (($ops - knownOps) | length) > 0 then
      {ported: false, why: "operator: \(($ops - knownOps) | join(", "))"}
    elif ($unknownAnalyzers | length) > 0 then
      # A constraint quietly dropped is a rule that reports more than it said.
      {ported: false, why: "analyzer: \($unknownAnalyzers | unique | join(", "))"}
    elif ($unreadable | length) > 0 then
      {ported: false, why: "comparison: \($unreadable | unique | join("; "))"}
    elif any($regexes[]; usableRegex(.) | not) then
      # A hole's regex is Perl where pwrq's is RE2, and RE2 has no lookaround.
      # A rule that needs one is not translated rather than translated with the
      # constraint quietly dropped.
      {ported: false, why: "regex needs a feature RE2 does not have (lookaround)"}
    elif $r.kind == "taint" then verdictTaint
    elif $r.kind == "ast" then
      ( verdictAst as $syntax
        | if $syntax.ported then $syntax
          else ($r | asText) as $t
            | ($t | verdictText) as $text
            | if $text.ported
              # The languages stay on the rule: they are what it was written
              # for, and they are how its fixture upstream is found. What
              # changes is that they are no longer how it is searched.
              then $text + {kind: $t.kind, globs: $t.globs, langs: $r.langs}
              # The syntax reading's reason is the one worth reporting: it says
              # what about the pattern a grammar could not follow, where the
              # text reading's says only that a regex would not compile.
              else $syntax end
          end )
    else verdictText
    end;

# rulesOf reads the rules out of one file and says of each whether it can be
# translated.
def rulesOf:
  [ (.rules // [])[]
    | select(type == "object")
    | { id: (.id // "unnamed"),
        message: (.message | firstSentence),
        severity: (.severity // "WARNING" | tostring),
        raw: [(.languages // [])[] | tostring],
        langs: ((.languages // []) | targets),
        mode: (.mode // "search"),
        globs: textGlobs,
        body: bodyOf }
    | . + {kind: kindOf}
    | . + verdict ];

# ruleExpr is the pipeline one rule narrows the search with, whichever of the
# three shapes it has.
# taintSide is one half of a taint rule as the search it describes.
#
# A source is not a list of patterns, it is an operator tree like any other:
# `$A`, inside a loop over the results of getopt, is a source, and `$A` on its
# own is every identifier in the file. Flattening the tree to its leaves and
# treating each as a source is how a rule meant to find one thing comes to
# report a hundred, so each entry is built with expr, the same as the search
# half of a rule.
def taintSide($section; $keep):
  [ (.[$section] // [])[]
    | if type == "string" then "$all | of([\(rewrite | tojson)])"
      else "(" + expr($keep; "ast") + ")" end ]
  | if length == 0 then "[]" else join("\n      + ") end;

def ruleExpr:
  . as $rule
  | if $rule.kind == "taint" then
      "(\($rule.body | taintSide("pattern-sinks"; $rule.pats)))\n"
      + "  | reaching((\($rule.body | taintSide("pattern-sources"; $rule.pats)));\n"
      + "      (\($rule.body | taintSide("pattern-sanitizers"; $rule.pats))))"
    elif $rule.kind == "ast" then
      ($rule.body | expr($rule.pats; "ast"))
    else
      ($rule.body | asTextBody | expr($rule.pats; $rule.kind))
    end;

# ---------------------------------------------------------------- the query

# indented lines up a rule's pipeline under the scan it reads from.
def indented($by): split("\n") | join("\n" + $by);

# query renders the translated rules of one file as a single pwrq query.
#
# Rules that look at the same files share a scan, because a scan is a walk of
# the tree and a parse of every file in it, and a file of eight rules about Go
# should cost one of those rather than eight.
#
# The header is what the loader reads: `rules:` names the ids the query reports
# under, so a caller can ask for one by name, and `from:` is the provenance
# that lets a translation be checked against its original.
def query($from):
  [.[] | select(.ported)] as $ok
  | ($ok | group_by([.kind, (if .kind == "ast" or .kind == "taint"
                             then (.langs | map(.glob) | unique) else .globs end)] | tojson)) as $groups
  | ([$ok[] | select(.kind == "ast" or .kind == "taint")
      | (.langs // [])[] | .grammar] | unique) as $grammars
  | ( "# rules: \([$ok[].id] | join(", "))\n"
    + (if ($grammars | length) > 0
       then "# languages: \($grammars | join(", "))\n" else "" end)
    + "# from: \($from)\n\n"
    + ". as $root\n"
    + "| "
    + ( [ $groups[]
          | .[0].kind as $kind
          | (if $kind == "ast" or $kind == "taint"
             then (.[0].langs | map(.glob) | unique) else .[0].globs end) as $globs
          | ([.[] | .pats[]] | unique) as $pats
          | ( "[\n      " + ([$pats[] | tojson] | join(",\n      ")) + "\n    ]" ) as $list
          | (if $kind == "ast" or $kind == "taint" then "scan_ast" else "scan_regex" end) as $scan
          | "( ($root | \($scan)(\($globs | tojson); \($list))) as $all\n"
            + "    | " + ( [ .[] | . as $rule
                             | "( \($rule | ruleExpr | indented("        "))\n"
                               + "        | finding(\($rule.id | tojson);\n"
                               + "            \($rule.message | tojson)) )" ]
                           | join("\n      + ") )
            + " )" ]
        | join("\n+ ") )
    + "\n| report\n" );

# ------------------------------------------------------------------ output
#
# One object per input file: the query text, and a line for every rule in it
# whether it was translated or not.
def port($from):
  rulesOf as $rules
  | { from: $from,
      rules: [ $rules[]
               | { id, from: $from,
                   languages: .raw,
                   ported: .ported,
                   why: (.why // null),
                   grammars: [(.langs // [])[] | .grammar],
                   searches: (if .kind == "ast" or .kind == "taint"
                              then ([(.langs // [])[] | .glob] | unique)
                              else (.globs // []) end) } ],
      query: (if ([$rules[] | select(.ported)] | length) == 0 then null
              else ($rules | query($from)) end) };
