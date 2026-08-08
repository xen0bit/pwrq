//go:build !viz

package cli

import (
	"errors"

	"github.com/itchyny/gojq"
)

// vizEnabled reports whether this build can render diagrams.
const vizEnabled = false

// generateGraph is unavailable in the default build.
//
// Diagram rendering pulls in d2, which brings a JavaScript engine, a syntax
// highlighter and a PDF writer with it - about 35MB of the binary, linked into
// every invocation of a tool whose usual job is to filter JSON. It lives in
// pwrq-viz instead.
func generateGraph(_ *gojq.Query, _ string) error {
	return errors.New("this build has no diagram support; use pwrq-viz, or build with -tags viz")
}
