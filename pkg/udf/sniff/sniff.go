// Package sniff inspects raw bytes: what kind of file they are, whether they
// are binary, and whether they are valid UTF-8. It is pure, so it works in the
// browser too.
package sniff

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every sniffing cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterFileType(),
		RegisterIsBinary(),
		RegisterIsUTF8(),
	}
}

// inputBytes resolves a string (or its bytes) from the pipeline, with an
// optional file flag.
func inputBytes(v any, args []any, name string) ([]byte, error) {
	inputVal, _, err := common.ParseFileArgs(v, args)
	if err != nil {
		return nil, err
	}
	switch val := common.BindValue(inputVal).(type) {
	case string:
		return []byte(val), nil
	case []byte:
		return val, nil
	default:
		return nil, fmt.Errorf("expected a string, got %T", inputVal)
	}
}

// RegisterFileType registers file_type, the kind of file the bytes are,
// detected from magic numbers.
func RegisterFileType() gojq.CompilerOption {
	return common.WithFunction("file_type", 0, 2, func(v any, args []any) any {
		data, err := inputBytes(v, args, "file_type")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("file_type: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(fileType(data), nil)
	})
}

func fileType(data []byte) string {
	switch {
	case len(data) == 0:
		return "empty"
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return "png"
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "jpeg"
	case bytes.HasPrefix(data, []byte("GIF8")):
		return "gif"
	case bytes.HasPrefix(data, []byte("%PDF")):
		return "pdf"
	case bytes.HasPrefix(data, []byte{'P', 'K', 0x03, 0x04}):
		return "zip"
	case bytes.HasPrefix(data, []byte{0x1F, 0x8B}):
		return "gzip"
	case bytes.HasPrefix(data, []byte{0x7F, 'E', 'L', 'F'}):
		return "elf"
	case bytes.HasPrefix(data, []byte{'M', 'Z'}):
		return "pe"
	case bytes.HasPrefix(data, []byte{0x00, 'a', 's', 'm'}):
		return "wasm"
	case len(data) > 262 && bytes.Contains(data[257:262], []byte("ustar")):
		return "tar"
	case bytes.HasPrefix(data, []byte("<")):
		return "xml"
	case utf8.Valid(data):
		return "text"
	default:
		return "binary"
	}
}

// RegisterIsBinary registers is_binary, whether the bytes contain a NUL byte
// or a high share of control characters.
func RegisterIsBinary() gojq.CompilerOption {
	return common.WithFunction("is_binary", 0, 2, func(v any, args []any) any {
		data, err := inputBytes(v, args, "is_binary")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("is_binary: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(isBinary(data), nil)
	})
}

func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	controls := 0
	for _, b := range data {
		if b == 0x00 {
			return true
		}
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			controls++
		}
	}
	return float64(controls)/float64(len(data)) > 0.30
}

// RegisterIsUTF8 registers is_utf8, whether the bytes are valid UTF-8.
func RegisterIsUTF8() gojq.CompilerOption {
	return common.WithFunction("is_utf8", 0, 2, func(v any, args []any) any {
		data, err := inputBytes(v, args, "is_utf8")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("is_utf8: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(utf8.Valid(data), nil)
	})
}
