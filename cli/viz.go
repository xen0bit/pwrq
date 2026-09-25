//go:build viz

package cli

import (
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/graph/graphsvg"
)

// renderSVG lets the TUI save a diagram as an image, which only this build
// can draw.
var renderSVG = graphsvg.GenerateSVGOpts

// generateGraph renders the query's structure as a diagram.
func generateGraph(query *gojq.Query, outputPath string) error {
	return graphsvg.GenerateGraph(query, outputPath)
}
