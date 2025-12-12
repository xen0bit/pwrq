# User-Defined Functions (UDF)

This package provides a registry system for user-defined functions that can be called from pwrq queries.

## Structure

- `registry.go` - Main registry for managing UDFs
- `find/` - Find function implementation (Unix find-like behavior)

## Adding New UDFs

To add a new UDF:

1. Create a new package under `pkg/udf/` (e.g., `pkg/udf/myfunction/`)
2. Implement your function following the gojq function signature
3. Create a `Register*()` function that returns a `gojq.CompilerOption`
4. Register it in `pkg/udf/registry.go` in the `DefaultRegistry()` function

Example:

```go
package myfunction

import "github.com/itchyny/gojq"

func RegisterMyFunction() gojq.CompilerOption {
    return gojq.WithFunction("myfunc", 1, 1, func(v any, args []any) any {
        // Implementation
        return result
    })
}
```

Then in `registry.go`:

```go
reg.Register(myfunction.RegisterMyFunction())
```

## Available UDFs

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

**Returns:** An iterator of absolute file paths (strings)

**Examples:**
```bash
# Find all Go files in current directory
pwrq '[find("."; "file")] | map(select(endswith(".go")))'

# Find directories only, max depth 2
pwrq '[find("pkg"; {"type": "dir", "maxdepth": 2})]'

# Count files in a directory
pwrq '[find("/tmp"; "file")] | length'
```

