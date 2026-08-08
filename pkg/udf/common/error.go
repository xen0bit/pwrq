package common

import (
	"fmt"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// MakeUDFErrorResult reports a UDF failure as a jq error.
//
// Failures used to be returned in-band as {_val: null, _err: "..."}, which meant
// a broken step produced a truthy object and the pipeline carried on with
// garbage. jq already has an error channel; using it means `try ... catch` and a
// non-zero exit status work on UDF failures exactly as they do on jq's own.
//
// The meta argument is retained so callers can supply context; it is folded into
// the message rather than travelling as a separate structure.
func MakeUDFErrorResult(err error, meta map[string]any) any {
	if err == nil {
		err = fmt.Errorf("unknown error")
	}
	if op, ok := meta["operation"].(string); ok && op != "" {
		return fmt.Errorf("%s: %w", op, err)
	}
	return err
}

// MakeUDFSuccessResult returns a UDF's value in the pipeline's own value space.
//
// Values used to be wrapped as {_val: value, _meta: {...}} and unwrapped again
// by the encoder, which made the metadata unreachable from the command line and
// silently rewrote any user JSON that happened to share those key names. A UDF
// now returns what it computed; cmdlets that genuinely produce structured output
// return a PSObject, whose properties are ordinary JSON keys.
func MakeUDFSuccessResult(value any, meta map[string]any) any {
	return psobject.NormalizeJSON(value)
}
