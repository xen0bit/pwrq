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
4. Create a `Register*()` function that returns a `gojq.CompilerOption`
5. Register it in `pkg/udf/registry.go` in the `DefaultRegistry()` function

Example:

```go
package myfunction

import "github.com/itchyny/gojq"

func RegisterMyFunction() gojq.CompilerOption {
    return gojq.WithIterFunction("myfunc", 1, 1, func(v any, args []any) gojq.Iter {
        // Implementation that returns objects with _val and _meta
        result := map[string]any{
            "_val": actualValue,
            "_meta": map[string]any{
                "metadata_key": "metadata_value",
            },
        }
        return gojq.NewIter(result)
    })
}
```

Then in `registry.go`:

```go
reg.Register(myfunction.RegisterMyFunction())
```

## UDF Return Format

All UDFs return objects with the following structure:
- `_val`: The actual value returned by the function
- `_meta`: Metadata associated with the value (function-specific)

This standardized format allows for consistent handling of UDF results and enables rich metadata to be attached to values.

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

# Round-trip encoding/decoding
pwrq '"hello world" | base64_encode | ._val | base64_decode | ._val'
# Output: "hello world"
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

