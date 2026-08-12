// Package mcpserver exposes pwrq as a Model Context Protocol server.
//
// A pwrq installation is a query engine: jq with a library of ~470
// PowerShell-style cmdlets for the filesystem, network, crypto, hashes, codecs
// and statistics. The server wraps that engine in MCP tools, so an agent can
// run any program against JSON or text, discover the cmdlet vocabulary, and
// check queries before running them. The same server is served over stdio
// (pwrq --mcp) or streamable HTTP (pwrq --mcp-http :port).
//
// # What a caller can reach
//
// run_query evaluates against the full CLI vocabulary. That includes the
// cmdlets that read and write files, run subprocesses and make network
// requests, so a client of this server can do anything the user running it
// could do. Over stdio that is exactly the intent: the client launched the
// process. Over HTTP it is a decision, which is why ServeHTTP will not open a
// port to the network without a shared secret to gate it.
package mcpserver

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TokenEnv is the environment variable holding the shared secret that gates
// the HTTP transport. It is an environment variable rather than a flag so the
// secret does not sit in the process table for every user on the machine to
// read.
const TokenEnv = "PWRQ_MCP_TOKEN"

// instructions is the guidance a connected client passes to its model. It
// points the model at the tools and at the shape of a pwrq query.
const instructions = `pwrq is a query language: jq augmented with ~470 PowerShell-style cmdlets for the filesystem, HTTP, crypto, hashes, encodings, compression, statistics and more. Write programs as pipelines - input | cmdlet | filter - and evaluate them with run_query. Discover what you can call with list_functions, and check a query before running it with validate_query. Runs are bounded by a result limit and a timeout, both configurable per call.`

// NewServer builds an MCP server over the full pwrq vocabulary. Tool calls are
// serialized internally, so multiple concurrent sessions share one server
// safely.
func NewServer(version string) *mcp.Server {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	server := mcp.NewServer(&mcp.Implementation{
		Name:        "pwrq",
		Title:       "pwrq",
		Description: "PowerShell-style cmdlets on top of jq: run jq/pwrq queries over JSON or text with ~470 cmdlets.",
		Version:     version,
	}, &mcp.ServerOptions{
		Instructions: instructions,
		Logger:       logger,
	})

	registerTools(server)
	return server
}

// Serve runs the server over stdio until the client closes the connection.
// Nothing but the JSON-RPC protocol may touch stdout, which is why all logging
// goes to stderr.
func Serve(version string) error {
	return NewServer(version).Run(context.Background(), &mcp.StdioTransport{})
}

// ServeHTTP runs the server as a streamable HTTP endpoint on the given listen
// address, e.g. ":8000" or "127.0.0.1:8000". The same server instance handles
// every connection.
//
// run_query is arbitrary code execution by design, so who can reach the port
// is the whole security model. A loopback address is reachable only by someone
// already on the machine and needs no more than that; any other address is
// open to the network and requires a shared secret in PWRQ_MCP_TOKEN, sent by
// the client as `Authorization: Bearer <token>`. Setting the token on a
// loopback bind works too, and is worth doing on a shared machine.
func ServeHTTP(addr, version string) error {
	token := os.Getenv(TokenEnv)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	defer func() { _ = listener.Close() }()

	if token == "" && !isLoopback(listener.Addr()) {
		return fmt.Errorf("refusing to serve MCP on %s without a shared secret: "+
			"run_query executes arbitrary queries, including the cmdlets that read files and run commands, "+
			"so anyone who can reach this port can act as you. Set %s to gate it, "+
			"or bind a loopback address such as 127.0.0.1%s",
			listener.Addr(), TokenEnv, portOf(addr))
	}

	server := NewServer(version)
	var handler http.Handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, nil)
	if token != "" {
		handler = requireBearer(token, handler)
	}

	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// requireBearer rejects every request that does not carry the shared secret.
// The comparison is constant-time so that a caller cannot recover the token by
// timing its guesses.
func requireBearer(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="pwrq"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopback reports whether an address is reachable only from this machine.
// A bare port like ":8000" resolves to the unspecified address, which is every
// interface, so it is not loopback.
func isLoopback(addr net.Addr) bool {
	tcp, ok := addr.(*net.TCPAddr)
	return ok && tcp.IP.IsLoopback()
}

// portOf pulls the port out of a listen address so the error can suggest the
// loopback form of what the user actually typed.
func portOf(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i:]
	}
	return ":" + addr
}
