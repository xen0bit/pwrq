//go:build !embed_web
// +build !embed_web

package cli

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// launchIDE starts an HTTP server to serve the IDE web interface from filesystem
func (cli *cli) launchIDE() error {
	// Use filesystem (for development)
	distPath := filepath.Join("pkg", "web", "dist")
	if _, err := os.Stat(distPath); os.IsNotExist(err) {
		return fmt.Errorf("pkg/web/dist directory not found. Please run 'make web.build' first, or build with 'make build-with-ide' to embed files")
	}
	
	fileSystem := http.Dir(distPath)
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
