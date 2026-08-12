// Package mcpserver exposes pwrq as a Model Context Protocol server.
//
// A pwrq installation is a query engine: jq with a library of ~470
// PowerShell-style cmdlets for the filesystem, network, crypto, hashes, codecs
// and statistics. The server wraps that engine in MCP tools, so an agent can
// run any program against JSON or text, discover the cmdlet vocabulary, and
// check queries before running them. The same server is served over stdio
// (pwrq --mcp) or streamable HTTP (pwrq --mcp-http :port).
package mcpserver

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
// address, e.g. ":8000". The same server instance handles every connection.
func ServeHTTP(addr, version string) error {
	server := NewServer(version)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, nil)
	return http.ListenAndServe(addr, handler)
}
