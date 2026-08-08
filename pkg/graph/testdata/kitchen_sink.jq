# ── helpers ────────────────────────────────────────────────────────────────
def human($n):
  if   $n >= 1048576 then "\($n / 1048576 * 10 | floor / 10)MB"
  elif $n >= 1024    then "\($n / 1024 * 10 | floor / 10)KB"
  else                    "\($n)B"
  end;

# a closure argument, invoked repeatedly: the classic jq generator idiom
def descend(f): def r: ., (f | r); r;

def bucket:
  if   .Length >= 20000 then "huge"
  elif .Length >=  5000 then "large"
  elif .Length >=   500 then "medium"
  else                       "small"
  end;

def stats(selector):
  [selector] as $xs
  | if ($xs | length) == 0 then null
    else { n:    ($xs | length)
         , min:  ($xs | min)
         , max:  ($xs | max)
         , total: ($xs | add)
         , mean: (($xs | add) / ($xs | length) | . * 100 | round / 100)
         }
    end;

def redact($secret): gsub($secret; "«redacted»");

# ── the pipeline ───────────────────────────────────────────────────────────
[ gci("."; { Recurse: true, Filter: "*.go" })
  | select(.Length > 0 and (.Name | test("_test\\.go$") | not))
  | . as $f
  | ($f.FullName | cat) as $src
  | { name:     $f.Name
    , path:     ($f.FullName | sub("^\(env.HOME // "~")"; "~"))
    , size:     $f.Length
    , pretty:   human($f.Length)
    , bucket:   ($f | bucket)
    , modified: ($f.LastWriteTime | split("T")[0])
    , sha:      ($src | sha256 | .[0:12])
    , entropy:  ($src | entropy | . * 1000 | round / 1000)
    , lines:    ($src | split("\n") | length)
    , imports:  [ $src | split("\n")[]
                | select(startswith("\t\"") or startswith("\t_ \""))
                | ltrimstr("\t") | ltrimstr("_ ") | gsub("\""; "")
                ]
    , funcs:    [ $src | split("\n")[]
                | capture("^func (?:\\([^)]*\\) )?(?<n>[A-Za-z_][A-Za-z0-9_]*)")?
                | .n
                ]
    }
] as $files

# ── the report ─────────────────────────────────────────────────────────────
| { generated:
      ( get_date
      | { date: .Date, time: .Time, weekday: .DayOfWeek
        , iso:  "\(.Year)-\(if .Month < 10 then "0" else "" end)\(.Month)"
        }
      )

  , where: ( gl.Path | sub("^\(env.HOME // "~")"; "~") )

  , took: ( new_timespan({ Minutes: 1, Seconds: 30 }) | .Duration )

  , totals:
      { files: ($files | length)
      , bytes: ($files | map(.size) | add)
      , human: human($files | map(.size) | add)
      , lines: ($files | map(.lines) | add)
      }

  , sizes: ($files | stats(.[].size))

  , by_bucket:
      ( $files
      | group_by(.bucket)
      | map({ key: .[0].bucket
            , value: { count: length
                     , bytes: (map(.size) | add)
                     , files: (map(.name) | sort | .[0:3])
                     }
            })
      | from_entries
      )

  , biggest:
      ( $files
      | sort_by(-.size)
      | .[0:5]
      | map({ name, pretty, sha, entropy })
      )

  # every import, ranked, with the files that pull it in
  , imports:
      ( [ $files[] | . as $f | .imports[] | { pkg: ., from: $f.name } ]
      | group_by(.pkg)
      | map({ pkg: .[0].pkg, uses: length, by: (map(.from) | unique | .[0:3]) })
      | sort_by(-.uses, .pkg)
      | .[0:8]
      )

  # a reduce that builds a histogram, keyed by first path segment
  , tree:
      ( reduce ($files[] | .path | split("/") | .[-2] // "·") as $dir
          ({}; .[$dir] = ((.[$dir] // 0) + 1))
      | to_entries
      | sort_by(-.value)
      | .[0:6]
      | from_entries
      )

  # foreach carrying a running total, to show cumulative share
  , cumulative:
      ( [ foreach ($files | sort_by(-.size) | .[]) as $f
            (0; . + $f.size; { file: $f.name, running: ., share: . })
        ] as $steps
      | ($files | map(.size) | add) as $all
      | $steps
      | map(.share = (.running / $all * 1000 | round / 10))
      | .[0:4]
      )

  # generators, laziness, and a closure argument
  , fibonacci:
      [ limit(10; [0,1] | descend([.[1], add]) | .[0]) ]

  # label/break: stop at the first file over a threshold
  , first_over_10k:
      ( label $found
      | ( ($files | sort_by(.size)[] | select(.size > 10000) | .name, break $found)
        , "none"
        )
      )

  # try/catch around something that genuinely fails
  , missing_file:
      ( try (cat("/nonexistent/definitely") | length)
        catch ("unreadable: " + (. | split(":")[0]))
      )

  # alternative operator and optional access on absent paths
  , defaults:
      { absent:  (.nothing.here? // "fallback")
      , present: ($files[0].name // "fallback")
      }

  # paths / getpath / setpath / delpaths on a synthetic document
  , surgery:
      ( { a: { b: { c: 1 } }, d: [10, 20], keep: true }
      | { paths:   [paths(scalars)]
        , got:     getpath(["a","b","c"])
        , set:     (setpath(["a","b","c"]; 99) | .a.b.c)
        , deleted: (delpaths([["a","b"],["d",0]]))
        , walked:  (walk(if type == "number" then . * 2 else . end))
        }
      )

  # streaming: tostream then rebuild with fromstream
  , streaming:
      ( { x: [1, { y: "z" }] }
      | { events: [tostream] | length
        , rebuilt: (fromstream(tostream))
        }
      )

  # string handling: interpolation, formats, regex captures, redaction
  , strings:
      ( "user=admin token=sk-SECRET-42 host=example.com" as $line
      | { csv:      ([$line, "b,c"] | @csv)
        , b64:      ($line | @base64)
        , sh:       (["echo", $line] | @sh)
        , captured: ($line | [scan("(\\w+)=([^ ]+)")] | map({ (.[0]): .[1] }) | add)
        , redacted: ($line | redact("sk-SECRET-42"))
        , upper:    ($line | ascii_upcase | .[0:12])
        }
      )

  # pwrq object cmdlets alongside jq's own, on the same data
  , cmdlets:
      ( [ { host: "a", up: true, ms: 12 }
        , { host: "b", up: false, ms: 99 }
        , { host: "c", up: true, ms: 34 }
        ] as $hosts
      | { where_script: ( where_object($hosts; { script: ".up and .ms < 40" })
                        | map(.host) )
        , where_op:     ( where_object($hosts; { property: "host"
                                               , operator: "like"
                                               , value: "[ab]" })
                        | map(.host) )
        , selected:     ( $hosts | map(select_object(.; "host"; "ms")) )
        , measured:     ( measure_object($hosts; { property: "ms" }) | .Count )
        , table:        ( format_table($hosts) | split("\n") | .[0:2] )
        }
      )

  # the live system, filtered two different ways
  , system:
      { procs:    ( [ gps | select(.CPU > 0) ] | length )
      , busiest:  ( [ gps | { n: .Name, cpu: .CPU } ]
                  | sort_by(-.cpu) | .[0:3] | map(.n) )
      , services: ( [ get_service | select(.Status == "Running") ] | length )
      , has_mod:  ( test_path("go.mod") )
      }

  # env and $ENV, and the builtins/input_filename metadata functions
  , provenance:
      { shell: (env.SHELL // "?" | split("/") | last)
      , builtins: (builtins | length)
      , pwrq_fns: ([builtins[] | select(test("^(get_|where_|format_|invoke_)"))] | length)
      , argv:  ($ARGS.named | keys)
      }
  }
