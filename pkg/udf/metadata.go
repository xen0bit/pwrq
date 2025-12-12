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
		{"trim", 0, 2, "Trim whitespace (optional file arg)", "String", []string{`trim`, `trim(true)`}},
		{"split", 1, 3, "Split string by separator (separator, [input], [file])", "String", []string{`split(",")`, `split(","; "a,b,c")`}},
		{"join_string", 1, 1, "Join array with separator (separator)", "String", []string{`join_string(",")`, `["a","b"] | join_string(",")`}},
		
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
	}
}

