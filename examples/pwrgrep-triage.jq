# pwrgrep-triage — a whole triage run as one query.
#
# The shape is the one examples/agent-triage.sh draws, with the model moved
# from the middle of the pipeline to the front of it: here it decides what to
# scan, and then judges what the scan found.
#
#   1. the vocabulary   get_pwrgrep_rule and get_ast_language, as a menu of the
#                       languages rules exist for and the extensions they wear
#   2. exploration      the model walks the tree with a fixed set of tools and
#                       names the languages it is written in
#   3. the scan         invoke_pwrgrep, over exactly those languages' rules
#   4. verification     one bounded agent loop per finding, run in parallel
#                       rounds by invoke_llm_batch
#   5. the answer       a stream of JSON records, one per finding, as each is
#                       decided — plus a summary at the end
#
# The agent loops are written here rather than delegated to invoke_agent on
# purpose. invoke_agent hands the model a jq compiler and lets it write
# queries, which is the right surface for a model that can write jq; a 2.6B
# model cannot, and spends its whole step budget producing programs that do not
# parse. So the tool surface is an enum instead — extensions/list/head/decide,
# widen/search/verdict — and pwrq runs the tool the model names. The model is
# still the one deciding what to look at next, which is what makes it a loop
# rather than a classifier; it just no longer has to write code to say so.
#
# Usage:
#
#   pwrq -n -f examples/pwrgrep-triage.jq                    # NDJSON, streamed
#   pwrq -n -f examples/pwrgrep-triage.jq \
#        --arg repo /tmp/kona \
#        --arg model openai-compatible/lfm2.5-2.6b \
#        --arg base http://127.0.0.1:1234/v1 \
#        --argjson parallel 4 --argjson verify_steps 3 --argjson max_findings 0
#
# Every record is a complete JSON value on its own line, emitted as soon as it
# is known, so the run is watchable:
#
#   pwrq -nc -f examples/pwrgrep-triage.jq | jq -c 'select(.Kind == "finding")'
#
# and one document instead of a stream is the usual slurp:
#
#   pwrq -nc -f examples/pwrgrep-triage.jq \
#     | pwrq -sc 'group_by(.Kind) | map({(.[0].Kind): .}) | add'

# ---------------------------------------------------------------- the knobs --
# Read from $ARGS so every one of them has a default and the query runs with no
# arguments at all. Numbers want --argjson; --arg would make them strings.
def opt($name; $default): ($ARGS.named[$name] // $default);

def repo:          (opt("repo"; "/tmp/getssl") | normalize_path);
def parallel:      opt("parallel"; 4);        # concurrent verification calls
def explore_steps: opt("explore_steps"; 6);   # tool calls the explorer may make
def verify_steps:  opt("verify_steps"; 3);    # tool calls per finding
def max_findings:  opt("max_findings"; 0);    # 0 is all of them

# MaxTokens is large because this model thinks at length before it answers, and
# a reply that hits the cap mid-thought is an error rather than a worse answer.
# The per-process call ceiling is lifted for the same reason the batch exists:
# one run is hundreds of calls by design, and 100 is the accident ceiling.
def llm:
  { Model:     opt("model"; "openai-compatible/lfm2.5-2.6b"),
    BaseUrl:   opt("base"; "http://127.0.0.1:1234/v1"),
    MaxTokens: opt("tokens"; 16000),
    MaxCalls:  1000000 };

# --------------------------------------------------- 1. what rules there are --
# One row per language the corpus has rules for, with the extensions that
# language is written in. This is the menu the model chooses from, and it is
# read out of the catalogue rather than written down here, so a rule pack added
# tomorrow is in it.
def catalogue:
  ([get_ast_language] | map({key: .Name, value: .Extensions}) | from_entries) as $ext
  | [get_pwrgrep_rule]
  | map({Language: .Languages[]?, Id})
  | group_by(.Language)
  | map({ Language:   .[0].Language,
          Rules:      (map(.Id) | unique | length),
          Extensions: ($ext[.[0].Language] // []) })
  | sort_by(-.Rules);

# ------------------------------------------------- 2. the model explores it --
# A path the model names is resolved against the root — absolute paths inside
# the tree are taken as they are, because a model told the root will sometimes
# repeat it — and anything that lands outside collapses back to the root. The
# model chooses where to look; it does not choose what it is allowed to see.
def under($rel):
  (($rel // ".") | if . == "" then "." else . end) as $a
  | ((if ($a | startswith("/")) then $a else repo + "/" + $a end) | normalize_path) as $p
  | if $p == repo or ($p | startswith(repo + "/")) then $p else repo end;

def explore_schema($supported):
  { type: "object",
    properties: {
      thought:   {type: "string"},
      action:    {type: "string", enum: ["extensions", "list", "head", "decide"]},
      argument:  {type: "string"},
      languages: {type: "array", items: {type: "string", enum: $supported}} },
    required: ["thought", "action", "argument", "languages"] };

def explore_tool($turn):
  under($turn.argument) as $path
  | try (
      if $turn.action == "extensions" then
        ([get_childitem($path; {Recurse: true})
          | select(.IsDirectory | not) | (.Extension // "(none)")]
         | value_counts | tojson)
      elif $turn.action == "list" then
        ([limit(60; get_childitem($path))
          | (if .IsDirectory then .Name + "/" else .Name end)] | join(" "))
      else
        (head($path; 25) | join("\n"))
      end)
    catch "error: \(.)";

# The loop. Every field of the reply is required, including the answer field:
# constrained decoding emits the cheapest document a schema allows, so an
# optional field is one a small model simply never fills in. `languages` is
# therefore a running best answer, and `decide` is what makes it final.
def explore($catalogue):
  ($catalogue | map(.Language)) as $supported
  | ($catalogue | map("  \(.Language)  (\(.Rules) rules; \(.Extensions | join(" ")))")
     | join("\n")) as $menu
  | reduce range(0; explore_steps) as $_ ({Done: false, Trace: [], Languages: []};
      if .Done then . else
        ( "You are exploring a source tree to work out which programming languages it is written in.\n\n"
        + "Root directory: \(repo)\n\n"
        + "Report only languages from this list. The number is how many analysis rules exist for it:\n\($menu)\n\n"
        + "Tools — name one in \"action\" and give its argument in \"argument\". Paths are relative to the root:\n"
        + "  extensions  a directory (\".\" is the whole tree) — how many files carry each extension\n"
        + "  list        a directory — its entries, directories marked with a trailing slash\n"
        + "  head        a file — its first 25 lines\n"
        + "  decide      no argument — you are finished, and \"languages\" is your answer\n\n"
        + "What you have looked at so far:\n"
        + (if (.Trace | length) == 0
           then "  nothing yet — start with extensions on \".\"\n"
           else (.Trace | map("> \(.Action) \(.Argument)\n\(.Result)") | join("\n\n")) end)
        + "\n\nFill in every field every turn. \"languages\" is your best answer so far and is read when action is \"decide\". "
        + "Name a language only if the tree really holds source written in it — not one merely mentioned in a README."
        ) as $prompt
        | invoke_llm($prompt; llm + {Schema: explore_schema($supported)}) as $turn
        | .Languages = ($turn.languages // [])
        | if $turn.action == "decide" then .Done = true
          else .Trace += [{ Action:   $turn.action,
                            Argument: ($turn.argument // "."),
                            Path:     under($turn.argument),
                            Result:   (explore_tool($turn) | .[0:2000]) }]
          end
      end)
  # Two guards on the answer, both of them cheap and neither of them a second
  # opinion about what the tree is written in.
  #
  # The first is that a language the model invented must not reach
  # invoke_pwrgrep, where "no rule called that" is an error rather than an
  # empty result.
  #
  # The second is that a language may only be named if files of that language
  # are actually there. It matters because `languages` is a running best answer
  # on every turn, so a run that spends its whole step budget without deciding
  # hands back whatever it guessed before it had looked — and for this model
  # that first guess is the entire menu, which is 1,785 rules over a tree that
  # wants 47. Extensions cannot make the decision on their own — a .yml beside
  # the source is not what a tree is written in, and kona's own .k files answer
  # to no language here at all, which is the judgement the loop is for. They can
  # only say that a tree holding no Ruby file is not Ruby.
  | ([get_childitem(repo; {Recurse: true}) | select(.IsDirectory | not) | .Extension]
     | unique) as $present
  | ($catalogue | map(select((.Extensions | length) == 0 or any(.Extensions[]; IN($present[]))))
     | map(.Language)) as $plausible
  | .Languages |= (map(select(IN($supported[]) and IN($plausible[]))) | unique);

# ---------------------------------------------- 4. the model checks the work --
def verify_schema:
  { type: "object",
    properties: {
      thought:    {type: "string"},
      action:     {type: "string", enum: ["widen", "search", "verdict"]},
      argument:   {type: "string"},
      verdict:    {type: "string", enum: ["true_positive", "false_positive", "uncertain"]},
      confidence: {type: "string", enum: ["low", "medium", "high"]},
      rationale:  {type: "string"} },
    required: ["thought", "action", "argument", "verdict", "confidence", "rationale"] };

# Numbered source lines, clamped to the file. The numbers are what let the
# model talk about the site: without them "the line above" is a guess.
def source_window($path; $from; $to):
  ([$from, 1] | max) as $a
  | (cat($path) | split("\n")) as $lines
  | ([$to, ($lines | length)] | min) as $b
  | [range($a; $b + 1) | "\(.)| \($lines[. - 1])"] | join("\n");

def verify_tool($turn):
  try (
    if $turn.action == "widen" then
      (($turn.argument | tonumber? // 20) as $n
       | source_window(.Finding.Path; .Finding.LineNumber - $n; .Finding.EndLineNumber + $n))
    else
      ([limit(15; select_string(repo; $turn.argument))
        | "\(.Path):\(.LineNumber): \(.Line // .Match)"]
       | join("\n")
       | if . == "" then "(nothing matched)" else . end)
    end)
  catch "error: \(.)";

# The first window is seeded rather than asked for. The model would spend its
# first turn on `widen` every time, and a turn is a round trip.
def verify_row($f; $ix):
  { Ix:      $ix,
    Finding: $f,
    Turns:   0,
    Done:    false,
    V:       null,
    Trace:   [{ Action:   "widen",
                Argument: "6",
                Result:   source_window($f.Path; $f.LineNumber - 6; $f.EndLineNumber + 6) }] };

def verify_prompt:
    "A static-analysis rule fired on one line of source. Decide whether it is a real problem at this site.\n\n"
  + "Rule:     \(.Finding.RuleId)\n"
  + "It says:  \(.Finding.Message)\n"
  + "File:     \(.Finding.Path)\n"
  + "Line:     \(.Finding.LineNumber)\n"
  + "Matched:  \(.Finding.Match)\n\n"
  + "Evidence so far:\n"
  + (.Trace | map("> \(.Action) \(.Argument)\n\(.Result)") | join("\n\n"))
  + "\n\nTools — name one in \"action\" and give its argument in \"argument\":\n"
  + "  widen    a number of extra source lines to show around the match\n"
  + "  search   a regular expression to look for anywhere in the tree — how a buffer is sized, where a value comes from, whether a length is checked\n"
  + "  verdict  no argument — you are finished\n\n"
  + "Fill in every field every turn. verdict, confidence and rationale are your judgement so far and are recorded when action is \"verdict\". "
  + "Answer false_positive when the code is safe here despite matching the pattern, true_positive when the rule is right about this site, "
  + "and uncertain when the evidence you have does not settle it.";

# One reply applied to one row. A failed call is a decided row rather than a
# failed run: ContinueOnError reports it on .Error, and a finding nobody could
# judge is worth reporting as exactly that.
def verify_apply($reply):
  ($reply.Value // null) as $turn
  | .Turns += 1
  | if $turn == null then
      .Done = true
      | .V = { verdict: "uncertain", confidence: "low",
               rationale: "the model call failed: \($reply.Error // "no structured reply")" }
    else
      .V = ($turn | {verdict, confidence, rationale})
      | if $turn.action == "verdict" then .Done = true
        else .Trace += [{ Action:   $turn.action,
                          Argument: ($turn.argument // ""),
                          Result:   (verify_tool($turn) | .[0:2500]) }]
        end
    end;

def verify_record($total):
  { Kind:          "finding",
    Index:         (.Ix + 1),
    Total:         $total,
    RuleId:        .Finding.RuleId,
    Message:       .Finding.Message,
    Path:          .Finding.Path,
    LineNumber:    .Finding.LineNumber,
    EndLineNumber: .Finding.EndLineNumber,
    Column:        .Finding.Column,
    Match:         .Finding.Match,
    Verdict:       (.V.verdict // "uncertain"),
    Confidence:    (.V.confidence // "low"),
    Rationale:     (.V.rationale // ""),
    Turns:         .Turns,
    Settled:       .Done,
    Evidence:      .Trace };

# gojq is synchronous, so a loop per finding is one round trip after another
# and a tree of any size is an afternoon. The rounds invert it: every finding
# still undecided takes its next turn in the same batch, so the wall clock is
# rounds rather than findings. Each round emits the rows it settled, which is
# what makes the run watchable rather than a long silence and a large array.
def verify($findings; $head):
  ($findings | length) as $total
  | [$findings | to_entries[] | verify_row(.value; .key)] as $rows
  | foreach (range(0; verify_steps), null) as $round
      ({Rows: $rows, Emit: [], Tally: [], Final: null};
        if $round == null then
          (.Rows | map(select(.Done | not))) as $left
          | (.Tally + ($left | map({RuleId: .Finding.RuleId, Verdict: (.V.verdict // "uncertain")}))) as $tally
          | { Rows: [], Emit: $left, Tally: $tally,
              Final: ($head + {
                Kind:     "summary",
                Findings: $total,
                Settled:  ($total - ($left | length)),
                Verdicts: ($tally | group_by(.Verdict) | map({(.[0].Verdict): length}) | add // {}),
                ByRule:   ($tally | group_by(.RuleId)
                           | map({ RuleId: .[0].RuleId, Findings: length,
                                   Verdicts: (group_by(.Verdict) | map({(.[0].Verdict): length}) | add) })
                           | sort_by(-.Findings)),
                Usage:    (get_llm_usage | {Calls, InputTokens, OutputTokens, TotalTokens}) }) }
        else
          (.Rows | map(select(.Done | not))) as $pending
          | if ($pending | length) == 0 then {Rows: .Rows, Emit: [], Tally: .Tally, Final: null}
            else
              [$pending[] | verify_prompt] as $prompts
              | [invoke_llm_batch($prompts;
                   llm + {Schema: verify_schema, Parallel: parallel, ContinueOnError: true})] as $replies
              | [range(0; $pending | length) as $i | $pending[$i] | verify_apply($replies[$i])] as $updated
              | (reduce $updated[] as $u ({}; .[$u.Ix | tostring] = $u)) as $byIx
              | ($updated | map(select(.Done))) as $settled
              | { Rows:  (.Rows | map($byIx[.Ix | tostring] // .)),
                  Emit:  $settled,
                  Tally: (.Tally + ($settled | map({RuleId: .Finding.RuleId, Verdict: .V.verdict}))),
                  Final: null }
            end
        end;
        (.Emit[] | verify_record($total)), (.Final // empty));

# ------------------------------------------------------------------- the run --
catalogue as $catalogue
| { Kind:      "run",
    Repo:      repo,
    Model:     (llm.Model),
    BaseUrl:   (llm.BaseUrl),
    Languages: $catalogue,
    Rules:     ($catalogue | map(.Rules) | add) },

  ( explore($catalogue) as $explored
  | { Kind:      "languages",
      Detected:  $explored.Languages,
      Steps:     ($explored.Trace | length),
      Explored:  ($explored.Trace | map({Action, Argument})) },

    ( if ($explored.Languages | length) == 0 then
        { Kind: "summary", Repo: repo, Languages: [], Findings: 0, Settled: 0,
          Verdicts: {}, ByRule: [],
          Note: "the model named no supported language, so nothing was scanned",
          Usage: (get_llm_usage | {Calls, InputTokens, OutputTokens, TotalTokens}) }
      else
        ( [invoke_pwrgrep(repo; $explored.Languages)]
          | (if max_findings > 0 then .[0:max_findings] else . end) ) as $findings
        | { Kind:      "scan",
            Languages: $explored.Languages,
            Rules:     ([get_pwrgrep_rule($explored.Languages[])] | map(.Id) | unique | length),
            Findings:  ($findings | length),
            ByRule:    ($findings | group_by(.RuleId)
                        | map({RuleId: .[0].RuleId, Findings: length}) | sort_by(-.Findings)) },
          verify($findings; {Repo: repo, Languages: $explored.Languages})
      end ) )
