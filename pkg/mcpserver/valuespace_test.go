package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// gojq's value space is nil, bool, int, float64, *big.Int, json.Number, string,
// []any and map[string]any. Anything else makes gojq panic - not only when
// encoding, but inside any builtin that inspects the value - and a panic in an
// MCP handler goroutine kills the server.
//
// Every cmdlet is registered through common.WithFunction, which normalizes
// results, so no cmdlet should be able to produce a value outside that space.
// This test checks the raw values rather than their encoding, which is what
// makes it able to catch a leak before some future query trips over it: the
// int32 that get_process used to emit encoded perfectly well and still crashed
// `get_process | .Handles | type`.

// outsideValueSpace returns a path to the first value under v that gojq cannot
// represent, or "" if the whole value is representable.
func outsideValueSpace(v any, depth int) string {
	if depth > 500 {
		return ""
	}
	switch x := v.(type) {
	case nil, bool, int, float64, string, *big.Int, json.Number:
		return ""
	case []any:
		for i, e := range x {
			if bad := outsideValueSpace(e, depth+1); bad != "" {
				return fmt.Sprintf("[%d]%s", i, bad)
			}
		}
		return ""
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if bad := outsideValueSpace(x[k], depth+1); bad != "" {
				return "." + k + bad
			}
		}
		return ""
	default:
		return fmt.Sprintf(" is %T", v)
	}
}

func TestCmdletsStayInGojqValueSpace(t *testing.T) {
	t.Chdir(t.TempDir())

	reg := udf.DefaultRegistry()
	common.SetScriptBlockOptions(reg.Options())
	opts := append([]gojq.CompilerOption{}, reg.Options()...)
	opts = append(opts, gojq.WithEnvironLoader(os.Environ))

	checked := 0
	for _, meta := range udf.GetFunctionMetadata() {
		for _, ex := range meta.Examples {
			if skipExample(ex) {
				continue
			}
			checked++
			checkExampleValues(t, opts, meta.Name, ex)
		}
	}
	t.Logf("checked %d documented examples", checked)
}

// checkExampleValues runs one example and inspects every value it yields.
func checkExampleValues(t *testing.T, opts []gojq.CompilerOption, name, example string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: %q panicked: %v", name, example, r)
		}
	}()

	query, err := gojq.Parse(example)
	if err != nil {
		return // not this test's business: the example is documentation
	}
	code, err := gojq.Compile(query, opts...)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	iter := code.RunWithContext(ctx, nil)
	for n := 0; n < 200; n++ {
		v, ok := iter.Next()
		if !ok {
			return
		}
		if _, isErr := v.(error); isErr {
			continue
		}
		if bad := outsideValueSpace(v, 0); bad != "" {
			t.Errorf("%s: %q leaked a value gojq cannot represent: result%s", name, example, bad)
			return
		}
	}
}
