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

## Development

### Building

```bash
go build ./cmd/pwrq
```

### Testing

```bash
go test ./cli -v
```

## License

MIT License (same as gojq)
