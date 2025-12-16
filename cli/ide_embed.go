//go:build embed_web
// +build embed_web

package cli

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"

	"github.com/xen0bit/pwrq/pkg/web"
)

// launchIDE starts an HTTP server to serve the IDE web interface using embedded files
func (cli *cli) launchIDE() error {
	// Use embedded filesystem
	// The embed includes dist from pkg/web's perspective
	distFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		// Try without the prefix
		distFS = web.Dist
	}
	
	fileSystem := http.FS(distFS)
	http.Handle("/", http.FileServer(fileSystem))

	port := os.Getenv("PWRQ_PORT")
	if port == "" {
		port = "8080"
	}

	addr := fmt.Sprintf(":%s", port)
	fmt.Fprintf(cli.outStream, "Starting IDE server on http://localhost%s\n", addr)
	fmt.Fprintf(cli.outStream, "Press Ctrl+C to stop\n")

	if err := http.ListenAndServe(addr, nil); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

