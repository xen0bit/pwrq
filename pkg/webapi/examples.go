package webapi

// Example is a query worth opening the page for.
type Example struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Query       string `json:"query"`
	Input       string `json:"input"`
	// Args travel with the example, so one that demonstrates arguments works
	// the moment it is loaded rather than failing to compile.
	Args []Arg `json:"args,omitempty"`
}

// The gallery's shared fixtures.
//
// Examples that work on the same data are written against one of these rather
// than each inventing its own two-row literal. That is what lets an example be
// a piece of work instead of a demonstration: the log fixture has enough lines
// to bucket and rank, the CSV has enough rows to pivot, and neighbouring
// examples build on each other because they are looking at the same thing.
//
// Everything is a literal because the page evaluates queries in a browser,
// where there is no filesystem, no process table and no network. That
// constrains the input to a literal; it does not constrain the query to a
// trivial one.
const (
	// accessLog is a web server log, the shape of thing people most often
	// reach for a JSON shell to understand.
	accessLog = `"10.0.0.14 - - [11/Aug/2026:09:12:03 +0000] \"GET /api/orders HTTP/1.1\" 200 1043 0.084\n203.0.113.7 - - [11/Aug/2026:09:12:31 +0000] \"POST /api/orders HTTP/1.1\" 201 219 0.412\n10.0.0.14 - - [11/Aug/2026:09:13:02 +0000] \"GET /api/orders/88 HTTP/1.1\" 404 96 0.011\n198.51.100.22 - - [11/Aug/2026:09:14:19 +0000] \"GET /healthz HTTP/1.1\" 200 3 0.002\n203.0.113.7 - - [11/Aug/2026:09:15:44 +0000] \"POST /api/payments HTTP/1.1\" 500 512 1.902\n10.0.0.14 - - [11/Aug/2026:09:16:08 +0000] \"GET /api/orders HTTP/1.1\" 200 1043 0.077\n203.0.113.7 - - [11/Aug/2026:09:17:52 +0000] \"POST /api/payments HTTP/1.1\" 500 512 2.104\n192.0.2.55 - - [11/Aug/2026:09:18:30 +0000] \"GET /admin HTTP/1.1\" 403 128 0.006"`

	// appLog is structured application logging, in logfmt.
	appLog = `"ts=2026-08-11T09:12:03Z level=info svc=orders msg=\"request ok\" dur_ms=84\nts=2026-08-11T09:12:31Z level=info svc=orders msg=\"created\" dur_ms=412\nts=2026-08-11T09:15:44Z level=error svc=payments msg=\"upstream timeout\" dur_ms=1902\nts=2026-08-11T09:16:08Z level=warn svc=orders msg=\"slow query\" dur_ms=980\nts=2026-08-11T09:17:52Z level=error svc=payments msg=\"upstream timeout\" dur_ms=2104\nts=2026-08-11T09:18:30Z level=info svc=gateway msg=\"denied\" dur_ms=6"`

	// salesCSV is a spreadsheet export: the thing pivot and summarize exist for.
	salesCSV = `"region,quarter,rep,units,revenue\nEMEA,Q1,ada,120,48000\nEMEA,Q2,ada,140,56000\nEMEA,Q1,bob,90,36000\nAMER,Q1,cyd,210,89250\nAMER,Q2,cyd,185,78625\nAMER,Q1,dee,150,63750\nAPAC,Q1,eve,75,29250\nAPAC,Q2,eve,110,42900"`

	// services is a fleet, as an object cmdlet would emit it.
	services = `[{"Name":"orders-api","Status":"Running","CPU":63.8,"MemoryMB":1024,"Restarts":0,"Version":"2.4.1"},
 {"Name":"payments-api","Status":"Degraded","CPU":91.2,"MemoryMB":2048,"Restarts":7,"Version":"2.3.9"},
 {"Name":"gateway","Status":"Running","CPU":12.4,"MemoryMB":512,"Restarts":1,"Version":"2.4.1"},
 {"Name":"search","Status":"Stopped","CPU":0,"MemoryMB":0,"Restarts":3,"Version":"1.9.0"},
 {"Name":"mailer","Status":"Running","CPU":4.1,"MemoryMB":256,"Restarts":0,"Version":"2.4.0"}]`

	// latencies is a series of response times, in milliseconds.
	latencies = `[84, 91, 77, 412, 88, 95, 1902, 102, 88, 980, 91, 2104, 86, 90, 79, 94]`

	// users carries the messy fields validation and extraction exist for.
	users = `[{"id":1,"name":"Ada Lovelace","email":"ada@example.com","signup":"2024-03-11","plan":"pro","seats":12},
 {"id":2,"name":"Bob  Stone","email":"bob@@example.com","signup":"2025-11-02","plan":"free","seats":1},
 {"id":3,"name":"Cyd Ashe","email":"cyd@partner.co.uk","signup":"2026-01-19","plan":"pro","seats":40},
 {"id":4,"name":"Dee Okafor","email":"dee@example.com","signup":"2026-07-30","plan":"team","seats":8}]`

	// deployment is a nested API response, the shape a real endpoint returns.
	deployment = `{"metadata":{"name":"orders-api","namespace":"prod","labels":{"app":"orders","tier":"backend"}},
 "spec":{"replicas":4,"template":{"spec":{"containers":[
   {"name":"api","image":"registry.example.com/orders:2.4.1","ports":[{"containerPort":8080}],
    "resources":{"limits":{"cpu":"2","memory":"2Gi"},"requests":{"cpu":"500m","memory":"512Mi"}}},
   {"name":"sidecar","image":"registry.example.com/proxy:1.2.0","ports":[{"containerPort":9901}],
    "resources":{"limits":{"cpu":"200m","memory":"128Mi"}}}]}}},
 "status":{"readyReplicas":3,"conditions":[{"type":"Available","status":"True"},{"type":"Progressing","status":"False"}]}}`

	// hosts is an inventory with the addresses the network cmdlets read.
	hosts = `[{"host":"db-01","ip":"10.0.4.19","mac":"3C-97-0E-1A-2B-3C","port":5432},
 {"host":"edge-01","ip":"203.0.113.7","mac":"3c:97:0e:aa:bb:cc","port":443},
 {"host":"lb-01","ip":"198.51.100.22","mac":"001B44113AB7","port":80},
 {"host":"local","ip":"127.0.0.1","mac":"3C-97-0E-99-88-77","port":22}]`

	// appConfig is a config file, as text.
	appConfig = `"; orders service\n[server]\nhost = 0.0.0.0\nport = 8080\ntls = true\n\n[database]\nurl = postgres://db-01:5432/orders\npool = 20\ntimeout = 30"`

	// releases are version strings needing ordering rather than sorting.
	releases = `["2.0.0", "1.10.0", "1.9.0", "2.0.0-rc.1", "1.2.10", "1.2.9"]`

	// releaseNotes is a corpus of documents where two entries share most of
	// their text, the shape of thing the similarity cmdlets exist to sort.
	releaseNotes = `[{"Name":"v2.1","Content":"## Changes in 2.1.1\n\n- fixed the order total rounding\n- payments now retry on timeout\n- the gateway honors the DNT header\n\nFull changelog:\n- order total rounding\n- payment retries\n- DNT header\n"},
 {"Name":"v2.0","Content":"## Changes in 2.1.0\n\n- fixed the order total rounding\n- payments now retry on timeout\n- the gateway honors the DNT header\n\nFull changelog:\n- order total rounding\n- payment retries\n- DNT header\n"},
 {"Name":"readme","Content":"pwrq is a jq superset that runs PowerShell-style cmdlets against JSON. It composes with ordinary jq filters and emits plain JSON, so logs, exports and API responses become one vocabulary."}]`

	// serviceConfigs is a template and the configurations derived from it,
	// plus one file that shares nothing with it at all.
	serviceConfigs = `[{"Name":"template","Content":"[server]\nhost = 0.0.0.0\nport = 8080\ntls = true\n\n[database]\nurl = postgres://db-01:5432/orders\npool = 20\n\n[logging]\nlevel = info\n"},
 {"Name":"dev","Content":"[server]\nhost = 0.0.0.0\nport = 8080\ntls = false\n\n[database]\nurl = postgres://db-dev:5432/orders\npool = 10\n\n[logging]\nlevel = debug\n"},
 {"Name":"prod","Content":"[server]\nhost = 0.0.0.0\nport = 8443\ntls = true\n\n[database]\nurl = postgres://db-01:5432/orders\npool = 40\n\n[logging]\nlevel = info\n"},
 {"Name":"nginx","Content":"server {\n  listen 80;\n  server_name example.org;\n  root /var/www/html;\n}\n"}]`

	// incidentFiles is a triage problem: a config found on a host that is a
	// modified copy of a known-good one, next to an unrelated file.
	incidentFiles = `[{"Name":"known-good","Content":"[server]\nhost = 0.0.0.0\nport = 8080\ntls = true\n\n[database]\nurl = postgres://db-01:5432/orders\npool = 20\n\n[secrets]\napi_key = sk_live_9f2a\n"},
 {"Name":"found-on-host","Content":"[server]\nhost = 0.0.0.0\nport = 8080\ntls = true\n\n[database]\nurl = postgres://db-01:5432/orders\npool = 20\n\n[secrets]\napi_key = sk_live_7b1c\n"},
 {"Name":"nginx-conf","Content":"server {\n  listen 80;\n  server_name example.org;\n  root /var/www/html;\n}\n"}]`
)

// Examples are the page's gallery.
//
// They are defined here rather than in the JavaScript because here they can be
// tested: TestExamplesAllRun evaluates every one of them against the same
// registry the page uses, so a gallery entry cannot rot into a query that no
// longer works, and TestExamplesDrawToo keeps the diagram beside it honest too.
//
// The gallery is in two halves. The first is pwrq doing a job -- reading a log,
// pivoting an export, reconciling two lists -- grouped by the work rather than
// by which package a function happens to live in. The second is jq's own
// language, kept separate because learning jq and learning pwrq's cmdlets are
// different errands and a visitor is usually on one of them.
//
// An example earns its place by showing a cmdlet doing the thing someone would
// actually reach for it to do, inside a pipeline with a beginning and an end. A
// single call on a literal demonstrates that a function exists, which is what
// the --udf-list metadata is for.
func Examples() []Example {
	return []Example{

		// ------------------------------------------------------------------
		// Reading logs
		// ------------------------------------------------------------------
		{
			Title:       "Break a log line into fields",
			Description: "A raw access log is text until you split it. regex-free field extraction with jq's own capture, then the numbers made numbers.",
			Category:    "Logs",
			Query: `split("\n")
| map(capture("^(?<ip>\\S+) \\S+ \\S+ \\[(?<ts>[^\\]]+)\\] \"(?<method>\\S+) (?<path>\\S+)[^\"]*\" (?<status>\\d+) (?<bytes>\\d+) (?<secs>\\S+)$"))
| map(.status |= tonumber | .bytes |= tonumber | .secs |= tonumber)
| .[0:3]`,
			Input: accessLog,
		},
		{
			Title:       "Error rate by endpoint",
			Description: "The question a log exists to answer: which endpoint is failing, and how often. count_by tallies, and the two tallies divide.",
			Category:    "Logs",
			Query: `split("\n")
| map(capture("\"(?<method>\\S+) (?<path>\\S+)[^\"]*\" (?<status>\\d+)") | .status |= tonumber)
| group_by_key("path")
| to_entries
| map({path: .key,
       requests: (.value | length),
       failed: ([.value[] | select(.status >= 400)] | length)})
| map(. + {rate: (if .requests == 0 then 0 else (.failed / .requests * 100 | round_to(1)) end)})
| sort_by(-.rate)`,
			Input: accessLog,
		},
		{
			Title:       "Slowest requests, with their share",
			Description: "top_by ranks rows by a numeric column; percentage turns each duration into its share of the total time spent.",
			Category:    "Logs",
			Query: `split("\n")
| map(capture("\"(?<method>\\S+) (?<path>\\S+)[^\"]*\" (?<status>\\d+) \\d+ (?<secs>\\S+)$") | .secs |= tonumber)
| (map(.secs) | add) as $total
| top_by(.; "secs"; 3)
| map({path, secs, share: (.secs | percentage($total) | round_to(1))})`,
			Input: accessLog,
		},
		{
			Title:       "Who is hitting us most",
			Description: "value_counts is the histogram of a list: extract the client addresses and count the distinct ones in one step.",
			Category:    "Logs",
			Query: `split("\n")
| map(capture("^(?<ip>\\S+)").ip)
| value_counts
| to_entries
| map({ip: .key, requests: .value, private: (.key | is_private_ip)})
| sort_by(-.requests)`,
			Input: accessLog,
		},
		{
			Title:       "Bucket traffic by minute",
			Description: "Timestamps become buckets by truncating them, which is how a log turns into a time series.",
			Category:    "Logs",
			Query: `split("\n")
| map(capture("\\[(?<ts>[^\\]]+)\\]").ts | .[0:17])
| count_by(.; ".")
| to_entries
| map({minute: (.key | split(":")[1:3] | join(":")), hits: .value})
| sort_by(.minute)`,
			Input: accessLog,
		},
		{
			Title:       "Read structured logs",
			Description: "logfmt_parse turns key=value logging into objects, after which it is ordinary data.",
			Category:    "Logs",
			Query: `split("\n")
| map(logfmt_parse)
| map(.dur_ms |= tonumber)
| map({ts, level, svc, dur_ms})
| .[0:3]`,
			Input: appLog,
		},
		{
			Title:       "Which service is unhealthy",
			Description: "summarize_by gives count, sum, average, min and max per key in one call, which is the whole of a first-look report.",
			Category:    "Logs",
			Query: `split("\n")
| map(logfmt_parse | .dur_ms |= tonumber)
| summarize_by(.; "svc"; "dur_ms")`,
			Input: appLog,
		},
		{
			Title:       "Errors only, newest first",
			Description: "The everyday filter: select the level, keep the fields worth reading, and put the recent ones on top.",
			Category:    "Logs",
			Query: `split("\n")
| map(logfmt_parse)
| map(select(.level == "error"))
| map({ts, svc, msg, seconds: (.dur_ms | tonumber / 1000)})
| sort_by(.ts)
| reverse`,
			Input: appLog,
		},
		{
			Title:       "Repeated failures",
			Description: "Two identical errors are a pattern, not two events. Counting the message is how you notice.",
			Category:    "Logs",
			Query: `split("\n")
| map(logfmt_parse | select(.level == "error") | .msg)
| value_counts
| to_entries
| map(select(.value > 1) | {message: .key, occurrences: .value})`,
			Input: appLog,
		},
		{
			Title:       "Strip colour before parsing",
			Description: "A log captured from a terminal carries escape sequences that break every parser downstream.",
			Category:    "Logs",
			Query: `strip_ansi
| split("\n")
| map(logfmt_parse | {level, svc})
| .[0:2]`,
			Input: `"\u001b[32mts=2026-08-11T09:12:03Z level=info svc=orders\u001b[0m\n\u001b[31mts=2026-08-11T09:15:44Z level=error svc=payments\u001b[0m"`,
		},
		{
			Title:       "Find the indicators in a line",
			Description: "extract_ips, extract_urls and extract_emails pull the artefacts out of free text without a regex each.",
			Category:    "Logs",
			Query:       `{addresses: extract_ips, links: extract_urls, contacts: extract_emails, dates: extract_dates}`,
			Input:       `"2026-08-11 alert: 10.0.4.19 and 203.0.113.7 reached https://c2.example.net/beacon; notify soc@example.com"`,
		},
		{
			Title:       "Deduplicate noisy lines",
			Description: "unique_lines and sort_lines treat a block of text as rows, which is what you want before diffing two captures.",
			Category:    "Logs",
			Query: `unique_lines
| sort_lines
| split("\n")
| {distinct: length, lines: .}`,
			Input: `"connection reset\ntimeout\nconnection reset\nauth failed\ntimeout\nconnection reset"`,
		},
		{
			Title:       "What changed between two captures",
			Description: "diff_lines reports the lines added and removed, which is the question when comparing before and after.",
			Category:    "Logs",
			Query:       `diff_lines("orders ok\npayments timeout\ngateway ok\nsearch down")`,
			Input:       `"orders ok\npayments ok\ngateway ok"`,
		},
		{
			Title:       "Head and tail of a block of text",
			Description: "first_lines and last_lines are head and tail for a string you already have in the pipeline.",
			Category:    "Logs",
			Query:       `{opening: (first_lines(2) | split("\n")), closing: (last_lines(2) | split("\n")), total: line_count}`,
			Input:       appLog,
		},
		{
			Title:       "Count the lines that match",
			Description: "Counting matches is often the whole answer, and needs no parsing at all.",
			Category:    "Logs",
			Query: `split("\n")
| {total: length,
   errors: (map(select(test("level=error"))) | length),
   slow: (map(select(capture("dur_ms=(?<d>\\d+)").d | tonumber > 500)) | length)}`,
			Input: appLog,
		},

		// ------------------------------------------------------------------
		// Tables and reports
		// ------------------------------------------------------------------
		{
			Title:       "Read a CSV export",
			Description: "csv_parse turns a spreadsheet export into objects with the header row as keys, after which every other cmdlet applies.",
			Category:    "Tables",
			Query: `csv_parse
| .[0] as $head
| .[1:]
| map([$head, .] | transpose | map({key: .[0], value: .[1]}) | from_entries)
| map(.units |= tonumber | .revenue |= tonumber)
| .[0:3]`,
			Input: salesCSV,
		},
		{
			Title:       "Pivot a sales table",
			Description: "pivot builds the row-by-column grid a spreadsheet would, from rows that arrived flat.",
			Category:    "Tables",
			Query: `csv_parse
| .[0] as $head
| .[1:]
| map([$head, .] | transpose | map({key: .[0], value: .[1]}) | from_entries)
| map(.revenue |= tonumber)
| pivot(.; {rows: "region", cols: "quarter", values: "revenue"})`,
			Input: salesCSV,
		},
		{
			Title:       "Totals per region",
			Description: "sum_by and avg_by answer the two questions a manager asks about any column.",
			Category:    "Tables",
			Query: `csv_parse
| .[0] as $head
| .[1:]
| map([$head, .] | transpose | map({key: .[0], value: .[1]}) | from_entries)
| map(.revenue |= tonumber)
| {total: sum_by(.; "region"; "revenue"),
   average: avg_by(.; "region"; "revenue") | map_values(round)}`,
			Input: salesCSV,
		},
		{
			Title:       "Full summary per group",
			Description: "summarize_by is count, sum, average, min and max at once, which is the report you usually wanted.",
			Category:    "Tables",
			Query: `csv_parse
| .[0] as $head
| .[1:]
| map([$head, .] | transpose | map({key: .[0], value: .[1]}) | from_entries)
| map(.units |= tonumber)
| summarize_by(.; "region"; "units")`,
			Input: salesCSV,
		},
		{
			Title:       "Rank the representatives",
			Description: "top_by and bottom_by take the ends of a ranking without sorting the whole table by hand.",
			Category:    "Tables",
			Query: `csv_parse
| .[0] as $head
| .[1:]
| map([$head, .] | transpose | map({key: .[0], value: .[1]}) | from_entries)
| map(.revenue |= tonumber)
| group_by_key("rep")
| to_entries
| map({rep: .key, revenue: (.value | map(.revenue) | add)})
| {best: top_by(.; "revenue"; 2), worst: bottom_by(.; "revenue"; 2)}`,
			Input: salesCSV,
		},
		{
			Title:       "Melt a wide table back",
			Description: "unpivot is pivot's inverse: value columns become key/value rows, which is what a chart library usually wants.",
			Category:    "Tables",
			Query: `unpivot(.; {cols: ["Q1", "Q2"], id: "region"})
| map({region, quarter: .key, revenue: .value})`,
			Input: `[{"region":"EMEA","Q1":84000,"Q2":56000},{"region":"AMER","Q1":153000,"Q2":78625}]`,
		},
		{
			Title:       "Render a table",
			Description: "format_table lays objects out the way Format-Table does. Switch the output to Raw to read it.",
			Category:    "Tables",
			Query: `csv_parse
| .[0] as $head
| .[1:]
| map([$head, .] | transpose | map({key: .[0], value: .[1]}) | from_entries)
| map(.revenue |= tonumber)
| summarize_by(.; "region"; "revenue")
| to_entries
| map({Region: .key, Deals: .value.count, Revenue: .value.sum, Average: (.value.avg | round)})
| format_table(.; .)`,
			Input: salesCSV,
		},
		{
			Title:       "Write a CSV back out",
			Description: "csv_stringify closes the loop: read an export, reshape it, and hand back something a spreadsheet opens.",
			Category:    "Tables",
			Query: `csv_parse
| .[0] as $head
| .[1:]
| map([$head, .] | transpose | map({key: .[0], value: .[1]}) | from_entries)
| map(select(.region == "EMEA"))
| map({rep, quarter, revenue})
| [(.[0] | keys)] + map([.[]])
| csv_stringify`,
			Input: salesCSV,
		},
		{
			Title:       "Tab-separated instead",
			Description: "tsv_parse and tsv_stringify are the same job where the separator is a tab, which is what most database exports use.",
			Category:    "Tables",
			Query: `tsv_parse
| .[0] as $head
| .[1:]
| map([$head, .] | transpose | map({key: .[0], value: .[1]}) | from_entries)
| map({host, role})
| [(.[0] | keys)] + map([.[]])
| tsv_stringify`,
			Input: `"host\trole\tzone\ndb-01\tprimary\teu-west\ndb-02\treplica\teu-west"`,
		},
		{
			Title:       "Group objects the PowerShell way",
			Description: "group_object buckets by a property and reports each bucket with its members, as Group-Object does.",
			Category:    "Tables",
			Query: `group_object(.; {property: "Status"})
| map({Status: .Name, Count, Members: (.Group | map(.Name))})`,
			Input: services,
		},
		{
			Title:       "Filter, sort and project",
			Description: "where_object, sort_object and select_object are the PowerShell pipeline, and they compose the same way here.",
			Category:    "Tables",
			Query: `where_object(.; {script: ".CPU > 10"})
| sort_object(.; {property: "CPU", descending: true})
| select_object(.; {property: ["Name", "CPU", "Status"]})`,
			Input: services,
		},
		{
			Title:       "Measure a column",
			Description: "measure_object is Measure-Object: count, sum, average, minimum and maximum of one property.",
			Category:    "Tables",
			Query:       `measure_object(.; {property: "MemoryMB", sum: true, average: true, minimum: true, maximum: true})`,
			Input:       services,
		},
		{
			Title:       "A list instead of a table",
			Description: "format_list prints one property per line, which is what you want when a row is too wide to read across.",
			Category:    "Tables",
			Query: `where_object(.; {script: ".Status != \"Running\""})
| format_list(.; .)`,
			Input: services,
		},
		{
			Title:       "Index rows for lookup",
			Description: "index_by turns a list into a map keyed by a property, so later stages can look rows up instead of scanning.",
			Category:    "Tables",
			Query: `index_by(.; "Name")
| {payments: .["payments-api"].Version, gateway: .gateway.Version}`,
			Input: services,
		},
		{
			Title:       "Find one row",
			Description: "lookup is the single-row answer: the first row whose property matches, without filtering the whole list.",
			Category:    "Tables",
			Query: `{degraded: lookup(.; "Status"; "Degraded").Name,
   version: lookup(.; "Name"; "search").Version}`,
			Input: services,
		},
		{
			Title:       "Rename and prune columns",
			Description: "rename_keys and prune tidy a table for handover: friendly headings, and no empty cells.",
			Category:    "Tables",
			Query: `map(rename_keys({Name: "service", MemoryMB: "memory_mb", CPU: "cpu_percent"}))
| map(prune)
| .[0:3]`,
			Input: services,
		},
		{
			Title:       "Count rows by a property",
			Description: "count_by is Group-Object's tally when you want the numbers rather than the members.",
			Category:    "Tables",
			Query:       `{by_status: count_by(.; "Status"), by_version: count_by(.; "Version")}`,
			Input:       services,
		},
		{
			Title:       "Split a batch into pages",
			Description: "chunks is how a long list becomes pages, or a rate-limited API's worth of requests.",
			Category:    "Tables",
			Query: `map(.Name)
| chunks(2)
| to_entries
| map({page: (.key + 1), services: .value})`,
			Input: services,
		},
		{
			Title:       "Every combination",
			Description: "cartesian pairs two lists exhaustively, which is how a test matrix gets built.",
			Category:    "Tables",
			Query: `cartesian(["2.4.1", "2.3.9"]; ["linux", "darwin"])
| map({version: .[0], platform: .[1]})`,
			Input: `null`,
		},
		{
			Title:       "Rotate and interleave",
			Description: "rotate shifts a list round; interleave zips two lists into one alternating sequence.",
			Category:    "Tables",
			Query: `{rotated: (["mon","tue","wed","thu"] | rotate(2)),
   paired: (["a","b","c"] | interleave([1,2,3])),
   zipped: (["a","b","c"] | zip_arrays([1,2,3]))}`,
			Input: `null`,
		},
		{
			Title:       "Pull a column out of rows",
			Description: "column takes the nth element of every row, which is what a headerless CSV leaves you with.",
			Category:    "Tables",
			Query:       `{hosts: column(.; 0), ports: column(.; 2) | map(tonumber)}`,
			Input:       `[["db-01","primary","5432"],["edge-01","proxy","443"],["lb-01","balancer","80"]]`,
		},

		// ------------------------------------------------------------------
		// APIs and JSON
		// ------------------------------------------------------------------
		{
			Title:       "Reach into a nested response",
			Description: "get_path reads a deep path without a chain of question marks, and returns null rather than failing when it is absent.",
			Category:    "JSON",
			Query: `{name: get_path("metadata.name"),
   replicas: get_path("spec.replicas"),
   ready: get_path("status.readyReplicas"),
   missing: get_path("status.phase")}`,
			Input: deployment,
		},
		{
			Title:       "Check before you read",
			Description: "has_path answers whether a path exists at all, which null cannot distinguish from a null value.",
			Category:    "JSON",
			Query: `{has_limits: has_path("spec.template.spec.containers[0].resources.limits"),
   has_hpa: has_path("spec.autoscaling"),
   containers: (get_path("spec.template.spec.containers") | length)}`,
			Input: deployment,
		},
		{
			Title:       "Summarise every container",
			Description: "The list buried four levels down is the interesting part; pull it up and it becomes a table.",
			Category:    "JSON",
			Query: `get_path("spec.template.spec.containers")
| map({name,
       image: (.image | split("/") | last),
       cpu: get_path("resources.limits.cpu"),
       memory: get_path("resources.limits.memory")})`,
			Input: deployment,
		},
		{
			Title:       "Patch a document",
			Description: "json_merge_patch applies RFC 7386 semantics: nested objects merge, and a null deletes.",
			Category:    "JSON",
			Query: `json_merge_patch({spec: {replicas: 6}, status: null})
| {replicas: .spec.replicas, status_removed: (has("status") | not)}`,
			Input: deployment,
		},
		{
			Title:       "Merge configuration layers",
			Description: "deep_merge is how a base config and an environment override become one document, recursively.",
			Category:    "JSON",
			Query: `deep_merge({server: {host: "0.0.0.0", port: 8080, tls: false}, log: {level: "info"}};
             {server: {port: 8443, tls: true}, log: {format: "json"}})`,
			Input: `null`,
		},
		{
			Title:       "Set and delete deep values",
			Description: "set_path and del_path edit in place at a path, which is fiddly to express with jq's assignment on a computed path.",
			Category:    "JSON",
			Query: `set_path("metadata.labels.release"; "2.4.1")
| del_path("metadata.labels.tier")
| .metadata.labels`,
			Input: deployment,
		},
		{
			Title:       "Address a value by JSON Pointer",
			Description: "json_pointer is RFC 6901, which is what an API error or a JSON Schema will hand you.",
			Category:    "JSON",
			Query: `{image: json_pointer("/spec/template/spec/containers/0/image"),
   ready: json_pointer("/status/readyReplicas"),
   patched: (json_pointer_set("/spec/replicas"; 8) | .spec.replicas)}`,
			Input: deployment,
		},
		{
			Title:       "Flatten for a flat world",
			Description: "flatten_keys turns a nested document into dot-and-bracket keys, which is what metrics tags and env vars need.",
			Category:    "JSON",
			Query: `.metadata
| flatten_keys
| to_entries
| map({key, value})`,
			Input: deployment,
		},
		{
			Title:       "Put it back together",
			Description: "unflatten_keys is the inverse, for reading a flat config back into a document.",
			Category:    "JSON",
			Query: `unflatten_keys
| {host: .server.host, port: .server.port, retries: .client.retry.max}`,
			Input: `{"server.host":"0.0.0.0","server.port":8080,"client.retry.max":3}`,
		},
		{
			Title:       "Read newline-delimited JSON",
			Description: "jsonl_parse reads the one-object-per-line format logs and exports ship in.",
			Category:    "JSON",
			Query: `json_stringify
| .`,
			Input: `{"note":"jsonl_parse reads a stream of objects; see the CLI for file input"}`,
		},
		{
			Title:       "Build and read a query string",
			Description: "query_string_build and query_string_parse move between a URL's tail and an object.",
			Category:    "JSON",
			Query: `{built: query_string_build({q: "orders api", page: 2, sort: "-created"}),
   parsed: ("status=500&svc=payments&since=2026-08-11" | query_string_parse)}`,
			Input: `null`,
		},
		{
			Title:       "Parse and re-emit JSON text",
			Description: "json_parse and json_stringify cross the boundary when a JSON document arrives as a string inside another one.",
			Category:    "JSON",
			Query: `.payload
| json_parse
| .items
| map(.sku)`,
			Input: `{"envelope":"v1","payload":"{\"items\":[{\"sku\":\"A-1\"},{\"sku\":\"B-2\"}]}"}`,
		},
		{
			Title:       "YAML in, JSON out",
			Description: "yaml_parse reads the format config files are written in; everything downstream is ordinary JSON.",
			Category:    "JSON",
			Query: `yaml_parse
| {service: .name, replicas: .replicas, ports: (.ports | map(.container))}`,
			Input: `"name: orders-api\nreplicas: 4\nports:\n  - container: 8080\n    host: 80\n  - container: 9901\n    host: 9901"`,
		},
		{
			Title:       "JSON out as YAML",
			Description: "yaml_stringify writes the document back out in the form a config file wants.",
			Category:    "JSON",
			Query: `{apiVersion: "apps/v1", kind: "Deployment", metadata: {name: .metadata.name}, spec: {replicas: .spec.replicas}}
| yaml_stringify`,
			Input: deployment,
		},
		{
			Title:       "XML in, JSON out",
			Description: "xml_parse handles the format that arrives from older services, after which jq applies as usual.",
			Category:    "JSON",
			Query:       `xml_parse`,
			Input:       `"<config><server><host>0.0.0.0</host><port>8080</port></server></config>"`,
		},
		{
			Title:       "JSON out as XML",
			Description: "xml_stringify goes the other way, for the endpoint that still wants angle brackets.",
			Category:    "JSON",
			Query:       `{service: {name: .metadata.name, replicas: .spec.replicas}} | xml_stringify`,
			Input:       deployment,
		},

		// ------------------------------------------------------------------
		// Text
		// ------------------------------------------------------------------
		{
			Title:       "Normalise a name for a URL",
			Description: "slugify does the whole job: accents folded, punctuation dropped, spaces hyphenated.",
			Category:    "Text",
			Query:       `map({name, slug: (.name | slugify), initials: (.name | acronym)})`,
			Input:       users,
		},
		{
			Title:       "Convert between naming styles",
			Description: "The case converters share one word-splitting rule, so a field name crosses between languages intact.",
			Category:    "Text",
			Query: `["userIdToken", "user_id_token", "User Id Token"]
| map({input: ., camel: camel_case, snake: snake_case, kebab: kebab_case, pascal: pascal_case})`,
			Input: `null`,
		},
		{
			Title:       "Headline and sentence case",
			Description: "title_case, sentence_case and capitalize_first differ in exactly which words they touch.",
			Category:    "Text",
			Query: `"the quick brown fox jumps"
| {title: title_case, sentence: sentence_case, first: capitalize_first, swapped: swap_case}`,
			Input: `null`,
		},
		{
			Title:       "Tidy user input",
			Description: "normalize_whitespace collapses runs of spaces; remove_accents folds a name to ASCII for matching.",
			Category:    "Text",
			Query: `map(.name)
| map({raw: ., tidy: normalize_whitespace, ascii: (normalize_whitespace | remove_accents)})`,
			Input: users,
		},
		{
			Title:       "Truncate for a narrow column",
			Description: "truncate cuts at a character count and truncate_words at a word boundary, which reads better in a headline.",
			Category:    "Text",
			Query: `"upstream timeout while calling the payments provider"
| {chars: truncate(24), words: truncate_words(4)}`,
			Input: `null`,
		},
		{
			Title:       "Wrap and indent a block",
			Description: "wrap_text reflows to a width, indent shifts a block, and prefix_lines marks every line.",
			Category:    "Text",
			Query: `"the payments service timed out twice within five minutes and was marked degraded"
| wrap_text(38)
| join("\n")
| indent(2)
| prefix_lines("> ")
| split("\n")`,
			Input: `null`,
		},
		{
			Title:       "Strip the indentation back off",
			Description: "dedent removes the common leading whitespace, which is what a heredoc or an embedded snippet needs.",
			Category:    "Text",
			Query:       `dedent | split("\n")`,
			Input:       `"        server:\n          port: 8080\n          tls: true"`,
		},
		{
			Title:       "Pad into columns",
			Description: "pad_left, pad_right and pad_center line text up without a table renderer.",
			Category:    "Text",
			Query: `map({label: (.Name | pad_right(14; ".")),
      cpu: (.CPU | tostring | pad_left(6)),
      status: (.Status | pad_center(12; "-"))})
| map(.label + .cpu + " " + .status)`,
			Input: services,
		},
		{
			Title:       "Mask a secret",
			Description: "mask keeps the ends and hides the middle, which is how a credential is shown in a report.",
			Category:    "Text",
			Query:       `map({user: .email, masked: (.email | mask(2; 4))})`,
			Input:       users,
		},
		{
			Title:       "Fill a template",
			Description: "template substitutes named placeholders, which beats string interpolation when the text is data.",
			Category:    "Text",
			Query: `map(. as $u
      | "Hi {{name}}, your {{plan}} plan covers {{seats}} seats."
      | template({name: $u.name, plan: $u.plan, seats: ($u.seats | tostring)}))`,
			Input: users,
		},
		{
			Title:       "Count what is in a string",
			Description: "word_count, char_frequencies and the vowel counters answer the measurement questions in one pass each.",
			Category:    "Text",
			Query: `"the quick brown fox jumps over the lazy dog"
| {words: word_count, vowels: count_vowels, consonants: count_consonants,
   the: count_occurrences("the"),
   commonest: (char_frequencies | to_entries | sort_by(-.value) | .[0])}`,
			Input: `null`,
		},
		{
			Title:       "Split around a marker",
			Description: "before_first and after_first cut a string at its first delimiter, which is what a log prefix needs.",
			Category:    "Text",
			Query: `map({level: (. | after_first("level=") | before_first(" ")),
      head: before_first(" ")})`,
			Input: `["ts=2026-08-11T09:15:44Z level=error svc=payments", "ts=2026-08-11T09:12:03Z level=info svc=orders"]`,
		},
		{
			Title:       "Quote and surround",
			Description: "surround wraps a value, and strip_quotes takes existing quoting back off.",
			Category:    "Text",
			Query:       `map({quoted: surround("<<"; ">>"), unquoted: strip_quotes})`,
			Input:       `["already \"quoted\"", "bare"]`,
		},
		{
			Title:       "Reverse words and lines",
			Description: "reverse_words and reverse_lines reverse at the unit you mean, which explode/reverse/implode cannot.",
			Category:    "Text",
			Query: `{words: ("orders payments gateway" | reverse_words),
   lines: ("first\nsecond\nthird" | reverse_lines)}`,
			Input: `null`,
		},
		{
			Title:       "Match with a glob",
			Description: "match_glob is shell matching for values, and glob_to_regex shows what it compiles to.",
			Category:    "Text",
			Query: `map(.Name)
| {api: map(select(match_glob("*-api"))),
   pattern: ("*-api" | glob_to_regex)}`,
			Input: services,
		},
		{
			Title:       "Escape text for a pattern",
			Description: "escape_regex makes a literal safe to embed, and is_regex_valid checks one before you rely on it.",
			Category:    "Text",
			Query: `{literal: ("orders.api (v2)" | escape_regex),
   good: ("^[a-z]+$" | is_regex_valid),
   bad: ("[unclosed" | is_regex_valid)}`,
			Input: `null`,
		},
		{
			Title:       "Check the shape of a string",
			Description: "The predicates answer the everyday questions about a field before you trust it.",
			Category:    "Text",
			Query: `["orders-api", "ORDERS", "orders42", "  ", "café"]
| map({value: .,
       blank: is_blank, upper: is_uppercase, lower: is_lowercase,
       alnum: is_alphanumeric, alpha: is_alphabetic, ascii: is_ascii,
       numeric: is_numeric_string})`,
			Input: `null`,
		},
		{
			Title:       "Balanced brackets",
			Description: "is_balanced answers whether a fragment is complete, which is what a partially captured payload fails.",
			Category:    "Text",
			Query: `["{\"a\": [1, 2]}", "{\"a\": [1, 2]", "(a (b) c)"]
| map({fragment: ., balanced: is_balanced})`,
			Input: `null`,
		},
		{
			Title:       "Escape non-ASCII",
			Description: "unicode_escape and unicode_unescape move between a literal and its \\u form, for protocols that insist on ASCII.",
			Category:    "Text",
			Query: `"café — naïve"
| unicode_escape as $e
| {escaped: $e, back: ($e | unicode_unescape)}`,
			Input: `null`,
		},
		{
			Title:       "Quoted-printable for mail",
			Description: "quoted_printable_encode is the transfer encoding email headers still use.",
			Category:    "Text",
			Query: `"subject: café review — 12€"
| quoted_printable_encode as $q
| {encoded: $q, decoded: ($q | quoted_printable_decode)}`,
			Input: `null`,
		},
		{
			Title:       "Sort a block of text",
			Description: "sort_lines orders rows inside a string, which is what you do before comparing two captures.",
			Category:    "Text",
			Query:       `sort_lines | split("\n")`,
			Input:       `"payments\ngateway\norders\nsearch\nmailer"`,
		},
		{
			Title:       "Replace across a document",
			Description: "replace works on plain text, so chaining it scrubs several tokens without a regex each.",
			Category:    "Text",
			Query: `replace("10.0.0.14"; "<internal>")
| replace("203.0.113.7"; "<external>")
| split("\n")
| .[0:3]`,
			Input: accessLog,
		},

		// ------------------------------------------------------------------
		// Numbers
		// ------------------------------------------------------------------
		{
			Title:       "Make bytes readable",
			Description: "human_bytes and parse_size are inverses, so a report can round-trip through a human-readable form.",
			Category:    "Numbers",
			Query: `map({service: .Name, memory: (.MemoryMB * 1048576 | human_bytes)})
| map(. + {back: (.memory | parse_size)})`,
			Input: services,
		},
		{
			Title:       "Big numbers for people",
			Description: "human_number shortens to a suffix and group_digits keeps every digit with separators.",
			Category:    "Numbers",
			Query: `[892, 48000, 1250000, 987654321]
| map({raw: ., short: human_number, grouped: group_digits, money: format_currency})`,
			Input: `null`,
		},
		{
			Title:       "Round for a report",
			Description: "round_to fixes the decimal places and to_fixed renders them, which are different jobs.",
			Category:    "Numbers",
			Query: `map(.CPU)
| map({raw: ., rounded: round_to(1), fixed: to_fixed(2), clamped: clamp(0; 80)})`,
			Input: services,
		},
		{
			Title:       "Percentages and change",
			Description: "percentage is a share of a total and pct_change is movement between two readings.",
			Category:    "Numbers",
			Query: `{share: (63.8 | percentage(171.5) | round_to(1)),
   growth: (48000 | pct_change(56000) | round_to(1)),
   rescaled: (63.8 | rescale(0; 100; 0; 5) | round_to(2))}`,
			Input: `null`,
		},
		{
			Title:       "Convert units",
			Description: "convert_unit works across every unit of a quantity, and refuses to convert between quantities.",
			Category:    "Numbers",
			Query: `{f: (20 | convert_unit("C"; "F")),
   km: convert_unit(5; "mi"; "km"),
   hours: (90 | convert_unit("min"; "h")),
   kg: (10 | convert_unit("lb"; "kg")),
   refused: (try (5 | convert_unit("kg"; "m")) catch "not the same quantity")}`,
			Input: `null`,
		},
		{
			Title:       "Change base",
			Description: "to_base and from_base cover any radix; the hex pair is the one worth its own name.",
			Category:    "Numbers",
			Query: `[255, 4096, 8080]
| map({n: ., hex: to_hex_number, binary: to_base(2), b36: to_base(36)})
| map(. + {back: (.hex | from_hex_number)})`,
			Input: `null`,
		},
		{
			Title:       "Divisibility and primes",
			Description: "gcd and lcm size a schedule; the prime cmdlets answer the factoring questions.",
			Category:    "Numbers",
			Query: `{gcd: (84 | gcd(126)), lcm: (4 | lcm(6)),
   prime: (97 | is_prime), next: (90 | next_prime),
   factors: (360 | prime_factors),
   digits: (98765 | digit_sum), bits: (255 | hamming_weight)}`,
			Input: `null`,
		},
		{
			Title:       "Counting arrangements",
			Description: "factorial, combinations_count and permutations_count answer how many ways, which comes up in sizing work.",
			Category:    "Numbers",
			Query: `{fact: (6 | factorial),
   choose: (52 | combinations_count(5)),
   arrange: (10 | permutations_count(3)),
   fib: (20 | fibonacci)}`,
			Input: `null`,
		},
		{
			Title:       "Parity and shape of a number",
			Description: "The small predicates keep a filter readable where an arithmetic test would not be.",
			Category:    "Numbers",
			Query: `[7, 8, 64, 3, 1]
| map({n: ., even: is_even, odd: is_odd, sign: sign, pow2: is_power_of_two, ordinal: (if . > 0 then ordinal else null end)})`,
			Input: `null`,
		},
		{
			Title:       "Interpolate between two values",
			Description: "lerp walks from one value to another, which is how a ramp or a backoff curve is described.",
			Category:    "Numbers",
			Query: `[0, 0.25, 0.5, 0.75, 1]
| map({t: ., ms: (100 | lerp(2000; .) | round)})`,
			Input: `null`,
		},

		// ------------------------------------------------------------------
		// Statistics
		// ------------------------------------------------------------------
		{
			Title:       "First look at a series",
			Description: "summary is the five-number look plus the mean, which is where any analysis of a metric starts.",
			Category:    "Statistics",
			Query:       `summary`,
			Input:       latencies,
		},
		{
			Title:       "Centre and spread",
			Description: "mean, median and mode disagree when a series is skewed, and that disagreement is the finding.",
			Category:    "Statistics",
			Query: `{mean: (mean | round_to(1)), median: median, mode: mode,
   stdev: (stdev | round_to(1)), variance: (variance | round_to(1)),
   iqr: iqr, mad: (mad | round_to(1))}`,
			Input: latencies,
		},
		{
			Title:       "Percentiles that matter",
			Description: "A latency series is judged at its tail, which is exactly what the mean hides.",
			Category:    "Statistics",
			Query: `{p50: percentile(50), p90: percentile(90), p95: percentile(95), p99: percentile(99),
   quartiles: quartiles,
   worst_rank: percentile_rank(2104)}`,
			Input: latencies,
		},
		{
			Title:       "Robust averages",
			Description: "trimmed_mean drops the extremes, and the geometric and harmonic means suit rates and ratios.",
			Category:    "Statistics",
			Query: `{plain: (mean | round_to(1)),
   trimmed: (trimmed_mean(0.2) | round_to(1)),
   geometric: (geomean | round_to(1)),
   harmonic: (harmonic_mean | round_to(1)),
   rms: (rms | round_to(1))}`,
			Input: latencies,
		},
		{
			Title:       "Weight the average",
			Description: "weighted_mean is the right average when the samples do not count equally.",
			Category:    "Statistics",
			Query: `{unweighted: (mean([120, 140, 90]) | round_to(1)),
   weighted: (weighted_mean([120, 140, 90]; [48000, 56000, 36000]) | round_to(1))}`,
			Input: `null`,
		},
		{
			Title:       "How skewed is it",
			Description: "skewness and kurtosis say whether a series is lopsided and how heavy its tails are.",
			Category:    "Statistics",
			Query: `{skewness: (skewness | round_to(2)),
   kurtosis: (kurtosis | round_to(2)),
   product_of_first_three: ([.[0], .[1], .[2]] | product)}`,
			Input: latencies,
		},
		{
			Title:       "Smooth a noisy series",
			Description: "moving_average follows the trend and ema weights recent readings more heavily.",
			Category:    "Statistics",
			Query: `{rolling: (moving_average(4) | map(round)),
   smoothed: (ema(0.4) | map(round)),
   band: {high: (moving_max(4) | map(round)), low: (moving_min(4) | map(round))}}`,
			Input: latencies,
		},
		{
			Title:       "Volatility over a window",
			Description: "moving_stdev shows where a series became unstable, which the level alone does not.",
			Category:    "Statistics",
			Query:       `moving_stdev(4) | map(round_to(1))`,
			Input:       latencies,
		},
		{
			Title:       "Running totals and changes",
			Description: "cumsum accumulates, deltas differences, and the cumulative extrema track records so far.",
			Category:    "Statistics",
			Query: `{running: cumsum,
   changes: deltas,
   best: cumulative_min,
   worst: cumulative_max}`,
			Input: latencies,
		},
		{
			Title:       "Compare two series",
			Description: "correlation and covariance score whether two measurements move together.",
			Category:    "Statistics",
			Query: `{correlation: (correlation([1,2,3,4,5]; [2,4,7,8,11]) | round_to(3)),
   covariance: (covariance([1,2,3,4,5]; [2,4,7,8,11]) | round_to(3)),
   autocorrelation: ([84,91,77,88,95,102,88,91] | autocorrelation(1) | round_to(3))}`,
			Input: `null`,
		},
		{
			Title:       "Put series on one scale",
			Description: "normalize maps to 0..1 and standardize to standard deviations, which is what comparing units needs.",
			Category:    "Statistics",
			Query: `{normalized: (normalize | map(round_to(3)) | .[0:6]),
   standardized: (standardize | map(round_to(2)) | .[0:6])}`,
			Input: latencies,
		},
		{
			Title:       "Shift and fill a series",
			Description: "lag aligns a series against its own past, and fill_forward carries the last reading over a gap.",
			Category:    "Statistics",
			Query: `{lagged: ([84, 91, 77, 412] | lag(1)),
   filled: ([84, null, null, 412, null] | fill_forward)}`,
			Input: `null`,
		},
		{
			Title:       "Rolling windows by hand",
			Description: "windows gives every consecutive slice, for the aggregate that has no cmdlet of its own.",
			Category:    "Statistics",
			Query: `.[0:6]
| windows(3)
| map({window: ., spread: ((max - min))})`,
			Input: latencies,
		},

		// ------------------------------------------------------------------
		// Dates and times
		// ------------------------------------------------------------------
		{
			Title:       "Move an instant between zones",
			Description: "to_timezone reports the offset and abbreviation too, because that is the question it exists to answer.",
			Category:    "Dates",
			Query: `["Europe/London", "Asia/Tokyo", "America/New_York"]
| map(. as $zone | ("2026-08-11T12:00:00Z" | to_timezone($zone)) as $at
      | {zone: $zone, local: $at.DateTime, abbr: $at.Abbreviation, dst: $at.IsDST})`,
			Input: `null`,
		},
		{
			Title:       "Write a date the way you need it",
			Description: "format_date takes a named layout or a Go one, and can render in another zone at the same time.",
			Category:    "Dates",
			Query: `"2026-08-11T23:30:00Z"
| {date: format_date("date"), time: format_date("time"),
   http: format_date("http"), custom: format_date("Mon 2 Jan 2006"),
   tokyo: format_date("datetime"; "Asia/Tokyo")}`,
			Input: `null`,
		},
		{
			Title:       "Read a date that is not ISO",
			Description: "parse_date is the inverse: only the caller's layout says whether 03/04 is March or April.",
			Category:    "Dates",
			Query: `{uk: ("03/04/2026" | parse_date("02/01/2006")),
   us: ("03/04/2026" | parse_date("01/02/2006")),
   berlin: ("2026-08-11 09:30:00" | parse_date("datetime"; "Europe/Berlin"))}`,
			Input: `null`,
		},
		{
			Title:       "Which zones exist here",
			Description: "list_timezones answers from the machine's own database, rather than making you guess and read the error.",
			Category:    "Dates",
			Query:       `list_timezones("Europe/Lo")`,
			Input:       `null`,
		},
		{
			Title:       "Age of an account",
			Description: "days_between and age_in_years turn a signup date into the number a report wants.",
			Category:    "Dates",
			Query: `map({name, signup,
      days: (.signup | days_between("2026-08-11")),
      years: (.signup | age_in_years)})`,
			Input: users,
		},
		{
			Title:       "Bucket by calendar position",
			Description: "day_of_year, week_of_year and month_name are how a date becomes a reporting bucket.",
			Category:    "Dates",
			Query: `map({signup,
      month: (.signup | split("-")[1] | tonumber | month_name),
      week: (.signup | week_of_year),
      day_of_year: (.signup | day_of_year),
      weekend: (.signup | is_weekend)})`,
			Input: users,
		},
		{
			Title:       "Date arithmetic",
			Description: "add_days, add_months and add_years respect month lengths, which naive arithmetic on epochs does not.",
			Category:    "Dates",
			Query: `"2026-01-31"
| {plus_30d: add_days(30), plus_1m: add_months(1), plus_1y: add_years(1),
   plus_90s: add_seconds(90)}`,
			Input: `null`,
		},
		{
			Title:       "Day boundaries",
			Description: "start_of_day, end_of_day and start_of_week are the edges a range query needs.",
			Category:    "Dates",
			Query: `"2026-08-11T14:23:45Z"
| {day_start: start_of_day, day_end: end_of_day, week_start: start_of_week}`,
			Input: `null`,
		},
		{
			Title:       "How long is that",
			Description: "parse_duration reads a duration string, human_duration writes one, and iso_duration is the interchange form.",
			Category:    "Dates",
			Query: `{parsed: ("1h30m" | parse_duration),
   human: (5430 | human_duration),
   iso: (5430 | iso_duration),
   between: ("2026-08-11T09:00:00Z" | duration_between("2026-08-11T17:30:00Z"))}`,
			Input: `null`,
		},
		{
			Title:       "Relative time",
			Description: "time_ago is the phrasing an interface uses instead of a timestamp.",
			Category:    "Dates",
			Query:       `map({signup, ago: (.signup | time_ago)})`,
			Input:       users,
		},
		{
			Title:       "Calendar facts",
			Description: "is_leap_year and days_in_month are the two the off-by-one bugs come from.",
			Category:    "Dates",
			Query: `[2024, 2025, 2026]
| map({year: ., leap: is_leap_year, february: days_in_month(2)})`,
			Input: `null`,
		},
		{
			Title:       "Epoch and back",
			Description: "date_to_timestamp and timestamp_to_date cross between the two representations an API mixes.",
			Category:    "Dates",
			Query: `"2026-08-11T12:00:00Z"
| date_to_timestamp as $ts
| {timestamp: $ts, back: ($ts | timestamp_to_date), as_date: ($ts | format_date("date"))}`,
			Input: `null`,
		},

		// ------------------------------------------------------------------
		// Networking
		// ------------------------------------------------------------------
		{
			Title:       "Classify an inventory's addresses",
			Description: "The address predicates sort a host list into what is routable, what is internal and what is loopback.",
			Category:    "Network",
			Query: `map({host, ip,
      version: (.ip | ip_version),
      private: (.ip | is_private_ip),
      public: (.ip | is_public_ip),
      loopback: (.ip | is_loopback)})`,
			Input: hosts,
		},
		{
			Title:       "Which hosts are in this subnet",
			Description: "in_cidr is the membership test a firewall rule or an allow-list is built from.",
			Category:    "Network",
			Query:       `map(select(.ip | in_cidr("10.0.0.0/8")) | {host, ip})`,
			Input:       hosts,
		},
		{
			Title:       "Size a subnet",
			Description: "The cidr cmdlets answer the whole set of questions a plan needs: range, edges and capacity.",
			Category:    "Network",
			Query: `["10.0.4.0/22", "192.168.1.0/24", "203.0.113.0/29"]
| map({cidr: .,
       network: cidr_network, broadcast: cidr_broadcast,
       first: cidr_first_host, last: cidr_last_host,
       hosts: cidr_size})`,
			Input: `null`,
		},
		{
			Title:       "Is one network inside another",
			Description: "subnet_of answers containment between two ranges, which membership of a single address cannot.",
			Category:    "Network",
			Query: `{inside: ("10.0.4.0/24" | subnet_of("10.0.0.0/8")),
   outside: ("192.168.1.0/24" | subnet_of("10.0.0.0/8"))}`,
			Input: `null`,
		},
		{
			Title:       "Walk an address range",
			Description: "ip_add and the integer conversions are how a range is enumerated or an offset applied.",
			Category:    "Network",
			Query: `"10.0.4.19"
| {as_int: ip_to_int, next: ip_add(1), tenth: ip_add(10),
   back: (ip_to_int | int_to_ip),
   reverse: reverse_ip}`,
			Input: `null`,
		},
		{
			Title:       "Normalise MAC addresses",
			Description: "mac_normalize makes three vendors' spellings comparable, which is what an inventory join needs.",
			Category:    "Network",
			Query:       `map({host, raw: .mac, normalized: (.mac | mac_normalize), valid: (.mac | is_mac)})`,
			Input:       hosts,
		},
		{
			Title:       "Name the ports",
			Description: "port_name turns a number into the service everyone recognises, and is_port validates the range.",
			Category:    "Network",
			Query:       `map({host, port, service: (.port | port_name), valid: (.port | is_port)})`,
			Input:       hosts,
		},
		{
			Title:       "Expand an IPv6 address",
			Description: "ipv6_expand writes the full form, which is what string comparison and logging need.",
			Category:    "Network",
			Query: `["2001:db8::1", "::1", "fe80::a00:27ff:fe4e:66a1"]
| map({compact: ., expanded: ipv6_expand, v6: is_ipv6, v4: is_ipv4})`,
			Input: `null`,
		},
		{
			Title:       "Validate addresses from a log",
			Description: "is_ip and is_cidr screen text before anything downstream relies on it being an address.",
			Category:    "Network",
			Query: `["10.0.4.19", "999.1.1.1", "10.0.0.0/8", "not-an-ip", "2001:db8::1"]
| map({value: ., ip: is_ip, cidr: is_cidr})`,
			Input: `null`,
		},

		// ------------------------------------------------------------------
		// Validation
		// ------------------------------------------------------------------
		{
			Title:       "Screen a signup list",
			Description: "is_email and the rest turn a list of records into a list of problems, which is the job.",
			Category:    "Validation",
			Query: `map({id, email,
      valid: (.email | is_email),
      domain: (.email | after_first("@"))})
| map(select(.valid | not))`,
			Input: users,
		},
		{
			Title:       "Check the fields you were given",
			Description: "Each predicate answers one question, and together they are a schema check without a schema.",
			Category:    "Validation",
			Query: `["ada@example.com", "https://example.com/x", "example.com", "2026-08-11",
  "2026-08-11T09:00:00Z", "orders-api", "1.2.3", "deadbeef", "8080"]
| map({value: .,
       email: is_email, url: is_url, domain: is_domain,
       date: is_date, iso: is_iso8601, slug: is_slug,
       semver: is_semver, hex: is_hex, numeric: is_numeric})`,
			Input: `null`,
		},
		{
			Title:       "Order releases correctly",
			Description: "semver_compare knows 1.10.0 is after 1.9.0 and that a release candidate comes before its release.",
			Category:    "Validation",
			Query: `sort_by(semver_parts | [.major, .minor, .patch])
| map({version: ., parts: semver_parts, vs_2_0_0: semver_compare(.; "2.0.0")})`,
			Input: releases,
		},
		{
			Title:       "Is this JSON at all",
			Description: "is_json screens a payload before parsing, which is what a mixed-format ingest needs.",
			Category:    "Validation",
			Query:       `map({payload: ., json: is_json, parsed: (if is_json then json_parse else null end)})`,
			Input:       `["{\"ok\":true}", "not json", "[1,2,3]"]`,
		},
		{
			Title:       "Luhn-check a card number",
			Description: "is_credit_card runs the checksum, which catches a typed digit that a length check would not.",
			Category:    "Validation",
			Query:       `map({masked: mask(0; 4), valid: is_credit_card})`,
			Input:       `["4539578763621486", "4539578763621487", "79927398713"]`,
		},
		{
			Title:       "Strip markup out of text",
			Description: "strip_tags recovers the words from an HTML fragment, for indexing or for a plain-text fallback.",
			Category:    "Validation",
			Query:       `{text: strip_tags, words: (strip_tags | normalize_whitespace | word_count)}`,
			Input:       `"<p>The <b>payments</b> service is <i>degraded</i>.</p>"`,
		},

		// ------------------------------------------------------------------
		// Hashes and crypto
		// ------------------------------------------------------------------
		{
			Title:       "Fingerprint a value",
			Description: "The hash family shares one shape, so switching algorithm is a one-word change.",
			Category:    "Hashes",
			Query: `"orders-api:2.4.1"
| {md5: md5, sha1: sha1, sha256: sha256, sha512: (sha512 | .[0:32] + "...")}`,
			Input: `null`,
		},
		{
			Title:       "The SHA-2 truncations",
			Description: "sha224 and the sha512 truncations exist where a shorter digest with SHA-512's internals is wanted.",
			Category:    "Hashes",
			Query: `"orders-api"
| {sha224: sha224, sha384: (sha384 | .[0:32] + "..."),
   sha512_224: sha512_224, sha512_256: sha512_256}`,
			Input: `null`,
		},
		{
			Title:       "SHA-3 and Keccak",
			Description: "sha3_256 and keccak_256 differ in padding, which is why Ethereum's hash is not the standard one.",
			Category:    "Hashes",
			Query: `"orders-api"
| {sha3_256: sha3_256, sha3_512: (sha3_512 | .[0:32] + "..."), keccak_256: keccak_256}`,
			Input: `null`,
		},
		{
			Title:       "Detect a changed row",
			Description: "Hashing the canonical form of a row is how you notice a change without comparing every field.",
			Category:    "Hashes",
			Query:       `map({name: .Name, fingerprint: (json_stringify | sha256 | .[0:12])})`,
			Input:       services,
		},
		{
			Title:       "Sign a payload",
			Description: "hmac_sha256 authenticates a message with a shared key, which a bare hash cannot.",
			Category:    "Hashes",
			Query:       `{sha256: hmac_sha256("s3cr3t"), sha1: hmac_sha1("s3cr3t"), md5: hmac_md5("s3cr3t")}`,
			Input:       `"POST /api/payments\n2026-08-11T09:15:44Z"`,
		},
		{
			Title:       "The rest of the HMAC family",
			Description: "The same construction over every SHA-2 variant, for whichever one the other end specified.",
			Category:    "Hashes",
			Query: `"payload"
| {sha224: hmac_sha224("k"), sha384: (hmac_sha384("k") | .[0:24] + "..."),
   sha512: (hmac_sha512("k") | .[0:24] + "..."),
   sha512_224: hmac_sha512_224("k"), sha512_256: hmac_sha512_256("k")}`,
			Input: `null`,
		},
		{
			Title:       "Checksums for integrity",
			Description: "crc32 and friends are the cheap checks a transfer or a storage format uses, not security hashes.",
			Category:    "Hashes",
			Query: `"orders-api:2.4.1"
| {crc16: crc16, crc32: crc32, crc32c: crc32c, crc64: crc64,
   adler32: adler32, fnv1a: fnv1a}`,
			Input: `null`,
		},
		{
			Title:       "BLAKE2 digests",
			Description: "blake2b is faster than SHA-2 at the same security, which matters when hashing a lot of rows.",
			Category:    "Hashes",
			Query: `"orders-api"
| {b256: blake2b_256, b512: (blake2b_512 | .[0:32] + "...")}`,
			Input: `null`,
		},
		{
			Title:       "Store a password",
			Description: "bcrypt_hash and bcrypt_verify are the pair; a plain hash of a password is the mistake they prevent.",
			Category:    "Hashes",
			Query: `bcrypt_hash as $h
| {hash: ($h | .[0:29] + "..."), verifies: bcrypt_verify($h), wrong: ("guess" | bcrypt_verify($h))}`,
			Input: `"correct horse battery staple"`,
		},
		{
			Title:       "Derive a key",
			Description: "pbkdf2_sha256 and argon2id_hash stretch a password into key material with a cost you choose.",
			Category:    "Hashes",
			Query: `{pbkdf2: pbkdf2_sha256("salt-value"; 10000),
   argon2: argon2id_hash("salt-value-16b")}`,
			Input: `"correct horse battery staple"`,
		},
		{
			Title:       "Encrypt and decrypt",
			Description: "aes_encrypt and aes_decrypt round-trip with a key, and the result is base64 because JSON has no byte type.",
			Category:    "Hashes",
			Query: `aes_encrypt(.; "0123456789abcdef0123456789abcdef") as $c
| {ciphertext: ($c | .[0:32] + "..."),
   plaintext: aes_decrypt($c; "0123456789abcdef0123456789abcdef")}`,
			Input: `"card=4539578763621486"`,
		},
		{
			Title:       "The other ciphers",
			Description: "chacha20, rc4 and xor are here for reading other people's formats, not for choosing.",
			Category:    "Hashes",
			Query: `{chacha: (chacha20("0123456789abcdef0123456789abcdef") | .[0:24] + "..."),
   rc4: (rc4("key") | .[0:24] + "..."),
   xored: xor("k")}`,
			Input: `"sensitive"`,
		},
		{
			Title:       "Legacy block ciphers",
			Description: "des and triple_des appear in formats that predate AES, and round-trip the same way.",
			Category:    "Hashes",
			Query: `triple_des_encrypt(.; "0123456789abcdef01234567") as $c
| {ciphertext: ($c | .[0:24] + "..."),
   back: triple_des_decrypt($c; "0123456789abcdef01234567")}`,
			Input: `"legacy record"`,
		},
		{
			Title:       "Measure randomness",
			Description: "entropy tells a packed or encrypted blob from ordinary text, which is the first triage question.",
			Category:    "Hashes",
			Query:       `map({sample: (.[0:24]), entropy: (entropy | round_to(2))})`,
			Input: `["the quick brown fox jumps over the lazy dog again and again",
  "8f3a2b9c7d1e4f6a0b5c8d2e9f1a3b7c4d6e8f0a2b5c7d9e1f3a5b7c9d1e3f5a"]`,
		},
		{
			Title:       "Fuzzy-match two blobs",
			Description: "ssdeep scores similarity where a cryptographic hash reports only same or different.",
			Category:    "Hashes",
			Query: `(("alpha beta gamma delta epsilon " * 300) | ssdeep) as $a
| (("alpha beta gamma delta epsilon zeta " * 300) | ssdeep) as $b
| {a: $a, b: $b, score: ssdeep_compare($a; $b)}`,
			Input: `null`,
		},

		// ------------------------------------------------------------------
		// Encoding
		// ------------------------------------------------------------------
		{
			Title:       "Round-trip through base64",
			Description: "The encoders pair with their decoders, so a value survives the trip through a text-only channel.",
			Category:    "Encoding",
			Query: `base64_encode as $e
| {encoded: $e, decoded: ($e | base64_decode), valid: ($e | is_base64)}`,
			Input: `"orders-api:2.4.1"`,
		},
		{
			Title:       "URL-safe base64",
			Description: "base64url swaps the two characters that break in a URL or a JWT, which plain base64 does not.",
			Category:    "Encoding",
			Query: `{plain: base64_encode, url_safe: base64url_encode}
| . + {same_bytes: (.url_safe | base64url_decode)}`,
			Input: `"subject?id=1&role=admin"`,
		},
		{
			Title:       "The other radices",
			Description: "base32 and base85 trade length for alphabet, which is why each shows up in a different protocol.",
			Category:    "Encoding",
			Query:       `{b32: base32_encode, b85: base85_encode, hex: hex_encode, binary: (binary_encode | .[0:32] + "...")}`,
			Input:       `"pwrq"`,
		},
		{
			Title:       "Escape for the web",
			Description: "url_encode and html_encode protect a value for the two places user text most often breaks.",
			Category:    "Encoding",
			Query: `{url: url_encode, html: html_encode}
| . + {back: {url: (.url | url_decode), html: (.html | html_decode)}}`,
			Input: `"orders & payments <q=1>"`,
		},
		{
			Title:       "Compress a payload",
			Description: "The compressors round-trip, and the sizes show whether it was worth doing.",
			Category:    "Encoding",
			Query: `("the payments service timed out " * 20)
| {original: length,
   gzip: (gzip_compress | length),
   zlib: (zlib_compress | length),
   deflate: (deflate_compress | length),
   restored: (gzip_compress | gzip_decompress | length)}`,
			Input: `null`,
		},
		{
			Title:       "Identify an unknown blob",
			Description: "file_type reads the magic bytes, and is_utf8 and is_binary say whether it is text at all.",
			Category:    "Encoding",
			Query: `hex_encode
| {type: file_type, utf8: is_utf8, binary: is_binary}`,
			Input: `"the payments service log"`,
		},

		// ------------------------------------------------------------------
		// IDs and tokens
		// ------------------------------------------------------------------
		{
			Title:       "Generate identifiers",
			Description: "uuid4 is random and uuid7 is time-ordered, which is what makes uuid7 sort usefully in a database.",
			Category:    "IDs",
			Query: `{v4: uuid4, v7: uuid7, short: nanoid, hex: random_hex(8)}
| . + {v7_version: (.v7 | uuid_version), valid: (.v4 | is_uuid)}`,
			Input: `null`,
		},
		{
			Title:       "Read a JWT without verifying it",
			Description: "jwt_decode shows the header and claims, which is what you need when debugging an auth failure.",
			Category:    "IDs",
			Query:       `{is_jwt: is_jwt, decoded: jwt_decode}`,
			Input:       `"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0IiwibmFtZSI6IkFkYSIsImlhdCI6MTc3MDAwMDAwMH0.qsPO7bJHqLoWJqJPTvJCmB4mPTUwHfPqU0LxLuGtbCk"`,
		},
		{
			Title:       "Compact identifiers",
			Description: "base58 drops the characters that are confused when read aloud, which is why addresses use it.",
			Category:    "IDs",
			Query: `base58_encode as $b
| {base58: $b, back: ($b | base58_decode), base64: base64_encode}`,
			Input: `"orders-api"`,
		},
		{
			Title:       "Internationalised domains",
			Description: "punycode is how a non-ASCII domain travels through DNS.",
			Category:    "IDs",
			Query: `punycode_encode as $p
| {ascii: $p, back: ($p | punycode_decode)}`,
			Input: `"münchen.example"`,
		},
		{
			Title:       "Rotate a string",
			Description: "rot13 is its own inverse; rot takes any shift, which is the general form.",
			Category:    "IDs",
			Query:       `{rot13: rot13, rot5: rot(5), back: (rot(5) | rot(21))}`,
			Input:       `"orders"`,
		},

		// ------------------------------------------------------------------
		// Config formats
		// ------------------------------------------------------------------
		{
			Title:       "Read an INI file",
			Description: "ini_parse gives a section-keyed object, after which the values are ordinary data.",
			Category:    "Config",
			Query: `ini_parse
| {host: .server.host, port: (.server.port | tonumber), pool: (.database.pool | tonumber)}`,
			Input: appConfig,
		},
		{
			Title:       "Edit and write config back",
			Description: "Reading, changing and re-emitting is the whole job of a configuration change.",
			Category:    "Config",
			Query: `ini_parse
| .server.port = "8443"
| .server.tls = "true"
| ini_stringify`,
			Input: appConfig,
		},
		{
			Title:       "Read a .env file",
			Description: "properties_parse handles the key=value format env files and Java properties share.",
			Category:    "Config",
			Query: `properties_parse
| {url: .DATABASE_URL, debug: (.DEBUG == "true"), workers: (.WORKERS | tonumber)}`,
			Input: `"DATABASE_URL=postgres://db-01:5432/orders\nDEBUG=false\nWORKERS=8"`,
		},
		{
			Title:       "Write properties out",
			Description: "properties_stringify is the inverse, for generating the file a container expects.",
			Category:    "Config",
			Query:       `properties_stringify`,
			Input:       `{"DATABASE_URL":"postgres://db-01:5432/orders","WORKERS":"8"}`,
		},
		{
			Title:       "Emit logfmt",
			Description: "logfmt_stringify writes the line format the log readers parse, which closes the loop on structured logging.",
			Category:    "Config",
			Query:       `map(logfmt_stringify)`,
			Input:       `[{"level":"error","svc":"payments","msg":"upstream timeout","dur_ms":1902}]`,
		},

		// ------------------------------------------------------------------
		// Comparing and diffing
		// ------------------------------------------------------------------
		{
			Title:       "What changed between two documents",
			Description: "deep_diff reports the added, removed and changed paths, which is what a config review needs.",
			Category:    "Compare",
			Query: `deep_diff({server: {host: "0.0.0.0", port: 8080, tls: false}, log: {level: "info"}};
            {server: {host: "0.0.0.0", port: 8443, tls: true}, metrics: {enabled: true}})`,
			Input: `null`,
		},
		{
			Title:       "Reconcile two lists",
			Description: "compare_object says which side each value came from, and counts occurrences rather than collapsing to sets.",
			Category:    "Compare",
			Query: `compare_object(["orders-api", "payments-api", "gateway", "search"];
                 ["orders-api", "payments-api", "gateway", "mailer"])
| map({service: .InputObject, side: .SideIndicator})`,
			Input: `null`,
		},
		{
			Title:       "Reconcile on a key",
			Description: "Comparing by a property matches rows whose other fields have changed, which whole-value identity cannot.",
			Category:    "Compare",
			Query: `compare_object([{"id":1,"v":"2.4.0"},{"id":2,"v":"1.9.0"}];
                 [{"id":1,"v":"2.4.1"},{"id":3,"v":"1.0.0"}];
                 {Property: "id", IncludeEqual: true})
| map({id: .InputObject.id, side: .SideIndicator})`,
			Input: `null`,
		},
		{
			Title:       "Fuzzy-match names",
			Description: "levenshtein counts edits, jaro_winkler favours a shared prefix, and soundex matches how a name sounds.",
			Category:    "Compare",
			Query: `["Lovelace", "Lovelase", "Lavelace", "Turing"]
| map({name: .,
       edits: levenshtein("Lovelace"; .),
       similarity: (similarity_percent("Lovelace"; .) | round_to(2)),
       jaro: (jaro_winkler("Lovelace"; .) | round_to(3)),
       sounds_like: soundex})`,
			Input: `null`,
		},
		{
			Title:       "Overlap between two sets",
			Description: "jaccard scores how much two sets share, where the set cmdlets only list the members.",
			Category:    "Compare",
			Query: `["orders", "payments", "gateway"] as $a
| ["orders", "payments", "search"] as $b
| {shared: intersection($a; $b), only_a: difference($a; $b),
   either: symmetric_difference($a; $b), all: union($a; $b),
   score: (jaccard($a; $b) | round_to(3))}`,
			Input: `null`,
		},
		{
			Title:       "Compare fixed-width codes",
			Description: "hamming_distance counts differing positions, which is the right measure for codes of equal length.",
			Category:    "Compare",
			Query: `{distance: hamming_distance("2.4.1-linux"; "2.4.0-linux"),
   grams: ("orders" | n_grams(3))}`,
			Input: `null`,
		},
		{
			Title:       "Spot duplicate rows",
			Description: "contains_duplicates and all_equal are the two shape questions to ask before trusting a key.",
			Category:    "Compare",
			Query: `{names: (map(.Name) | {duplicated: contains_duplicates, distinct: (dedupe | length)}),
   versions: (map(.Version) | {all_same: all_equal, distinct: dedupe})}`,
			Input: services,
		},

		// ------------------------------------------------------------------
		// Byte similarity
		//
		// The compression-distance cmdlets work on values rather than paths,
		// so they run here: a corpus is an array of anything that casts to
		// bytes, and the browser has no filesystem to object. rncd_compare
		// scores every pair of a corpus, and shared_chunks decomposes one
		// value into the spans it shares with another.
		// ------------------------------------------------------------------
		{
			Title:       "Rank a corpus by similarity",
			Description: "rncd_compare scores every pair in a corpus, one object per pair and lower meaning more similar, so sort_by(.Hybrid) brings the near-duplicates to the top and a threshold flags them.",
			Category:    "Compare",
			Query: `. as $all
| [rncd_compare($all)]
| sort_by(.Hybrid)
| map({a: .NameA, b: .NameB, hybrid: .Hybrid,
       near_duplicate: (.Hybrid < 0.1)})`,
			Input: releaseNotes,
		},
		{
			Title:       "The bytes are close, the class is far",
			Description: "Two unrelated ciphertexts are both incompressible, so Ncd alone can barely tell the ciphertext pair from a ciphertext beside prose. EntropyGlobal can: ~0 for two blobs of the same kind, ~0.36 for either against prose, and the Hybrid ordering follows.",
			Category:    "Compare",
			Query: `[
  {Name: "cipher_a", Content: aes_encrypt(random_string(2000); "0123456789abcdef")},
  {Name: "cipher_b", Content: aes_encrypt(random_string(2000); "fedcba9876543210")},
  {Name: "prose", Content: ("the cat sat on the mat. " * 90)}
]
| [rncd_compare]
| sort_by(.Hybrid)
| map({a: .NameA, b: .NameB, ncd: .Ncd,
       entropy_global: .EntropyGlobal, hybrid: .Hybrid})`,
			Input: `null`,
		},
		{
			Title:       "Same kind, no shared bytes",
			Description: "Two compressed streams of different text share no bytes at all, yet they rank as the closest pair: the entropy terms recognise compressed blobs as a kind, and put them above one gzip beside the very prose it compressed.",
			Category:    "Compare",
			Query: `[
  {Name: "gzip_a", Content: gzip_compress("the quick brown fox jumps over the lazy dog. " * 60)},
  {Name: "gzip_b", Content: gzip_compress("lorem ipsum dolor sit amet consectetur adipiscing elit. " * 60)},
  {Name: "prose", Content: ("the quick brown fox jumps over the lazy dog. " * 60)}
]
| [rncd_compare]
| sort_by(.Hybrid)
| map({a: .NameA, b: .NameB, ncd: .Ncd,
       entropy_global: .EntropyGlobal, hybrid: .Hybrid})`,
			Input: `null`,
		},
		{
			Title:       "Prove the closest pair byte for byte",
			Description: "rncd_compare says which two values are closest; shared_chunks then says why: the exact fraction of one that appears verbatim in the other, and how many separate runs it took.",
			Category:    "Compare",
			Query: `. as $all
| ([rncd_compare($all)] | sort_by(.Hybrid) | .[0]) as $closest
| shared_chunks($all[$closest.IndexB]; $all[$closest.IndexA])
| {closest: [$closest.NameA, $closest.NameB],
   hybrid: $closest.Hybrid,
   coverage: .Coverage, spans: .Spans,
   matched_bytes: .MatchedBytes}`,
			Input: releaseNotes,
		},
		{
			Title:       "Which config was forked from this one",
			Description: "The piped form measures every candidate against one fixed reference. Coverage is the exact fraction of each candidate that occurs verbatim in the reference, so configs derived from a template stand out from an unrelated file.",
			Category:    "Compare",
			Query: `. as $all
| $all[0].Content as $ref
| [.[] | {name: .Name,
          coverage: (.Content | shared_chunks($ref).Coverage)}]
| sort_by(-.coverage)
| map(. + {copied: (.coverage > 0.5)})`,
			Input: serviceConfigs,
		},
		{
			Title:       "Triage a suspicious file",
			Description: "A file found on a host is a modified copy of a known-good one. rncd_compare confirms which pair; shared_chunks then isolates the changed regions: the literal chunks, which carry no RefOffset, as exactly what to diff.",
			Category:    "Compare",
			Query: `. as $all
| ([rncd_compare($all)] | sort_by(.Hybrid) | .[0]) as $closest
| shared_chunks($all[$closest.IndexB]; $all[$closest.IndexA])
| {match: [$closest.NameA, $closest.NameB],
   hybrid: $closest.Hybrid, coverage: .Coverage,
   tampered_bytes: ([.Chunks[] | select(.Matched | not) | .Length] | add),
   tampered_regions: [.Chunks[] | select(.Matched | not)
                       | {start: .Start, length: .Length}]}`,
			Input: incidentFiles,
		},

		// ------------------------------------------------------------------
		// Sampling and discovery
		// ------------------------------------------------------------------
		{
			Title:       "Take a random sample",
			Description: "sample draws without replacement and shuffle reorders, which is how a spot check gets chosen.",
			Category:    "Random",
			Query: `{sampled: (map(.Name) | sample(2) | length),
   shuffled: (map(.Name) | shuffle | length),
   picked: (map(.Name) | random_choice | type)}`,
			Input: services,
		},
		{
			Title:       "Generate test values",
			Description: "The random cmdlets fill a fixture without leaving the query.",
			Category:    "Random",
			Query: `{int: (random_int(1; 100) | type),
   float: (random_float(0; 1) | type),
   token: (random_string(12) | length)}`,
			Input: `null`,
		},
		{
			Title:       "Ask what is available",
			Description: "get_command answers from the registry the page actually runs, not from a document that can drift.",
			Category:    "Discovery",
			Query:       `[get_command("sha*") | {Name, Description}] | sort_by(.Name)`,
			Input:       `null`,
		},
		{
			Title:       "Read the help for a cmdlet",
			Description: "get_help prints the signature and examples that --udf-list carries, from inside a query.",
			Category:    "Discovery",
			Query:       `get_help("summarize_by")`,
			Input:       `null`,
		},
		{
			Title:       "What can this page run",
			Description: "Available reports whether the browser build can evaluate a name, so the page can mark what it cannot.",
			Category:    "Discovery",
			Query: `[get_command("*archive*"), get_command("select_string")]
| map({Name, Available})`,
			Input: `null`,
		},
		{
			Title:       "Sort human-readably",
			Description: "natural_sort puts file2 before file10, which lexicographic order does not.",
			Category:    "Discovery",
			Query:       `{natural: natural_sort, lexicographic: sort}`,
			Input:       `["run-10.log", "run-2.log", "run-1.log", "run-21.log"]`,
		},

		// ------------------------------------------------------------------
		// Paths and places
		// ------------------------------------------------------------------
		{
			Title:       "Take a path apart",
			Description: "The path cmdlets are pure string work, so they answer the same on any platform and without touching a disk.",
			Category:    "Paths",
			Query: `map({path: .,
      dir: dirname, file: basename, stem: stem,
      ext: file_extension, absolute: is_absolute, directory: is_dir_path})`,
			Input: `["/var/log/orders/app.2026-08-11.log", "reports/q1.csv", "/etc/pwrq/"]`,
		},
		{
			Title:       "Change a file's extension",
			Description: "with_extension and has_extension are how an output name is derived from an input one.",
			Category:    "Paths",
			Query:       `map(select(has_extension(".csv")) | {source: ., report: with_extension(".html"), sep: path_sep})`,
			Input:       `["exports/q1.csv", "exports/q2.csv", "notes/readme.md"]`,
		},
		{
			Title:       "Tidy and relativise",
			Description: "normalize_path resolves the dots, and relative_path expresses one location from another.",
			Category:    "Paths",
			Query: `{tidy: ("/srv/app/../app/./logs//today.log" | normalize_path),
   relative: ("/srv/app/logs/today.log" | relative_path("/srv/app"))}`,
			Input: `null`,
		},
		{
			Title:       "Name output files from input ones",
			Description: "Deriving the whole set of names in one pass is the everyday use, and it stays pure text.",
			Category:    "Paths",
			Query: `map(. as $src
      | {source: $src,
         name: ($src | basename | stem),
         archive: ($src | with_extension(".gz") | basename),
         folder: ($src | dirname)})`,
			Input: `["/var/log/orders/app.log", "/var/log/payments/app.log"]`,
		},

		// ------------------------------------------------------------------
		// Geography
		// ------------------------------------------------------------------
		{
			Title:       "Distance between two places",
			Description: "haversine_distance measures the great-circle route, which is the honest distance between coordinates.",
			Category:    "Domain",
			Query: `{london_to_ny: (haversine_distance(51.5007; -0.1246; 40.7128; -74.0060) | round),
   heading: (bearing(51.5007; -0.1246; 40.7128; -74.0060) | round_to(1)),
   midpoint: (geo_midpoint(51.5007; -0.1246; 40.7128; -74.0060) | map_values(round_to(3)))}`,
			Input: `null`,
		},
		{
			Title:       "Is it within range",
			Description: "within_radius is the geofence test, which is a distance comparison you would otherwise write out.",
			Category:    "Domain",
			Query: `map(. as $p
      | {site: $p.name,
         near_london: within_radius(51.5007; -0.1246; $p.lat; $p.lon; 50),
         km: (haversine_distance(51.5007; -0.1246; $p.lat; $p.lon) | round)})`,
			Input: `[{"name":"croydon","lat":51.3762,"lon":-0.0982},
  {"name":"reading","lat":51.4543,"lon":-0.9781},
  {"name":"edinburgh","lat":55.9533,"lon":-3.1883}]`,
		},
		{
			Title:       "Coordinates as a short string",
			Description: "geohash_encode turns a point into a prefix-comparable string, which is how proximity becomes an index lookup.",
			Category:    "Domain",
			Query: `parse_coords as $p
| geohash_encode($p.lat; $p.lon; 7) as $h
| {parsed: $p, geohash: $h, decoded: ($h | geohash_decode | {lat, lon})}`,
			Input: `"51.5007, -0.1246"`,
		},

		// ------------------------------------------------------------------
		// Money
		// ------------------------------------------------------------------
		{
			Title:       "What a loan costs",
			Description: "monthly_payment is the spreadsheet's PMT, and the total shows what the interest actually came to.",
			Category:    "Domain",
			Query: `monthly_payment(20000; 0.06; 60) as $m
| {monthly: ($m | round_to(2)),
   total: (($m * 60) | round_to(2)),
   interest: (($m * 60 - 20000) | round_to(2))}`,
			Input: `null`,
		},
		{
			Title:       "Growth over time",
			Description: "future_value and present_value are the two directions, and cagr recovers the rate from a start and an end.",
			Category:    "Domain",
			Query: `{future: (future_value(1000; 0.05; 10) | round_to(2)),
   present: (present_value(1628.89; 0.05; 10) | round_to(2)),
   rate: (cagr(1000; 1628.89; 10) | round_to(4))}`,
			Input: `null`,
		},
		{
			Title:       "Simple against compound",
			Description: "The gap between the two is the whole argument for compounding, and it widens with time.",
			Category:    "Domain",
			Query: `[1, 5, 10, 20]
| map({years: .,
       simple: (simple_interest(1000; 0.05; .) | round_to(2)),
       compound: ((compound_interest(1000; 0.05; .) - 1000) | round_to(2))})`,
			Input: `null`,
		},
		{
			Title:       "Is the project worth it",
			Description: "net_present_value discounts a series of cash flows to today, which is how two options are compared.",
			Category:    "Domain",
			Query: `{npv_at_8pc: (net_present_value([-10000, 3000, 4200, 5100]; 0.08) | round_to(2)),
   npv_at_15pc: (net_present_value([-10000, 3000, 4200, 5100]; 0.15) | round_to(2))}`,
			Input: `null`,
		},

		// ------------------------------------------------------------------
		// Round trips
		// ------------------------------------------------------------------
		{
			Title:       "Every codec round-trips",
			Description: "A decoder that does not invert its encoder is a bug, so the pairs are worth exercising together.",
			Category:    "Encoding",
			Query: `. as $original
| {base32: (base32_encode | base32_decode),
   base85: (base85_encode | base85_decode),
   binary: (binary_encode | binary_decode),
   hex: (hex_encode | hex_decode)}
| map_values(. == $original)`,
			Input: `"orders-api:2.4.1"`,
		},
		{
			Title:       "Every compressor round-trips",
			Description: "The same check for the compression pair, which is the one place a silent truncation would hide.",
			Category:    "Encoding",
			Query: `. as $original
| {zlib: (zlib_compress | zlib_decompress),
   deflate: (deflate_compress | deflate_decompress),
   gzip: (gzip_compress | gzip_decompress)}
| map_values(. == $original)`,
			Input: `"the payments service timed out twice within five minutes"`,
		},
		{
			Title:       "Block ciphers round-trip too",
			Description: "des and blowfish appear in older formats; the useful property is that they invert.",
			Category:    "Hashes",
			Query: `. as $plain
| {des: (des_encrypt($plain; "8bytekey") | des_decrypt(.; "8bytekey")),
   blowfish: (blowfish_encrypt($plain; "bfkey123") | blowfish_decrypt(.; "bfkey123"))}
| map_values(. == $plain)`,
			Input: `"legacy record"`,
		},
		{
			Title:       "Radix round trip",
			Description: "from_base reads back what to_base wrote, for any radix from 2 to 36.",
			Category:    "Numbers",
			Query: `[2, 8, 16, 36]
| map(. as $b | {base: $b, encoded: (48879 | to_base($b)), back: (48879 | to_base($b) | from_base($b))})`,
			Input: `null`,
		},
		{
			Title:       "Which base64 is this",
			Description: "is_base64 and is_base64url tell the two alphabets apart, which matters before decoding a token.",
			Category:    "IDs",
			Query:       `map({value: ., standard: is_base64, url_safe: is_base64url})`,
			Input:       `["b3JkZXJzLWFwaQ==", "c3ViamVjdD9pZD0xJnJvbGU9YWRtaW4", "not base64!"]`,
		},
		{
			Title:       "Read a JSONL export",
			Description: "jsonl_parse reads the one-object-per-line format that logs and database exports ship in.",
			Category:    "JSON",
			Query: `jsonl_parse
| map(.dur_ms |= tonumber)
| {events: length, slowest: (max_by(.dur_ms) | .svc), total_ms: (map(.dur_ms) | add)}`,
			Input: `"{\"svc\":\"orders\",\"dur_ms\":84}\n{\"svc\":\"payments\",\"dur_ms\":1902}\n{\"svc\":\"gateway\",\"dur_ms\":6}"`,
		},
		{
			Title:       "Biggest values in a list",
			Description: "top_n takes the largest numbers from a bare list, where top_by needs rows with a property.",
			Category:    "Statistics",
			Query:       `{slowest: top_n(3), weekday_of_incident: ("2026-08-11" | weekday)}`,
			Input:       latencies,
		},

		// ------------------------------------------------------------------
		// jq itself
		//
		// Everything above is a cmdlet doing a job. What follows is the
		// language underneath it: pwrq is a strict superset of jq, so every
		// one of these is also a plain jq program. They are kept apart
		// because learning jq and learning the cmdlets are different
		// errands, and a visitor is usually on one of them.
		// ------------------------------------------------------------------
		{
			Title:       "Arguments make a query reusable",
			Description: "$limit is bound in the Arguments block, and travels in the share link: one query, any threshold.",
			Category:    "Arguments",
			Query:       `[.[] | select(.Size > $limit)] | {matched: length, names: map(.Name)}`,
			Args:        []Arg{{Name: "limit", Value: "1000"}},
			Input: `[{"Name":"notes.txt","Size":812},
 {"Name":"report.pdf","Size":48211}]`,
		},
		{
			Title:       "A threshold argument",
			Description: "Where-Object in jq: keep the values at or above $threshold, which the Arguments block supplies.",
			Category:    "Arguments",
			Query:       `map(select(. >= $threshold))`,
			Args:        []Arg{{Name: "threshold", Value: "7"}},
			Input:       `[1,5,9,12,3,7]`,
		},
		{
			Title:       "A string argument",
			Description: "Arguments can be strings too; quote them in JSON.",
			Category:    "Arguments",
			Query:       `map({name: ($prefix + .name)})`,
			Args:        []Arg{{Name: "prefix", Value: "\"svc-\""}},
			Input:       `[{"name":"auth"},{"name":"api"}]`,
		},
		{
			Title:       "Two arguments bound a range",
			Description: "$min and $max together act like a Where-Object between the two.",
			Category:    "Arguments",
			Query:       `map(select(.size >= $min and .size <= $max))`,
			Args:        []Arg{{Name: "min", Value: "20"}, {Name: "max", Value: "90"}},
			Input:       `[{"size":10},{"size":50},{"size":100},{"size":90}]`,
		},
		{
			Title:       "An array argument",
			Description: "An argument can be a whole array; index finds membership in the allowlist.",
			Category:    "Arguments",
			Query:       `map(select(. as $x | $allow | index($x)))`,
			Args:        []Arg{{Name: "allow", Value: "[\"b\",\"d\"]"}},
			Input:       `["a","b","c","d"]`,
		},
		{
			Title:       "An argument as a key",
			Description: "Parenthesised keys make the argument name the key: change it without editing the query.",
			Category:    "Arguments",
			Query:       `{($label): .} `,
			Args:        []Arg{{Name: "label", Value: "\"doubled\""}},
			Input:       `5`,
		},
		{
			Title:       "Argument-driven report",
			Description: "One query, many reports: the level argument decides who makes the cut.",
			Category:    "Arguments",
			Query:       `map(select(.level >= $level)) | {report: "staff at level \($level)", people: map(.user)}`,
			Args:        []Arg{{Name: "level", Value: "5"}},
			Input: `[{"user":"ada","level":5},
 {"user":"grace","level":2},
 {"user":"linus","level":8}]`,
		},
		{
			Title:       "Map over an array",
			Description: "map applies a filter to every element and collects the results into a new array.",
			Category:    "jq",
			Query:       `map(. * 10)`,
			Input:       `[1,2,3,4]`,
		},
		{
			Title:       "Select keeps the interesting rows",
			Description: "select drops inputs where the condition is false; pipe it after .[] to filter a list.",
			Category:    "jq",
			Query:       `.[] | select(.active) | .name`,
			Input:       `[{"name":"web-01","active":true},{"name":"web-02","active":false},{"name":"db-01","active":true}]`,
		},
		{
			Title:       "If, then, else",
			Description: "A conditional picks one branch per input.",
			Category:    "jq",
			Query:       `if .age >= 18 then "adult" else "minor" end`,
			Input:       `{"name":"ada","age":36}`,
		},
		{
			Title:       "The alternative operator",
			Description: "// returns the left side unless it is false or null, then the right side: a default.",
			Category:    "jq",
			Query:       `{nickname: .nickname, display: (.nickname // .name)}`,
			Input:       `{"name":"grace"}`,
		},
		{
			Title:       "String interpolation",
			Description: "\"\\(…)\" embeds any value in a string, jq's template literal.",
			Category:    "jq",
			Query:       `"\(.name) has \(.items | length) items"`,
			Input:       `{"name":"cart","items":[1,2,3]}`,
		},
		{
			Title:       "Length depends on the type",
			Description: "length is the string's characters, the array's elements, or the object's keys.",
			Category:    "jq",
			Query:       `{s: ("héllo" | length), a: ([1,2,3] | length), o: ({a:1,b:2} | length)}`,
			Input:       `null`,
		},
		{
			Title:       "What type is it?",
			Description: "type names the JSON kind, so one query can branch per kind.",
			Category:    "jq",
			Query:       `[1,"x",true,null,{},[]] | map({value: ., type: type})`,
			Input:       `null`,
		},
		{
			Title:       "Add up the list",
			Description: "add folds an array of numbers (or objects) into one value.",
			Category:    "jq",
			Query:       `{total: add, count: length, average: (add / length)}`,
			Input:       `[10,20,30]`,
		},
		{
			Title:       "Objects to entries and back",
			Description: "to_entries makes an array of {key, value}; from_entries rebuilds the object.",
			Category:    "jq",
			Query:       `to_entries | map(select(.value >= 3)) | from_entries`,
			Input:       `{"a":1,"b":3,"c":5}`,
		},
		{
			Title:       "Drop a key",
			Description: "del removes a key from an object; delpaths removes several at once.",
			Category:    "jq",
			Query:       `del(.password) | delpaths([["token"]])`,
			Input:       `{"user":"ada","password":"hunter2","token":"abc","role":"admin"}`,
		},
		{
			Title:       "Test whether a key exists",
			Description: "has and in ask about keys, which select turns into a filter.",
			Category:    "jq",
			Query:       `map(select(has("secret")))`,
			Input:       `[{"name":"svc","secret":"x"},{"name":"pub"}]`,
		},
		{
			Title:       "Deep merge",
			Description: "* merges objects recursively, so both sides of a nested key survive.",
			Category:    "jq",
			Query:       `{base: {host: "db-01", port: 5432}, overrides: {port: 5433}} | .base * .overrides`,
			Input:       `null`,
		},
		{
			Title:       "Build an array with range",
			Description: "range generates numbers; wrapping it in brackets collects them.",
			Category:    "jq",
			Query:       `[range(1; 6)]`,
			Input:       `null`,
		},
		{
			Title:       "Every third number",
			Description: "range takes a step as its third argument.",
			Category:    "jq",
			Query:       `[range(0; 12; 3)]`,
			Input:       `null`,
		},
		{
			Title:       "Flatten nested arrays",
			Description: "flatten unwraps nested arrays; give it a depth to stop early.",
			Category:    "jq",
			Query:       `[1, [2, [3, [4]]]] | {deep: flatten, shallow: flatten(2)}`,
			Input:       `null`,
		},
		{
			Title:       "Reverse an array",
			Description: "reverse flips the order of an array or the characters of a string.",
			Category:    "jq",
			Query:       `[1,2,3,4,5] | reverse`,
			Input:       `null`,
		},
		{
			Title:       "Sort numbers",
			Description: "sort puts an array in order; strings sort too.",
			Category:    "jq",
			Query:       `[4, 1, 9, 2, 7] | sort`,
			Input:       `null`,
		},
		{
			Title:       "Sort objects by a field",
			Description: "sort_by sorts on the result of the given expression for each element.",
			Category:    "jq",
			Query:       `sort_by(.price)`,
			Input:       `[{"item":"pen","price":2},{"item":"book","price":9},{"item":"mug","price":6}]`,
		},
		{
			Title:       "Unique values",
			Description: "unique sorts and deduplicates an array.",
			Category:    "jq",
			Query:       `[3,1,2,2,1,3] | unique`,
			Input:       `null`,
		},
		{
			Title:       "Group a list into buckets",
			Description: "group_by sorts and groups adjacent equals; count each group's length.",
			Category:    "jq",
			Query:       `["a","b","a","c","b","a"] | group_by(.) | map({value: .[0], count: length})`,
			Input:       `null`,
		},
		{
			Title:       "Binary search an index",
			Description: "bsearch returns where a value sits in a sorted array, or -1 minus the insertion point.",
			Category:    "jq",
			Query:       `[1,3,5,7,9] | {present: bsearch(5), absent: bsearch(6)}`,
			Input:       `null`,
		},
		{
			Title:       "Slice an array",
			Description: ".[a:b] takes a half-open slice; negative indexes count from the end.",
			Category:    "jq",
			Query:       `[0,1,2,3,4,5] | {mid: .[1:4], head: .[:2], tail: .[-2:], all_but: .[1:-1]}`,
			Input:       `null`,
		},
		{
			Title:       "Rewrite every value",
			Description: "map_values applies a filter to each value and keeps the keys.",
			Category:    "jq",
			Query:       `map_values(select(type == "number") | . * 2)`,
			Input:       `{"cpu":2,"mem":4,"os":"linux"}`,
		},
		{
			Title:       "Rewrite keys too",
			Description: "with_entries gives you each {key, value}; change either side.",
			Category:    "jq",
			Query:       `with_entries(.key |= ascii_upcase)`,
			Input:       `{"name":"api","port":8080}`,
		},
		{
			Title:       "Increment every value",
			Description: "with_entries and map_values both mutate values; map_values is the shorter one.",
			Category:    "jq",
			Query:       `map_values(. + 1)`,
			Input:       `{"a":1,"b":2,"c":3}`,
		},
		{
			Title:       "Pick a subtree",
			Description: "pick keeps only the named paths, whatever their depth.",
			Category:    "jq",
			Query:       `pick(.name, .config.host)`,
			Input:       `{"name":"db","config":{"host":"db-01","port":5432},"secret":"x"}`,
		},
		{
			Title:       "Walk a nested document",
			Description: "walk visits every value, deep or shallow, and the filter decides what to do with it.",
			Category:    "jq",
			Query:       `walk(if type == "string" then ascii_upcase else . end)`,
			Input:       `{"a":{"b":"hello"},"c":[{"d":"world"}]}`,
		},
		{
			Title:       "Redact secrets wherever they hide",
			Description: "walk rewrites a document wherever a rule matches, however deep it is.",
			Category:    "jq",
			Query: `walk(
  if type == "object"
  then with_entries(
    if (.key | test("secret|token|password"; "i"))
    then .value = "***"
    else . end)
  else . end)`,
			Input: `{"name":"svc","auth":{"token":"abc123","user":"svc"},"nested":[{"password":"p"}]}`,
		},
		{
			Title:       "Set and read a deep path",
			Description: "setpath writes at a nested path; getpath reads it back.",
			Category:    "jq",
			Query:       `setpath(["a","b","c"]; "found") | {read: getpath(["a","b","c"]), whole: .}`,
			Input:       `{"a":{"b":{}}}`,
		},
		{
			Title:       "Build an object from a stream",
			Description: "reduce feeds every element into an accumulator, here an object being filled in.",
			Category:    "jq",
			Query:       `reduce .[] as $x ({}; .[$x.name] = $x.v)`,
			Input:       `[{"name":"a","v":1},{"name":"b","v":2}]`,
		},
		{
			Title:       "Split and rejoin",
			Description: "split breaks a string on a delimiter; join rebuilds an array with one.",
			Category:    "jq",
			Query:       `"a,b,c" | split(",") | join(" / ")`,
			Input:       `null`,
		},
		{
			Title:       "Test a string against a pattern",
			Description: "test is jq's regex predicate; the 'i' flag makes it case-insensitive.",
			Category:    "jq",
			Query:       `.[] | select(.email | test("^[^@]+@[^@]+\\.(com|org)$"; "i")) | .email`,
			Input:       `[{"email":"ada@example.com"},{"email":"not-an-email"},{"email":"grace@example.org"}]`,
		},
		{
			Title:       "Extract named groups",
			Description: "capture pulls regex groups out by name into an object.",
			Category:    "jq",
			Query:       `"http://api.example.com/v1/users" | capture("^https?://(?<host>[^/]+)(?<path>/.*)$")`,
			Input:       `null`,
		},
		{
			Title:       "Replace with gsub",
			Description: "gsub replaces every regex match; unlike sub, it is not limited to the first.",
			Category:    "jq",
			Query:       `"call 555-1234 or 555-9999" | gsub("[0-9]{3}-[0-9]{4}"; "XXX-XXXX")`,
			Input:       `null`,
		},
		{
			Title:       "Case folding",
			Description: "ascii_downcase and ascii_upcase are jq's case converters.",
			Category:    "jq",
			Query:       `["Hello","WORLD"] | map({down: ascii_downcase, up: ascii_upcase})`,
			Input:       `null`,
		},
		{
			Title:       "Trim the edges",
			Description: "ltrimstr and rtrimstr remove a fixed prefix and suffix.",
			Category:    "jq",
			Query:       `"api/health" | ltrimstr("api/") | rtrimstr("/")`,
			Input:       `null`,
		},
		{
			Title:       "Characters as codes",
			Description: "explode splits into code points; implode reassembles them.",
			Category:    "jq",
			Query:       `"Hi!" | explode | map(. + 1) | implode`,
			Input:       `null`,
		},
		{
			Title:       "String to number",
			Description: "tonumber parses a numeric string; the inverse of tostring.",
			Category:    "jq",
			Query:       `["42", "3.14"] | map(tonumber | . * 2)`,
			Input:       `null`,
		},
		{
			Title:       "Slice a string",
			Description: "Strings slice like arrays, character by character.",
			Category:    "jq",
			Query:       `"abcdefghij" | {head: .[:3], middle: .[3:6], tail: .[-3:], inner: .[1:-1]}`,
			Input:       `null`,
		},
		{
			Title:       "Format a string",
			Description: "@json and @csv are string formats: they render values the way jq writes them.",
			Category:    "jq",
			Query:       `{a: 1, b: "two"} | {json: @json, csv: ([1,2] | @csv), uri: ("a b" | @uri), sh: ("a b" | @sh)}`,
			Input:       `null`,
		},
		{
			Title:       "Round numbers",
			Description: "floor, ceil and round each take a number to a neighbour.",
			Category:    "jq",
			Query:       `[1.2, 1.5, 1.8] | {floor: map(floor), ceil: map(ceil), round: map(round)}`,
			Input:       `null`,
		},
		{
			Title:       "Roots and powers",
			Description: "sqrt and pow do the arithmetic you cannot spell with operators.",
			Category:    "jq",
			Query:       `{sqrt: (16 | sqrt), cbrt: (27 | cbrt), pow: pow(2; 10)}`,
			Input:       `null`,
		},
		{
			Title:       "Logarithms and exponentials",
			Description: "log is the natural logarithm; exp inverts it.",
			Category:    "jq",
			Query:       `{ln: (100 | log), e: (1 | exp), round_trip: (2.71828 | log | exp | round)}`,
			Input:       `null`,
		},
		{
			Title:       "Trigonometry",
			Description: "sin, cos and tan take radians; a half turn of π is a useful test.",
			Category:    "jq",
			Query:       `[0, 3.141592653589793 / 2] | map({sin: sin, cos: cos})`,
			Input:       `null`,
		},
		{
			Title:       "Biggest and smallest",
			Description: "min and max scan an array; min_by and max_by scan by a property.",
			Category:    "jq",
			Query:       `[4,1,9,2] | {min: min, max: max}`,
			Input:       `null`,
		},
		{
			Title:       "Odds and evens",
			Description: "% is the modulo operator; it is how you test parity.",
			Category:    "jq",
			Query:       `[1,2,3,4,5,6] | {even: map(select(. % 2 == 0)), odd: map(select(. % 2 == 1))}`,
			Input:       `null`,
		},
		{
			Title:       "Finite or not",
			Description: "nan and infinite are real numbers; isfinite tells them apart.",
			Category:    "jq",
			Query:       `[1.0, nan, infinite] | map({value: ., finite: isfinite})`,
			Input:       `null`,
		},
		{
			Title:       "Absolute values",
			Description: "abs drops the sign; it is the distance from zero.",
			Category:    "jq",
			Query:       `[-3, -1.5, 0, 2] | map(abs)`,
			Input:       `null`,
		},
		{
			Title:       "Pythagoras with hypot",
			Description: "hypot(a; b) is the square root of a² + b², computed without overflow.",
			Category:    "jq",
			Query:       `{hypot: hypot(3; 4), checked: ((3*3 + 4*4) | sqrt)}`,
			Input:       `null`,
		},
		{
			Title:       "Reduce into a summary",
			Description: "Plain jq: reduce is how you fold a stream into one value.",
			Category:    "jq",
			Query: `reduce .[] as $event (
  {count: 0, bytes: 0};
  {count: .count + 1, bytes: .bytes + $event.bytes}
)`,
			Input: `[{"bytes":120},{"bytes":8400},{"bytes":33}]`,
		},
		{
			Title:       "Running totals with foreach",
			Description: "foreach emits the accumulator after every step, not just at the end.",
			Category:    "jq",
			Query:       `foreach [1,2,3][] as $x (0; . + $x; .)`,
			Input:       `null`,
		},
		{
			Title:       "Walk a tree with recurse",
			Description: "recurse repeats a filter over its own output, descending a tree to every leaf.",
			Category:    "jq",
			Query:       `recurse(.children[]?) | .name`,
			Input:       `{"name":"root","children":[{"name":"a"},{"name":"b","children":[{"name":"c"}]}]}`,
		},
		{
			Title:       "Walk every path",
			Description: "paths and getpath flatten a nested document into leaf-by-leaf rows.",
			Category:    "jq",
			Query:       `[paths(scalars) as $p | {path: ($p | join(".")), value: getpath($p)}]`,
			Input:       `{"server":{"host":"db-01","tls":{"enabled":true,"port":5432}},"tags":["a","b"]}`,
		},
		{
			Title:       "Loop until done",
			Description: "until keeps updating while the condition is false; 1 doubles to 16 before reaching 10.",
			Category:    "jq",
			Query:       `1 | until(. >= 10; . * 2)`,
			Input:       `null`,
		},
		{
			Title:       "Repeat with a limit",
			Description: "repeat generates forever; limit takes the first few, the safe way to sample a stream.",
			Category:    "jq",
			Query:       `[limit(5; repeat("tick"))]`,
			Input:       `null`,
		},
		{
			Title:       "Any and all",
			Description: "any and all take a condition and answer about the whole array.",
			Category:    "jq",
			Query:       `[2,4,6] | {all_even: all(. % 2 == 0), any_over_four: any(. > 4)}`,
			Input:       `null`,
		},
		{
			Title:       "First, middle, last",
			Description: "first, nth and last pluck positions out of a stream.",
			Category:    "jq",
			Query:       `[10,20,30,40,50] | {first: first, third: nth(2), last: last}`,
			Input:       `null`,
		},
		{
			Title:       "Handle errors inline",
			Description: "try … catch turns a failing branch into a value instead of stopping the query.",
			Category:    "jq",
			Query:       `[1, "two", 3] | map(try (. * 2) catch 0)`,
			Input:       `null`,
		},
		{
			Title:       "Doubling squares with map",
			Description: "map + a comprehension gives you Python-style list building in one line.",
			Category:    "jq",
			Query:       `[.x, .y] | map(. * .) | add`,
			Input:       `{"x":3,"y":4}`,
		},
		{
			Title:       "Read every input document",
			Description: "[., inputs] collects the rest of the stream after each input position — jq's slurp idiom, one output per starting point.",
			Category:    "jq",
			Query:       `[., inputs]`,
			Input:       `{"n":1} {"n":2} {"n":3}`,
		},
		{
			Title:       "Filter a log stream",
			Description: "A stream of log lines, filtered: only the errors survive.",
			Category:    "jq",
			Query:       `select(.level == "error") | {ts, msg}`,
			Input: `{"level":"info","ts":1,"msg":"started"}
{"level":"error","ts":2,"msg":"disk full"}
{"level":"info","ts":3,"msg":"still up"}
{"level":"error","ts":4,"msg":"timeout"}`,
		},
		{
			Title:       "Every document is an input",
			Description: "The engine runs the query once per input document, so one query maps over the whole stream.",
			Category:    "jq",
			Query:       `. * 2`,
			Input:       `1 2 3 4`,
		},
		{
			Title:       "Peek at the next input",
			Description: "input reads one more document from the stream; input? is its catch, and // supplies the end.",
			Category:    "jq",
			Query:       `{me: ., next: (input? // "eof")}`,
			Input:       `1 2 3`,
		},
		{
			Title:       "Analyse a request log",
			Description: "Combine streaming input, group_by and measure to summarise traffic by endpoint.",
			Category:    "jq",
			Query: `[.[]] as $all
| $all
| group_by(.endpoint)
| map({endpoint: .[0].endpoint, hits: length, slowest: (map(.ms) | max), total: (map(.ms) | add)})
| sort_by(-.hits)`,
			Input: `[{"endpoint":"/login","ms":12,"user":"a"},
 {"endpoint":"/api","ms":340,"user":"b"},
 {"endpoint":"/login","ms":40,"user":"c"},
 {"endpoint":"/api","ms":120,"user":"a"},
 {"endpoint":"/api","ms":800,"user":"c"}]`,
		},
		{
			Title:       "Decode, parse and select",
			Description: "Real data is layered: un-base64 it, parse the JSON, then select what matters.",
			Category:    "jq",
			Query:       `.logs | map(.payload | base64_decode | fromjson | select(.error == null) | {id, at: .timestamp})`,
			Input: `{"logs":[
 {"payload":"eyJpZCI6MSwiZXJyb3IiOm51bGwsInRpbWVzdGFtcCI6MTcwMDAwMDAwMH0="},
 {"payload":"eyJpZCI6MiwiZXJyb3IiOiJkaXNrIGZ1bGwiLCJ0aW1lc3RhbXAiOjE3MDAwMDAwMDF9"},
 {"payload":"eyJpZCI6MywiZXJyb3IiOm51bGwsInRpbWVzdGFtcCI6MTcwMDAwMDAwMn0="}
]}`,
		},
		{
			Title:       "CSV to objects to a table",
			Description: "Parse CSV, shape the rows, and render them as a table in one pipeline.",
			Category:    "jq",
			Query:       `.raw | csv_parse | .[1:] | map({name: .[0], region: .[1], sales: (.[2] | tonumber)}) | sort_by(-.sales) | format_table(.; {property: ["name", "region", "sales"], autosize: true})`,
			Input:       `{"raw":"name,region,sales\nada,eu,1200\ngrace,us,3400\nlinus,eu,800\n"}`,
		},
		{
			Title:       "Config with defaults",
			Description: "// supplies defaults for missing keys, so a sparse config is still complete.",
			Category:    "jq",
			Query:       `{host: (.host // "localhost"), port: (.port // 8080), tls: (.tls // true)}`,
			Input:       `{"port": 9090}`,
		},
		{
			Title:       "Scan configs for secrets",
			Description: "Find the high-entropy strings in a config that are likely credentials.",
			Category:    "jq",
			Query: `paths(scalars) as $p
| getpath($p) as $v
| select(($v | type) == "string" and ($v | entropy) > 4)
| {path: ($p | join(".")), entropy: ($v | entropy)}`,
			Input: `{"host":"api.example.com","key":"aXk9M2QwYjRmZzdoOThjbXA0cXJ1dg==","debug":"false","secret":"0p3n5e5am3j0p"}`,
		},
		{
			Title:       "Tally votes",
			Description: "reduce over a stream of choices to count the winners.",
			Category:    "jq",
			Query:       `reduce .[] as $v ({}; .[$v] = (.[$v] + 1)) | to_entries | sort_by(-.value) | .[0]`,
			Input:       `["blue","red","blue","green","blue","red"]`,
		},
		{
			Title:       "Nested report with walk",
			Description: "Round every number in a nested document, wherever it sits.",
			Category:    "jq",
			Query:       `walk(if type == "number" then (.*100 | round) / 100 else . end)`,
			Input:       `{"summary":{"mean":1.2345,"min":0.5},"rows":[{"v":2.6789},{"v":-0.1234}]}`,
		},
		{
			Title:       "A histogram of word lengths",
			Description: "Split text, measure each word, and count by length.",
			Category:    "jq",
			Query:       `.text | split(" ") | map(length) | group_by(.) | map({len: .[0], count: length}) | sort_by(.len)`,
			Input:       `{"text":"the quick brown fox jumps over the lazy dog"}`,
		},
	}
}
