//go:build js && wasm

// Package sqlite is unavailable in the browser build.
//
// The driver is modernc.org/sqlite - SQLite transpiled to Go rather than linked
// through cgo - and it has no js/wasm target: modernc.org/libc's own packages
// exclude that platform, so importing it would break `make web.wasm` rather
// than produce a cmdlet that fails at runtime.
//
// Registering nothing is the honest answer, and it is the one the rest of pwrq
// is already built for: WebRegistry marks a documented command Available:false
// when the registry did not register it, so the browser IDE lists the SQLite
// cmdlets and shows them as unavailable rather than hiding them or offering a
// pipeline the page can never run.
package sqlite

import "github.com/itchyny/gojq"

// RegisterAll registers nothing on js/wasm. A browser tab has no filesystem, so
// there is no database file for these cmdlets to open in any case.
func RegisterAll() []gojq.CompilerOption { return nil }
