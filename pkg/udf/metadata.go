package udf

// FunctionMetadata holds information about a UDF
type FunctionMetadata struct {
	Name        string
	MinArgs     int
	MaxArgs     int
	Description string
	Category    string
	Examples    []string
}

// GetFunctionMetadata returns metadata for all registered functions
func GetFunctionMetadata() []FunctionMetadata {
	return []FunctionMetadata{
		// File operations
		{"find", 1, 2, "Find files/directories matching criteria", "File Operations", []string{`find("path"; "file")`, `find("path"; "dir")`}},
		{"cat", 0, 2, "Read and return contents of a file, decoded as text (filepath from pipe or argument)", "File Operations", []string{`cat("file.txt")`, `"file.txt" | cat`, `find("."; "file") | cat`, `cat("app.log"; {tail: 20})`}},
		{"read_bytes", 0, 1, "Read a file's bytes verbatim, with no text decoding (use utf8bytelength, not length, to count them)", "File Operations", []string{`read_bytes("a.bin") | sha256`, `"a.bin" | read_bytes | utf8bytelength`}},
		{"mkdir", 1, 1, "Create a directory (creates parent directories if needed)", "File Operations", []string{`mkdir("/tmp/mydir")`, `mkdir("nested/path/to/dir")`}},
		{"rm", 2, 2, "Remove a file or folder (path, type: 'file' or 'folder')", "File Operations", []string{`rm("/tmp/file.txt"; "file")`, `rm("/tmp/mydir"; "folder")`}},

		// Archives
		{"read_archive", 0, 1, "One object per entry in a .zip or .tar archive, without extracting", "Archives", []string{`read_archive("release.zip")`, `"backup.tar.gz" | read_archive | map(.Name)`}},
		{"expand_archive", 1, 2, "Extract an archive into a directory, returning the paths written (destination)", "Archives", []string{`expand_archive("release.zip"; "./out")`, `"backup.tar.gz" | expand_archive("./restore")`}},
		{"compress_archive", 1, 2, "Build a .zip, .tar or .tar.gz from a path or list of paths (destination)", "Archives", []string{`compress_archive("src"; "src.zip")`, `["a.txt","b.txt"] | compress_archive("pair.tar.gz")`}},

		// Encoding/Decoding
		{"base64_encode", 0, 2, "Encode to base64 (optional file arg)", "Encoding", []string{`base64_encode`, `base64_encode(true)`}},
		{"base64_decode", 0, 2, "Decode from base64 (optional file arg)", "Encoding", []string{`base64_decode`, `base64_decode(true)`}},
		{"hex_encode", 0, 2, "Encode to hexadecimal (optional file arg)", "Encoding", []string{`hex_encode`, `hex_encode(true)`}},
		{"hex_decode", 0, 2, "Decode from hexadecimal (optional file arg)", "Encoding", []string{`hex_decode`, `hex_decode(true)`}},
		{"base32_encode", 0, 2, "Encode to base32 (optional file arg)", "Encoding", []string{`base32_encode`, `base32_encode(true)`}},
		{"base32_decode", 0, 2, "Decode from base32 (optional file arg)", "Encoding", []string{`base32_decode`, `base32_decode(true)`}},
		{"base85_encode", 0, 2, "Encode to base85 (optional file arg)", "Encoding", []string{`base85_encode`, `base85_encode(true)`}},
		{"base85_decode", 0, 2, "Decode from base85 (optional file arg)", "Encoding", []string{`base85_decode`, `base85_decode(true)`}},
		{"binary_encode", 0, 2, "Encode to binary (optional file arg)", "Encoding", []string{`binary_encode`, `binary_encode(true)`}},
		{"binary_decode", 0, 2, "Decode from binary (optional file arg)", "Encoding", []string{`binary_decode`, `binary_decode(true)`}},
		{"url_encode", 0, 2, "URL encode (optional file arg)", "Encoding", []string{`url_encode`, `url_encode(true)`}},
		{"url_decode", 0, 2, "URL decode (optional file arg)", "Encoding", []string{`url_decode`, `url_decode(true)`}},
		{"html_encode", 0, 2, "HTML entity encode (optional file arg)", "Encoding", []string{`html_encode`, `html_encode(true)`}},
		{"html_decode", 0, 2, "HTML entity decode (optional file arg)", "Encoding", []string{`html_decode`, `html_decode(true)`}},

		// Compression
		{"gzip_compress", 0, 2, "Compress with gzip (optional file arg)", "Compression", []string{`gzip_compress`, `gzip_compress(true)`}},
		{"gzip_decompress", 0, 2, "Decompress gzip (optional file arg)", "Compression", []string{`gzip_decompress`, `gzip_decompress(true)`}},
		{"zlib_compress", 0, 2, "Compress with zlib (optional file arg)", "Compression", []string{`zlib_compress`, `zlib_compress(true)`}},
		{"zlib_decompress", 0, 2, "Decompress zlib (optional file arg)", "Compression", []string{`zlib_decompress`, `zlib_decompress(true)`}},
		{"deflate_compress", 0, 2, "Compress with deflate (optional file arg)", "Compression", []string{`deflate_compress`, `deflate_compress(true)`}},
		{"deflate_decompress", 0, 2, "Decompress deflate (optional file arg)", "Compression", []string{`deflate_decompress`, `deflate_decompress(true)`}},

		// String operations
		{"replace", 2, 4, "Replace substring (old, new, [input], [file])", "String", []string{`replace("old"; "new")`, `replace("old"; "new"; "text")`}},

		// Hash functions
		{"md5", 0, 2, "MD5 hash (optional file arg)", "Hash", []string{`md5`, `md5(true)`}},
		{"sha1", 0, 2, "SHA1 hash (optional file arg)", "Hash", []string{`sha1`, `sha1(true)`}},
		{"sha224", 0, 2, "SHA224 hash (optional file arg)", "Hash", []string{`sha224`, `sha224(true)`}},
		{"sha256", 0, 2, "SHA256 hash (optional file arg)", "Hash", []string{`sha256`, `sha256(true)`}},
		{"sha384", 0, 2, "SHA384 hash (optional file arg)", "Hash", []string{`sha384`, `sha384(true)`}},
		{"sha512", 0, 2, "SHA512 hash (optional file arg)", "Hash", []string{`sha512`, `sha512(true)`}},
		{"sha512_224", 0, 2, "SHA512/224 hash (optional file arg)", "Hash", []string{`sha512_224`, `sha512_224(true)`}},
		{"sha512_256", 0, 2, "SHA512/256 hash (optional file arg)", "Hash", []string{`sha512_256`, `sha512_256(true)`}},

		// HMAC functions
		{"hmac_md5", 1, 3, "HMAC-MD5 (key, [message], [file])", "HMAC", []string{`hmac_md5("key")`, `hmac_md5("key"; "message")`}},
		{"hmac_sha1", 1, 3, "HMAC-SHA1 (key, [message], [file])", "HMAC", []string{`hmac_sha1("key")`, `hmac_sha1("key"; "message")`}},
		{"hmac_sha224", 1, 3, "HMAC-SHA224 (key, [message], [file])", "HMAC", []string{`hmac_sha224("key")`, `hmac_sha224("key"; "message")`}},
		{"hmac_sha256", 1, 3, "HMAC-SHA256 (key, [message], [file])", "HMAC", []string{`hmac_sha256("key")`, `hmac_sha256("key"; "message")`}},
		{"hmac_sha384", 1, 3, "HMAC-SHA384 (key, [message], [file])", "HMAC", []string{`hmac_sha384("key")`, `hmac_sha384("key"; "message")`}},
		{"hmac_sha512", 1, 3, "HMAC-SHA512 (key, [message], [file])", "HMAC", []string{`hmac_sha512("key")`, `hmac_sha512("key"; "message")`}},
		{"hmac_sha512_224", 1, 3, "HMAC-SHA512/224 (key, [message], [file])", "HMAC", []string{`hmac_sha512_224("key")`, `hmac_sha512_224("key"; "message")`}},
		{"hmac_sha512_256", 1, 3, "HMAC-SHA512/256 (key, [message], [file])", "HMAC", []string{`hmac_sha512_256("key")`, `hmac_sha512_256("key"; "message")`}},

		// Timestamp operations
		{"timestamp_to_date", 0, 2, "Convert Unix timestamp to date (optional file arg)", "Timestamp", []string{`timestamp_to_date`, `1609459200 | timestamp_to_date`}},
		{"date_to_timestamp", 0, 2, "Convert date to Unix timestamp (optional file arg)", "Timestamp", []string{`date_to_timestamp`, `"2021-01-01T00:00:00Z" | date_to_timestamp`}},

		// JSON operations
		{"json_parse", 0, 2, "Parse JSON string (optional file arg)", "JSON", []string{`json_parse`, `"{\"key\":\"value\"}" | json_parse`}},
		{"json_stringify", 0, 2, "Convert to JSON string (optional file arg)", "JSON", []string{`json_stringify`, `{"key":"value"} | json_stringify`}},

		// CSV operations
		{"csv_parse", 0, 3, "Parse CSV (delimiter, [input], [file])", "CSV", []string{`csv_parse`, `csv_parse(",")`, `csv_parse(","; "a,b,c")`}},
		{"csv_stringify", 0, 3, "Convert to CSV (delimiter, [input], [file])", "CSV", []string{`csv_stringify`, `csv_stringify(",")`, `[[["a","b"]]] | csv_stringify(",")`}},

		// XML operations
		{"xml_parse", 0, 2, "Parse XML string (optional file arg)", "XML", []string{`xml_parse`, `"<root>test</root>" | xml_parse`}},
		{"xml_stringify", 0, 2, "Convert to XML string (optional file arg)", "XML", []string{`xml_stringify`, `{"_tag":"root","_content":"test"} | xml_stringify`}},

		// Entropy
		{"entropy", 0, 2, "Calculate Shannon entropy (optional file arg)", "Entropy", []string{`entropy`, `entropy(true)`, `"hello" | entropy`}},

		// SSDeep (fuzzy hashing)
		{"ssdeep", 0, 2, "Calculate ssdeep fuzzy hash (optional file arg)", "SSDeep", []string{`ssdeep`, `ssdeep(true)`, `"hello" | ssdeep`}},
		{"ssdeep_compare", 2, 2, "Compare two ssdeep hashes (hash1, hash2)", "SSDeep", []string{`ssdeep_compare("hash1"; "hash2")`, `ssdeep("text1") | ssdeep_compare(.; ssdeep("text2"))`}},

		// Tee (write to stderr or file)
		{"tee", 0, 1, "Write JSON to stderr (default) or file (optional filepath arg)", "File Operations", []string{`tee`, `tee("/tmp/output.json")`, `{"key":"value"} | tee`}},

		// Shell command execution
		{"sh", 0, 1, "Execute a shell command (command from pipe or argument)", "System", []string{`sh("echo hello")`, `"echo test" | sh(.)`, `sh("ls -la")`}},

		// Temporary directory
		{"tempdir", 0, 2, "Create a temporary directory (optional prefix, optional dir)", "File Operations", []string{`tempdir`, `tempdir("prefix_")`, `tempdir("prefix_"; "/tmp")`, `tempdir(""; "/tmp")`}},

		// HTTP requests
		{"http", 0, 2, "Make an HTTP request, returning a response object whose .Content is the undecoded body", "HTTP", []string{`http("GET"; "https://example.com") | .StatusCode`, `http("GET"; "https://api.example.com/x") | .Content | json_parse`, `"https://example.com" | http`, `{"key":"value"} | http("POST"; "https://api.example.com")`, `http("GET"; "https://example.com/a.tar.gz") | .Content | out_file("a.tar.gz")`}},
		{"http_serve", 2, 2, "Start HTTP server (host, port) - returns server URL", "HTTP", []string{`http_serve("127.0.0.1"; 8080)`, `http_serve("0.0.0.0"; 0)`}},

		// Encryption/Decryption
		{"aes_encrypt", 2, 5, "AES encryption (data, key, [mode=CBC], [keyFormat=raw], [dataFormat=raw])", "Encryption", []string{`aes_encrypt("data"; "key")`, `aes_encrypt("data"; "key"; "CBC")`, `aes_encrypt("data"; "key"; "ECB")`}},
		{"aes_decrypt", 2, 5, "AES decryption (data, key, [mode=CBC], [keyFormat=raw], [dataFormat=base64])", "Encryption", []string{`aes_decrypt("encrypted"; "key")`, `aes_decrypt("encrypted"; "key"; "CBC")`}},
		{"des_encrypt", 2, 4, "DES encryption (data, key, [mode=CBC], [keyFormat=raw])", "Encryption", []string{`des_encrypt("data"; "key")`, `des_encrypt("data"; "key"; "CBC")`}},
		{"des_decrypt", 2, 4, "DES decryption (data, key, [mode=CBC], [keyFormat=raw])", "Encryption", []string{`des_decrypt("encrypted"; "key")`, `des_decrypt("encrypted"; "key"; "CBC")`}},
		// Named triple_des_encrypt rather than 3des_encrypt because jq identifiers
		// cannot start with a digit; a 3des_encrypt function could never be
		// called from a query.
		{"triple_des_encrypt", 2, 4, "Triple DES encryption (data, key, [mode=CBC], [keyFormat=raw])", "Encryption", []string{`triple_des_encrypt("data"; "key")`, `triple_des_encrypt("data"; "key"; "CBC")`}},
		{"triple_des_decrypt", 2, 4, "Triple DES decryption (data, key, [mode=CBC], [keyFormat=raw])", "Encryption", []string{`triple_des_decrypt("encrypted"; "key")`, `triple_des_decrypt("encrypted"; "key"; "CBC")`}},
		{"blowfish_encrypt", 2, 4, "Blowfish encryption (data, key, [mode=CBC], [keyFormat=raw])", "Encryption", []string{`blowfish_encrypt("data"; "key")`, `blowfish_encrypt("data"; "key"; "CBC")`}},
		{"blowfish_decrypt", 2, 4, "Blowfish decryption (data, key, [mode=CBC], [keyFormat=raw])", "Encryption", []string{`blowfish_decrypt("encrypted"; "key")`, `blowfish_decrypt("encrypted"; "key"; "CBC")`}},
		{"rc4", 1, 3, "RC4 encryption/decryption (key, [keyFormat=raw], [dataFormat=raw])", "Encryption", []string{`rc4("key")`, `"data" | rc4("key")`}},
		{"chacha20", 1, 4, "ChaCha20 encryption/decryption (key, [nonce], [keyFormat=raw], [dataFormat=raw])", "Encryption", []string{`chacha20("key")`, `"data" | chacha20("key")`}},
		{"xor", 1, 3, "XOR encryption/decryption (key, [keyFormat=raw], [dataFormat=raw])", "Encryption", []string{`xor("key")`, `"data" | xor("key")`}},

		// Text utilities
		{"slugify", 0, 2, "Lowercase string with words joined by hyphens (URL/file safe)", "String", []string{`"Hello World!" | slugify`}},
		{"snake_case", 0, 2, "Join words with underscores", "String", []string{`"FooBar baz" | snake_case`}},
		{"kebab_case", 0, 2, "Join words with hyphens", "String", []string{`"FooBar baz" | kebab_case`}},
		{"camel_case", 0, 2, "Join words with the first lower and the rest capitalized", "String", []string{`"hello world" | camel_case`}},
		{"pascal_case", 0, 2, "Join words each capitalized", "String", []string{`"hello world" | pascal_case`}},
		{"title_case", 0, 2, "Capitalize the first letter of every word", "String", []string{`"the quick brown fox" | title_case`}},
		{"truncate", 1, 3, "Cut a string to a length, appending a suffix (n, [suffix], [file])", "String", []string{`"hello world" | truncate(5)`, `"hello world" | truncate(5; "...")`}},
		{"pad_left", 1, 3, "Left-pad a string to a width with a repeated character (width, [pad], [file])", "String", []string{`"5" | pad_left(3; "0")`}},
		{"pad_right", 1, 3, "Right-pad a string to a width with a repeated character (width, [pad], [file])", "String", []string{`"5" | pad_right(3; "0")`}},
		{"mask", 0, 2, "Hide the middle of a string, keeping the first and last visible characters", "String", []string{`"hunter2" | mask`, `"hunter2" | mask(2)`}},
		{"count_occurrences", 1, 2, "Count non-overlapping occurrences of a substring (sub, [file])", "String", []string{`"banana" | count_occurrences("an")`}},

		// Numbers and radix
		{"to_base", 1, 1, "Render a number in a base from 2 to 36", "Numbers", []string{`42 | to_base(16)`, `255 | to_base(2)`}},
		{"from_base", 1, 1, "Parse a number written in a base from 2 to 36", "Numbers", []string{`"2a" | from_base(16)`}},
		{"to_hex_number", 0, 0, "A number as a hex string", "Numbers", []string{`255 | to_hex_number`}},
		{"from_hex_number", 0, 0, "A hex string as a number", "Numbers", []string{`"ff" | from_hex_number`}},
		{"clamp", 2, 2, "Bound a number to a range (lo, hi)", "Numbers", []string{`99 | clamp(0; 10)`}},
		{"gcd", 1, 1, "Greatest common divisor (a, b)", "Numbers", []string{`12 | gcd(18)`}},
		{"lcm", 1, 1, "Least common multiple (a, b)", "Numbers", []string{`4 | lcm(6)`}},
		{"round_to", 1, 1, "Round a number to decimal places (places)", "Numbers", []string{`3.14159 | round_to(2)`, `1234 | round_to(-2)`}},
		{"human_bytes", 0, 0, "A byte count as binary units (KiB, MiB, GiB, ...)", "Numbers", []string{`1048576 | human_bytes`}},
		{"percentage", 1, 1, "Part as a percentage of whole (part, whole)", "Numbers", []string{`40 | percentage(200)`}},

		// Paths
		{"basename", 0, 1, "The last component of a path", "Paths", []string{`"/tmp/data/file.txt" | basename`}},
		{"dirname", 0, 1, "A path minus its last component", "Paths", []string{`"/tmp/data/file.txt" | dirname`}},
		{"file_extension", 0, 1, "The suffix after the final dot, or empty", "Paths", []string{`"/tmp/data/file.txt" | file_extension`}},
		{"is_absolute", 0, 1, "Whether a path is absolute", "Paths", []string{`"/tmp/x" | is_absolute`}},

		// Statistics
		{"mean", 0, 1, "Arithmetic mean of an array", "Statistics", []string{`[1,2,3,4] | mean`}},
		{"median", 0, 1, "Middle value of an array", "Statistics", []string{`[1,2,3,4] | median`}},
		{"mode", 0, 1, "Most frequent value of an array", "Statistics", []string{`["a","b","a"] | mode`}},
		{"variance", 0, 1, "Sample variance of an array (n-1)", "Statistics", []string{`[2,4,4,4,5,5,7,9] | variance`}},
		{"stdev", 0, 1, "Sample standard deviation of an array", "Statistics", []string{`[2,4,4,4,5,5,7,9] | stdev`}},
		{"percentile", 1, 2, "Value below which p percent of an array falls (p)", "Statistics", []string{`[1,2,3,4] | percentile(50)`}},
		{"summary", 0, 1, "count, min, max, mean, median and stdev of an array", "Statistics", []string{`[1,2,3,4] | summary`}},

		// Duration and time
		{"human_duration", 0, 0, "Seconds as a compact 1d 2h 3m 4s string", "Duration", []string{`3661 | human_duration`}},
		{"parse_duration", 0, 0, "A duration string like 2h30m to seconds", "Duration", []string{`"2h30m" | parse_duration`}},
		{"time_ago", 0, 0, "A timestamp rendered relative to now", "Duration", []string{`1700000000 | time_ago`}},
		{"weekday", 0, 0, "The day name for a timestamp or date", "Duration", []string{`"2026-08-10" | weekday`}},
		{"is_weekend", 0, 0, "Whether a date is Saturday or Sunday", "Duration", []string{`"2026-08-08" | is_weekend`}},

		// Random
		{"random_int", 0, 2, "A uniform integer in [min, max] (one arg: 0..max)", "Random", []string{`random_int(1; 6)`, `random_int(100)`}},
		{"random_float", 0, 2, "A uniform float in [0,1) or a range", "Random", []string{`random_float`, `random_float(10; 20)`}},
		{"random_string", 1, 2, "n random characters from an alphabet (n, [alphabet])", "Random", []string{`random_string(16)`, `random_string(8; "01")`}},
		{"random_choice", 0, 1, "A uniformly chosen element of an array", "Random", []string{`[10,20,30] | random_choice`}},
		{"shuffle", 0, 1, "A random permutation of an array", "Random", []string{`[1,2,3,4,5] | shuffle`}},
		{"sample", 1, 2, "n distinct elements chosen at random (n)", "Random", []string{`[1,2,3,4,5] | sample(3)`}},

		// IP and network
		{"is_ip", 0, 2, "Whether a string is an IPv4 or IPv6 address", "IP & Network", []string{`"192.168.1.1" | is_ip`}},
		{"is_ipv4", 0, 2, "Whether a string is an IPv4 address", "IP & Network", []string{`"192.168.1.1" | is_ipv4`}},
		{"is_ipv6", 0, 2, "Whether a string is an IPv6 address", "IP & Network", []string{`"2001:db8::1" | is_ipv6`}},
		{"ip_to_int", 0, 2, "An address as a decimal integer (IPv6 as a decimal string)", "IP & Network", []string{`"192.168.1.1" | ip_to_int`}},
		{"int_to_ip", 0, 1, "A decimal integer back to an address", "IP & Network", []string{`3232235777 | int_to_ip`}},
		{"in_cidr", 1, 1, "Whether an address falls inside a CIDR block (ip, cidr)", "IP & Network", []string{`"10.1.2.3" | in_cidr("10.0.0.0/8")`}},
		{"cidr_size", 0, 2, "How many addresses a CIDR block holds", "IP & Network", []string{`"10.0.0.0/24" | cidr_size`}},
		{"is_mac", 0, 2, "Whether a string is a MAC address", "IP & Network", []string{`"00:11:22:33:44:55" | is_mac`}},
		{"mac_normalize", 0, 2, "A MAC address lowercased and colon-separated", "IP & Network", []string{`"00-11-22-33-44-55" | mac_normalize`}},

		// IDs and tokens
		{"uuid4", 0, 0, "A freshly generated version-4 UUID", "IDs & Tokens", []string{`uuid4`}},
		{"is_uuid", 0, 2, "Whether a string is a UUID", "IDs & Tokens", []string{`"550e8400-e29b-41d4-a716-446655440000" | is_uuid`}},
		{"uuid_version", 0, 2, "The version nibble of a UUID, or null", "IDs & Tokens", []string{`"550e8400-e29b-41d4-a716-446655440000" | uuid_version`}},
		{"jwt_decode", 0, 2, "Split a JWT into decoded header, payload and signature", "IDs & Tokens", []string{`"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.sig" | jwt_decode`}},
		{"is_jwt", 0, 2, "Whether a string is three base64url segments", "IDs & Tokens", []string{`"a.b.c" | is_jwt`}},
		{"base64url_encode", 0, 2, "Unpadded URL-safe base64", "IDs & Tokens", []string{`"hello?" | base64url_encode`}},
		{"base64url_decode", 0, 2, "Decode unpadded URL-safe base64", "IDs & Tokens", []string{`"aGVsbG8_" | base64url_decode`}},
		{"rot13", 0, 2, "ROT-13 over ASCII letters", "IDs & Tokens", []string{`"hello" | rot13`}},
		{"rot", 1, 2, "A Caesar cipher with a shift (shift)", "IDs & Tokens", []string{`"hello" | rot(1)`}},

		{"to_timezone", 1, 2, "The same instant in another IANA time zone, with its offset (zone)", "Duration", []string{`"2026-08-11T12:00:00Z" | to_timezone("Europe/London")`, `to_timezone(1770000000; "Asia/Tokyo")`}},
		{"format_date", 1, 3, "Write an instant in a layout, optionally in a zone (layout, [zone])", "Duration", []string{`"2026-08-11T12:00:00Z" | format_date("date")`, `format_date(1770000000; "rfc1123"; "UTC")`}},
		{"parse_date", 1, 3, "Read a date string with an explicit layout (layout, [zone])", "Duration", []string{`"11/08/2026" | parse_date("02/01/2006")`, `parse_date("2026-08-11 09:30:00"; "datetime"; "Europe/Berlin")`}},
		{"list_timezones", 0, 1, "The IANA zone names this build can resolve, filtered by a substring", "Duration", []string{`list_timezones("Europe")`, `"Tokyo" | list_timezones`}},

		// Validation and extraction
		{"is_email", 0, 2, "Whether a string looks like an email", "Validation", []string{`"ada@example.com" | is_email`}},
		{"is_url", 0, 2, "Whether a string is an http(s) URL", "Validation", []string{`"https://example.com/x" | is_url`}},
		{"is_domain", 0, 2, "Whether a string looks like a hostname", "Validation", []string{`"example.com" | is_domain`}},
		{"is_json", 0, 2, "Whether a string parses as JSON", "Validation", []string{`"{\"a\":1}" | is_json`}},
		{"extract_emails", 0, 2, "Every email-looking token in a string", "Validation", []string{`"mail ada@example.com" | extract_emails`}},
		{"extract_urls", 0, 2, "Every http(s) URL in a string", "Validation", []string{`"see https://a.com/x" | extract_urls`}},
		{"extract_ips", 0, 2, "Every IPv4 address in a string", "Validation", []string{`"from 10.0.0.1" | extract_ips`}},
		{"strip_tags", 0, 2, "HTML tags removed from a string", "Validation", []string{`"<b>hi</b>" | strip_tags`}},

		// Similarity
		{"levenshtein", 2, 2, "Edit distance between two strings", "Similarity", []string{`levenshtein("kitten"; "sitting")`}},
		{"hamming_distance", 2, 2, "Positions at which two equal-length strings differ", "Similarity", []string{`hamming_distance("karolin"; "kathrin")`}},
		{"jaccard", 2, 2, "Jaccard similarity of two strings or arrays, 0 to 1", "Similarity", []string{`jaccard([1,2,3]; [2,3,4])`}},
		{"deep_diff", 2, 2, "Structural JSON diff as {added, removed, changed}", "Similarity", []string{`deep_diff({a:1}; {a:2, b:3})`}},
		{"rncd_compare", 0, 2, "One object per pair of values, scored by compression distance and entropy", "Similarity", []string{`[$a, $b, $c] | rncd_compare`, `[read_bytes("a.bin"), read_bytes("b.bin")] | rncd_compare | .Hybrid`, `[find("samples"; "file") | {Name: ., Content: read_bytes(.)}] | [rncd_compare] | sort_by(.Hybrid)`}},
		{"shared_chunks", 1, 3, "Byte ranges of one value that occur verbatim in another, plus coverage", "Similarity", []string{`shared_chunks("the quick brown fox"; "the quick red fox")`, `read_bytes("suspect.bin") | shared_chunks(read_bytes("known.bin")) | .Coverage`}},

		// YAML
		{"yaml_parse", 0, 2, "A YAML document to a JSON value", "YAML", []string{`"name: ada\nrole: engineer\n" | yaml_parse`}},
		{"yaml_stringify", 0, 1, "A value to a YAML document", "YAML", []string{`{name: "ada"} | yaml_stringify`}},

		// Checksums
		{"crc32", 0, 2, "IEEE CRC-32 checksum", "Checksum", []string{`"hello" | crc32`}},
		{"crc32c", 0, 2, "Castagnoli CRC-32 checksum", "Checksum", []string{`"hello" | crc32c`}},
		{"crc64", 0, 2, "ECMA CRC-64 checksum", "Checksum", []string{`"hello" | crc64`}},
		{"fnv1a", 0, 2, "64-bit FNV-1a hash", "Checksum", []string{`"hello" | fnv1a`}},
		{"adler32", 0, 2, "Adler-32 checksum", "Checksum", []string{`"hello" | adler32`}},
		{"blake2b_256", 0, 2, "BLAKE2b truncated to 256 bits", "Checksum", []string{`"hello" | blake2b_256`}},
		{"blake2b_512", 0, 2, "BLAKE2b-512 hash", "Checksum", []string{`"hello" | blake2b_512`}},
		{"bcrypt_hash", 0, 2, "A bcrypt password hash (optional cost)", "Checksum", []string{`"hunter2" | bcrypt_hash`}},
		{"bcrypt_verify", 1, 2, "Whether a password matches a bcrypt hash (hash)", "Checksum", []string{`"hunter2" | bcrypt_verify("$2a$...")`}},

		// Line-oriented log readers (CLI only)
		{"head", 0, 2, "The first n lines of a file (path, [n])", "File Operations", []string{`head("app.log")`, `head("app.log"; 5)`}},
		{"tail", 0, 2, "The last n lines of a file (path, [n])", "File Operations", []string{`tail("app.log"; 5)`}},
		{"grep_lines", 1, 2, "The lines of a file matching a pattern (path, pattern)", "File Operations", []string{`grep_lines("app.log"; "error")`}},
		{"select_string", 1, 3, `Matching lines across a file or tree, with path, line number and context (pattern, [options]). The pattern is a jq string literal first and a regex second, so a backslash must be doubled: "\\.html", "join\\(".`, "File Operations", []string{`select_string("src"; "TODO") | .Path`, `[select_string("src"; "panic"; {Include: "*.go", Context: 2})]`, `first(select_string("src"; "\\.html$"))`, `select_string("src"; "join\\(|walk\\(")`}},
		{"wc_lines", 0, 1, "The number of lines in a file", "File Operations", []string{`wc_lines("app.log")`}},

		// Text predicates and inspection
		{"is_blank", 0, 2, "Whether a string is empty or whitespace", "String", []string{`"" | is_blank`}},
		{"is_alphanumeric", 0, 2, "Whether every character is a letter or digit", "String", []string{`"abc123" | is_alphanumeric`}},
		{"is_alphabetic", 0, 2, "Whether every character is a letter", "String", []string{`"abc" | is_alphabetic`}},
		{"is_numeric_string", 0, 2, "Whether every character is a digit", "String", []string{`"12345" | is_numeric_string`}},
		{"is_uppercase", 0, 2, "Whether a string's letters are all uppercase", "String", []string{`"HELLO" | is_uppercase`}},
		{"is_lowercase", 0, 2, "Whether a string's letters are all lowercase", "String", []string{`"hello" | is_lowercase`}},
		{"is_ascii", 0, 2, "Whether every byte is in the ASCII range", "String", []string{`"plain" | is_ascii`}},
		{"word_count", 0, 2, "How many whitespace-separated words a string has", "String", []string{`"the quick brown fox" | word_count`}},
		{"normalize_whitespace", 0, 2, "Collapse runs of whitespace to single spaces", "String", []string{`"  a   b  " | normalize_whitespace`}},
		{"acronym", 0, 2, "The uppercase initials of a string's words", "String", []string{`"International Business Machines" | acronym`}},
		{"escape_regex", 0, 2, "Quote a string so it matches literally in a regex", "String", []string{`"a.b" | escape_regex`}},
		{"is_regex_valid", 0, 2, "Whether a string compiles as a regular expression", "String", []string{`"^[a-z]+$" | is_regex_valid`}},
		{"glob_to_regex", 0, 2, "Turn a glob like *.txt into an anchored regex", "String", []string{`"*.txt" | glob_to_regex`}},
		{"match_glob", 1, 2, "Whether a string matches a glob (pattern)", "String", []string{`"notes.txt" | match_glob("*.txt")`}},

		// Collections
		{"chunks", 1, 2, "Split an array into chunks of at most n (n)", "Collections", []string{`[1,2,3,4,5] | chunks(2)`}},
		{"dedupe", 0, 1, "Remove duplicate values keeping first-occurrence order", "Collections", []string{`[3,1,2,1] | dedupe`}},
		{"deep_merge", 1, 2, "Recursively merge two objects, the second winning", "Collections", []string{`deep_merge({a: {x: 1}}; {a: {y: 2}})`}},
		{"prune", 0, 1, "Recursively remove empty values from objects and arrays", "Collections", []string{`{a: 1, b: null, c: {d: ""}} | prune`}},
		{"flatten_keys", 0, 1, "Turn a nested object into flat dot-and-bracket keys", "Collections", []string{`{a: {b: 1}} | flatten_keys`}},
		{"unflatten_keys", 0, 1, "The inverse of flatten_keys", "Collections", []string{`{"a.b": 1} | unflatten_keys`}},
		{"zip_arrays", 1, 2, "Pair two arrays element by element (other)", "Collections", []string{`[1,2,3] | zip_arrays(["a","b"])`}},

		// JSON pointer and query strings
		{"json_pointer", 1, 1, "Read the value at an RFC 6901 JSON pointer", "JSON", []string{`{a: {b: 1}} | json_pointer("/a/b")`}},
		{"json_pointer_set", 2, 2, "Return the document with a value at a pointer", "JSON", []string{`{a: 1} | json_pointer_set("/b"; 2)`}},
		{"query_string_parse", 0, 2, "Parse a URL query string into an object", "JSON", []string{`"a=1&b=two" | query_string_parse`}},
		{"query_string_build", 0, 1, "The inverse of query_string_parse", "JSON", []string{`{a: "1"} | query_string_build`}},

		// Time and date
		{"duration_between", 1, 1, "Seconds between two timestamps or dates", "Duration", []string{`"2026-01-01" | duration_between("2026-01-03")`}},
		{"add_seconds", 1, 1, "A timestamp plus n seconds", "Duration", []string{`0 | add_seconds(3600)`}},
		{"add_days", 1, 1, "A timestamp plus n days", "Duration", []string{`0 | add_days(1)`}},
		{"start_of_day", 0, 0, "A timestamp at its local midnight", "Duration", []string{`1700000000 | start_of_day`}},
		{"end_of_day", 0, 0, "A timestamp at the last second of its local day", "Duration", []string{`1700000000 | end_of_day`}},
		{"is_leap_year", 0, 0, "Whether a year has 366 days", "Duration", []string{`2024 | is_leap_year`}},
		{"days_in_month", 1, 1, "Days in a month of a year (year, month)", "Duration", []string{`2024 | days_in_month(2)`}},
		{"month_name", 0, 0, "The name of a month 1-12", "Duration", []string{`2 | month_name`}},

		// IP and network extras
		{"ip_version", 0, 2, "v4 or v6 for an address", "IP & Network", []string{`"192.168.1.1" | ip_version`}},
		{"is_private_ip", 0, 2, "Whether an address is private or loopback", "IP & Network", []string{`"10.0.0.1" | is_private_ip`}},
		{"is_loopback", 0, 2, "Whether an address is loopback", "IP & Network", []string{`"127.0.0.1" | is_loopback`}},
		{"cidr_network", 0, 2, "The base address of a CIDR block", "IP & Network", []string{`"192.168.1.55/24" | cidr_network`}},
		{"cidr_broadcast", 0, 2, "The last address of a CIDR block", "IP & Network", []string{`"192.168.1.55/24" | cidr_broadcast`}},
		{"ip_add", 1, 1, "An address shifted by n", "IP & Network", []string{`"192.168.1.1" | ip_add(1)`}},
		{"ipv6_expand", 0, 2, "An IPv6 address in full eight-group form", "IP & Network", []string{`"2001:db8::1" | ipv6_expand`}},
		{"reverse_ip", 0, 2, "The PTR record name for an address", "IP & Network", []string{`"192.168.1.1" | reverse_ip`}},

		// Hashes and key derivation
		{"sha3_256", 0, 2, "SHA-3-256 hash", "Checksum", []string{`"hello" | sha3_256`}},
		{"sha3_512", 0, 2, "SHA-3-512 hash", "Checksum", []string{`"hello" | sha3_512`}},
		{"keccak_256", 0, 2, "Legacy Keccak-256 hash", "Checksum", []string{`"hello" | keccak_256`}},
		{"crc16", 0, 2, "CRC-16/CCITT-FALSE checksum", "Checksum", []string{`"hello" | crc16`}},
		{"pbkdf2_sha256", 1, 3, "PBKDF2-SHA256 derived key as hex (salt, [iterations], [keyLen])", "Checksum", []string{`"password" | pbkdf2_sha256("salt"; 100000; 32)`}},
		{"argon2id_hash", 1, 3, "Argon2id derived key as hex (salt, [time], [memoryMiB])", "Checksum", []string{`"password" | argon2id_hash("salt"; 1; 8)`}},
		{"random_hex", 0, 1, "n cryptographically random bytes as hex", "Checksum", []string{`random_hex(16)`}},

		// IDs and tokens extras
		{"uuid7", 0, 0, "A time-ordered version-7 UUID", "IDs & Tokens", []string{`uuid7`}},
		{"nanoid", 0, 1, "n URL-safe characters from the nanoid alphabet", "IDs & Tokens", []string{`nanoid(21)`}},
		{"is_base64", 0, 2, "Whether a string decodes as standard base64", "IDs & Tokens", []string{`"aGVsbG8=" | is_base64`}},
		{"is_base64url", 0, 2, "Whether a string decodes as URL-safe base64", "IDs & Tokens", []string{`"aGVsbG8_" | is_base64url`}},
		{"base58_encode", 0, 2, "Encode bytes as base58", "IDs & Tokens", []string{`"hello" | base58_encode`}},
		{"base58_decode", 0, 2, "Decode base58 to bytes", "IDs & Tokens", []string{`"Cn8eVZg" | base58_decode`}},

		// Number extras
		{"factorial", 0, 0, "n! for an integer n", "Numbers", []string{`5 | factorial`}},
		{"is_prime", 0, 0, "Whether an integer is prime", "Numbers", []string{`13 | is_prime`}},
		{"fibonacci", 0, 0, "The nth Fibonacci number", "Numbers", []string{`10 | fibonacci`}},
		{"combinations_count", 1, 1, "How many ways to choose k of n (k)", "Numbers", []string{`5 | combinations_count(2)`}},
		{"permutations_count", 1, 1, "How many ways to order k of n (k)", "Numbers", []string{`5 | permutations_count(2)`}},
		{"ordinal", 0, 0, "An integer as 1st, 2nd, 3rd, ...", "Numbers", []string{`3 | ordinal`}},
		{"lerp", 2, 2, "Linear interpolation between a and b at t (b, t)", "Numbers", []string{`0 | lerp(10; 0.5)`}},
		{"human_number", 0, 0, "A count rendered compactly with k, M, B, T", "Numbers", []string{`1500 | human_number`}},
		{"is_even", 0, 0, "Whether an integer is even", "Numbers", []string{`4 | is_even`}},
		{"is_odd", 0, 0, "Whether an integer is odd", "Numbers", []string{`3 | is_odd`}},

		// Sniffing
		{"file_type", 0, 2, "The kind of file the bytes are, from magic numbers", "Sniff", []string{`"%PDF-1.7" | file_type`}},
		{"is_binary", 0, 2, "Whether bytes contain NULs or many control characters", "Sniff", []string{`"text" | is_binary`}},
		{"is_utf8", 0, 2, "Whether bytes are valid UTF-8", "Sniff", []string{`"héllo" | is_utf8`}},

		// Text tools
		{"strip_ansi", 0, 2, "Remove ANSI terminal escape sequences", "String", []string{`"\\u001b[31mred\\u001b[0m" | strip_ansi`}},
		{"template", 1, 1, "Replace {{key}} placeholders with object values (vars)", "String", []string{`"hello {{name}}" | template({name: "ada"})`}},
		{"wrap_text", 1, 2, "Word-wrap a string to a width, as lines (width)", "String", []string{`"the quick brown fox" | wrap_text(10)`}},
		{"indent", 1, 2, "Prefix every line with n spaces (width)", "String", []string{`"a\\nb" | indent(2)`}},

		// Statistics tools
		{"moving_average", 1, 2, "Rolling mean over a window of n (window)", "Statistics", []string{`[1,2,3,4,5] | moving_average(3)`}},
		{"geomean", 0, 1, "Geometric mean of positive numbers", "Statistics", []string{`[1,4,16] | geomean`}},
		{"normalize", 0, 1, "Min-max scale an array to [0,1]", "Statistics", []string{`[2,4,6] | normalize`}},

		// Number tools
		{"rescale", 4, 4, "Map a value between ranges (fromLo, fromHi, toLo, toHi)", "Numbers", []string{`5 | rescale(0; 10; 0; 100)`}},
		{"pct_change", 1, 1, "Percentage change from one value to another (b)", "Numbers", []string{`100 | pct_change(120)`}},
		{"digit_sum", 0, 0, "Sum of an integer's digits", "Numbers", []string{`1234 | digit_sum`}},
		{"hamming_weight", 0, 0, "Number of set bits in an integer", "Numbers", []string{`255 | hamming_weight`}},

		// Collection tools
		{"rotate", 1, 2, "Rotate an array left by n (negative rotates right)", "Collections", []string{`[1,2,3,4,5] | rotate(2)`}},
		{"top_n", 1, 2, "The n largest values, descending (n)", "Collections", []string{`[1,9,3,7,5] | top_n(2)`}},
		{"interleave", 1, 2, "Alternate two arrays' elements (other)", "Collections", []string{`[1,2,3] | interleave(["a","b","c"])`}},

		// JSON tools
		{"json_merge_patch", 1, 2, "Apply an RFC 7386 merge patch", "JSON", []string{`json_merge_patch({a: 1}; {a: 2, b: null})`}},
		{"jsonl_parse", 0, 2, "Parse newline-delimited JSON into an array", "JSON", []string{`"{\\"a\\":1}\\n{\\"a\\":2}" | jsonl_parse`}},
		{"get_path", 1, 1, "Read the value at a dot-and-bracket path", "JSON", []string{`{a: {b: [1, 2]}} | get_path("a.b[1]")`}},

		// Network tools
		{"subnet_of", 1, 1, "Whether one CIDR block is inside another (supernet)", "IP & Network", []string{`"10.0.0.0/24" | subnet_of("10.0.0.0/8")`}},
		{"cidr_first_host", 0, 2, "The first usable host of a CIDR block", "IP & Network", []string{`"10.0.0.0/24" | cidr_first_host`}},
		{"cidr_last_host", 0, 2, "The last usable host of a CIDR block", "IP & Network", []string{`"10.0.0.0/24" | cidr_last_host`}},
		{"is_public_ip", 0, 2, "Whether an address is not private or reserved", "IP & Network", []string{`"8.8.8.8" | is_public_ip`}},
		{"port_name", 0, 0, "The common service name for a port", "IP & Network", []string{`443 | port_name`}},

		// Similarity tools
		{"similarity_percent", 2, 2, "1 minus normalized Levenshtein distance", "Similarity", []string{`similarity_percent("kitten"; "sitting")`}},
		{"n_grams", 1, 2, "The n-character substrings of a string (n)", "Similarity", []string{`"hello" | n_grams(2)`}},
		{"soundex", 0, 2, "The four-character Soundex code of a word, for matching names that sound alike", "Similarity", []string{`"Robert" | soundex`}},
		{"jaro_winkler", 2, 2, "Jaro-Winkler similarity, favoring shared prefixes", "Similarity", []string{`jaro_winkler("MARTHA"; "MARHTA")`}},

		// Validation tools
		{"is_semver", 0, 2, "Whether a string is a semantic version", "Validation", []string{`"1.2.3" | is_semver`}},
		{"is_credit_card", 0, 2, "Whether digits pass the Luhn checksum", "Validation", []string{`"4111111111111111" | is_credit_card`}},

		// IDNA
		{"punycode_encode", 0, 2, "An internationalized domain to its ASCII (punycode) form", "IDs & Tokens", []string{`"bücher.example" | punycode_encode`}},
		{"punycode_decode", 0, 2, "A punycode domain back to internationalized form", "IDs & Tokens", []string{`"xn--bcher-kva.example" | punycode_decode`}},

		// System lookups (CLI only)
		{"resolve_host", 0, 1, "The addresses a hostname resolves to", "System", []string{`resolve_host("example.com")`}},
		{"reverse_dns", 0, 1, "The hostnames an address points back to", "System", []string{`reverse_dns("8.8.8.8")`}},
		{"which", 0, 1, "The path to an executable on PATH", "System", []string{`which("go")`}},

		// PowerShell - File System
		{"get_childitem", 1, 2, "Get items at a specified location (path, [options])", "PowerShell", []string{`get_childitem(".")`, `get_childitem("src"; {"Recurse": true})`}},
		{"set_content", 2, 2, "Set content of a file (path, value)", "PowerShell", []string{`set_content("file.txt"; "content")`}},
		{"add_content", 1, 3, "Append a value to a file, creating it if absent (path, [value], [options])", "PowerShell", []string{`"a line" | add_content("out.log")`, `add_content("out.log"; "a line")`}},
		{"out_file", 1, 3, "Write a value to a file and pass it on (path, [value], [options])", "PowerShell", []string{`out_file("report.txt")`, `out_file("run.log"; {Append: true})`}},
		{"test_path", 1, 1, "Test if a path exists", "PowerShell", []string{`test_path("file.txt")`, `test_path("/tmp")`}},
		{"join_path", 2, 2, "Join path segments", "PowerShell", []string{`join_path("/tmp"; "file.txt")`}},
		{"split_path", 1, 1, "Split a path into components", "PowerShell", []string{`split_path("/tmp/file.txt")`}},

		// PowerShell - Objects
		{"select_object", 1, 2, "Select object properties (properties, [input])", "PowerShell", []string{`select_object("Name")`, `{"Name":"test"} | select_object("Name")`}},
		{"where_object", 1, 2, "Filter objects by condition (condition, [input])", "PowerShell", []string{`where_object({ . > 10 })`, `[1,5,10,15] | where_object({ . > 10 })`}},
		{"sort_object", 1, 2, "Sort objects by property (property, [input])", "PowerShell", []string{`sort_object("Name")`, `[{"Name":"b"},{"Name":"a"}] | sort_object("Name")`}},
		{"group_object", 1, 2, "Group objects by property (property, [input])", "PowerShell", []string{`group_object("Category")`}},
		{"measure_object", 1, 2, "Measure object properties ([property], [input])", "PowerShell", []string{`measure_object`, `[1,2,3] | measure_object`}},

		// PowerShell - Formatting
		{"format_list", 1, 2, "Format output as a list ([properties], [input])", "PowerShell", []string{`format_list`, `{"Name":"test"} | format_list`}},
		{"format_table", 1, 2, "Format output as a table ([properties], [input])", "PowerShell", []string{`format_table`, `[{"Name":"a"},{"Name":"b"}] | format_table`}},

		// PowerShell - Variables
		{"set_variable", 1, 3, "Set a variable (name, [value], [options])", "PowerShell", []string{`set_variable("count"; 42)`, `set_variable("name"; "test"; {"Scope": "global"})`}},
		{"get_variable", 0, 2, "Get a variable (name, [options])", "PowerShell", []string{`get_variable("count")`, `get_variable("*")`, `get_variable("name"; {"ValueOnly": true})`}},
		{"remove_variable", 1, 2, "Remove a variable (name, [options])", "PowerShell", []string{`remove_variable("temp")`, `remove_variable("*"; {"Exclude": "ErrorActionPreference"})`}},
		// PowerShell - File System (continued)
		{"copy_item", 2, 3, "Copy an item (source, destination, [options])", "PowerShell", []string{`copy_item("a.txt"; "b.txt")`, `copy_item("src"; "dst"; {"Recurse": true})`}},
		{"move_item", 2, 3, "Move an item (source, destination, [options])", "PowerShell", []string{`move_item("a.txt"; "b.txt")`}},
		{"new_item", 1, 3, "Create a file or directory (path, [type], [options])", "PowerShell", []string{`new_item("/tmp/d"; "directory")`, `new_item("/tmp/f.txt"; "file")`}},
		{"resolve_path", 1, 2, "Resolve a path to its absolute form (path, [options])", "PowerShell", []string{`resolve_path("~/..")`, `resolve_path(".")`}},

		// PowerShell - Location
		{"get_location", 0, 1, "Get the current working directory", "PowerShell", []string{`get_location`}},
		{"set_location", 1, 2, "Change the current working directory (path)", "PowerShell", []string{`set_location("/tmp")`}},
		{"push_location", 1, 2, "Push the current location onto the stack and change to path", "PowerShell", []string{`push_location("/tmp")`}},
		{"pop_location", 0, 1, "Pop a location off the stack and change to it", "PowerShell", []string{`pop_location`}},

		// PowerShell - Processes
		{"get_process", 0, 2, "List running processes ([name], [options])", "PowerShell", []string{`get_process`, `[get_process | select(.Name == "go")]`}},
		{"start_process", 1, 2, "Start a process (path, [options])", "PowerShell", []string{`start_process("echo"; {"ArgumentList": ["hi"]})`}},
		{"stop_process", 1, 2, "Stop a process (id or name, [options])", "PowerShell", []string{`stop_process(1234)`}},

		// PowerShell - Services
		{"get_service", 0, 2, "List system services ([name], [options])", "PowerShell", []string{`get_service`, `[get_service | select(.Status == "Running")]`}},
		{"start_service", 1, 2, "Start a service (name, [options])", "PowerShell", []string{`start_service("nginx")`}},
		{"stop_service", 1, 2, "Stop a service (name, [options])", "PowerShell", []string{`stop_service("nginx")`}},

		// PowerShell - Web
		{"invoke_web_request", 1, 2, "Make an HTTP request returning a response object (uri, [options])", "PowerShell", []string{`invoke_web_request("https://example.com")`, `invoke_web_request("https://example.com") | .StatusCode`}},
		{"invoke_rest_method", 1, 2, "Call a REST endpoint, parsing JSON responses (uri, [options])", "PowerShell", []string{`invoke_rest_method("https://api.example.com/items")`}},
		{"test_connection", 1, 2, "Test network connectivity to a host (host, [options])", "PowerShell", []string{`test_connection("example.com")`}},

		// PowerShell - Date and Time
		{"get_date", 0, 2, "Get the current date and time as an object ([options])", "PowerShell", []string{`get_date`, `get_date | .Year`}},
		{"set_date", 1, 2, "Set the system date (requires privileges)", "PowerShell", []string{`set_date("2026-01-01T00:00:00Z")`}},
		{"new_timespan", 0, 2, "Create a timespan from components or two dates", "PowerShell", []string{`new_timespan({"Hours": 2})`}},

		// Discovery
		{"get_command", 0, 2, "List commands, optionally filtered by a wildcard on name or alias", "Discovery", []string{`get_command`, `get_command("get_*")`, `[get_command | select(.Category == "PowerShell") | .Name]`}},
		{"get_help", 0, 2, "Show usage for the commands matching a name or alias", "Discovery", []string{`get_help("get_childitem")`, `"gci" | get_help`}},

		// Grouping and summarising (fourth round)
		{"group_by_key", 1, 2, "Bucket an array of objects by a property into {value: [rows]}", "Collections", []string{`[{"dept":"eng"},{"dept":"ops"}] | group_by_key("dept")`}},
		{"count_by", 1, 2, "A property's value to a row count", "Collections", []string{`[{"dept":"eng"},{"dept":"eng"}] | count_by("dept")`}},
		{"sum_by", 1, 3, "A numeric column summed per key value (key, [column])", "Collections", []string{`[{"d":"eng","pay":90}] | sum_by("d"; "pay")`}},
		{"avg_by", 1, 3, "A numeric column averaged per key value (key, [column])", "Collections", []string{`[{"d":"eng","pay":90},{"d":"eng","pay":110}] | avg_by("d"; "pay")`}},
		{"index_by", 1, 2, "The first row seen per key value", "Collections", []string{`[{"d":"eng","id":1},{"d":"eng","id":2}] | index_by("d")`}},
		{"value_counts", 0, 1, "Whole-value frequencies of an array as {value: count}", "Collections", []string{`["a","b","a"] | value_counts`}},
		{"summarize_by", 1, 3, "count, sum, avg, min and max per key value (key, [column])", "Collections", []string{`[{"d":"eng","pay":90},{"d":"eng","pay":110}] | summarize_by("d"; "pay")`}},
		{"pivot", 1, 2, "A row x column table: {rows, cols, values}", "Collections", []string{`[{"dept":"eng","year":2020,"amt":10}] | pivot({rows: "dept", cols: "year", values: "amt"})`}},
		{"unpivot", 1, 2, "Melt value columns into {key, value} rows: {cols, [id]}", "Collections", []string{`[{"dept":"eng","y2020":10}] | unpivot({cols: ["y2020"], id: "dept"})`}},
		{"top_by", 1, 3, "The n rows with the largest value of a numeric property", "Collections", []string{`[{"d":"a","s":1},{"d":"b","s":9}] | top_by("s"; 1)`}},
		{"bottom_by", 1, 3, "The n rows with the smallest value of a numeric property", "Collections", []string{`[{"d":"a","s":1},{"d":"b","s":9}] | bottom_by("s"; 1)`}},

		// Units (fourth round)
		{"convert_unit", 2, 3, "Convert a number between two units of the same quantity (from, to)", "Domain", []string{`20 | convert_unit("C"; "F")`, `convert_unit(5; "mi"; "km")`, `90 | convert_unit("min"; "h")`}},
		{"parse_size", 0, 1, "A size string like 1.5 MiB to its byte count (binary units)", "Domain", []string{`"1.5 MiB" | parse_size`}},

		// Geo (fourth round)
		{"haversine_distance", 4, 4, "Great-circle distance in km between two coordinates (lat1, lon1, lat2, lon2)", "Domain", []string{`haversine_distance(51.5007; -0.1246; 40.7128; -74.0060)`}},
		{"bearing", 4, 4, "Compass bearing in degrees from one coordinate to another", "Domain", []string{`bearing(37.7749; -122.4194; 34.0522; -118.2437)`}},
		{"geo_midpoint", 4, 4, "The halfway point of the great-circle arc between two coordinates", "Domain", []string{`geo_midpoint(0; 0; 0; 10)`}},
		{"within_radius", 5, 5, "Whether a point is within km of a centre (lat, lon, clat, clon, km)", "Domain", []string{`within_radius(51.5; -0.12; 51.5007; -0.1246; 1)`}},
		{"parse_coords", 0, 1, "A lat, lon string to {lat, lon}", "Domain", []string{`parse_coords("51.5007, -0.1246")`}},
		{"geohash_encode", 2, 3, "A coordinate to a geohash string (lat, lon, [precision])", "Domain", []string{`geohash_encode(42.6; -5.6; 6)`}},
		{"geohash_decode", 0, 1, "A geohash to its centre {lat, lon, latErr, lonErr}", "Domain", []string{`"ezs42" | geohash_decode`}},

		// Finance (fourth round)
		{"cagr", 3, 3, "Compound annual growth rate (start, end, years)", "Domain", []string{`cagr(100; 200; 1)`}},
		{"future_value", 3, 3, "Principal grown over periods at a rate (principal, rate, periods)", "Domain", []string{`future_value(100; 0.1; 10)`}},
		{"present_value", 3, 3, "The principal that grows to a target (target, rate, periods)", "Domain", []string{`present_value(259.37; 0.1; 10)`}},
		{"monthly_payment", 3, 3, "Fixed monthly payment amortizing a loan (principal, annualRate, months)", "Domain", []string{`monthly_payment(10000; 0.06; 36)`}},
		{"compound_interest", 3, 3, "Interest earned over periods at a rate (principal, rate, periods)", "Domain", []string{`compound_interest(100; 0.1; 10)`}},
		{"simple_interest", 3, 3, "Interest on the principal only (principal, rate, years)", "Domain", []string{`simple_interest(100; 0.1; 3)`}},

		// Time series and relations (fourth round)
		{"cumsum", 0, 1, "The running total of an array", "Statistics", []string{`[1,2,3] | cumsum`}},
		{"cumulative_max", 0, 1, "The largest value seen so far at each position", "Statistics", []string{`[3,1,4,2] | cumulative_max`}},
		{"cumulative_min", 0, 1, "The smallest value seen so far at each position", "Statistics", []string{`[3,1,4,2] | cumulative_min`}},
		{"deltas", 0, 1, "First differences between consecutive values", "Statistics", []string{`[1,4,9,2] | deltas`}},
		{"lag", 1, 2, "Shift an array right by n, filling with null (n)", "Statistics", []string{`[1,2,3] | lag(1)`}},
		{"fill_forward", 0, 1, "Carry the last non-null value forward over nulls", "Statistics", []string{`[1,null,2] | fill_forward`}},
		{"ema", 1, 2, "Exponential moving average with smoothing alpha", "Statistics", []string{`[1,2,3] | ema(0.5)`}},
		{"moving_max", 1, 2, "The largest value in each window of n", "Statistics", []string{`[3,1,4,1,5] | moving_max(3)`}},
		{"moving_min", 1, 2, "The smallest value in each window of n", "Statistics", []string{`[3,1,4,1,5] | moving_min(3)`}},
		{"correlation", 1, 2, "Pearson correlation of two equal-length arrays", "Statistics", []string{`[1,2,3] | correlation([2,4,6])`, `correlation([1,2,3]; [2,4,6])`}},
		{"covariance", 1, 2, "Sample covariance of two equal-length arrays", "Statistics", []string{`[1,2,3] | covariance([2,4,6])`}},
		{"skewness", 0, 1, "Sample skewness of an array", "Statistics", []string{`[1,1,1,1,10] | skewness`}},
		{"kurtosis", 0, 1, "Sample excess kurtosis of an array", "Statistics", []string{`[1,2,3,4,5,6,7,8] | kurtosis`}},
		{"weighted_mean", 1, 2, "Mean of values weighted by parallel weights", "Statistics", []string{`[1,2,3] | weighted_mean([1,1,1])`, `weighted_mean([1,2,3]; [1,1,1])`}},
		{"harmonic_mean", 0, 1, "Harmonic mean of an array of positive numbers", "Statistics", []string{`[1,2,4] | harmonic_mean`}},
		{"quartiles", 0, 1, "Five-number summary [min, q1, median, q3, max]", "Statistics", []string{`[1,2,3,4,5,6,7,8] | quartiles`}},
		{"trimmed_mean", 1, 2, "Mean with a fraction trimmed from each end", "Statistics", []string{`[1,2,3,100] | trimmed_mean(0.25)`}},
		{"standardize", 0, 1, "Each value as its z-score", "Statistics", []string{`[1,2,3] | standardize`}},
		{"rms", 0, 1, "Root mean square of an array", "Statistics", []string{`[1,2,3,4] | rms`}},
		{"product", 0, 1, "The product of an array's numbers", "Statistics", []string{`[1,2,3] | product`}},

		// Regex (fourth round)

		// Text tools (fourth round)
		{"reverse_words", 0, 2, "The words of a string in reverse order", "String", []string{`"the quick brown" | reverse_words`}},
		{"truncate_words", 1, 2, "A string cut to at most n words (n)", "String", []string{`"the quick brown" | truncate_words(2)`}},
		{"remove_accents", 0, 2, "Diacritics stripped from a string", "String", []string{`"café" | remove_accents`}},
		{"sentence_case", 0, 2, "First letter capitalized, the rest lowercased", "String", []string{`"hELLO World" | sentence_case`}},
		{"line_count", 0, 2, "How many lines a string has", "String", []string{`"a\nb\nc" | line_count`}},
		{"dedent", 0, 2, "Common leading whitespace removed from every line", "String", []string{`"  a\n    b" | dedent`}},
		{"swap_case", 0, 2, "Uppercase and lowercase letters exchanged", "String", []string{`"HeLLo" | swap_case`}},
		{"char_frequencies", 0, 2, "How often each character appears, as {char: count}", "String", []string{`"aab" | char_frequencies`}},
		{"reverse_lines", 0, 2, "The lines of a string reversed", "String", []string{`"a\nb\nc" | reverse_lines`}},
		{"unique_lines", 0, 2, "Duplicate lines removed keeping first-occurrence order", "String", []string{`"b\na\nb" | unique_lines`}},
		{"sort_lines", 0, 2, "The lines of a string sorted", "String", []string{`"b\na\nc" | sort_lines`}},
		{"strip_quotes", 0, 2, "One layer of matching quotes removed from each end", "String", []string{`"\"quoted\"" | strip_quotes`}},
		{"pad_center", 1, 2, "A string centered in a field of n characters (n, [pad])", "String", []string{`"hi" | pad_center(5)`}},

		// Paths (fourth round)
		{"normalize_path", 0, 1, "A path with redundant separators and .. resolved lexically", "Paths", []string{`"/a//b/../c" | normalize_path`}},
		{"relative_path", 1, 2, "Target expressed relative to base", "Paths", []string{`"/a/b/c" | relative_path("/a")`}},
		{"stem", 0, 1, "The file name without its extension", "Paths", []string{`"/tmp/report.pdf" | stem`}},
		{"with_extension", 1, 2, "A file name with its extension replaced (ext)", "Paths", []string{`"c.txt" | with_extension(".md")`}},
		{"has_extension", 0, 1, "Whether a path's final component carries an extension", "Paths", []string{`"a.txt" | has_extension`}},
		{"is_dir_path", 0, 1, "Whether a path ends in a directory separator", "Paths", []string{`"/a/b/" | is_dir_path`}},
		{"path_sep", 0, 0, "The OS path separator", "Paths", []string{`path_sep`}},
		{"expand_home", 0, 1, "~ and ~/... expanded to the home directory (CLI only)", "Paths", []string{`"~/x" | expand_home`}},
		{"home_dir", 0, 0, "The current user's home directory (CLI only)", "Paths", []string{`home_dir`}},

		// Validation extras (fourth round)
		{"semver_compare", 1, 2, "-1, 0 or 1 comparing two semantic versions", "Validation", []string{`semver_compare("1.2.3"; "2.0.0")`, `"1.2.3" | semver_compare("2.0.0")`}},
		{"semver_parts", 0, 1, "A semantic version as {major, minor, patch, prerelease, build}", "Validation", []string{`"2.1.3-rc.1+build5" | semver_parts`}},
		{"is_hex", 0, 2, "Whether a string is a hexadecimal number", "Validation", []string{`"ff00" | is_hex`}},
		{"is_cidr", 0, 2, "Whether a string is an IP/CIDR block", "Validation", []string{`"10.0.0.0/8" | is_cidr`}},
		{"is_port", 0, 1, "Whether a number is a valid port (1-65535)", "Validation", []string{`443 | is_port`}},
		{"is_date", 0, 2, "Whether a string is a YYYY-MM-DD date", "Validation", []string{`"2026-08-10" | is_date`}},
		{"is_iso8601", 0, 2, "Whether a string is an ISO 8601 timestamp or date", "Validation", []string{`"2026-08-10T12:34:56Z" | is_iso8601`}},
		{"is_slug", 0, 2, "Whether a string is lowercase letters, digits and hyphens", "Validation", []string{`"my-cool-slug-2" | is_slug`}},
		{"extract_dates", 0, 2, "Every YYYY-MM-DD date in a string", "Validation", []string{`"born 2024-01-02" | extract_dates`}},

		// Set and slice operations (fourth round)
		{"intersection", 1, 2, "The elements present in both arrays", "Collections", []string{`[1,2,3] | intersection([2,3,4])`}},
		{"union", 1, 2, "The unique elements of both arrays", "Collections", []string{`[1,2] | union([2,3])`}},
		{"difference", 1, 2, "The elements of the first array not in the second", "Collections", []string{`[1,2,3] | difference([2])`}},
		{"symmetric_difference", 1, 2, "The elements in exactly one of two arrays", "Collections", []string{`[1,2] | symmetric_difference([2,3])`}},
		{"all_equal", 0, 1, "Whether every element of an array is the same value", "Collections", []string{`[1,1,1] | all_equal`}},
		{"contains_duplicates", 0, 1, "Whether any value appears more than once", "Collections", []string{`[1,2,1] | contains_duplicates`}},
		{"cartesian", 1, 2, "The array of [a, b] pairs from two arrays", "Collections", []string{`cartesian([1,2]; ["a","b"])`}},
		{"column", 1, 2, "The nth element of every row (n)", "Collections", []string{`[[1,2],[3,4]] | column(1)`}},
		{"lookup", 2, 3, "The first row whose property equals a value (key, value)", "Collections", []string{`[{"name":"ada"}] | lookup("name"; "ada")`}},
		{"compare_object", 1, 3, "What differs between two collections, each value tagged with its side (difference, [options])", "Collections", []string{`compare_object(["a","b"]; ["b","c"])`, `compare_object($old; $new; {Property: "id", IncludeEqual: true})`}},
		{"natural_sort", 0, 1, "An array in human order, file2 before file10", "Collections", []string{`["file2","file10","file1"] | natural_sort`}},

		// Number theory (fourth round)
		{"sign", 0, 1, "-1, 0 or 1 for a number", "Numbers", []string{`-5 | sign`}},
		{"next_prime", 0, 1, "The smallest prime at least n", "Numbers", []string{`14 | next_prime`}},
		{"prime_factors", 0, 1, "The prime factors of an integer with multiplicity", "Numbers", []string{`60 | prime_factors`}},

		// Config formats (fourth round, part two)
		{"ini_parse", 0, 1, "An INI document to an object of sections", "Config", []string{`"[server]\nhost=10.0.0.1" | ini_parse`}},
		{"ini_stringify", 0, 1, "An object to an INI document", "Config", []string{`{server: {host: "10.0.0.1"}} | ini_stringify`}},
		{"properties_parse", 0, 1, "A .env / Java properties document to an object", "Config", []string{`"DB_HOST=db.internal" | properties_parse`}},
		{"properties_stringify", 0, 1, "An object to a .env / properties document", "Config", []string{`{DB_HOST: "db.internal"} | properties_stringify`}},
		{"logfmt_parse", 0, 1, "A logfmt line to an object, numbers and booleans typed", "Config", []string{`"level=info n=3 ok=true" | logfmt_parse`}},
		{"logfmt_stringify", 0, 1, "An object to a logfmt line", "Config", []string{`{level: "info", n: 3} | logfmt_stringify`}},

		// Text extras (fourth round, part two)
		{"before_first", 1, 1, "The part of a string before the first separator (sep)", "String", []string{`"user@example.com" | before_first("@")`}},
		{"after_first", 1, 1, "The part of a string after the first separator (sep)", "String", []string{`"user@example.com" | after_first("@")`}},
		{"surround", 2, 2, "A string wrapped in a prefix and suffix", "String", []string{`"x" | surround("[ "; " ]")`}},
		{"count_vowels", 0, 2, "How many vowel letters a string has", "String", []string{`"hello" | count_vowels`}},
		{"count_consonants", 0, 2, "How many consonant letters a string has", "String", []string{`"hello" | count_consonants`}},
		{"capitalize_first", 0, 2, "The first letter uppercased, the rest untouched", "String", []string{`"hELLO" | capitalize_first`}},
		{"unicode_escape", 0, 2, "Non-ASCII and control characters as \\uXXXX", "String", []string{`"héllo" | unicode_escape`}},
		{"unicode_unescape", 0, 2, "\\uXXXX escapes rendered back into characters", "String", []string{`"h\\u00e9llo" | unicode_unescape`}},
		{"diff_lines", 1, 2, "Which lines are in only one of two texts: {added, removed}", "String", []string{`"a\nb" | diff_lines("b\nc")`}},

		// Object helpers (fourth round, part two)
		{"rename_keys", 1, 2, "An object's keys renamed by a {old: new} mapping", "Collections", []string{`{"a":1} | rename_keys({"a": "x"})`}},

		// Statistics extras (fourth round, part two)
		{"autocorrelation", 0, 2, "Correlation of an array with itself lag steps back", "Statistics", []string{`[1,2,3,4,5] | autocorrelation`, `[1,2,3,4,5] | autocorrelation(2)`}},
		{"iqr", 0, 1, "The interquartile range (q3 - q1)", "Statistics", []string{`[1,2,3,4,5,6,7,8] | iqr`}},
		{"mad", 0, 1, "Median absolute deviation", "Statistics", []string{`[1,2,3,4,5] | mad`}},
		{"moving_stdev", 1, 2, "The rolling sample standard deviation over a window of n", "Statistics", []string{`[1,2,3,4,5] | moving_stdev(2)`}},

		// Duration extras (fourth round, part two)
		{"days_between", 1, 1, "Calendar days between two dates or timestamps", "Duration", []string{`"2026-01-01" | days_between("2026-01-10")`}},
		{"day_of_year", 0, 0, "The day number within the year (1-366)", "Duration", []string{`"2026-08-10" | day_of_year`}},
		{"week_of_year", 0, 0, "The ISO 8601 week number (1-53)", "Duration", []string{`"2026-08-10" | week_of_year`}},
		{"start_of_week", 0, 0, "A timestamp at the Monday midnight of its ISO week", "Duration", []string{`"2026-08-13T12:00:00Z" | start_of_week`}},
		{"add_months", 1, 1, "A timestamp plus n months", "Duration", []string{`"2026-01-15" | add_months(1)`}},
		{"add_years", 1, 1, "A timestamp plus n years", "Duration", []string{`"2026-01-15" | add_years(2)`}},
		{"age_in_years", 0, 1, "Whole years between a birth date and now (or a given timestamp)", "Duration", []string{`"2000-06-15" | age_in_years`}},

		// Number extras (fourth round, part two)
		{"to_fixed", 1, 1, "A number with a fixed number of decimal places (places)", "Numbers", []string{`3.14159 | to_fixed(2)`}},
		{"is_power_of_two", 0, 1, "Whether a positive integer is a power of two", "Numbers", []string{`16 | is_power_of_two`}},

		// Third batch of the fourth round
		{"quoted_printable_encode", 0, 2, "Text as MIME quoted-printable, for email bodies", "String", []string{`"hello=world" | quoted_printable_encode`}},
		{"quoted_printable_decode", 0, 2, "MIME quoted-printable back to text", "String", []string{`"hello=3Dworld" | quoted_printable_decode`}},
		{"prefix_lines", 1, 1, "Every line prefixed (prefix)", "String", []string{`"a\nb" | prefix_lines("> ")`}},
		{"first_lines", 1, 1, "The first n lines of a string (n)", "String", []string{`"a\nb\nc" | first_lines(2)`}},
		{"last_lines", 1, 1, "The last n lines of a string (n)", "String", []string{`"a\nb\nc" | last_lines(2)`}},
		{"is_balanced", 0, 2, "Whether () [] {} nest without mismatching", "String", []string{`"(a[b])" | is_balanced`}},
		{"tsv_parse", 0, 1, "A tab-separated document to an array of rows", "CSV", []string{`"a\tb\n1\t2" | tsv_parse`}},
		{"tsv_stringify", 0, 1, "An array of rows to a tab-separated document", "CSV", []string{`[["a","b"],["1","2"]] | tsv_stringify`}},
		{"windows", 1, 2, "The rolling n-element windows of an array (n)", "Collections", []string{`[1,2,3,4] | windows(3)`}},
		{"set_path", 2, 3, "The document with a value written at a dot-and-bracket path", "JSON", []string{`{a: 1} | set_path("a"; 2)`, `{a: {b: [1,2]}} | set_path("a.b[1]"; 9)`}},
		{"has_path", 1, 2, "Whether a dot-and-bracket path exists in a document", "JSON", []string{`{a: 1} | has_path("a")`}},
		{"del_path", 1, 2, "The document with a value removed at a dot-and-bracket path", "JSON", []string{`{a: {b: 1, c: 2}} | del_path("a.b")`}},
		{"group_digits", 0, 1, "An integer with thousands separators", "Numbers", []string{`1234567 | group_digits`}},
		{"format_currency", 0, 1, "A number as a currency string (symbol)", "Numbers", []string{`1234.5 | format_currency`, `1234.5 | format_currency("€")`}},
		{"is_numeric", 0, 2, "Whether a string parses as a number", "Validation", []string{`"-1.5e3" | is_numeric`}},
		{"percentile_rank", 1, 2, "The percentage of an array's values at or below a value", "Statistics", []string{`[1,2,3,4,5] | percentile_rank(3)`}},
		{"net_present_value", 2, 2, "Present value of cash flows at a rate (flows, rate)", "Domain", []string{`net_present_value([-100, 50, 60]; 0.1)`}},
		{"iso_duration", 0, 0, "Seconds as an ISO 8601 duration", "Duration", []string{`93784 | iso_duration`}},

		// Censys Platform. Every one of these authenticates with
		// CENSYS_PLATFORM_TOKEN and bills the wallet named by
		// CENSYS_PLATFORM_ORGID; get_censys_context reports what it resolved.
		{"get_censys_context", 0, 1, "Which credentials and server the Censys cmdlets resolved, without the token", "Censys", []string{`get_censys_context`, `get_censys_context | .OrganizationId`}},
		{"get_censys_host", 0, 2, "A host by IP (AtTime for a point in time)", "Censys", []string{`get_censys_host("1.1.1.1")`, `"1.1.1.1" | get_censys_host({AtTime: "2026-01-01T00:00:00Z"})`}},
		{"get_censys_certificate", 0, 2, "A certificate by SHA-256 fingerprint (Raw for the PEM)", "Censys", []string{`get_censys_certificate($fp)`, `get_censys_certificate($fp; {Raw: true})`}},
		{"get_censys_webproperty", 0, 2, "A web property by hostname:port (AtTime for a point in time)", "Censys", []string{`get_censys_webproperty("example.com:443")`}},
		{"get_censys_enrichment", 0, 2, "The lightweight enrichment record for a host IP", "Censys", []string{`"1.1.1.1" | get_censys_enrichment`}},
		{"get_censys_host_timeline", 0, 2, "One object per host event (StartTime is the recent end, EndTime the older)", "Censys", []string{`[get_censys_host_timeline("1.1.1.1")] | length`}},
		{"get_censys_webproperty_timeline", 0, 2, "One object per web property event", "Censys", []string{`[get_censys_webproperty_timeline("example.com:443")] | length`}},
		{"get_censys_host_service", 0, 2, "One object per observed service range on a host (Port, Protocol, Pages)", "Censys", []string{`[get_censys_host_service("1.1.1.1"; {Port: 443})]`}},
		{"search_censys", 0, 2, "One object per CenQL search hit (Fields, PageSize, Pages, CollectionId)", "Censys", []string{`[search_censys("host.services.port=22")] | length`, `search_censys("host.location.country=\"Chile\""; {Pages: 3})`}},
		{"get_censys_aggregate", 1, 3, "Bucket counts for a CenQL query (query, field; Buckets, CollectionId)", "Censys", []string{`get_censys_aggregate("host.services.port=443"; "host.location.country")`}},
		{"get_censys_collection", 0, 2, "A collection by UID, or every collection when given none", "Censys", []string{`[get_censys_collection] | map(.name)`, `get_censys_collection($uid)`}},
		{"new_censys_collection", 0, 1, "Create a collection (Name, Query, Description)", "Censys", []string{`new_censys_collection({Name: "edge", Query: "host.services.port=8080"})`}},
		{"set_censys_collection", 1, 2, "Replace a collection's name, query and description", "Censys", []string{`set_censys_collection($uid; {Name: "edge", Query: "host.services.port=8443"})`}},
		{"remove_censys_collection", 0, 2, "Delete a collection, returning its UID", "Censys", []string{`remove_censys_collection($uid)`}},
		{"get_censys_collection_event", 0, 2, "One object per change in a collection (ChangeType, StartTime, Pages)", "Censys", []string{`[get_censys_collection_event($uid)] | length`}},
		{"new_censys_censeye_job", 0, 2, "Start a CensEye pivot analysis on an asset (Type: host, webproperty, certificate)", "Censys", []string{`new_censys_censeye_job("1.1.1.1")`}},
		{"get_censys_censeye_job", 0, 2, "A CensEye job by ID, or every job when given none", "Censys", []string{`get_censys_censeye_job($id) | .status`, `[get_censys_censeye_job({HostId: "1.1.1.1"})]`}},
		{"get_censys_censeye_result", 0, 2, "One object per pivot a finished CensEye job found", "Censys", []string{`[get_censys_censeye_result($id)] | length`}},
		{"get_censys_threat", 0, 2, "One object per tracked threat, optionally filtered by a CenQL query", "Censys", []string{`[get_censys_threat] | length`}},
		{"get_censys_tag", 0, 2, "A tag by ID or name, or every tag when given none (Privacy, OrderBy, Pages)", "Censys", []string{`[get_censys_tag] | map(.name)`}},
		{"new_censys_tag", 0, 1, "Create a tag (Name, Description, Privacy: shared or private)", "Censys", []string{`new_censys_tag({Name: "compromised"})`}},
		{"set_censys_tag", 1, 2, "Change a tag's name, description or privacy", "Censys", []string{`set_censys_tag($id; {Description: "seen in incident 42"})`}},
		{"remove_censys_tag", 0, 2, "Delete a tag, returning its ID", "Censys", []string{`remove_censys_tag($id)`}},
		{"get_censys_tag_assignment", 0, 2, "One object per asset a tag is attached to (AssetType, Pages)", "Censys", []string{`[get_censys_tag_assignment($id)] | map(.asset_id)`}},
		{"add_censys_tag_assignment", 1, 3, "Attach a tag to an asset (tag; asset, or the asset piped in)", "Censys", []string{`add_censys_tag_assignment($tag; "1.1.1.1")`, `search_censys("...") | .host.ip | add_censys_tag_assignment($tag)`}},
		{"remove_censys_tag_assignment", 1, 3, "Detach a tag by assignment ID, returning that ID", "Censys", []string{`remove_censys_tag_assignment($tag; $assignment)`}},
		{"get_censys_organization", 0, 2, "Organization details (IncludeMemberCounts)", "Censys", []string{`get_censys_organization`, `get_censys_organization($orgId; {IncludeMemberCounts: true})`}},
		{"get_censys_credits", 0, 1, "The credit balance being spent (Scope: user or organization)", "Censys", []string{`get_censys_credits`, `get_censys_credits({Scope: "user"})`}},
	}
}
