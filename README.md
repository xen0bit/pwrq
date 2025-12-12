# pwrq

Enhanced Go implementation of jq, extending [gojq](https://github.com/itchyny/gojq).

## Overview

`pwrq` is a command-line JSON processor that functions identically to `gojq`, providing a drop-in replacement with the same features and behavior. It's built on top of the excellent `gojq` library and will be enhanced with additional features in the future.

## Installation

```bash
go install github.com/xen0bit/pwrq/cmd/pwrq@latest
```

## Usage

`pwrq` works exactly like `gojq`. Here are some examples:

```bash
# Basic query
echo '{"foo": 128}' | pwrq '.foo'

# Array processing
echo '[1, 2, 3]' | pwrq '.[]'

# Raw output
echo '{"foo": "bar"}' | pwrq -r '.foo'

# Compact output
echo '{"foo": 128}' | pwrq -c '.'
```

## Features

- All features from `gojq`:
  - JSON querying with jq-compatible syntax
  - YAML input/output support
  - Raw input/output
  - Streaming JSON parsing
  - Module support
  - Color output
  - And more...

- **User-Defined Functions (UDF)**: Extensible function system
  - `find` - Unix find-like file/directory search function
  - `base64_encode` / `base64_decode` - Base64 encoding/decoding
  - Easy to add custom functions via the `pkg/udf` package

## User-Defined Functions

### find

The `find` function provides Unix find-like functionality. It returns objects with `_val` (the path) and `_meta` (metadata including type):

```bash
# Find all files and directories
pwrq '[find("pkg/udf")]'

# Extract just the paths
pwrq '[find("pkg/udf")] | map(._val)'

# Filter by type using metadata
pwrq '[find("pkg/udf")] | map(select(._meta.type == "file"))'
```

### base64_encode / base64_decode

Base64 encoding and decoding functions:

```bash
# Encode a string
pwrq '"hello" | base64_encode | ._val'

# Decode a base64 string
pwrq '"aGVsbG8=" | base64_decode | ._val'

# Round-trip
pwrq '"hello world" | base64_encode | ._val | base64_decode | ._val'
```

See [pkg/udf/README.md](pkg/udf/README.md) for more details.

## Development

### Building

```bash
# Using Makefile (recommended)
make build

# Or directly with go
go build ./cmd/pwrq
```

### Testing

```bash
# Using Makefile
make test          # Run all tests with race detector
make test-short    # Run tests without race detector
make test-coverage # Generate coverage report

# Or directly with go
go test ./...
```

### Other Makefile Targets

```bash
make install   # Install to $GOPATH/bin
make clean     # Remove build artifacts
make fmt       # Format code
make lint      # Run linters (requires golangci-lint)
make example   # Run example queries
make help      # Show all available targets
```

See `make help` for all available targets.

## License

MIT License (same as gojq)
