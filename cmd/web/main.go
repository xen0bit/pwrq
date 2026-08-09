//go:build js && wasm

// Command web is the WASM engine behind the browser IDE.
//
// It is deliberately thin. Everything the page can ask for lives in
// pkg/webapi, which is ordinary Go with ordinary tests; this file only carries
// strings across the JavaScript boundary and makes sure a panic on the Go side
// arrives as a message rather than as a dead worker.
package main

import (
	"fmt"
	"syscall/js"

	"github.com/xen0bit/pwrq/pkg/webapi"
)

func main() {
	js.Global().Set("pwrqCall", js.FuncOf(call))
	js.Global().Set("pwrqVersion", js.ValueOf(webapi.Version))

	// The page polls for pwrqCall, but announcing readiness lets it stop
	// polling the moment the engine is up.
	if ready := js.Global().Get("pwrqReady"); ready.Type() == js.TypeFunction {
		ready.Invoke(webapi.Version)
	}

	// The Go program must outlive main: every exported function is called
	// later, from JavaScript.
	select {}
}

// call dispatches one request. It returns a JSON string in every case,
// including the ones where the Go side failed, because the page has no other
// channel: a reply it cannot parse is indistinguishable from a crash.
func call(_ js.Value, args []js.Value) (result any) {
	defer func() {
		if r := recover(); r != nil {
			// A panic would otherwise take the whole Go runtime down and leave
			// the worker alive but useless.
			result = fmt.Sprintf(`{"error":%q}`, fmt.Sprintf("engine panic: %v", r))
		}
	}()

	if len(args) < 2 {
		return `{"error":"pwrqCall needs a method and a request"}`
	}
	return webapi.Call(args[0].String(), args[1].String())
}
