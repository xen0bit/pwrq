#!/usr/bin/env bash
# Exercises every corpus entry through the installed pwrq binary.
#
#   corpus/tools/validate.sh              # parse, fill, run and audit everything
#   corpus/tools/validate.sh fs.pwrq      # one file only
#
# A corpus entry is a recipes.pwrq block: `## id`, `# task:`, optional
# `# also:` and `# slot <name> <kind> = <default>` lines, then the query.
# The harness fills each slot with its default (the normalization
# pkg/ideengine's Fill applies to the plain-literal defaults the corpus
# uses), compiles and runs the query in a scratch fixture directory, and
# classifies the result:
#
#   PASS    ran, exited 0, produced output
#   EMPTY   ran, exited 0, produced no output  (only entries listed in
#           MAYBE_EMPTY may: a getter that finds nothing still ran)
#   ENV     failed with the environmental error the entry declares (no
#           systemd, no CENSYS token, no API key) - what the entry is
#           expected to meet on this machine, as opposed to a compile error
#   TMO     blocked, declared: a server or watcher, killed at its timeout
#   FAIL    anything else, including parse, arity and runtime errors
#
# The scratch directory is rebuilt under /tmp/opencode on every run, so
# destructive entries (rm, move, stop_process, SQL deletes) only ever hit
# throwaway files.

set -u
CORPUS="$(cd "$(dirname "$0")/.." && pwd)"
REPO="$(cd "$CORPUS/.." && pwd)"
SCRATCH="/tmp/opencode/corpus-scratch"
FILLED="/tmp/opencode/corpus-filled"
mkdir -p "$SCRATCH" "$FILLED"

# ------------------------------------------------------------- fixtures --
fixtures() {
  rm -rf "$SCRATCH"
  mkdir -p "$SCRATCH"/{src/util,web,img,vid,a,b,configs}
  cd "$SCRATCH" || exit 1

  printf 'the quick brown fox\njumps over the lazy dog\nTODO: rewrite this ending\nthe end\n' > notes.txt
  printf 'the payload aGVsbG8gd29ybGQgZnJvbSB0aGUgY29ycHVz is hidden here\n' > blob.txt
  printf 'H4sIAAAAAAAAAyvJSFUoLM1MzlZIKsovz1NIy69QyCrNLUhNUcgvSy1SKAHK5yRWVSqk5KcrlJRnJqcCANlkAiAyAAAA\n' > blob-gz.b64
  printf 'eyJhIjp7ImIiOjJ9fQ==\n' > doc.b64
  printf 'aHR0cHM6Ly9leGFtcGxlLmNvbQ==\n' > url.b64
  printf 'MjAyNi0wOC0xMVQxMjowMDowMFo=\n' > date.b64
  printf '' > empty.txt
  printf 'a hidden note\n' > .hidden.txt
  printf 'line one\nline two\nline three\n' > a/dup.txt
  printf 'line one\nline two\nline three\n' > b/dup.txt
  printf 'different content\n' > b/other.txt
  printf 'ts=2026-08-11T09:12:03Z level=info svc=orders msg="request ok" dur_ms=84\nts=2026-08-11T09:12:31Z level=info svc=orders msg="created" dur_ms=412\nts=2026-08-11T09:15:44Z level=error svc=payments msg="upstream timeout" dur_ms=1902\nts=2026-08-11T09:16:08Z level=warn svc=orders msg="slow query" dur_ms=980\nts=2026-08-11T09:17:52Z level=error svc=payments msg="upstream timeout" dur_ms=2104\nts=2026-08-11T09:18:30Z level=info svc=gateway msg="denied" dur_ms=6\n' > metrics.logfmt
  printf 'INFO start\nERROR failed to open config\nWARN slow query took 980ms\nERROR failed again\nINFO done\n' > app.log
  printf 'INFO yesterday: started batch\nERROR yesterday: payment gateway timeout\nINFO yesterday: batch done\n' > app.log.1
  printf 'ERROR rotated: disk nearly full\nWARN rotated: queue backing up\n' > app.log.2
  printf 'example.com\napi.github.com\none.one.one.one\n' > hosts.txt
  printf 'https://example.com\nhttps://api.github.com\n' > urls.txt
  head -c 8192 /dev/urandom > sample1.bin
  { cat sample1.bin; printf 'similar but not quite the same'; } > sample2.bin
  cat > access.log <<'EOF'
10.0.0.14 - - [11/Aug/2026:09:12:03 +0000] "GET /api/orders HTTP/1.1" 200 1043 0.084
203.0.113.7 - - [11/Aug/2026:09:12:31 +0000] "POST /api/orders HTTP/1.1" 201 219 0.412
10.0.0.14 - - [11/Aug/2026:09:13:02 +0000] "GET /api/orders/88 HTTP/1.1" 404 96 0.011
198.51.100.22 - - [11/Aug/2026:09:14:19 +0000] "GET /healthz HTTP/1.1" 200 3 0.002
203.0.113.7 - - [11/Aug/2026:09:15:44 +0000] "POST /api/payments HTTP/1.1" 500 512 1.902
10.0.0.14 - - [11/Aug/2026:09:16:08 +0000] "GET /api/orders HTTP/1.1" 200 1043 0.077
203.0.113.7 - - [11/Aug/2026:09:17:52 +0000] "POST /api/payments HTTP/1.1" 500 512 2.104
192.0.2.55 - - [11/Aug/2026:09:18:30 +0000] "GET /admin HTTP/1.1" 403 128 0.006
EOF
  printf 'name,age,city\nada,36,london\nbob,25,paris\ncyd,40,london\ndee,29,rome\neve,51,oslo\nfrank,33,paris\n' > data.csv
  printf 'name\tage\tcity\nada\t36\tlondon\nbob\t25\tparis\ncyd\t40\tlondon\n' > data.tsv
  printf 'name: ada\nrole: engineer\nseats: 12\nlabels:\n  tier: backend\n  team: orders\n' > config.yaml
  printf '[server]\nhost = 0.0.0.0\nport = 8080\ntls = true\n\n[database]\nurl = postgres://db-01:5432/orders\npool = 20\n' > configs/app.ini
  printf 'timeout=30\nretries=3\nurl=https://api.example.com\n' > configs/app.properties
  printf '{"ts":"2026-08-11T09:12:03Z","level":"info","svc":"orders","msg":"request ok","dur_ms":84}\n{"ts":"2026-08-11T09:15:44Z","level":"error","svc":"payments","msg":"upstream timeout","dur_ms":1902}\n{"ts":"2026-08-11T09:16:08Z","level":"warn","svc":"orders","msg":"slow query","dur_ms":980}\n{"ts":"2026-08-11T09:17:52Z","level":"error","svc":"payments","msg":"upstream timeout","dur_ms":2104}\n{"ts":"2026-08-11T09:18:30Z","level":"info","svc":"gateway","msg":"denied","dur_ms":6}\n' > events.jsonl
  printf '[{"id":1,"name":"Ada Lovelace","email":"ada@example.com","signup":"2024-03-11","plan":"pro","seats":12},{"id":2,"name":"Bob Stone","email":"bob@@example.com","signup":"2025-11-02","plan":"free","seats":1},{"id":3,"name":"Cyd Ashe","email":"cyd@partner.co.uk","signup":"2026-01-19","plan":"pro","seats":40},{"id":4,"name":"Dee Okafor","email":"dee@example.com","signup":"2026-07-30","plan":"team","seats":8}]' > users.json
  printf '{"metadata":{"name":"orders-api","namespace":"prod","labels":{"app":"orders","tier":"backend"}},"spec":{"replicas":4},"status":{"readyReplicas":3}}' > nested.json
  printf '["2.0.0", "1.10.0", "1.9.0", "2.0.0-rc.1", "1.2.10", "1.2.9"]' > releases.json
  printf '<catalog><book id="b1"><title>Go</title><price currency="USD">32</price></book><book id="b2"><title>jq</title><price currency="EUR">19</price></book></catalog>' > doc.xml
  printf 'package main\n\n// TODO: handle the error case\nfunc main() {\n\tpanic("not implemented")\n}\n' > src/main.go
  printf 'package util\n\nimport (\n\t"fmt"\n\t"os"\n)\n\n// TODO: add tests\nfunc Add(a, b int) int {\n\treturn a + b\n}\n\nfunc Load(path string) error {\n\tf, err := os.Open(path)\n\tif err != nil {\n\t\treturn err\n\t}\n\treturn f.Close()\n}\n\nfunc swallow() {\n\tf, _ := os.Open("x")\n\tif err != nil {\n\t}\n\t_ = f\n}\n' > src/util/util.go
  printf 'package util\n\nimport "crypto/tls"\n\nvar insecure = tls.Config{MinVersion: 0, InsecureSkipVerify: true}\n' > src/tls.go
  printf 'import os\nimport subprocess\n\ndef load(path):\n    # TODO: validate path\n    with open(path) as f:\n        return f.read()\n\nclass Client:\n    def __init__(self, token):\n        self.token = token\n\n    def go(self, cmd):\n        return subprocess.run(cmd, shell=False)\n' > src/app.py
  printf '<html><head><title>Home</title></head><body><a href="/about">About</a><a href="https://example.com/docs">Docs</a><p>Email support@example.com for help.</p></body></html>' > web/index.html
  printf 'contact ada@example.com or bob@partner.co.uk\nsee https://example.com/a and http://example.com/b\nhosts 10.0.0.1 and 192.168.1.24 came from 8.8.8.8\non 2026-08-11 we shipped; due 11/08/2026\n' > quotes.txt
  printf '2.0.0\n1.10.0\n1.9.0\n1.2.10\n1.2.9\n' > versions.txt
  printf 'password=hunter2\napi_key=sk-live-abcdef123456\nhost=db-01\n' > configs/creds.properties
  printf '{"refund":{"type":"noul","instructions":"Does the customer want a refund?"}}\n' > question.json

  # The archive and the database, through pwrq itself.
  pwrq -nc 'compress_archive(["notes.txt","quotes.txt"]; "archive.zip")' >/dev/null
  pwrq -nc 'invoke_sqlite_command("app.db"; "create table users (id integer primary key, name text, plan text, seats integer)"), invoke_sqlite_command("app.db"; "insert into users (name, plan, seats) values (\"ada\", \"pro\", 12)"), invoke_sqlite_command("app.db"; "insert into users (name, plan, seats) values (\"bob\", \"free\", 1)"), invoke_sqlite_command("app.db"; "insert into users (name, plan, seats) values (\"cyd\", \"pro\", 40)")' >/dev/null

  # Tiny media for the vision and video entries; several images so the
  # triage-and-move chains have more than one to rank.
  if command -v ffmpeg >/dev/null; then
    ffmpeg -loglevel error -y -f lavfi -i color=c=steelblue:s=96x96:d=1 -frames:v 1 img/cat.png
    ffmpeg -loglevel error -y -f lavfi -i color=c=firebrick:s=96x96:d=1 -frames:v 1 img/red.png
    ffmpeg -loglevel error -y -f lavfi -i color=c=seagreen:s=96x96:d=1 -frames:v 1 img/green.png
    ffmpeg -loglevel error -y -f lavfi -i color=c=darkred:s=96x96:d=2 -pix_fmt yuv420p vid/clip.mp4
  fi
  cd - >/dev/null
}

# ---------------------------------------------------------------- parse --
OKKINDS="path,file,text,phrase,pattern,number,size,host,url,ext,field,ip,days,input,value,cidr"

parse_corpus() {
  {
    for f in "$@"; do
      echo "-- $f"
      cat "$CORPUS/$f"
      printf '\n'
    done
  } | awk -v okkinds="$OKKINDS" '
    BEGIN { nk = split(okkinds, t, ","); for (i = 1; i <= nk; i++) ok[t[i]] = 1 }
    function fail(msg) { printf "parse: %s: %s (line %d)\n", id, msg, NR > "/dev/stderr"; bad = 1 }
    function replace_all(s, pat, repl,   out, i, n) {
      out = ""; n = length(pat)
      while ((i = index(s, pat)) > 0) {
        out = out substr(s, 1, i - 1) repl
        s = substr(s, i + n)
      }
      return out s
    }
    function fillslot(kind, def,   n, u, m) {
      if (kind == "size") {
        n = def + 0
        u = def; sub(/^[0-9]+(\.[0-9]+)?[ \t]*/, "", u); u = tolower(u)
        m = 1
        if (u == "") { if (n < 10000) m = 1048576 }
        else if (substr(u, 1, 1) == "k") m = 1024
        else if (substr(u, 1, 1) == "m") m = 1048576
        else if (substr(u, 1, 1) == "g") m = 1073741824
        else if (substr(u, 1, 1) == "t") m = 1099511627776
        return sprintf("%d", n * m)
      }
      if (kind == "number" || kind == "days" || kind == "value") {
        if (match(def, /[0-9]+/)) return substr(def, RSTART, RLENGTH)
        fail("a " kind " default needs a number: " def)
        return def
      }
      if (def ~ /["\\]/) fail("defaults must be plain literals: " def)
      return def
    }
    function flush(   q, name) {
      if (id == "") return
      if (task == "") { fail("needs a # task: line"); return }
      if (body == "") { fail("needs a query"); return }
      q = body
      sub(/^\n+/, "", q); sub(/\n+$/, "", q)
      for (name in kind) {
        if (index(q, "{{" name "}}") == 0) {
          fail("slot " name " is declared and never used")
          continue
        }
        q = replace_all(q, "{{" name "}}", fillslot(kind[name], def[name]))
      }
      if (index(q, "{{") > 0) fail("a {{...}} in the query is not a declared slot")
      if (!bad) printf "%s\t%s\t%s\n", id, file, replace_all(q, "\n", "\037")
      id = ""
      task = ""
      body = ""
      delete kind
      delete def
    }
    /^-- / {
      flush()
      file = $2
      next
    }
    /^## / {
      flush()
      id = substr($0, 4)
      if (id !~ /^[a-z0-9][a-z0-9-]*$/) { fail("id must be kebab-case"); id = "" }
      if (id in seenid) fail("id is defined twice")
      seenid[id] = 1
      task = ""; body = ""; inbody = 0; bad = 0
      delete kind; delete def
      next
    }
    id == "" { next }
    inbody && /^#/ { fail("a comment inside a block is an unknown header: " $0); next }
    /^# task: / { task = substr($0, 9); next }
    /^# also: / { next }
    /^# slot / {
      rest = substr($0, 8)
      split(rest, t, " ")
      k = t[2]; sub(/:.*/, "", k)
      if (!(k in ok)) { fail("slot kind " t[2] " is not a known kind"); next }
      i = index(rest, " = ")
      if (i == 0) { fail("a slot line needs \" = default\""); next }
      def[t[1]] = substr(rest, i + 3)
      kind[t[1]] = k
      next
    }
    /^#/ { fail("unknown header: " $0); next }
    { body = body $0 "\n"; inbody = 1 }
    END { flush() }
  ' > "$FILLED/queries.tsv" 2> "$FILLED/parse.log"
}

# ------------------------------------------------------------------ run --
# Editor-input queries read the pipeline value; each declares its sample.
# (The corpus keeps its sample data inside each query, so only the
# save-what-arrives entry needs one.)
declare -A INPUT
INPUT[io-save-stdin]='"a line from stdin"'

# Entries that may rightly produce no output in the scratch directory.
declare -A MAYBE_EMPTY
MAYBE_EMPTY[fs-older-than]=1
MAYBE_EMPTY[code-scan-one-rule]=1
MAYBE_EMPTY[flow-cleanup-candidates]=1
MAYBE_EMPTY[fs-entropy-high]=1
MAYBE_EMPTY[fs-readonly-files]=1
MAYBE_EMPTY[fs-media-files]=1

# Entries that must fail with a specific environmental error here; the
# value is an extended regex the stderr must match.
declare -A ENV_ERR
ENV_ERR[sys-service-list]='failed to list services'
ENV_ERR[sys-service-running]='failed to list services'
ENV_ERR[sys-service-stopped]='failed to list services'
ENV_ERR[sys-service-find]='failed to list services'
ENV_ERR[sys-service-count]='failed to list services'
ENV_ERR[sys-service-start]='service'
ENV_ERR[sys-service-stop]='service'
ENV_ERR[ai-llm-ask]='no model|no API key'
ENV_ERR[ai-llm-summarize]='no model|no API key'
ENV_ERR[ai-llm-structured]='no model|no API key'
ENV_ERR[ai-llm-request]='no model|no API key'
ENV_ERR[ai-llm-batch]='no model|no API key'
ENV_ERR[ai-llm-models]='no provider|no model'
ENV_ERR[ai-llm-context]='no provider|no model'
ENV_ERR[ai-embedding]='no model|no API key'
ENV_ERR[ai-embedding-similar]='no model|no API key'
ENV_ERR[ai-agent-count-files]='no model|no API key'
ENV_ERR[ai-agent-outlier]='no model|no API key'
ENV_ERR[ai-agent-request]='no model|no API key'
ENV_ERR[censys-collections]='no personal access token'
ENV_ERR[censys-collection-events]='no personal access token'
ENV_ERR[censys-organization]='no personal access token'
ENV_ERR[censys-tag-assets]='no personal access token'
ENV_ERR[censys-threats]='no personal access token'
ENV_ERR[censys-censeye-job]='no personal access token'
ENV_ERR[censys-censeye-result]='no personal access token'
ENV_ERR[censys-search-hosts]='no personal access token'
ENV_ERR[censys-search-country]='no personal access token'
ENV_ERR[censys-host-detail]='no personal access token'
ENV_ERR[censys-host-at-time]='no personal access token'
ENV_ERR[censys-host-services]='no personal access token'
ENV_ERR[censys-host-timeline]='no personal access token'
ENV_ERR[censys-aggregate]='no personal access token'
ENV_ERR[censys-certificate]='no personal access token'
ENV_ERR[censys-webproperty]='no personal access token'
ENV_ERR[censys-webproperty-timeline]='no personal access token'
ENV_ERR[censys-enrichment]='no personal access token'
ENV_ERR[censys-credits]='no personal access token'
ENV_ERR[censys-context]='no personal access token'
ENV_ERR[censys-tags]='no personal access token'

# Entries expected to block (servers, watchers): a timeout is a pass.
declare -A EXPECT_TIMEOUT
EXPECT_TIMEOUT[net-serve]=1

# d1-backed entries: the model load dominates, so they get a longer rope.
declare -A MODEL
MODEL[ai-yes-no]=1
MODEL[ai-classify-choice]=1
MODEL[ai-urgency-score]=1
MODEL[ai-read-number]=1
MODEL[ai-read-date]=1
MODEL[ai-multi-question]=1
MODEL[ai-sentiment]=1
MODEL[ai-flag-lines]=1
MODEL[ai-batch-questions]=1
MODEL[ai-image-contains]=1
MODEL[ai-image-rate]=1
MODEL[ai-image-kind]=1
MODEL[ai-image-count]=1
MODEL[ai-video-shows]=1
MODEL[ai-video-when]=1
MODEL[flow-photo-rank]=1
MODEL[ai-systemone-request]=1
MODEL[ai-systemone-status]=1
MODEL[chain-fs-images-blue-move]=1
MODEL[chain-fs-videos-motion-when]=1
MODEL[chain-fs-images-region-count]=1
MODEL[chain-search-ai-urgency]=1
MODEL[chain-search-errors-ai-triage]=1
MODEL[chain-net-page-d1]=1
MODEL[chain-log-ai-payment-issue]=1
MODEL[chain-log-slowest-ai-cause]=1
MODEL[chain-db-rows-ai]=1
MODEL[chain-ai-move-blue-images]=1
MODEL[chain-ai-videos-motion-list]=1
MODEL[chain-ai-scores-to-sqlite]=1
MODEL[chain-ai-two-questions]=1
MODEL[chain-ai-sentiment-table]=1
MODEL[chain-ai-confidence-stats]=1
MODEL[chain-ai-segments-table]=1
MODEL[chain-ai-urgent-logs]=1
MODEL[chain-ai-spam-report]=1
MODEL[chain-formats-csv-ai-cities]=1
MODEL[chain-triple-find-search-urgent]=1
MODEL[chain-triple-db-ai-file]=1
MODEL[chain-triple-ship-reviews-aggregate]=1
TIMEOUT_DEFAULT=60
TIMEOUT_MODEL=300

run_all() {
  local fails=0 passes=0 empties=0 envs=0 tmos=0 n=0 id src query tmo in rc cur=""
  while IFS=$'\t' read -r id src query; do
    # Rebuild the fixtures at each file boundary: files mutate the scratch
    # tree, and a later file must not inherit an earlier file's writes.
    if [[ "$src" != "$cur" ]]; then
      cur="$src"
      echo "-- $src"
      fixtures
    fi
    n=$((n + 1))
    query=${query//$'\037'/$'\n'}
    tmo=$TIMEOUT_DEFAULT
    [[ -n "${MODEL[$id]:-}" ]] && tmo=$TIMEOUT_MODEL
    in=""
    [[ -n "${INPUT[$id]:-}" ]] && in="${INPUT[$id]}"
    local errf="$FILLED/$id.err" outf="$FILLED/$id.out"
    # Queries that read the pipeline value take it on stdin; every other
    # query runs once on null input (-n), the way the shipped recipe tests
    # do: an empty stdin would run the program zero times.
    if [[ -n "$in" ]]; then
      printf '%s' "$in" | (cd "$SCRATCH" && timeout "$tmo" pwrq -c "$query" >"$outf" 2>"$errf")
    else
      (cd "$SCRATCH" && timeout "$tmo" pwrq -nc "$query" >"$outf" 2>"$errf")
    fi
    rc=$?
    if [[ $rc -eq 124 || $rc -eq 137 ]]; then
      if [[ -n "${EXPECT_TIMEOUT[$id]:-}" ]]; then tmos=$((tmos + 1)); echo "TMO     $id"
      else fails=$((fails + 1)); echo "FAIL    $id  (timeout after ${tmo}s)"; fi
      continue
    fi
    if [[ $rc -ne 0 ]]; then
      if [[ -n "${ENV_ERR[$id]:-}" ]] && grep -Eq "${ENV_ERR[$id]}" "$errf"; then
        envs=$((envs + 1))
      else
        fails=$((fails + 1))
        echo "FAIL    $id  (exit $rc)"
        sed 's/^/          /' "$errf" | head -4
      fi
      continue
    fi
    if [[ ! -s "$outf" ]]; then
      if [[ -n "${MAYBE_EMPTY[$id]:-}" ]]; then
        empties=$((empties + 1))
      else
        fails=$((fails + 1)); echo "FAIL    $id  (no output)"
      fi
      continue
    fi
    passes=$((passes + 1))
  done < "$FILLED/queries.tsv"
  echo
  echo "entries: $n   pass: $passes   empty(ok): $empties   env: $envs   timeout(ok): $tmos   FAIL: $fails"
  [[ $fails -eq 0 ]]
}

# -------------------------------------------------------------- coverage --
coverage() {
  pwrq -ncr '[get_command] | map("\(.Category)\t\(.Name)") | .[]' > "$FILLED/cmdlets.tsv" 2>/dev/null
  local total=0 covered=0
  : > "$FILLED/uncovered.txt"
  while IFS=$'\t' read -r cat name; do
    total=$((total + 1))
    if grep -qE "(^|[^A-Za-z0-9_])${name}($|[^A-Za-z0-9_])" "$FILLED/all-queries.txt"; then
      covered=$((covered + 1))
    else
      printf '%s\t%s\n' "$cat" "$name" >> "$FILLED/uncovered.txt"
    fi
  done < "$FILLED/cmdlets.tsv"
  echo
  echo "cmdlet coverage: $covered / $total"
  if [[ -s "$FILLED/uncovered.txt" ]]; then
    echo "uncovered, by category:"
    awk -F'\t' '{ a[$1] = a[$1] " " $2 } END { for (c in a) print "  " c ": " a[c] }' "$FILLED/uncovered.txt" | sort
  fi
}

# ----------------------------------------------------------------- main --
FILES=()
for f in "$CORPUS"/*.pwrq; do [[ -e "$f" ]] && FILES+=("${f##*/}"); done
[[ $# -gt 0 ]] && FILES=("$@")
if [[ ${#FILES[@]} -eq 0 ]]; then
  echo "no .pwrq files in $CORPUS"
  exit 1
fi

echo "== fixtures =="
fixtures
echo "== parse =="
parse_corpus "${FILES[@]}"
if [[ -s "$FILLED/parse.log" ]]; then
  echo "parse errors:"
  cat "$FILLED/parse.log"
  echo
fi
if [[ ! -s "$FILLED/queries.tsv" ]]; then
  echo "nothing parsed"
  exit 1
fi
# IDs must be unique and must not collide with the shipped hand recipes.
cut -f1 "$FILLED/queries.tsv" | sort > "$FILLED/ids.txt"
grep -h '^## ' "$REPO/pkg/ideengine/recipes.pwrq" | sed 's/^## //' | sort > "$FILLED/library-ids.txt"
if comm -12 "$FILLED/ids.txt" "$FILLED/library-ids.txt" | grep -q .; then
  echo "IDs colliding with the shipped library:"
  comm -12 "$FILLED/ids.txt" "$FILLED/library-ids.txt"
  exit 1
fi
cut -f3 "$FILLED/queries.tsv" > "$FILLED/all-queries.txt"
echo "parsed $(wc -l < "$FILLED/queries.tsv") entries"
echo "== run =="
# Rules written by the corpus land in the scratch tree, never in the
# user's config directory, and vanish with the next fixture rebuild.
export PWRQ_RULES="$SCRATCH/rules"
run_all
run_rc=$?
echo "== coverage =="
coverage
exit $run_rc
