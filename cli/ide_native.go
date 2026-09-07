//go:build viz && ide_native

package cli

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xen0bit/pwrq/pkg/webnative"
)

// TokenEnv is the environment variable holding the shared secret that gates
// the native IDE when it listens beyond loopback. It is an environment
// variable rather than a flag so the secret does not sit in the process table.
//
// PWRQ_MCP_TOKEN is accepted as a fallback, so one secret can gate both
// servers on a shared machine.
const IDETokenEnv = "PWRQ_IDE_TOKEN"

// mcpTokenEnv is the fallback variable; the MCP server's own secret gates
// this server too when the IDE one is unset.

// nativeAPIPath is where the page's engine calls land, under the same route
// prefix as the editor so a reverse proxy keeps working. It exists only on
// this server: the WASM-only IDE mux (newIDEMux) has no such route, which is
// what keeps a static deployment from ever evaluating a query.
const nativeAPIPath = "/api/call"

// nativeHealthPath reports what the page runs against, for the banner.
const nativeHealthPath = "/api/health"

// launchNativeIDE serves the native editor: the browser page backed by this
// machine's full cmdlet vocabulary.
//
// A query typed here runs here, as the user running this process, in its
// working directory - sh, rm, file writes, network calls and all. That is the
// point, and it is also the whole security model: who can reach the port can
// act as you. A loopback bind is reachable only from this machine and needs
// nothing more; any other bind is refused unless a bearer token is set, sent
// by the page as `Authorization: Bearer <token>`. There is no TLS of its own:
// put it behind a reverse proxy if it needs to cross a network.
func (cli *cli) launchNativeIDE() error {
	dist, err := nativeDist()
	if err != nil {
		return err
	}

	engine := webnative.New()
	mux := newNativeMux(dist, engine)

	host := os.Getenv("PWRQ_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("PWRQ_PORT")
	if port == "" {
		port = "8080"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("PWRQ_PORT=%q is not a port number", port)
	}

	addr := net.JoinHostPort(host, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		if strings.Contains(err.Error(), "address already in use") {
			return fmt.Errorf("port %s is already in use; set PWRQ_PORT to another one", port)
		}
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	defer func() { _ = listener.Close() }()

	token := ideNativeToken()
	if token == "" && !isNativeLoopback(listener.Addr()) {
		return fmt.Errorf("refusing to serve the native IDE on %s without a shared secret: "+
			"queries run on this machine as you, including the cmdlets that read files and run commands, "+
			"so anyone who can reach this port can act as you. Set %s to gate it, "+
			"or bind a loopback address such as 127.0.0.1%s",
			listener.Addr(), IDETokenEnv, nativePortOf(addr))
	}

	auth := "loopback, no token required"
	if token != "" {
		auth = "bearer token"
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	logger.Info("native ide listening",
		"addr", listener.Addr().String(), "auth", auth)

	_, _ = fmt.Fprintf(cli.outStream, "pwrq native editor: http://localhost:%s%s/\n", port, routePrefix)
	_, _ = fmt.Fprintf(cli.outStream, "Queries run on this machine. Press Ctrl+C to stop\n")

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}

// newNativeMux serves the native page and its engine API. dist holds the
// native frontend (dist-native), never the WASM one.
func newNativeMux(dist fs.FS, engine *webnative.Engine) *http.ServeMux {
	mux := http.NewServeMux()
	files := http.FileServer(http.FS(dist))

	handler := http.StripPrefix(routePrefix, assetHandler(dist, files))
	mux.Handle(routePrefix+"/", handler)
	mux.Handle(routePrefix, http.RedirectHandler(routePrefix+"/", http.StatusMovedPermanently))
	mux.Handle("/", http.RedirectHandler(routePrefix+"/", http.StatusFound))

	mux.Handle(routePrefix+nativeHealthPath, requireNativeBearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(engine.Health()))
	})))
	mux.Handle(routePrefix+nativeAPIPath, requireNativeBearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var call struct {
			Method  string `json:"method"`
			Request string `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"error":"malformed request: ` + jsonEscapeNative(err.Error()) + `"}`))
			return
		}
		// r.Context() carries the client's connection: aborting the fetch
		// cancels a running query rather than orphaning it.
		raw := engine.Call(r.Context(), call.Method, call.Request)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(raw))
	})))

	return mux
}

// ideNativeToken resolves the bearer secret: the IDE's own variable first,
// the MCP server's as a fallback so one secret gates both.
func ideNativeToken() string {
	if token := os.Getenv(IDETokenEnv); token != "" {
		return token
	}
	return os.Getenv(mcpTokenEnv)
}

// mcpTokenEnv names the MCP server's secret, accepted here as a fallback.
const mcpTokenEnv = "PWRQ_MCP_TOKEN"

// requireNativeBearer rejects every API call that does not carry the shared
// secret. On a loopback bind with no token set it passes everything through,
// which is what keeps local use frictionless. The comparison is constant-time
// so that a caller cannot recover the token by timing its guesses.
func requireNativeBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ideNativeToken()
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
		want := []byte("Bearer " + token)
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="pwrq-native-ide"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isNativeLoopback reports whether an address is reachable only from this
// machine. A bare port like ":8080" resolves to the unspecified address,
// which is every interface, so it is not loopback. It mirrors the MCP
// server's check so the two servers agree about what "local" means.
func isNativeLoopback(addr net.Addr) bool {
	tcp, ok := addr.(*net.TCPAddr)
	return ok && tcp.IP.IsLoopback()
}

// nativePortOf pulls the port out of a listen address so the refusal can
// suggest the loopback form of what the user actually typed.
func nativePortOf(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i:]
	}
	return ":" + addr
}

func jsonEscapeNative(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		return "encoding error"
	}
	return string(encoded[1 : len(encoded)-1])
}
