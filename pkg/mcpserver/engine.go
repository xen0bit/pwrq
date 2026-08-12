package mcpserver

import (
	"sync"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// engine holds the vocabulary every query evaluates against. Building it is not
// free - alias resolution compiles a program - and a server may serve hundreds
// of calls, so it is built once.
//
// The registry, its compiler options and the alias definitions are immutable
// after construction, so concurrent queries share them freely. What is not
// shared is the session state: the cmdlets read it through a package-level
// global, and the go-sdk dispatches tool calls asynchronously, so a fresh,
// private session is installed under execMu for the duration of each run.
type engine struct {
	options   []gojq.CompilerOption
	aliasDefs []*gojq.FuncDef

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

		eng = &engine{
			options:   reg.Options(),
			aliasDefs: aliasDefs,
		}
	})
	return eng
}
