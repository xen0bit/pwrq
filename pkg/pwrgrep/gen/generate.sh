#!/usr/bin/env bash
#
# Regenerate ../rules from a checkout of the rule set it was translated from.
#
#   git clone --depth 1 https://github.com/opengrep/opengrep-rules /tmp/rules-src
#   go build -o /tmp/pwrq ./cmd/pwrq
#   SRC=/tmp/rules-src PWRQ=/tmp/pwrq pkg/pwrgrep/gen/generate.sh
#
# Every generated rule file is written by port.jq, and MANIFEST.json accounts
# for every rule in the source set - translated or not, with the reason.
# Nothing is edited by hand: a fix goes in port.jq and the corpus is
# regenerated. The rules under ../rules/pwrq are written rather than generated
# and are left alone.
set -uo pipefail

SRC=${SRC:-/tmp/opengrep-rules}
PWRQ=${PWRQ:-./pwrq}
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
OUT=$(cd "$HERE/../rules" && pwd)
JOBS=${JOBS:-8}

# handWritten is what in the corpus generate.sh does not own: the rules
# written by hand rather than translated.
handWritten=pwrq

[ -d "$SRC" ] || { echo "no rule checkout at $SRC" >&2; exit 1; }
command -v "$PWRQ" >/dev/null 2>&1 || [ -x "$PWRQ" ] || {
  echo "no pwrq at $PWRQ; set PWRQ=" >&2; exit 1; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# port one rule file: the rules to their place in the tree, every rule in it to
# the manifest.
port_one() {
  local f=$1 rel dir out
  rel=${f#"$SRC"/}
  dir=$(dirname "$rel")
  "$PWRQ" --yaml-input -c -L "$HERE" 'include "port"; port($from)' \
      --arg from "$rel" < "$f" > "$work/one.$$" 2>/dev/null || return 0
  [ -s "$work/one.$$" ] || return 0
  "$PWRQ" -c '.rules[]' < "$work/one.$$" >> "$work/manifest.$$.ndjson" 2>/dev/null
  out="$OUT/$dir/$(basename "${rel%.*}").pwrq"
  mkdir -p "$(dirname "$out")"
  "$PWRQ" -j -r '.query // empty' < "$work/one.$$" > "$out.tmp" 2>/dev/null
  if [ -s "$out.tmp" ]; then mv "$out.tmp" "$out"; else rm -f "$out.tmp"; fi
}
export -f port_one
export SRC PWRQ HERE OUT work

# Everything generated goes; what is written by hand stays. Done by name
# rather than by glob so that the paths being removed are the ones listed.
python3 - "$OUT" "$handWritten" <<'PY'
import os, shutil, sys
out, keep = sys.argv[1], set(sys.argv[2].split(","))
for name in sorted(os.listdir(out)):
    if name in keep:
        continue
    path = os.path.join(out, name)
    shutil.rmtree(path) if os.path.isdir(path) else os.remove(path)
PY

find "$SRC" \( -name '*.yaml' -o -name '*.yml' \) -type f \
  | grep -vE '/(\.git|scripts|stats|libsonnet)/|/template\.yaml$|\.test\.ya?ml$' \
  | sort \
  | xargs -P "$JOBS" -I{} bash -c 'port_one "$@"' _ {}

cat "$work"/manifest.*.ndjson \
  | "$PWRQ" -s -c 'sort_by(.from, .id)' > "$HERE/MANIFEST.json"

echo "rule files: $(find "$OUT" -name '*.pwrq' | wc -l)"
echo "rules:      $("$PWRQ" -r 'length' < "$HERE/MANIFEST.json") ($("$PWRQ" -r '[.[]|select(.ported)]|length' < "$HERE/MANIFEST.json") translated)"
