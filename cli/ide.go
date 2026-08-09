//go:build viz && !embed_web

package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// launchIDE serves the editor from the working tree, which is what makes the
// page editable without rebuilding the binary. The embedded variant, built
// with -tags embed_web, serves the same files from inside the binary.
func (cli *cli) launchIDE() error {
	distPath := filepath.Join("pkg", "web", "dist")
	if _, err := os.Stat(distPath); os.IsNotExist(err) {
		return fmt.Errorf("pkg/web/dist not found: run 'make web.build' first, or build with 'make build-viz-with-ide' to embed the page")
	}
	return serveIDE(cli, os.DirFS(distPath))
}
