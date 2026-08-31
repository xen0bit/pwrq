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

# rewrite is the source hole syntax in pwrq's. The two agree on `$NAME`; they
# differ on the variadic, which is spelled `...` where it means "and anything
# else here" and `$...NAME` where it names the run. The named form goes first,
# because `$...ARGS` contains the anonymous one. What is left is every
# remaining `...`, with no attempt to spot the ones that are not holes: Ruby
# spells an exclusive range `1...5`, and a pattern that writes one is
# mistranslated. None in this corpus does, and a rule that did would fail to
# compile rather than match the wrong thing.
def rewrite:
  gsub("\\$\\.\\.\\.(?<n>[A-Z_][A-Z_0-9]*)"; "$$$\(.n)")
  | gsub("\\.\\.\\."; "$$$_")
  | sub("^\\s+"; "") | sub("\\s+$"; "");

# ------------------------------------------------------- the operator tree
#
# A rule's body is a tree of operators over patterns. These walk it twice: once
# to collect every pattern that has to be compiled, and once to build the match
# tree that combines them.

def leafOps: ["pattern", "pattern-not", "pattern-inside", "pattern-not-inside"];

# patternsIn collects every leaf pattern in a rule body, as written.
def patternsIn:
  [ .. | objects | to_entries[] | select(.key | IN(leafOps[])) | .value
        | select(type == "string") | rewrite ] | unique;

# knownOps are the operators this translator has a step for. Anything else -
# taint mode, focus-metavariable, metavariable-comparison - is a rule left
# untranslated rather than approximated.
def knownOps:
  ["pattern", "patterns", "pattern-either", "pattern-inside", "pattern-not",
   "pattern-not-inside", "metavariable-regex", "metavariable", "regex"];

def opsIn:
  [ .. | objects | keys[] ] | unique;

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
def expr($keep):
  # operand is the matches one side of an operator describes: a pattern
  # written inline, or a whole nested block of them. It sits inside expr
  # because the two call each other.
  # Every operand is a question about the whole search, not about what the
  # pipeline has narrowed to so far: "not inside a span this matched" means any
  # span in the file, not one among the matches left. So an operand starts from
  # $all again.
  def operand:
    if type == "string" then "$all | of([\(rewrite | tojson)])"
    else "(" + expr($keep) + ")"
    end;
  if has("pattern") and (keys | length) == 1 then
    "$all | of([\(.["pattern"] | rewrite | tojson)])"
  elif has("pattern-either") then
    ( .["pattern-either"] as $alts
      | if ($alts | all(has("pattern") and (keys | length) == 1)) then
          "$all | of(\([$alts[].["pattern"] | rewrite] | tojson))"
        else
          "(" + ([$alts[] | expr($keep)] | join(")\n  + (")) + ")"
        end )
  elif has("patterns") then
    ( .["patterns"] as $parts
      | [$parts[] | select(has("pattern") or has("pattern-either"))] as $positive
      | [$parts[] | select((has("pattern") or has("pattern-either")) | not)] as $guards
      | (if ($positive | length) == 0 then "$all" else ($positive[0] | expr($keep)) end) as $base
      | ( [ ($positive[1:][] | "  | at_same_place(\(. | expr($keep) | "(" + . + ")"))"),
            ( $guards[]
              | if has("pattern-inside") then
                  (.["pattern-inside"] as $in
                   | if ($in | type) == "string" and ($in | rewrite | fileScope)
                     then "  | in_files_with($all | of(\($in | rewrite | spellings | map(select(IN($keep[]))) | tojson)))"
                     else "  | within(\($in | operand))" end)
                elif has("pattern-not-inside") then
                  (.["pattern-not-inside"] as $in
                   | if ($in | type) == "string" and ($in | rewrite | fileScope)
                     then "  | in_files_without($all | of(\($in | rewrite | spellings | map(select(IN($keep[]))) | tojson)))"
                     else "  | outside(\($in | operand))" end)
                elif has("pattern-not") then
                  "  | not_at(\(.["pattern-not"] | operand))"
                elif has("metavariable-regex") then
                  ( .["metavariable-regex"]
                    | "  | where_capture(\(.metavariable | ltrimstr("$") | tojson); \(.regex | tojson))" )
                else empty end ) ]
          | join("\n") ) as $steps
      | if $steps == "" then $base else "\($base)\n\($steps)" end )
  else "$all"
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
def verdict:
  . as $r
  | ($r.body | opsIn) as $ops
  | ($r.body | patternsIn) as $pats
  | [ $r.body | .. | objects | .["metavariable-regex"]? | select(. != null) | .regex ] as $regexes
  | if $r.mode != "search" then {ported: false, why: "mode: \($r.mode)"}
    elif ($r.langs | length) == 0 then
      {ported: false, why: "no grammar for \($r.raw | join(", "))"}
    elif ($pats | length) == 0 then
      {ported: false, why: "no structural pattern; it matches text"}
    elif (($ops - knownOps) | length) > 0 then
      {ported: false, why: "operator: \(($ops - knownOps) | join(", "))"}
    elif any($regexes[]; . as $re | (try ("" | test($re) | false) catch true)) then
      # A hole's regex is Perl where pwrq's is RE2, and RE2 has no lookaround.
      # A rule that needs one is not translated rather than translated with the
      # constraint quietly dropped.
      {ported: false, why: "regex needs a feature RE2 does not have (lookaround)"}
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
        body: bodyOf }
    | . + verdict ];

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
  | ($ok | group_by(.langs | map(.glob) | unique | tojson)) as $groups
  | ( "# rules: \([$ok[].id] | join(", "))\n"
    + "# from: \($from)\n\n"
    + ". as $root\n"
    + ( [ $groups[]
          | (.[0].langs | map(.glob) | unique) as $globs
          | ([.[] | .pats[]] | unique) as $pats
          | ( "[\n      " + ([$pats[] | tojson] | join(",\n      ")) + "\n    ]" ) as $list
          | "| ( ($root | scan_ast(\($globs | tojson); \($list))) as $all\n"
            + "    | " + ( [ .[] | . as $rule
                             | "( \($rule.body | expr($rule.pats) | indented("        "))\n"
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
                   grammars: [(.langs // [])[] | .grammar] } ],
      query: (if ([$rules[] | select(.ported)] | length) == 0 then null
              else ($rules | query($from)) end) };
