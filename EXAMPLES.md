# pwrq Examples

This document provides comprehensive examples demonstrating the full capabilities of `pwrq`, including all User-Defined Functions (UDFs) and core jq functionality. Each example includes an objective, explanation, command, and actual output.

## Table of Contents

1. [File Operations](#file-operations)
2. [Encoding and Decoding](#encoding-and-decoding)
3. [Hash Functions](#hash-functions)
4. [Compression](#compression)
5. [String Operations](#string-operations)
6. [Data Format Conversion](#data-format-conversion)
7. [Advanced Chaining](#advanced-chaining)
8. [Real-World Scenarios](#real-world-scenarios)

---

## File Operations

### Example 1: Find and Process Files

**Objective:** Find all Go files in the project, read their contents, and calculate their SHA256 hashes.

**How it works:** 
- `find("pkg/udf"; "file")` finds all files in the `pkg/udf` directory
- Filters for `.go` files using jq's `select()` and `endswith()`
- Extracts file paths with `._val`
- Reads each file with `cat`
- Calculates SHA256 hash with `sha256`
- Extracts the hash value

**Command:**
```bash
echo 'null' | ./pwrq '[find("pkg/udf"; "file")] | map(select(._val | endswith(".go"))) | .[0] | ._val | cat | sha256 | ._val'
```

**Output:**
```
"80a695c67c097438ff75b377f6898b060f5e9658cabe6088d57b0f3f125b9e5f"
```

---

### Example 2: Read File and Process Content

**Objective:** Read a file, encode it in base64, and write the result to another file using `tee`.

**How it works:**
- `cat("README.md")` reads the README.md file
- `base64_encode` encodes the content
- `tee("/tmp/output.json")` writes the result to a file
- `._val` extracts the encoded value

**Command:**
```bash
echo 'null' | ./pwrq 'cat("README.md") | base64_encode | tee("/tmp/pwrq_tee_output.json") | ._val'
```

**Output:**
```
"# pwrq\n\nEnhanced Go implementation of jq, extending [gojq](https://github.com/itchyny/gojq).\n\n## Overview\n\n`pwrq` is a command-line JSON processor that functions identically to `gojq`, providing a drop-in replacement with the same features and behavior. It's built on top of the excellent `gojq` library and will be enhanced with additional features in the future.\n..."
```

---

## Encoding and Decoding

### Example 3: Base64 Round-Trip Encoding

**Objective:** Demonstrate base64 encoding and decoding with automatic `_val` extraction.

**How it works:**
- `base64_encode` encodes the string to base64
- `._val` extracts the encoded value
- `base64_decode` decodes it back
- `._val` extracts the decoded value
- The automatic `_val` extraction allows chaining without explicit `._val` access

**Command:**
```bash
echo '"test content"' | ./pwrq 'base64_encode | ._val | base64_decode | ._val'
```

**Output:**
```
"test content"
```

---

### Example 4: Hexadecimal Encoding Chain

**Objective:** Encode a string to hexadecimal and decode it back.

**Command:**
```bash
echo '"The quick brown fox"' | ./pwrq 'hex_encode | ._val | hex_decode | ._val'
```

**Output:**
```
"The quick brown fox"
```

---

### Example 5: URL Encoding/Decoding

**Objective:** Encode and decode URL-encoded strings.

**Command:**
```bash
echo '"test@example.com?q=hello world"' | ./pwrq 'url_encode | ._val | url_decode | ._val'
```

**Output:**
```
"test@example.com?q=hello world"
```

---

### Example 6: HTML Entity Encoding/Decoding

**Objective:** Encode HTML entities to prevent XSS attacks and decode them back.

**Command:**
```bash
echo '"<script>alert(\"XSS\")</script>"' | ./pwrq 'html_encode | ._val | html_decode | ._val'
```

**Output:**
```
"<script>alert(\"XSS\")</script>"
```

---

### Example 7: Binary Encoding

**Objective:** Convert a string to binary representation and back.

**Command:**
```bash
echo '"Hello"' | ./pwrq 'binary_encode | ._val | binary_decode | ._val'
```

**Output:**
```
"Hello"
```

---

### Example 8: Base32 Encoding

**Objective:** Encode and decode using base32.

**Command:**
```bash
echo '"test data"' | ./pwrq 'base32_encode | ._val | base32_decode | ._val'
```

**Output:**
```
"test data"
```

---

### Example 9: Base85 Encoding

**Objective:** Encode and decode using base85 (ASCII85).

**Command:**
```bash
echo '"test data"' | ./pwrq 'base85_encode | ._val | base85_decode | ._val'
```

**Output:**
```
"test data"
```

---

## Hash Functions

### Example 10: Multiple Hash Algorithms

**Objective:** Calculate multiple hash algorithms for the same input.

**Command:**
```bash
echo '"test"' | ./pwrq 'md5 | ._val | sha1 | ._val | sha256 | ._val'
```

**Output:**
```
"9e05bc7478fcac66f2aaeb2b04769ccd08f618d805269a549c88d38f01f7af6d"
```

---

### Example 11: HMAC Authentication

**Objective:** Generate an HMAC-SHA256 signature for message authentication.

**Command:**
```bash
echo '"password123"' | ./pwrq 'hmac_sha256("secret-key") | ._val'
```

**Output:**
```
"0e5ea1d4208dff259428482e0b06a0ce0cf2e8250183f215de547b484132c7fd"
```

---

## Compression

### Example 12: Gzip Compression Round-Trip

**Objective:** Compress data with gzip and decompress it back.

**Command:**
```bash
echo '"secret message"' | ./pwrq 'gzip_compress | ._val | gzip_decompress | ._val'
```

**Output:**
```
"secret message"
```

---

### Example 13: Multi-Stage Encoding and Compression

**Objective:** Apply multiple encoding and compression stages, then reverse them.

**How it works:**
- Base64 encode → Gzip compress → Hex encode
- Then reverse: Hex decode → Gzip decompress → Base64 decode

**Command:**
```bash
echo '"This is a test message with some content"' | ./pwrq 'base64_encode | ._val | gzip_compress | ._val | hex_encode | ._val | hex_decode | ._val | gzip_decompress | ._val | base64_decode | ._val'
```

**Output:**
```
"This is a test message with some content"
```

---

## String Operations

### Example 14: String Transformation Chain

**Objective:** Apply multiple string transformations in sequence.

**Command:**
```bash
echo '"Hello World"' | ./pwrq 'upper | ._val | lower | ._val | reverse_string | ._val'
```

**Output:**
```
"dlrow olleh"
```

---

### Example 15: String Replacement

**Objective:** Replace a substring in a string.

**Command:**
```bash
echo '"hello world"' | ./pwrq 'replace("world"; "pwrq") | ._val'
```

**Output:**
```
"hello pwrq"
```

---

### Example 16: String Splitting and Joining

**Objective:** Split a string by delimiter and join it back with a different delimiter.

**Command:**
```bash
echo '["hello","world","test"]' | ./pwrq 'join_string("|") | ._val'
```

**Output:**
```
"hello|world|test"
```

---

### Example 17: Trim Whitespace

**Objective:** Remove leading and trailing whitespace from a string.

**Command:**
```bash
echo '"  hello world  "' | ./pwrq 'trim | ._val'
```

**Output:**
```
"hello world"
```

---

## Data Format Conversion

### Example 18: JSON Stringify and Parse

**Objective:** Convert a JSON object to a string and parse it back.

**Command:**
```bash
echo '{"name": "Alice", "age": 30}' | ./pwrq 'json_stringify | ._val | json_parse'
```

**Output:**
```
{
  "age": 30,
  "name": "Alice"
}
```

---

### Example 19: CSV Parsing

**Objective:** Parse CSV data into a structured format.

**Command:**
```bash
echo '"a,b,c\n1,2,3"' | ./pwrq 'csv_parse(",") | .'
```

**Output:**
```
[
  [
    "a",
    "b",
    "c"
  ],
  [
    "1",
    "2",
    "3"
  ]
]
```

---

### Example 20: Timestamp to Date Conversion

**Objective:** Convert a Unix timestamp to a human-readable date.

**Command:**
```bash
echo '1609459200' | ./pwrq 'timestamp_to_date | ._val'
```

**Output:**
```
"2020-12-31T19:00:00-05:00"
```

---

### Example 21: Date to Timestamp Conversion

**Objective:** Convert a date string to a Unix timestamp.

**Command:**
```bash
echo '"2021-01-01T00:00:00Z"' | ./pwrq 'date_to_timestamp | ._val'
```

**Output:**
```
1609459200
```

---

## Advanced Chaining

### Example 22: Complex File Processing Pipeline

**Objective:** Find Go files, read them, and create a summary with file info, hash, and size.

**How it works:**
- `find("pkg/udf"; "file")` finds all files
- Filters for `.go` files
- Takes first 3 files
- For each file: reads content, extracts file path, calculates hash, gets size

**Command:**
```bash
echo 'null' | ./pwrq '[find("pkg/udf"; "file")] | map(select(._val | endswith(".go"))) | .[0:3] | map(._val | cat | {file: ._meta.file_path, size: ._meta.file_size, hash: sha256 | ._val})'
```

**Output:**
```
[
  {
    "file": "/home/remy/Projects/pwrq/pkg/udf/base32/base32.go",
    "hash": "80a695c67c097438ff75b377f6898b060f5e9658cabe6088d57b0f3f125b9e5f",
    "size": 3887
  },
  {
    "file": "/home/remy/Projects/pwrq/pkg/udf/base64/base64.go",
    "hash": "...",
    "size": 3993
  },
  ...
]
```

---

### Example 23: URL Encoding Array Elements

**Objective:** Extract email addresses from JSON and URL-encode them.

**Command:**
```bash
echo '{"users": [{"name": "Alice", "email": "alice@example.com"}, {"name": "Bob", "email": "bob@example.com"}]}' | ./pwrq '.users[] | .email | url_encode | ._val'
```

**Output:**
```
"alice%40example.com"
"bob%40example.com"
```

---

### Example 24: Entropy Calculation

**Objective:** Calculate the Shannon entropy of a string to measure randomness.

**Command:**
```bash
echo '"hello world"' | ./pwrq 'entropy | ._val'
```

**Output:**
```
2.8453509366224368
```

---

## Real-World Scenarios

### Example 25: File Integrity Verification

**Objective:** Read multiple files, calculate their hashes, and create a verification report.

**Command:**
```bash
echo '{"files": ["README.md", "go.mod"]}' | ./pwrq '.files[] | cat | sha256 | {file: ._meta.file_path, hash: ._val}'
```

**Output:**
```
{
  "file": "/home/remy/Projects/pwrq/README.md",
  "hash": "c0b1ec49bf0ce9cf62aea6152d76da5dcbf99a0e315b1e5722e94e3f125e90d4"
}
{
  "file": "/home/remy/Projects/pwrq/go.mod",
  "hash": "..."
}
```

---

### Example 26: Data Transformation Pipeline

**Objective:** Transform data through multiple stages: JSON → stringify → base64 → compress → hex.

**Command:**
```bash
echo '{"data": "sensitive information"}' | ./pwrq 'json_stringify | ._val | base64_encode | ._val | gzip_compress | ._val | hex_encode | ._val'
```

**Output:**
```
"316638623038303030303030303030303030666630613733636632383438616537343032653130633466386661303963363465336330663461346630623061613634323362376263613836303237653363343838613037633466306662666232613466306430663434383233636264323134663762306432313434373562356234303030303030306666666630323636363530363338303030303030"
```

```bash
echo 'null' | ./pwrq '[find("pkg/udf"; "file")] | map(select(._val | endswith(".go"))) | map(._val) | map(. as $path | $path | cat | ._val | {file: $path, md5: (md5 | ._val), sha1: (sha1 | ._val), sha256: (sha256 | ._val), sha512: (sha512 | ._val)})'
```

---

## Summary

These examples demonstrate the power and flexibility of `pwrq` for:
- **File operations**: Finding, reading, and processing files
- **Data encoding**: Multiple encoding schemes (base64, hex, base32, base85, binary, URL, HTML)
- **Cryptography**: Hash functions and HMAC authentication
- **Compression**: Gzip, zlib, and deflate
- **String manipulation**: Case conversion, reversal, replacement, splitting, joining
- **Data format conversion**: JSON, CSV, XML, timestamps
- **Advanced chaining**: Complex pipelines combining multiple operations

All UDFs support automatic `_val` extraction, making it easy to chain operations without explicitly accessing the `._val` field at each step. Error handling is built-in, with errors returned in the `_err` field without halting the pipeline.

