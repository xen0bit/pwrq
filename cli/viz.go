//go:build viz

package cli

import (
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/graph"
)

// vizEnabled reports whether this build can render diagrams.
const vizEnabled = true

// generateGraph renders the query's structure as a diagram.
func generateGraph(query *gojq.Query, outputPath string) error {
	return graph.GenerateGraph(query, outputPath)
}
