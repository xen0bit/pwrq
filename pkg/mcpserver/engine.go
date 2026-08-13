package mcpserver

import (
	"os"
	"sync"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/udf"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// engine holds the vocabulary every query evaluates against. Building it is not
// free - alias resolution compiles a program - and a server may serve hundreds
// of calls, so it is built once.
//
// The runner and everything it holds are immutable after construction, so
// concurrent queries share them freely. What is not shared is the session
// state: the cmdlets read it through a package-level global, and the go-sdk
// dispatches tool calls asynchronously, so a fresh, private session is
// installed under execMu for the duration of each run.
type engine struct {
	runner *queryrun.Runner

	// execMu serializes query execution. Every run installs its own session
	// state into the shared global; without the mutex two concurrent runs
	// would overwrite each other's.
	execMu sync.Mutex
}

var (
	engineOnce sync.Once
	eng        *engine
)

func getEngine() *engine {
	engineOnce.Do(func() {
		reg := udf.DefaultRegistry()

		var aliasDefs []*gojq.FuncDef
		if known, err := reg.KnownAliases(udf.StandardAliases); err == nil {
			if defs, err := reg.AliasFuncDefs(known); err == nil {
				aliasDefs = defs
			}
		}

		// Script blocks passed to cmdlets compile against the same vocabulary
		// as the surrounding query, so `where_object($x; {script: "sha256 == $h"})`
		// can reach the UDFs.
		common.SetScriptBlockOptions(reg.Options())

		options := append([]gojq.CompilerOption{}, reg.Options()...)
		options = append(options, gojq.WithEnvironLoader(os.Environ))
		// debug and stderr are registered here exactly as the CLI registers
		// them; gojq has no such builtins. stderr is the only safe sink: it
		// never corrupts the JSON-RPC channel the client is listening on.
		options = append(options,
			common.WithFunction("debug", 0, 0, writeToStderr),
			common.WithFunction("stderr", 0, 0, writeToStderr),
		)

		eng = &engine{runner: &queryrun.Runner{Options: options, AliasDefs: aliasDefs}}
	})
	return eng
}

func writeToStderr(v any, _ []any) any {
	if b, err := gojq.Marshal(v); err == nil {
		_, _ = os.Stderr.Write(append(b, '\n'))
	}
	return v
}
