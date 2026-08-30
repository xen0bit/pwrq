# Composing structural matches into rules.
#
# select_ast answers one question - where does this piece of syntax occur - and
# a real rule is several of those answers combined. "MD5, but only in a file
# that imports crypto/md5." "Taking the address of a variable, but only inside
# a range loop." "A call to Query, but not one whose argument is a constant."
# The patterns are the easy half; the combining is where a rule lives, and this
# module is that half.
#
# The vocabulary is Semgrep's, because that is the vocabulary the rules being
# ported are written in. Each operator below names the one it stands in for, so
# a port can be read against its original.
#
# The input to every one of these is an array of matches from a single
# select_ast call. One call rather than one per pattern is the point: the tree
# is walked once and each file parsed once however many patterns a rule has, so
# a rule with six alternatives costs what a rule with one costs.

# scan($files; $patterns) is the search a rule starts from. The input is the
# path to search; $files is the glob naming the files the rule is about; the
# output is every match of every pattern, in file order.
#
# The glob is not an optimisation. A pattern is code in more languages than the
# one it was written for - `$A != $A` is a useless comparison in Go and in
# JavaScript both - so a Go rule run over a repository with a vendored
# jquery.min.js in it reports findings in jquery.min.js, correctly and
# uselessly. Naming the files is how a rule says which language it is about.
#
# It is also what makes a rule for one language runnable over a tree that has
# none of it: a glob that matches no file leaves nothing skipped, and an empty
# result rather than "no pattern was code in anything found here".
def scan($files; $patterns): . as $root | [select_ast($root; $patterns; {Include: $files})];

# of($p) keeps the matches that one pattern found, or that any pattern in a
# list found. This is `pattern`, and a list of them is `pattern-either`.
def of($p):
  (if ($p | type) == "array" then $p else [$p] end) as $wanted
  | map(select(.Pattern | IN($wanted[])));

# encloses($m) is true when this match's span covers $m's, which is what makes
# "inside" mean anything. It is why a match carries byte offsets: line numbers
# cannot tell whether two things on one line contain each other.
def encloses($m):
  .Path == $m.Path and .Offset <= $m.Offset and .EndOffset >= $m.EndOffset;

# within($outer) keeps the matches some match in $outer encloses. This is
# `pattern-inside`.
def within($outer): map(. as $m | select(any($outer[]; encloses($m))));

# outside($outer) keeps the rest. This is `pattern-not-inside`.
#
# An empty $outer keeps everything, which is the right answer and worth saying
# out loud: "not inside anything, because there was nothing to be inside".
def outside($outer): map(. as $m | select(all($outer[]; encloses($m) | not)));

# in_files_with($other) keeps the matches in files that $other also matched.
#
# This is `pattern-inside` used the way half the rules in the corpus use it -
# an import at the top of the file, meant as "this file uses that package"
# rather than as a span the call sits inside. Written as within() it would
# match nothing, because a call is not inside an import statement.
def in_files_with($other):
  ($other | map(.Path) | unique) as $paths
  | map(select(.Path | IN($paths[])));

# in_files_without($other) is the same question answered the other way, and
# stands in for `pattern-not-inside` used at file scope.
def in_files_without($other):
  ($other | map(.Path) | unique) as $paths
  | map(select(.Path | IN($paths[]) | not));

# at_same_place($other) keeps the matches that start and end exactly where a
# match in $other does. This is `patterns` - two patterns that must both
# describe the same code - as opposed to two patterns that must both appear.
def at_same_place($other):
  map(. as $m | select(any($other[]; .Path == $m.Path and .Offset == $m.Offset and .EndOffset == $m.EndOffset)));

# not_at($other) is its negation, and stands in for `pattern-not`.
def not_at($other):
  map(. as $m | select(all($other[]; .Path != $m.Path or .Offset != $m.Offset or .EndOffset != $m.EndOffset)));

# where_capture($name; $re) keeps the matches whose named hole caught text
# matching a regex. This is `metavariable-regex`.
#
# A hole the pattern did not have catches nothing, and nothing matches no
# regex, so a misspelled name filters everything out rather than filtering
# nothing - the safe direction for a rule that decides what to report.
def where_capture($name; $re): map(select((.Captures[$name] // "") | test($re)));

# where_capture_not($name; $re) is the negation.
def where_capture_not($name; $re): map(select(((.Captures[$name] // "") | test($re)) | not));

# where_same($a; $b) keeps the matches whose two named holes caught the same
# text, and where_different($a; $b) keeps the rest.
#
# This is what Semgrep writes as one metavariable used twice - `$X == $X` - and
# it has to be written as two, because a match keeps one capture per name and
# so a repeated name cannot constrain anything. select_ast refuses a pattern
# that repeats one, rather than matching everywhere and calling it a hit.
def where_same($a; $b): map(select(.Captures[$a] != null and .Captures[$a] == .Captures[$b]));

# where_different($a; $b) is the negation, for a rule that needs the two holes
# to have caught different code.
def where_different($a; $b): map(select(.Captures[$a] != .Captures[$b]));

# finding($id; msg) renders matches as findings. msg is a filter rather than a
# string, so a rule can say what it found - `"key \(.Captures.K) is hardcoded"`
# - as easily as it can say the same sentence every time.
def finding($id; msg):
  map({
    RuleId: $id,
    Path: .Path,
    LineNumber: .LineNumber,
    Column: .Column,
    EndLineNumber: .EndLineNumber,
    Message: msg,
    Match: .Text,
  });

# report puts findings in the order a person reads them, and drops the
# duplicates that alternatives produce when two of them describe the same code.
def report: unique_by([.RuleId, .Path, .LineNumber, .Column, .Match]);
