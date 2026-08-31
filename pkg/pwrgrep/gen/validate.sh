#!/usr/bin/env bash
#
# Run every translated rule against the fixture its original was tested with,
# and write the result into VALIDATION.json.
#
#   SRC=/tmp/rules-src PWRQ=/tmp/pwrq pkg/pwrgrep/gen/validate.sh
#
# A rule is not measured on whether it produces findings; it is measured on
# whether it produces the ones the fixture marks, and none of the ones it marks
# as fine. See validate.jq.
set -uo pipefail

SRC=${SRC:-/tmp/opengrep-rules}
PWRQ=${PWRQ:-./pwrq}
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
JOBS=${JOBS:-8}

[ -d "$SRC" ] || { echo "no rule checkout at $SRC" >&2; exit 1; }
[ -f "$HERE/MANIFEST.json" ] || { echo "no MANIFEST.json; run generate.sh" >&2; exit 1; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# The file a rule's fixture is written in, by the grammar the rule was
# translated for. A rule for several languages has a fixture for each.
suffixes() {
  case $1 in
    go) echo .go;; python) echo .py;; javascript) echo .js;;
    typescript) echo ".ts .tsx";; java) echo .java;; ruby) echo .rb;;
    php) echo .php;; c) echo .c;; cpp) echo .cpp;; c_sharp) echo .cs;;
    kotlin) echo .kt;; scala) echo .scala;; rust) echo .rs;; swift) echo .swift;;
    solidity) echo .sol;; elixir) echo .ex;; clojure) echo .clj;; ocaml) echo .ml;;
    bash) echo ".sh .bash";; dockerfile) echo ".dockerfile";; json) echo .json;;
    yaml) echo ".yaml .yml";; hcl) echo .tf;; apex) echo .cls;; html) echo .html;;
    lua) echo .lua;; dart) echo .dart;; *) echo "";;
  esac
}

check_one() {
  local rel=$1 ids=$2 grammars=$3
  local dir base tmp fixtures=() s g
  dir=$(dirname "$rel"); base=$(basename "${rel%.*}")
  # Named for the rule file, not for the shell: these run several at a time in
  # one shell, where $$ is the same number in all of them.
  tmp="$work/$(printf '%s' "$rel" | tr '/' '_')"
  for g in $grammars; do
    for s in $(suffixes "$g"); do
      [ -f "$SRC/$dir/$base$s" ] && fixtures+=("$SRC/$dir/$base$s")
    done
  done
  if [ ${#fixtures[@]} -eq 0 ]; then
    "$PWRQ" -n -c --arg from "$rel" --argjson ids "$ids" \
      '$ids[] | {from: $from, id: ., validated: false, why: "no fixture upstream"}' > "$tmp.json"
    return 0
  fi
  # One run for the whole rule file, named by its path rather than by an id:
  # two rules may report under one id, and a file is measured on its own.
  printf '%s\n' "${fixtures[@]}" | "$PWRQ" -R -s -c 'split("\n") | map(select(. != ""))' > "$tmp.fixtures"
  "$PWRQ" -n -c --arg rule "${rel%.*}" --slurpfile fixtures "$tmp.fixtures" '
    [ $fixtures[0][] as $f
      | (try [invoke_pwrgrep($f; $rule)] catch [{Error: .}])[] ]' > "$tmp.found" 2>/dev/null
  [ -s "$tmp.found" ] || printf '[]' > "$tmp.found"
  # Findings and fixture text are passed as files rather than as arguments:
  # some of both are longer than a command line is allowed to be.
  cat "${fixtures[@]}" > "$tmp.text"
  "$PWRQ" -n -c -L "$HERE" --arg from "$rel" --rawfile text "$tmp.text" \
      --argjson ids "$ids" --slurpfile found "$tmp.found" '
    include "validate";
    ($found[0] // []) as $findings
    | ($text | marks) as $m
    | $ids[] as $id
    | [$findings[] | select(has("Error") or .RuleId == $id)] as $mine
    | { from: $from, id: $id }
      + if ($mine | any(has("Error"))) then
          {validated: false, why: ([$mine[] | .Error // empty] | first | tostring | .[0:200])}
        else ( score($id; $m; [$mine[] | .LineNumber])
               | if .marked then {validated: true} + .
                 else {validated: false, why: "fixture marks no line for this rule"} end )
        end' > "$tmp.json"
}
export -f check_one suffixes
export SRC PWRQ HERE work

"$PWRQ" -r '[.[] | select(.ported)] | group_by(.from)[]
     | "\(.[0].from)\t\([.[].id] | tojson)\t\([.[].grammars[]] | unique | join(" "))"' \
    < "$HERE/MANIFEST.json" > "$work/plan.tsv"

while IFS=$'\t' read -r rel ids grammars; do
  [ -n "$rel" ] || continue
  check_one "$rel" "$ids" "$grammars" &
  while [ "$(jobs -rp | wc -l)" -ge "$JOBS" ]; do wait -n; done
done < "$work/plan.tsv"
wait

cat "$work"/*.json | "$PWRQ" -s -c 'sort_by(.from, .id)' > "$HERE/VALIDATION.json"
"$PWRQ" -r '
  [.[] | select(.validated)] as $v
  | "rules with a fixture that marks them: \($v | length) of \(length)",
    "  exactly as the fixture marks:  \([$v[] | select(.exact)] | length)",
    "  some of what it marks:         \([$v[] | select(.exact | not) | select((.found|length) > 0)] | length)",
    "  none of what it marks:         \([$v[] | select((.found|length) == 0)] | length)"' \
  < "$HERE/VALIDATION.json"
