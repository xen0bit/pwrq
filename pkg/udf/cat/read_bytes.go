package cat

import (
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterReadBytes registers read_bytes, which returns a file's contents
// exactly as they are on disk.
//
// It is cat's counterpart, and the difference is the whole reason it exists.
// cat decodes text: it runs the bytes through a UTF-8 (or UTF-16) decoder, so
// every byte that is not valid in that encoding comes back as U+FFFD and the
// content no longer round-trips. That is the right behaviour for logs, source
// and CSV, and the wrong behaviour for an executable, an image or a captured
// packet — anything measured, hashed or compared byte for byte.
//
// read_bytes does no decoding at all. The result is a jq string holding the
// file's bytes, which is what every cmdlet that works on bytes wants:
//
//	read_bytes("a.bin") | sha256
//	[find("samples"; "file") | {Name: ., Content: read_bytes(.)}] | rncd_compare
//
// A jq string is a byte string, so this is lossless through the pipeline. What
// it is not is safe to *print*: a value holding arbitrary bytes has no valid
// JSON spelling, so send it on to a hash, a codec or a comparison rather than
// to stdout.
func RegisterReadBytes() gojq.CompilerOption {
	return gojq.WithFunction("read_bytes", 0, 1, func(v any, args []any) any {
		in, _ := common.SplitInput(v, args, 0)

		path, ok := common.BindValue(in).(string)
		if !ok {
			return common.MakeUDFErrorResult(
				fmt.Errorf("read_bytes: expected a string file path, got %T", common.BindValue(in)), nil)
		}

		data, absPath, size, err := common.ReadFileFromPath(path)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("read_bytes: %v", err),
				map[string]any{"operation": "read_bytes", "file_path": path})
		}

		return common.MakeUDFSuccessResult(string(data), map[string]any{
			"operation": "read_bytes",
			"file_path": absPath,
			"file_size": int(size),
		})
	})
}
