//go:build viz

package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// routePrefix is where the IDE lives, so it can be mounted behind a reverse
// proxy on the same path without the page's relative URLs breaking.
const routePrefix = "/tools/pwrq"

// serveIDE serves the editor from a filesystem, embedded or not.
//
// It is a file server with three things the standard one does not do, each of
// which the page needs:
//
//   - the WebAssembly module is 27 MB, and pre-compressed alongside it. Serving
//     the .gz to a browser that says it accepts gzip turns a 27 MB download
//     into 7 MB, which is the difference between the page feeling slow and
//     feeling broken.
//   - .wasm has to arrive as application/wasm or the streaming instantiation
//     path refuses it.
//   - the hashed bundle files are immutable and the rest is not, so they are
//     cached differently.
func serveIDE(cli *cli, dist fs.FS) error {
	mux := http.NewServeMux()
	files := http.FileServer(http.FS(dist))

	handler := http.StripPrefix(routePrefix, assetHandler(dist, files))
	mux.Handle(routePrefix+"/", handler)
	mux.Handle(routePrefix, http.RedirectHandler(routePrefix+"/", http.StatusMovedPermanently))
	// Someone who opens the bare address should land on the editor rather than
	// on a 404 that makes the server look broken.
	mux.Handle("/", http.RedirectHandler(routePrefix+"/", http.StatusFound))

	port := os.Getenv("PWRQ_PORT")
	if port == "" {
		port = "8080"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("PWRQ_PORT=%q is not a port number", port)
	}

	addr := ":" + port
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// The one failure worth naming: it has an obvious fix, and the raw
		// syscall error does not suggest it.
		if strings.Contains(err.Error(), "address already in use") {
			return fmt.Errorf("port %s is already in use; set PWRQ_PORT to another one", port)
		}
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	_, _ = fmt.Fprintf(cli.outStream, "pwrq editor: http://localhost:%s%s/\n", port, routePrefix)
	_, _ = fmt.Fprintf(cli.outStream, "Press Ctrl+C to stop\n")

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}

func assetHandler(dist fs.FS, files http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}

		switch {
		case strings.HasSuffix(name, ".wasm"):
			w.Header().Set("Content-Type", "application/wasm")
			// The module changes whenever it is rebuilt and has no hash in its
			// name, so it must be revalidated rather than cached blind.
			w.Header().Set("Cache-Control", "no-cache")
			if serveCompressed(w, r, dist, name) {
				return
			}
		case strings.HasPrefix(name, "index-"):
			// Bundled assets carry a content hash: they can never change under
			// a given name.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}

		files.ServeHTTP(w, r)
	})
}

// serveCompressed sends the pre-compressed copy of a file when the client
// accepts gzip and the copy exists. It reports whether it handled the request.
func serveCompressed(w http.ResponseWriter, r *http.Request, dist fs.FS, name string) bool {
	// Vary regardless of what is sent: a cache that stored the uncompressed
	// answer must not hand it to a client that asked for gzip, or the reverse.
	w.Header().Add("Vary", "Accept-Encoding")

	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		return false
	}

	file, err := dist.Open(name + ".gz")
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return false
	}

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))

	if r.Method == http.MethodHead {
		return true
	}
	if _, err := io.Copy(w, file); err != nil {
		// The client went away mid-download; there is nothing useful to say
		// and the response has already begun.
		return true
	}
	return true
}
