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
// touching the filesystem, a process table or the network, and it has to draw,
// so TestExamplesDrawToo keeps the diagram honest as well.
//
// The gallery deliberately spans the whole vocabulary the page can run: the jq
// language itself, every codec, hash, cipher, compression scheme, format
// converter, the object and formatting cmdlets, the discovery cmdlets, and the
// page's own features (arguments, slurp-style streams, raw/compact output
// patterns). No two examples teach the same thing if it can be helped.
func Examples() []Example {
	return []Example{
		// ------------------------------------------------------------------
		// Objects
		// ------------------------------------------------------------------
		{
			Title:       "Sort and shape objects",
			Description: "The PowerShell pipeline, in jq: filter, sort, then pick the properties you want.",
			Category:    "Objects",
			Query: `where_object(.; {script: ".Size > 1000"})
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
			Query:       `format_table(.; .)`,
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
			Title:       "Take the first and last rows",
			Description: "select_object reads like Select-Object: -First, -Last, -Skip and -Property in one call.",
			Category:    "Objects",
			Query:       `{head: (select_object(.; {first: 2})), tail: (select_object(.; {last: 2}))}`,
			Input:       `[10,20,30,40,50]`,
		},
		{
			Title:       "Project only the columns you want",
			Description: "select_object with a property list keeps the named fields and drops the rest, like Select-Object.",
			Category:    "Objects",
			Query:       `select_object(.; {property: ["Name", "Role"]})`,
			Input: `[{"Name":"ada","Role":"engineer","SshKey":"secret"},
 {"Name":"grace","Role":"admiral","SshKey":"secret"}]`,
		},
		{
			Title:       "Filter with Where-Object's operators",
			Description: "where_object compares a property with -eq, -gt, -like, -match and friends without writing jq.",
			Category:    "Objects",
			Query:       `where_object(.; {property: "Age", operator: "ge", value: 18})`,
			Input: `[{"Name":"ada","Age":36},
 {"Name":"baby","Age":2},
 {"Name":"grace","Age":99}]`,
		},
		{
			Title:       "Filter with a script block",
			Description: "where_object accepts any jq expression as a script, so the filter can be as powerful as jq.",
			Category:    "Objects",
			Query:       `where_object(.; {script: ".load / .cores > 0.7"})`,
			Input: `[{"Name":"db-01","load":6.4,"cores":8},
 {"Name":"web-01","load":0.2,"cores":4},
 {"Name":"db-02","load":7.1,"cores":8}]`,
		},
		{
			Title:       "Sort a list in either direction",
			Description: "sort_object sorts on a property; descending flips it, just like Sort-Object -Descending.",
			Category:    "Objects",
			Query:       `{asc: (sort_object(.; {property: "Price"})), desc: (sort_object(.; {property: "Price", descending: true}))}`,
			Input: `[{"Item":"pen","Price":2},{"Item":"book","Price":9},{"Item":"mug","Price":6}]`,
		},
		{
			Title:       "Measure a column",
			Description: "measure_object reports count, sum, average, minimum and maximum over a property.",
			Category:    "Objects",
			Query:       `measure_object(.; {property: "Size", sum: true, average: true, minimum: true, maximum: true})`,
			Input: `[{"Name":"a","Size":120},{"Name":"b","Size":8400},{"Name":"c","Size":33}]`,
		},
		{
			Title:       "Count things",
			Description: "measure_object with no property just counts the objects, the Measure-Object default.",
			Category:    "Objects",
			Query:       `measure_object(.; .)`,
			Input:       `["a", "b", "c", "d"]`,
		},

		// ------------------------------------------------------------------
		// Formatting
		// ------------------------------------------------------------------
		{
			Title:       "Format a list",
			Description: "format_list renders each object as label/value lines, the way Format-List does.",
			Category:    "Formatting",
			Query:       `format_list(.; .)`,
			Input:       `{"Name":"web-01","Status":"Running","UptimeDays":214}`,
		},
		{
			Title:       "A table with chosen columns",
			Description: "Pass a property list and autosize: true to format_table for a tidy report.",
			Category:    "Formatting",
			Query:       `format_table(.; {property: ["Name", "Status", "CPU"], autosize: true})`,
			Input: `[{"Name":"web-01","Status":"Running","CPU":12.4,"OS":"linux"},
 {"Name":"db-01","Status":"Running","CPU":63.8,"OS":"linux"},
 {"Name":"web-02","Status":"Stopped","CPU":0,"OS":"linux"}]`,
		},
		{
			Title:       "Hash every value into a table",
			Description: "Cmdlets compose with jq: build rows, hash each one, and format the result as a table.",
			Category:    "Formatting",
			Query:       `["alpha","beta","gamma"] | map({name: ., sha: sha256}) | format_table(.; {property: ["name", "sha"]})`,
			Input:       `null`,
		},

		// ------------------------------------------------------------------
		// Hashes
		// ------------------------------------------------------------------
		{
			Title:       "Hash every value",
			Description: "Cmdlets are ordinary jq functions, so they compose with map, select and everything else.",
			Category:    "Hashes",
			Query:       `to_entries | map({key, sha256: (.value | sha256)}) | from_entries`,
			Input:       `{"alice":"correct horse","bob":"battery staple"}`,
		},
		{
			Title:       "MD5 of a string",
			Description: "md5 is the classic fingerprint; fine for checksums, not for passwords.",
			Category:    "Hashes",
			Query:       `"password" | md5`,
			Input:       `null`,
		},
		{
			Title:       "SHA-1 of a string",
			Description: "sha1 is deprecated for security but still common in deduplication and git.",
			Category:    "Hashes",
			Query:       `"hello" | sha1`,
			Input:       `null`,
		},
		{
			Title:       "SHA-256 of a string",
			Description: "The workhorse hash: sha256 returns the hex digest of its input.",
			Category:    "Hashes",
			Query:       `"the quick brown fox" | sha256`,
			Input:       `null`,
		},
		{
			Title:       "SHA-224 of a string",
			Description: "sha224 is the shorter member of the SHA-2 family.",
			Category:    "Hashes",
			Query:       `"hello" | sha224`,
			Input:       `null`,
		},
		{
			Title:       "SHA-384 of a string",
			Description: "sha384 is a SHA-2 variant with a 384-bit digest.",
			Category:    "Hashes",
			Query:       `"hello" | sha384`,
			Input:       `null`,
		},
		{
			Title:       "SHA-512 of a string",
			Description: "sha512 gives the longest SHA-2 digest, 128 hex characters.",
			Category:    "Hashes",
			Query:       `"hello" | sha512`,
			Input:       `null`,
		},
		{
			Title:       "SHA-512/224 and SHA-512/256",
			Description: "The truncated SHA-512 variants run 64-bit arithmetic even for short digests.",
			Category:    "Hashes",
			Query:       `{t224: ("hello" | sha512_224), t256: ("hello" | sha512_256)}`,
			Input:       `null`,
		},
		{
			Title:       "Hash a whole document, not a string",
			Description: "tojson flattens any value into a canonical string, so objects and arrays hash too.",
			Category:    "Hashes",
			Query:       `{user: "ada", roles: ["admin", "dev"]} | tojson | sha256`,
			Input:       `null`,
		},
		{
			Title:       "Hash a list, then count the unique ones",
			Description: "map(sha256) hashes every row; unique_by collapses duplicates.",
			Category:    "Hashes",
			Query:       `["a","b","c","a"] | map({name: ., sha: sha256}) | unique_by(.sha) | length`,
			Input:       `null`,
		},

		// ------------------------------------------------------------------
		// HMAC
		// ------------------------------------------------------------------
		{
			Title:       "Sign a message with HMAC-SHA256",
			Description: "hmac_sha256 takes a key and a message; the same inputs always give the same signature.",
			Category:    "HMAC",
			Query:       `{sig: hmac_sha256("s3cr3t"; "payload"), again: hmac_sha256("s3cr3t"; "payload")}`,
			Input:       `null`,
		},
		{
			Title:       "HMAC-MD5",
			Description: "hmac_md5 is HMAC with the MD5 digest, used by some legacy APIs.",
			Category:    "HMAC",
			Query:       `"message" | hmac_md5("key")`,
			Input:       `null`,
		},
		{
			Title:       "HMAC-SHA512 over a JSON body",
			Description: "Sign the canonical JSON of a request body so the signature covers the payload.",
			Category:    "HMAC",
			Query:       `{body: {amount: 100, to: "bob"}} | {signature: (.body | tojson | hmac_sha512("api-key"))}`,
			Input:       `null`,
		},

		// ------------------------------------------------------------------
		// Ciphers
		// ------------------------------------------------------------------
		{
			Title:       "Encrypt and decrypt",
			Description: "A cipher round-trip, entirely inside the tab: nothing here is sent anywhere.",
			Category:    "Ciphers",
			Query:       `aes_encrypt(.; "hunter2hunter2hu") | {ciphertext: ., plaintext: aes_decrypt(.; "hunter2hunter2hu")}`,
			Input:       `"attack at dawn"`,
		},
		{
			Title:       "AES in ECB mode",
			Description: "AES takes a mode argument; ECB is the stateless one, unsafe for repeated blocks but easy to read.",
			Category:    "Ciphers",
			Query:       `"attack at dawn" | aes_encrypt(.; "hunter2hunter2hu"; "ECB") as $c | {ciphertext: $c, plaintext: aes_decrypt($c; "hunter2hunter2hu"; "ECB")}`,
			Input:       `null`,
		},
		{
			Title:       "DES round-trip",
			Description: "DES is the 56-bit cipher that AES replaced; its 8-byte key shows how small that is.",
			Category:    "Ciphers",
			Query:       `"attack at dawn" | des_encrypt(.; "12345678") as $c | {ciphertext: $c, plaintext: des_decrypt($c; "12345678")}`,
			Input:       `null`,
		},
		{
			Title:       "Blowfish round-trip",
			Description: "Blowfish accepts keys of any length, which is why it lingered in password managers.",
			Category:    "Ciphers",
			Query:       `"attack at dawn" | blowfish_encrypt(.; "hunter2") as $c | {ciphertext: $c, plaintext: blowfish_decrypt($c; "hunter2")}`,
			Input:       `null`,
		},
		{
			Title:       "RC4 round-trip",
			Description: "RC4 is symmetric: encrypting twice with the same key restores the plaintext.",
			Category:    "Ciphers",
			Query:       `"attack at dawn" | rc4("hunter2") as $c | {ciphertext: $c, plaintext: ($c | rc4("hunter2"; "raw"; "base64") | base64_decode)}`,
			Input:       `null`,
		},
		{
			Title:       "ChaCha20 stream cipher",
			Description: "chacha20 needs a 256-bit key; the output carries the nonce it used, so it decrypts with the same key.",
			Category:    "Ciphers",
			Query:       `{ciphertext: ("attack at dawn" | chacha20("0123456789abcdef0123456789abcdef")), bytes: ("attack at dawn" | chacha20("0123456789abcdef0123456789abcdef") | length)}`,
			Input:       `null`,
		},
		{
			Title:       "XOR round-trip",
			Description: "xor is the trivial cipher: one byte of key per byte of data. Its output is hex.",
			Category:    "Ciphers",
			Query:       `"attack at dawn" | xor("hunter2") as $c | {ciphertext: $c, plaintext: ($c | xor("hunter2"; "raw"; "hex") | hex_decode)}`,
			Input:       `null`,
		},

		// ------------------------------------------------------------------
		// Encoding
		// ------------------------------------------------------------------
		{
			Title:       "Decode a base64 payload",
			Description: "Chain codecs the way you would in a shell, but over structured data.",
			Category:    "Encoding",
			Query:       `.payload | base64_decode | fromjson | .user`,
			Input:       `{"payload":"eyJ1c2VyIjoiYWRtaW4iLCJyb2xlIjoicm9vdCJ9"}`,
		},
		{
			Title:       "Round-trip through gzip",
			Description: "Compression cmdlets work on strings, and compose both ways.",
			Category:    "Encoding",
			Query:       `{original: length, compressed: (gzip_compress | length), same: (gzip_compress | gzip_decompress) == .}`,
			Input:       `"the quick brown fox jumps over the lazy dog, again and again and again"`,
		},
		{
			Title:       "Encode a string to base64",
			Description: "base64_encode turns any string into a padded base64 value, like jq's @base64.",
			Category:    "Encoding",
			Query:       `"hello world" | base64_encode`,
			Input:       `null`,
		},
		{
			Title:       "Decode a string from base64",
			Description: "base64_decode is the inverse of base64_encode.",
			Category:    "Encoding",
			Query:       `"aGVsbG8gd29ybGQ=" | base64_decode`,
			Input:       `null`,
		},
		{
			Title:       "Encode a document as base64",
			Description: "tojson then base64_encode is how structured data travels as a token.",
			Category:    "Encoding",
			Query:       `{user: "admin", role: "root"} | tojson | base64_encode`,
			Input:       `null`,
		},
		{
			Title:       "base32 round-trip",
			Description: "base32_encode is the case-insensitive cousin of base64; the decode brings the text back.",
			Category:    "Encoding",
			Query:       `"hello" | base32_encode as $e | {encoded: $e, decoded: ($e | base32_decode)}`,
			Input:       `null`,
		},
		{
			Title:       "base85 round-trip",
			Description: "base85_encode squeezes four bytes into five characters, which is what git packs use.",
			Category:    "Encoding",
			Query:       `"hello" | base85_encode as $e | {encoded: $e, decoded: ($e | base85_decode)}`,
			Input:       `null`,
		},
		{
			Title:       "hex round-trip",
			Description: "hex_encode is the two-hex-digits-per-byte view; hex_decode turns it back into text.",
			Category:    "Encoding",
			Query:       `"hello" | hex_encode as $e | {encoded: $e, decoded: ($e | hex_decode)}`,
			Input:       `null`,
		},
		{
			Title:       "binary round-trip",
			Description: "binary_encode spells a string out as 0s and 1s; binary_decode reverses it.",
			Category:    "Encoding",
			Query:       `"AB" | binary_encode as $e | {encoded: $e, decoded: ($e | binary_decode)}`,
			Input:       `null`,
		},
		{
			Title:       "URL encode and decode",
			Description: "url_encode quotes a string for a query string; url_decode restores it.",
			Category:    "Encoding",
			Query:       `"a b&c=d" | url_encode as $e | {encoded: $e, decoded: ($e | url_decode)}`,
			Input:       `null`,
		},
		{
			Title:       "HTML entity encode and decode",
			Description: "html_encode escapes markup so text is not interpreted as tags; html_decode unescapes it.",
			Category:    "Encoding",
			Query:       `"<b>R&D</b>" | html_encode as $e | {encoded: $e, decoded: ($e | html_decode)}`,
			Input:       `null`,
		},
		{
			Title:       "A codec pipeline",
			Description: "Codecs chain like shell pipes: encode, decode, and transform between formats in one query.",
			Category:    "Encoding",
			Query:       `"attack at dawn" | base64_encode | base64_decode | ascii_upcase`,
			Input:       `null`,
		},

		// ------------------------------------------------------------------
		// Compression
		// ------------------------------------------------------------------
		{
			Title:       "zlib round-trip",
			Description: "zlib_compress adds a header and checksum to deflate; zlib_decompress removes them.",
			Category:    "Compression",
			Query:       `"hello hello hello" | zlib_compress as $c | {bytes: ($c | length), back: ($c | zlib_decompress)}`,
			Input:       `null`,
		},
		{
			Title:       "deflate round-trip",
			Description: "deflate_compress is the raw compressed stream; deflate_decompress reverses it.",
			Category:    "Compression",
			Query:       `"hello hello hello" | deflate_compress as $c | {bytes: ($c | length), back: ($c | deflate_decompress)}`,
			Input:       `null`,
		},
		{
			Title:       "How much does gzip save?",
			Description: "Compare lengths to see the compression ratio on repetitive data.",
			Category:    "Compression",
			Query:       `"x" * 10000 | {plain: length, gzip: (gzip_compress | length), zlib: (zlib_compress | length), deflate: (deflate_compress | length)}`,
			Input:       `null`,
		},

		// ------------------------------------------------------------------
		// Conversion
		// ------------------------------------------------------------------
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
			Title:       "Timestamps to dates",
			Description: "Unix time in, ISO out.",
			Category:    "Conversion",
			Query:       `map({raw: ., when: timestamp_to_date})`,
			Input:       `[0, 1000000000, 1700000000]`,
		},
		{
			Title:       "Parse a JSON string",
			Description: "json_parse turns a JSON string back into data; fromjson is the same thing.",
			Category:    "Conversion",
			Query:       `"{\"service\":\"api\",\"port\":8080}" | json_parse`,
			Input:       `null`,
		},
		{
			Title:       "Stringify a document",
			Description: "json_stringify is tojson: a value becomes one JSON string.",
			Category:    "Conversion",
			Query:       `{name: "api", port: 8080} | json_stringify`,
			Input:       `null`,
		},
		{
			Title:       "Build a CSV document",
			Description: "csv_stringify writes rows of an array of arrays, header and all.",
			Category:    "Conversion",
			Query:       `[["name","role"],["ada","engineer"],["grace","admiral"]] | csv_stringify`,
			Input:       `null`,
		},
		{
			Title:       "Turn objects into XML",
			Description: "xml_stringify renders _tag and _content as a document, the inverse of xml_parse.",
			Category:    "Conversion",
			Query:       `{_tag: "config", _content: "enabled"} | xml_stringify`,
			Input:       `null`,
		},
		{
			Title:       "Date to timestamp",
			Description: "date_to_timestamp parses an ISO date back into Unix seconds.",
			Category:    "Conversion",
			Query:       `"2026-01-01" | date_to_timestamp`,
			Input:       `null`,
		},

		// ------------------------------------------------------------------
		// Analysis
		// ------------------------------------------------------------------
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
			Title:       "Rank values by entropy",
			Description: "entropy scores how unpredictable a string is; sort the scores to see which value is most random.",
			Category:    "Analysis",
			Query:       `to_entries | map({key, entropy: (.value | entropy)}) | sort_by(-.entropy)`,
			Input:       `{"username":"admin","api_key":"8f3a91c2e7d4b506","email":"user@example.com","password":"P@ssw0rd!2024"}`,
		},
		{
			Title:       "Fuzzy hash with ssdeep",
			Description: "ssdeep needs enough input to matter; give it a paragraph, not a word.",
			Category:    "Analysis",
			Query:       `"The quick brown fox jumps over the lazy dog. " * 120 | ssdeep`,
			Input:       `null`,
		},
		{
			Title:       "Compare fuzzy hashes",
			Description: "ssdeep_compare scores how similar two hashes are, 0 to 100.",
			Category:    "Analysis",
			Query:       `("hello " * 1000) | ssdeep as $a | ("hello " * 1000) | ssdeep as $b | {score: ssdeep_compare($a; $b)}`,
			Input:       `null`,
		},

		// ------------------------------------------------------------------
		// Strings
		// ------------------------------------------------------------------
		{
			Title:       "Shout and whisper",
			Description: "upper and lower fold a string's case.",
			Category:    "Strings",
			Query:       `"Hello, World" | {upper: upper, lower: lower}`,
			Input:       `null`,
		},
		{
			Title:       "Reverse a string",
			Description: "reverse_string mirrors a string; handy for palindrome checks.",
			Category:    "Strings",
			Query:       `"stressed" | {reversed: reverse_string, palindrome: (. == reverse_string)}`,
			Input:       `null`,
		},
		{
			Title:       "Replace text",
			Description: "replace swaps every occurrence of one substring for another.",
			Category:    "Strings",
			Query:       `"hello world" | replace("world"; "there")`,
			Input:       `null`,
		},
		{
			Title:       "Censor with replace",
			Description: "replace works on plain text; chain it to scrub several tokens at once.",
			Category:    "Strings",
			Query:       `"login: alice, token: abc123" | replace("abc123"; "***")`,
			Input:       `null`,
		},

		// ------------------------------------------------------------------
		// Discovery
		// ------------------------------------------------------------------
		{
			Title:       "What can I call here?",
			Description: "get_command answers from the registry the page actually runs, not from a document.",
			Category:    "Discovery",
			Query:       `[get_command("sha*") | {Name, Description}] | sort_by(.Name)`,
			Input:       ``,
		},
		{
			Title:       "List every command",
			Description: "get_command with no pattern lists the whole vocabulary, the page's own --udf-list.",
			Category:    "Discovery",
			Query:       `[get_command | .Name] | length`,
			Input:       ``,
		},
		{
			Title:       "Find commands by category",
			Description: "get_command returns objects, so you can filter them with ordinary jq.",
			Category:    "Discovery",
			Query:       `[get_command | select(.Category == "PowerShell") | .Name] | sort`,
			Input:       ``,
		},
		{
			Title:       "Read the help for a cmdlet",
			Description: "get_help renders the NAME, SYNOPSIS, SYNTAX and EXAMPLES sections for a command.",
			Category:    "Discovery",
			Query:       `get_help("sha256")`,
			Input:       ``,
		},
		{
			Title:       "Which commands still run here?",
			Description: "get_command reports an Available flag, so the page can show what a query is allowed to use.",
			Category:    "Discovery",
			Query:       `[get_command | select(.Available) | .Name] | length as $n | {here: $n, total: ([get_command | .Name] | length)}`,
			Input:       ``,
		},

		// ------------------------------------------------------------------
		// Arguments
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

		// ------------------------------------------------------------------
		// jq — fundamentals
		// ------------------------------------------------------------------
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

		// ------------------------------------------------------------------
		// jq — arrays
		// ------------------------------------------------------------------
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

		// ------------------------------------------------------------------
		// jq — objects
		// ------------------------------------------------------------------
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
			Input:       `{"name":"svc","auth":{"token":"abc123","user":"svc"},"nested":[{"password":"p"}]}`,
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

		// ------------------------------------------------------------------
		// jq — strings
		// ------------------------------------------------------------------
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

		// ------------------------------------------------------------------
		// jq — numbers
		// ------------------------------------------------------------------
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

		// ------------------------------------------------------------------
		// jq — iteration and generators
		// ------------------------------------------------------------------
		{
			Title:       "Reduce into a summary",
			Description: "Plain jq: reduce is how you fold a stream into one value.",
			Category:    "jq",
			Query: `reduce .[] as $event (
  {count: 0, bytes: 0};
  {count: .count + 1, bytes: .bytes + $event.bytes}
)`,
			Input:       `[{"bytes":120},{"bytes":8400},{"bytes":33}]`,
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

		// ------------------------------------------------------------------
		// jq — input streams
		// ------------------------------------------------------------------
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
			Input:       `{"level":"info","ts":1,"msg":"started"}
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

		// ------------------------------------------------------------------
		// jq — realistic pipelines
		// ------------------------------------------------------------------
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
			Input:       `{"host":"api.example.com","key":"aXk9M2QwYjRmZzdoOThjbXA0cXJ1dg==","debug":"false","secret":"0p3n5e5am3j0p"}`,
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
