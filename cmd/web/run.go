//go:build js && wasm

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"syscall/js"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// maxResults caps how much a single run will produce. A query like `repeat(1)`
// is an infinite stream, and in a browser tab there is no Ctrl-C: without a
// cap the page locks up with no way out but closing it.
const maxResults = 10000

// webRegistry is built once. Registering is cheap but signature discovery,
// which both the catalog and the alias table need, compiles a program, and the
// page runs this on every keystroke.
var (
	webRegistry = udf.WebRegistry()
	webAliases  = knownAliases(webRegistry)
)

func knownAliases(reg *udf.Registry) []udf.Alias {
	known, err := reg.KnownAliases(udf.StandardAliases)
	if err != nil {
		return nil
	}
	return known
}

// runQuery evaluates a query against sample input, which is what the IDE was
// missing: it could draw a query's structure but never show what it produced.
//
// Returns: {output: string, err: string}. A mid-stream error is reported
// alongside the results that preceded it, because a query that produces ten
// good values and then fails has told you something about all eleven.
func runQuery(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return result("", "runQuery requires at least 1 argument: query string")
	}

	queryStr := args[0].String()
	if strings.TrimSpace(queryStr) == "" {
		return result("", "query string cannot be empty")
	}

	inputStr := ""
	if len(args) > 1 && args[1].Type() == js.TypeString {
		inputStr = args[1].String()
	}

	query, err := gojq.Parse(queryStr)
	if err != nil {
		return result("", fmt.Sprintf("failed to parse query: %v", err))
	}

	inputs, err := decodeInputs(inputStr)
	if err != nil {
		return result("", fmt.Sprintf("failed to parse input JSON: %v", err))
	}

	options := webRegistry.Options()

	// Aliases are jq definitions prepended to the query, exactly as the CLI
	// does it: gojq binds function names at compile time, so this is the only
	// point at which `ft` can mean anything. Only the ones this registry can
	// resolve are compiled - `gci` names a cmdlet the page does not have.
	aliasDefs, err := webRegistry.AliasFuncDefs(webAliases)
	if err != nil {
		return result("", err.Error())
	}
	query.FuncDefs = append(aliasDefs, query.FuncDefs...)

	// Script blocks handed to cmdlets compile against the same vocabulary as
	// the surrounding query.
	common.SetScriptBlockOptions(options)

	code, err := gojq.Compile(query, options...)
	if err != nil {
		return result("", fmt.Sprintf("compile error: %v", err))
	}

	var out strings.Builder
	count := 0
	for _, input := range inputs {
		iter := code.Run(input)
		for {
			v, ok := iter.Next()
			if !ok {
				break
			}
			if err, isErr := v.(error); isErr {
				// `halt` and `halt_error` end the run deliberately, so what
				// they carry is the message, not a failure of the page. Match
				// the concrete type: the interface every gojq error satisfies
				// would swallow `error("boom")` as a clean stop.
				if halt, ok := err.(*gojq.HaltError); ok {
					return result(out.String(), haltMessage(halt))
				}
				return result(out.String(), err.Error())
			}

			encoded, err := gojq.Marshal(v)
			if err != nil {
				return result(out.String(), err.Error())
			}
			out.Write(encoded)
			out.WriteByte('\n')

			count++
			if count >= maxResults {
				return result(out.String(),
					fmt.Sprintf("stopped after %d results; the query may produce an unbounded stream", maxResults))
			}
		}
	}

	return result(out.String(), "")
}

// decodeInputs reads the sample input as a stream of JSON values, matching how
// the CLI reads a file: `{"a":1} {"a":2}` is two inputs, not a syntax error.
// Empty input is a single null, so a generator query like `[limit(3; repeat(1))]`
// still runs.
func decodeInputs(s string) ([]any, error) {
	if strings.TrimSpace(s) == "" {
		return []any{nil}, nil
	}

	dec := json.NewDecoder(bytes.NewReader([]byte(s)))
	// The CLI decodes with UseNumber() and so must this: json.Number is a
	// fmt.Stringer, and without it every number in the input is silently
	// turned into a string somewhere downstream.
	dec.UseNumber()

	var inputs []any
	for {
		var v any
		if err := dec.Decode(&v); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		inputs = append(inputs, v)
	}
	if len(inputs) == 0 {
		return []any{nil}, nil
	}
	return inputs, nil
}

// haltMessage renders what a halt carried, mirroring how the CLI writes it to
// stderr: a string goes out as text, anything else as JSON, and a bare `halt`
// carries nothing at all.
func haltMessage(halt *gojq.HaltError) string {
	v := halt.Value()
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	encoded, err := gojq.Marshal(v)
	if err != nil {
		return halt.Error()
	}
	return string(encoded)
}

func result(output, errMsg string) map[string]interface{} {
	return map[string]interface{}{
		"output": output,
		"err":    errMsg,
	}
}
