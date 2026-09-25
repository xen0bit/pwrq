//go:build viz

package cli

import (
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/graph/graphsvg"
)

// generateGraph renders the query's structure as a diagram.
func generateGraph(query *gojq.Query, outputPath string) error {
	return graphsvg.GenerateGraph(query, outputPath)
}
