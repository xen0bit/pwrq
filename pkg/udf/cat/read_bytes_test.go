package cat

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// invalidUTF8 is a byte a UTF-8 decoder cannot represent: 0xFF never appears
// in valid UTF-8, so a decoding reader replaces it with U+FFFD.
var binaryContent = []byte{0x00, 0xFF, 0xFE, 'h', 'i', 0x80, 0x00}

func writeSample(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(path, binaryContent, 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	return path
}

func run(t *testing.T, program string, input any) any {
	t.Helper()
	query, err := gojq.Parse(program)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	code, err := gojq.Compile(query, RegisterReadBytes(), RegisterCat())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	iter := code.Run(input)
	v, ok := iter.Next()
	if !ok {
		t.Fatal("no result")
	}
	return v
}

// TestReadBytesIsExact is the reason the cmdlet exists: cat decodes text and
// so cannot round-trip a binary file, and read_bytes does not.
func TestReadBytesIsExact(t *testing.T) {
	path := writeSample(t)

	got, ok := common.BindValue(run(t, "read_bytes", path)).(string)
	if !ok {
		t.Fatalf("read_bytes returned %T, want a string", run(t, "read_bytes", path))
	}
	if got != string(binaryContent) {
		t.Fatalf("read_bytes = % x, want % x", got, binaryContent)
	}

	decoded, ok := common.BindValue(run(t, "cat", path)).(string)
	if !ok {
		t.Fatalf("cat returned a non-string")
	}
	if decoded == string(binaryContent) {
		t.Fatal("cat round-tripped the bytes; read_bytes would then be redundant")
	}
}

func TestReadBytesArgumentForm(t *testing.T) {
	path := writeSample(t)

	// The path may come from the pipeline or as an argument, as cat's does.
	piped := common.BindValue(run(t, "read_bytes", path))
	arg := common.BindValue(run(t, `read_bytes("`+path+`")`, nil))
	if piped != arg {
		t.Fatalf("the piped and argument forms disagree: %q vs %q", piped, arg)
	}
}

func TestReadBytesErrors(t *testing.T) {
	missing := run(t, `read_bytes`, filepath.Join(t.TempDir(), "nope.bin"))
	if _, ok := missing.(error); !ok {
		t.Fatalf("a missing file should be an error, got %T", missing)
	}

	query, err := gojq.Parse("read_bytes")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	code, err := gojq.Compile(query, RegisterReadBytes())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	iter := code.Run(42)
	v, _ := iter.Next()
	if _, ok := v.(error); !ok {
		t.Fatalf("a non-string path should be an error, got %T", v)
	}
}
