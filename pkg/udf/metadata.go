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
