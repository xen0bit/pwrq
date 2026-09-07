//go:build viz && ide_native && !embed_web_native

package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// nativeDist serves the native editor from the working tree, which is what
// makes the page editable without rebuilding the binary. The embedded
// variant, built with -tags embed_web_native, serves the same files from
// inside the binary.
func nativeDist() (fs.FS, error) {
	distPath := filepath.Join("pkg", "web", "dist-native")
	if _, err := os.Stat(distPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("pkg/web/dist-native not found: run 'make web.build-native' first, or build with 'make build-viz-native-with-ide' to embed the page")
	}
	return os.DirFS(distPath), nil
}
