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

// Examples are the page's gallery.
//
// They are defined here rather than in the JavaScript because here they can be
// tested: TestExamplesAllRun evaluates every one of them against the same
// registry the page uses, so a gallery entry cannot rot into a query that no
// longer works. Each one has to run in a browser tab, which rules out anything
// touching the filesystem, a process table or the network.
func Examples() []Example {
	return []Example{
		{
			Title:       "Sort and shape objects",
			Description: "The PowerShell pipeline, in jq: filter, sort, then pick the properties you want.",
			Category:    "Objects",
			Query: `[.[]
  | select_object(.; {script: ".Size > 1000"})]
| sort_object(.; {property: "Size", descending: true})
| .[]
| {Name, Size}`,
			Input: `[{"Name":"notes.txt","Size":812},
 {"Name":"report.pdf","Size":48211},
 {"Name":"image.png","Size":10240}]`,
		},
		{
			Title:       "Format a table",
			Description: "format_table renders objects the way Format-Table does. Switch the output to Raw to read it.",
			Category:    "Objects",
			Query:       `format_table(.)`,
			Input: `[{"Name":"web-01","Status":"Running","CPU":12.4},
 {"Name":"web-02","Status":"Stopped","CPU":0},
 {"Name":"db-01","Status":"Running","CPU":63.8}]`,
		},
		{
			Title:       "Group and measure",
			Description: "group_object buckets objects by a property, and reports each bucket with its members.",
			Category:    "Objects",
			Query:       `group_object(.; {property: "Status"}) | .[] | {Status: .Name, Count: .Count}`,
			Input: `[{"Name":"web-01","Status":"Running"},
 {"Name":"web-02","Status":"Stopped"},
 {"Name":"db-01","Status":"Running"}]`,
		},
		{
			Title:       "Hash every value",
			Description: "Cmdlets are ordinary jq functions, so they compose with map, select and everything else.",
			Category:    "Hashes",
			Query:       `to_entries | map({key, sha256: (.value | sha256)}) | from_entries`,
			Input:       `{"alice":"correct horse","bob":"battery staple"}`,
		},
		{
			Title:       "Decode a base64 payload",
			Description: "Chain codecs the way you would in a shell, but over structured data.",
			Category:    "Encoding",
			Query:       `.payload | base64_decode | fromjson | .user`,
			Input:       `{"payload":"eyJ1c2VyIjoiYWRtaW4iLCJyb2xlIjoicm9vdCJ9"}`,
		},
		{
			Title:       "Spot high-entropy strings",
			Description: "Shannon entropy over each value: the usual first pass for finding secrets in a config.",
			Category:    "Analysis",
			Query: `to_entries
| map(select(.value | type == "string") | {key, entropy: (.value | entropy)})
| map(select(.entropy > 3.5))`,
			Input: `{"host":"api.example.com",
 "port":"8443",
 "token":"hunter2xK9pLmQ7vB3nR8sT2wY6zA4",
 "debug":"false"}`,
		},
		{
			Title:       "CSV to JSON",
			Description: "csv_parse turns a CSV document into rows you can query.",
			Category:    "Conversion",
			Query:       `.raw | csv_parse | .[1:] | map({name: .[0], role: .[1]})`,
			Input:       `{"raw":"name,role\nada,engineer\ngrace,admiral\n"}`,
		},
		{
			Title:       "XML to JSON",
			Description: "xml_parse gives plain JSON, so the rest of the query is ordinary jq.",
			Category:    "Conversion",
			Query:       `.doc | xml_parse`,
			Input:       `{"doc":"<config><host>db-01</host><port>5432</port></config>"}`,
		},
		{
			Title:       "Round-trip through gzip",
			Description: "Compression cmdlets work on strings, and compose both ways.",
			Category:    "Encoding",
			Query:       `{original: length, compressed: (gzip_compress | length), same: (gzip_compress | gzip_decompress) == .}`,
			Input:       `"the quick brown fox jumps over the lazy dog, again and again and again"`,
		},
		{
			Title:       "Encrypt and decrypt",
			Description: "A cipher round-trip, entirely inside the tab: nothing here is sent anywhere.",
			Category:    "Ciphers",
			Query:       `aes_encrypt(.; "hunter2hunter2hu") | {ciphertext: ., plaintext: aes_decrypt(.; "hunter2hunter2hu")}`,
			Input:       `"attack at dawn"`,
		},
		{
			Title:       "Timestamps to dates",
			Description: "Unix time in, ISO out.",
			Category:    "Conversion",
			Query:       `map({raw: ., when: timestamp_to_date})`,
			Input:       `[0, 1000000000, 1700000000]`,
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
			Title:       "Walk every path",
			Description: "paths and getpath flatten a nested document into leaf-by-leaf rows.",
			Category:    "jq",
			Query:       `[paths(scalars) as $p | {path: ($p | join(".")), value: getpath($p)}]`,
			Input:       `{"server":{"host":"db-01","tls":{"enabled":true,"port":5432}},"tags":["a","b"]}`,
		},
		{
			Title:       "Redact secrets in place",
			Description: "walk rewrites a document wherever a rule matches, however deep it is.",
			Category:    "jq",
			Query: `walk(
  if type == "object"
  then with_entries(
    if (.key | test("secret|token|password"; "i"))
    then .value = "***"
    else .
    end)
  else . end)`,
			Input: `{"name":"svc","auth":{"token":"abc123","user":"svc"},"nested":[{"password":"p"}]}`,
		},
		{
			Title:       "What can I call here?",
			Description: "get_command answers from the registry the page actually runs, not from a document.",
			Category:    "Discovery",
			Query:       `[get_command("sha*") | {Name, Description}] | sort_by(.Name)`,
			Input:       ``,
		},
		{
			Title:       "Arguments make a query reusable",
			Description: "$limit is bound in the Arguments block, and travels in the share link: one query, any threshold.",
			Category:    "Objects",
			Query:       `[.[] | select(.Size > $limit)] | {matched: length, names: map(.Name)}`,
			Args:        []Arg{{Name: "limit", Value: "1000"}},
			Input: `[{"Name":"notes.txt","Size":812},
 {"Name":"report.pdf","Size":48211}]`,
		},
	}
}
