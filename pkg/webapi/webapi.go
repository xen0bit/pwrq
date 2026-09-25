// Package webapi is the engine behind the browser IDE, as the WASM worker
// sees it.
//
// The engine itself is pkg/ideengine, shared with the native page and the
// terminal UI. What this package adds is the browser's configuration of it:
// the in-memory vocabulary of udf.WebRegistry, a deadline that works without
// timers, and SVG diagrams. The WASM entry point in cmd/web does nothing but
// hand strings to Call, which is what makes this testable: the IDE's
// behaviour is exercised by ordinary Go tests rather than by clicking around
// in a browser.
package webapi

import (
	"context"
	"sync"

	"github.com/xen0bit/pwrq/pkg/graph/graphsvg"
	"github.com/xen0bit/pwrq/pkg/ideengine"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// Version is stamped at build time with the revision the page was built from,
// so a shared link's behaviour can be traced to a commit.
var Version = "dev"

// The page's wire types are the engine's. They are named here too because
// this is where the page's protocol is documented and tested.
type (
	ValidateRequest  = ideengine.ValidateRequest
	ValidateResponse = ideengine.ValidateResponse
	RunRequest       = ideengine.RunRequest
	RunResponse      = ideengine.RunResponse
	Arg              = ideengine.Arg
	FormatRequest    = ideengine.FormatRequest
	FormatResponse   = ideengine.FormatResponse
	InlineRequest    = ideengine.InlineRequest
	InlineResponse   = ideengine.InlineResponse
	DiagramRequest   = ideengine.DiagramRequest
	DiagramResponse  = ideengine.DiagramResponse
	Command          = ideengine.Command
	AliasInfo        = ideengine.AliasInfo
	ClassStyle       = ideengine.ClassStyle
	CatalogResponse  = ideengine.CatalogResponse
	Example          = ideengine.Example
)

// Examples is the gallery the page opens with.
func Examples() []Example { return ideengine.Examples() }

// Call dispatches a named request. The page speaks one protocol - a method
// name and a JSON string - so adding a capability to the engine needs no
// change to the worker or the WASM glue.
func Call(method, request string) string {
	return getEngine().Call(context.Background(), method, request)
}

var (
	engineOnce sync.Once
	eng        *ideengine.Engine
)

// getEngine builds the page's engine once: the page calls in on every
// keystroke, and building it compiles a program. It is built lazily rather
// than at init so Version has been stamped by then.
func getEngine() *ideengine.Engine {
	engineOnce.Do(func() {
		eng = ideengine.New(ideengine.Config{
			Registry:  udf.WebRegistry(),
			Version:   Version,
			Deadline:  newDeadline,
			RenderSVG: graphsvg.GenerateSVGOpts,
		})
	})
	return eng
}
