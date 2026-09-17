#!/usr/bin/env bash
#
# An end-to-end run: logs on disk, in, and an answer out.
#
# Five stages, each one a pwrq query, and each one handing JSON to the next:
#
#   1. find the errors          filesystem and search cmdlets
#   2. classify them            invoke_llm_batch with a Schema, so the answers
#                               are rows rather than prose
#   3. score them               invoke_systemone, which answers typed questions
#                               with probabilities instead of generated text
#   4. summarise                ordinary jq over those rows
#   5. ask about them           invoke_agent, which writes its own queries
#                               against the data it is given
#
# Nothing here is LLM-specific except stages 2, 3 and 5, which is the point:
# the model is a stage in a pipeline, not a thing you build a pipeline around.
#
# Usage:
#   export PWRQ_LLM_MODEL=anthropic/claude-sonnet-4-5   # or any provider/model
#   export ANTHROPIC_API_KEY=...                        # what that provider documents
#   examples/agent-triage.sh
#
# Against a local server instead:
#   export PWRQ_LLM_MODEL=openai-compatible/your-model
#   export OPENAI_BASE_URL=http://127.0.0.1:1234/v1
#
# Stage 3 needs a System One endpoint: TypeSafe's, or llama.cpp's /v1/systemone
# serving a local GGUF. Without one it is skipped and the run goes on:
#   export PWRQ_SYSTEMONE_MODEL=systemone/your-model
#   export TYPESAFE_BASE_URL=http://127.0.0.1:8080      # and TYPESAFE_API_KEY if hosted
#
# Stage 5 asks more of the model than stages 1-4 do: classifying one line is a
# single judgement, while writing a query, reading its result and deciding what
# to do next is a loop. A small local model does the first well and the second
# badly, so PWRQ_AGENT_MODEL runs that stage on a bigger one:
#   export PWRQ_AGENT_MODEL=openai-compatible/a-larger-model
set -euo pipefail

PWRQ=${PWRQ:-pwrq}
if ! command -v "$PWRQ" >/dev/null 2>&1; then
  echo "pwrq not on PATH; build it with 'go build ./cmd/pwrq' and set PWRQ=./pwrq" >&2
  exit 1
fi
if [ -z "${PWRQ_LLM_MODEL:-}" ]; then
  echo "set PWRQ_LLM_MODEL to a provider/model name first (see the header of this script)" >&2
  exit 1
fi

# A corpus, written fresh so the run is reproducible and costs the same every
# time. Real logs would be wherever they already are.
logs=$(mktemp -d)
trap 'rm -rf "$logs"' EXIT

cat > "$logs/api.log" <<'LOG'
2026-08-18T09:14:02Z INFO  request served in 12ms
2026-08-18T09:14:44Z ERROR connection refused talking to postgres at 10.0.0.4:5432
2026-08-18T09:15:01Z INFO  request served in 9ms
2026-08-18T09:15:12Z ERROR context deadline exceeded after 30s calling billing-svc
LOG

cat > "$logs/worker.log" <<'LOG'
2026-08-18T09:12:00Z INFO  picked up job 8812
2026-08-18T09:12:31Z ERROR panic: runtime error: index out of range [7] with length 4
2026-08-18T09:13:02Z ERROR failed to unmarshal payload: unexpected end of JSON input
2026-08-18T09:13:40Z ERROR panic: runtime error: invalid memory address or nil pointer dereference
LOG

cat > "$logs/auth.log" <<'LOG'
2026-08-18T09:10:00Z INFO  token issued for user 4471
2026-08-18T09:11:20Z ERROR signature verification failed for token from 203.0.113.9
LOG

echo "== 1. the errors on disk =================================================="
# select_string streams one object per match, so [...] collects it. This stage
# never talks to a model: finding the lines is a job the cmdlets already do.
"$PWRQ" -nc --arg dir "$logs" '
  [select_string($dir; "ERROR"; {Include: "*.log"})
   | {File: (.Path | split("/") | last), Line: .LineNumber, Text: .Line}]
' | tee "$logs/errors.json" | "$PWRQ" -r 'length | "\(.) error lines"'

echo
echo "== 2. classified, one call per line, in parallel =========================="
# The Schema is what makes this a pipeline stage rather than a chat: the answers
# come back as values that stage 3 can group by, and an answer that does not fit
# the schema is an error rather than something almost right.
"$PWRQ" -c '
  [.[] | "Classify this log line. Answer only with the JSON.\n\(.Text)"]
  | [invoke_llm_batch({Parallel: 4,
      Schema: {type: "object",
               properties: {
                 category: {type: "string",
                            enum: ["dependency", "timeout", "crash", "data", "auth"]},
                 severity: {type: "string", enum: ["low", "medium", "high"]}},
               required: ["category", "severity"]}})]
  | map(.Value)
' < "$logs/errors.json" > "$logs/classified.json"

# Rejoin the classifications with the lines they came from. Batch preserves
# input order, which is what makes a positional join correct here.
"$PWRQ" -c --slurpfile c "$logs/classified.json" '
  . as $rows | [range(0; $rows | length) | $rows[.] + $c[0][.]]
' < "$logs/errors.json" > "$logs/triaged.json"
"$PWRQ" -r 'map({File, Line, category, severity}) | format_table(.)' < "$logs/triaged.json"

echo
echo "== 3. scored by System One ==============================================="
# Stage 2 asked a model to write an enum; this asks one to weigh options. Each
# question's options get single-token labels and one forward pass reads how
# likely each is, so page is a probability, urgency a weighted position on its
# scale and owner the likeliest team — never a word outside the list. All three
# questions about a line are one request.
if [ -n "${PWRQ_SYSTEMONE_MODEL:-}" ]; then
  "$PWRQ" -c '
    {page:    {type: "noul", instructions: "Should this error wake the on-call engineer?"},
     urgency: {type: "score", instructions: "How urgent is fixing this error?",
               criteria: ["can wait for the next working day", "fix today", "fix now"]},
     owner:   {type: "choice", instructions: "Which team owns this error?",
               criteria: {platform: "databases, networks and calls to other services",
                          backend:  "application code and the bugs in it",
                          security: "authentication, tokens and signatures"}}} as $q
    | map(. + ({Text, category, severity} | invoke_systemone($q)
               | {page: .page.noul, urgency: .urgency.score, owner: .owner.choice}))
  ' < "$logs/triaged.json" > "$logs/scored.json"
  "$PWRQ" -r 'map({File, Line, owner, urgency: (.urgency * 100 | round / 100),
                   page: (.page * 100 | round / 100)}) | format_table(.)' < "$logs/scored.json"
else
  echo "skipped: set PWRQ_SYSTEMONE_MODEL and TYPESAFE_BASE_URL to score with System One"
  "$PWRQ" -c 'map(. + {page: null, urgency: null, owner: null})' < "$logs/triaged.json" > "$logs/scored.json"
fi

echo
echo "== 4. summarised with plain jq ==========================================="
# No cmdlet at all: once the model's answers are values, the rest is jq.
"$PWRQ" -r '
  group_by(.category)
  | map({category: .[0].category, count: length,
         pages: (map(select((.page // 0) > 0.5)) | length),
         files: (map(.File) | unique | join(", "))})
  | sort_by(-.count) | format_table(.)
' < "$logs/scored.json"

echo
echo "== 5. the agent, asked about the same data ==============================="
# The data is piped in, so `.` is the triaged rows inside every query the agent
# writes. Its vocabulary is the read-only default: it can look, and that is all.
"$PWRQ" -c --arg model "${PWRQ_AGENT_MODEL:-$PWRQ_LLM_MODEL}" '
  invoke_agent_request(
    "Which file has the most high-severity errors, and what kind are they? Answer in one sentence.";
    {Model: $model, MaxSteps: 5, MaxTokens: 8000})
  | {Answer: .Content, Queries: [.Steps[] | .Query | select(. != null)], Tokens: .TotalTokens}
' < "$logs/triaged.json"

echo
echo "== what the run cost ====================================================="
# get_llm_usage counts one process, and every stage above is its own pwrq
# invocation — so it is asked from inside a query rather than after the fact.
# Stage 5 reported its own total on .Tokens for the same reason.
# -n, because nothing is piped in: without it pwrq reads an empty stdin, has no
# input to run the query on, and prints nothing at all.
"$PWRQ" -nc '
  [invoke_llm_batch(["Reply with only: done"])] | .[0].Content as $reply
  | {Reply: $reply, Usage: get_llm_usage}
'
