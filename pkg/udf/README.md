# User-Defined Functions (UDF)

This package provides a registry system for user-defined functions that can be called from pwrq queries.

## Structure

- `registry.go` - Main registry for managing UDFs
- `find/` - Find function implementation (Unix find-like behavior)

## UDF Return Format

All UDFs must return objects with the following structure:
- `_val`: The actual value returned by the function
- `_meta`: Metadata associated with the value (function-specific)

This standardized format allows for consistent handling of UDF results and enables rich metadata to be attached to values.

Example return format:
```go
map[string]any{
    "_val": actualValue,
    "_meta": map[string]any{
        "key1": "value1",
        "key2": "value2",
    },
}
```

## Adding New UDFs

To add a new UDF:

1. Create a new package under `pkg/udf/` (e.g., `pkg/udf/myfunction/`)
2. Implement your function following the gojq function signature
3. **Return objects with `_val` and `_meta` keys** (see UDF Return Format above)
4. **Use `common.ExtractUDFValue()` to automatically extract `_val` from UDF result inputs** (see Automatic `_val` Extraction below)
5. Create a `Register*()` function that returns a `gojq.CompilerOption`
6. Register it in `pkg/udf/registry.go` in the `DefaultRegistry()` function

Example:

```go
package myfunction

import (
    "github.com/itchyny/gojq"
    "github.com/xen0bit/pwrq/pkg/udf/common"
)

func RegisterMyFunction() gojq.CompilerOption {
    return gojq.WithFunction("myfunc", 0, 1, func(v any, args []any) any {
        // Extract _val from UDF result objects (standard behavior)
        var inputVal any
        if len(args) > 0 {
            inputVal = common.ExtractUDFValue(args[0])
        } else {
            inputVal = common.ExtractUDFValue(v)
        }
        
        // Process inputVal...
        
        // Return object with _val and _meta
        return map[string]any{
            "_val": resultValue,
            "_meta": map[string]any{
                "metadata_key": "metadata_value",
            },
        }
    })
}
```

Then in `registry.go`:

```go
reg.Register(myfunction.RegisterMyFunction())
```

### Automatic `_val` Extraction

**This is standard behavior for ALL UDFs.** When chaining UDFs together, if a UDF receives a UDF result object (an object with `_val` and `_meta` keys) as input, it will automatically extract the `_val` field. This allows for cleaner chaining:

```bash
# Instead of this:
pwrq '"hello" | base64_encode | ._val | base64_decode | ._val'

# You can do this:
pwrq '"hello" | base64_encode | base64_decode | ._val'
```

This works for all UDFs:
- `base64_encode | base64_decode`
- `find("path") | map(._val) | base64_encode`
- Any future UDFs you create

However, if you need to access `_meta` in between UDF calls, you can still do so:

```bash
# Access metadata
pwrq '"hello" | base64_encode | ._meta.encoding'
# Output: "base64"

# Then continue with the value
pwrq '"hello" | base64_encode | ._meta.encoding | base64_encode'
```

**Implementation Note:** All UDFs should use `common.ExtractUDFValue()` from `pkg/udf/common` to extract `_val` from UDF result objects. This ensures consistent behavior across all UDFs.

## Available UDFs

### base64_encode

Encodes a string to base64 format.

**Usage:**
```jq
# Encode current value
. | base64_encode

# Encode a specific string
base64_encode("hello")
```

**Arguments:**
- `input` (string, optional) - The string to encode. If not provided, uses the current value (`.`)

**Returns:** An object with:
- `_val`: The base64-encoded string
- `_meta`: Object containing:
  - `encoding`: "base64"
  - `original_length`: Length of the original string
  - `encoded_length`: Length of the encoded string

**Example:**
```bash
# Encode a string
pwrq '"hello" | base64_encode'
# Output: {"_val": "aGVsbG8=", "_meta": {...}}

# Extract just the encoded value
pwrq '"hello" | base64_encode | ._val'
# Output: "aGVsbG8="

# Chain UDFs - _val is automatically extracted when chaining
pwrq '"hello" | base64_encode | base64_decode'
# Output: {"_val": "hello", "_meta": {...}}
```

### base64_decode

Decodes a base64-encoded string.

**Usage:**
```jq
# Decode current value
. | base64_decode

# Decode a specific base64 string
base64_decode("aGVsbG8=")
```

**Arguments:**
- `input` (string, optional) - The base64-encoded string to decode. If not provided, uses the current value (`.`)

**Returns:** An object with:
- `_val`: The decoded string
- `_meta`: Object containing:
  - `encoding`: "base64"
  - `original_length`: Length of the encoded string
  - `decoded_length`: Length of the decoded string

**Example:**
```bash
# Decode a base64 string
pwrq '"aGVsbG8=" | base64_decode'
# Output: {"_val": "hello", "_meta": {...}}

# Extract just the decoded value
pwrq '"aGVsbG8=" | base64_decode | ._val'
# Output: "hello"

# Round-trip encoding/decoding (automatic _val extraction)
pwrq '"hello world" | base64_encode | base64_decode'
# Output: {"_val": "hello world", "_meta": {...}}

# Or extract the final value
pwrq '"hello world" | base64_encode | base64_decode | ._val'
# Output: "hello world"
```

### md5

Computes the MD5 hash of a string or bytes.

**Usage:**
```jq
# Hash current value
. | md5

# Hash a specific string
md5("hello")
```

**Arguments:**
- `input` (string or bytes, optional) - The string or bytes to hash. If not provided, uses the current value (`.`)

**Returns:** An object with:
- `_val`: The MD5 hash as a hexadecimal string (32 characters)
- `_meta`: Object containing:
  - `algorithm`: "md5"
  - `input_length`: Length of the input in bytes
  - `hash_length`: Length of the hash string (always 32 for MD5)

**Example:**
```bash
# Hash a string
pwrq '"hello" | md5'
# Output: {"_val": "5d41402abc4b2a76b9719d911017c592", "_meta": {...}}

# Extract just the hash
pwrq '"hello" | md5 | ._val'
# Output: "5d41402abc4b2a76b9719d911017c592"

# Chain UDFs - _val is automatically extracted when chaining
pwrq '"hello" | base64_encode | md5 | ._val'
# Output: "0733351879b2fa9bd05c7ca3061529c0"
```

### Hash Functions

pwrq supports all hash algorithms available in Go's crypto package:

- **md5** / **md5_file** - MD5 hash (128 bits, 32 hex chars)
- **sha1** / **sha1_file** - SHA-1 hash (160 bits, 40 hex chars)
- **sha224** / **sha224_file** - SHA-224 hash (224 bits, 56 hex chars)
- **sha256** / **sha256_file** - SHA-256 hash (256 bits, 64 hex chars)
- **sha384** / **sha384_file** - SHA-384 hash (384 bits, 96 hex chars)
- **sha512** / **sha512_file** - SHA-512 hash (512 bits, 128 hex chars)
- **sha512_224** / **sha512_224_file** - SHA-512/224 hash (224 bits, 56 hex chars)
- **sha512_256** / **sha512_256_file** - SHA-512/256 hash (256 bits, 64 hex chars)

All hash functions follow the same pattern:
- Accept 0 or 1 arguments (use current value if no argument)
- Automatically extract `_val` from UDF result objects
- Return object with `_val` (hex hash) and `_meta` (algorithm, input_length/file_size, hash_length, file_path for file versions)

**Example:**
```bash
# Hash a string
pwrq '"hello" | sha256 | ._val'
# Output: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

# Hash a file
pwrq '"README.md" | sha256_file | ._val'

# Chain with find
pwrq '[find("pkg/udf"; "file")] | .[0] | sha256_file | ._val'
```

### md5_file

Computes the MD5 hash of a file on disk.

**Usage:**
```jq
# Hash a file
md5_file("path/to/file")

# Hash current value (file path)
. | md5_file
```

**Arguments:**
- `file_path` (string, optional) - The path to the file to hash. If not provided, uses the current value (`.`). Supports `~` for home directory.

**Returns:** An object with:
- `_val`: The MD5 hash as a hexadecimal string (32 characters)
- `_meta`: Object containing:
  - `algorithm`: "md5"
  - `file_path`: The absolute path of the file that was hashed
  - `file_size`: Size of the file in bytes
  - `hash_length`: Length of the hash string (always 32 for MD5)

**Example:**
```bash
# Hash a file
pwrq '"README.md" | md5_file'
# Output: {"_val": "ca6ecf3dfb0e5ddb43f2ab7e3d08fdac", "_meta": {...}}

# Extract just the hash
pwrq '"README.md" | md5_file | ._val'
# Output: "ca6ecf3dfb0e5ddb43f2ab7e3d08fdac"

# Chain with find - _val is automatically extracted when chaining
pwrq '[find("pkg/udf/md5"; "file")] | .[0] | md5_file | ._val'
# Output: "b7b02e202e9965d5cfdd685f912fea0f"

# Get file metadata
pwrq '"README.md" | md5_file | ._meta'
# Output: {"algorithm": "md5", "file_path": "/path/to/README.md", "file_size": 2964, "hash_length": 32}
```

### find

The `find` function works like the Unix `find` command, returning a list of files and directories.

**Usage:**
```jq
# Find all files and directories recursively
[find("path/to/search")]

# Find only files
[find("path/to/search"; "file")]

# Find only directories
[find("path/to/search"; "dir")]

# Find with max depth
[find("path/to/search"; 2)]  # maxdepth of 2

# Find with options object
[find("path/to/search"; {"type": "file", "maxdepth": 3, "mindepth": 1})]
```

**Arguments:**
1. `path` (string, required) - The starting path to search. Supports `~` for home directory.
2. `type` (string, optional) - Filter by type: `"file"` or `"dir"`
3. `maxdepth` (number, optional) - Maximum depth to search (-1 for unlimited)
4. `options` (object, optional) - Object with `type`, `maxdepth`, and `mindepth` properties

**Returns:** An iterator of objects with:
- `_val`: The absolute file path (string)
- `_meta`: Object containing:
  - `type`: Either `"file"` or `"dir"` indicating the path type

**Example Output:**
```json
[
  {
    "_meta": {
      "type": "dir"
    },
    "_val": "/home/user/project/pkg"
  },
  {
    "_meta": {
      "type": "file"
    },
    "_val": "/home/user/project/pkg/file.go"
  }
]
```

**Usage Examples:**
```bash
# Find all files and directories
pwrq '[find("pkg/udf")]'

# Extract just the paths
pwrq '[find("pkg/udf")] | map(._val)'

# Filter by type using metadata
pwrq '[find("pkg/udf")] | map(select(._meta.type == "file"))'

# Find all Go files in current directory
pwrq '[find("."; "file")] | map(select(._val | endswith(".go"))) | map(._val)'

# Find directories only, max depth 2
pwrq '[find("pkg"; {"type": "dir", "maxdepth": 2})]'

# Count files in a directory
pwrq '[find("/tmp"; "file")] | length'
```

