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
		{"cat", 0, 1, "Read and return contents of a file (filepath from pipe or argument)", "File Operations", []string{`cat("file.txt")`, `"file.txt" | cat`, `find("."; "file") | cat`}},
		{"mkdir", 1, 1, "Create a directory (creates parent directories if needed)", "File Operations", []string{`mkdir("/tmp/mydir")`, `mkdir("nested/path/to/dir")`}},
		{"rm", 2, 2, "Remove a file or folder (path, type: 'file' or 'folder')", "File Operations", []string{`rm("/tmp/file.txt"; "file")`, `rm("/tmp/mydir"; "folder")`}},

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
		{"upper", 0, 2, "Convert to uppercase (optional file arg)", "String", []string{`upper`, `upper(true)`}},
		{"lower", 0, 2, "Convert to lowercase (optional file arg)", "String", []string{`lower`, `lower(true)`}},
		{"reverse_string", 0, 2, "Reverse string (optional file arg)", "String", []string{`reverse_string`, `reverse_string(true)`}},
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
		{"http", 0, 2, "Make HTTP request (method default POST, url required)", "HTTP", []string{`http("https://example.com")`, `"https://example.com" | http`, `http("GET"; "https://example.com")`, `{"key":"value"} | http("POST"; "https://api.example.com")`}},
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
		{"wc_lines", 0, 1, "The number of lines in a file", "File Operations", []string{`wc_lines("app.log")`}},

		// PowerShell - File System
		{"get_childitem", 1, 2, "Get items at a specified location (path, [options])", "PowerShell", []string{`get_childitem(".")`, `get_childitem("src"; {"Recurse": true})`}},
		{"set_content", 2, 2, "Set content of a file (path, value)", "PowerShell", []string{`set_content("file.txt"; "content")`}},
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
	}
}
