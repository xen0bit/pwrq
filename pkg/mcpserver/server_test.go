package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newTestClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return res
}

func runQuery(t *testing.T, cs *mcp.ClientSession, args runQueryArgs) runQueryResult {
	t.Helper()
	res := callTool(t, cs, "run_query", args)
	if res.IsError {
		t.Fatalf("run_query unexpected tool error: %s", res.Content)
	}
	var out runQueryResult
	decodeStructured(t, res, &out)
	return out
}

func decodeStructured(t *testing.T, res *mcp.CallToolResult, out any) {
	t.Helper()
	encoded, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(encoded, out); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
}

func TestRunQuery(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	out := runQuery(t, cs, runQueryArgs{
		Query: ".foo + 1",
		Input: `{"foo": 41}`,
	})
	if out.Error != "" {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	if out.Count != 1 || len(out.Values) != 1 || strings.TrimSpace(out.Values[0]) != "42" {
		t.Fatalf("got %d values %q, want [42]", out.Count, out.Values)
	}
}

func TestRunQueryCmdlets(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	out := runQuery(t, cs, runQueryArgs{
		Query:     `"hello world" | sha256`,
		NullInput: true,
	})
	if out.Error != "" {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	if out.Count != 1 || len(out.Values) != 1 {
		t.Fatalf("got %d values, want 1", out.Count)
	}
	// "hello world" in SHA-256 is a known vector.
	const want = `"b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"`
	if got := strings.TrimSpace(out.Values[0]); got != want {
		t.Fatalf("sha256 = %s, want %s", got, want)
	}
}

func TestRunQueryArgs(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	out := runQuery(t, cs, runQueryArgs{
		Query: "$x + $y",
		Args:  []namedArg{{Name: "x", Value: "1"}, {Name: "y", Value: "2"}},
	})
	if out.Error != "" {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	if got := strings.TrimSpace(out.Values[0]); got != "3" {
		t.Fatalf("got %s, want 3", got)
	}
}

func TestRunQueryRawOutput(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	out := runQuery(t, cs, runQueryArgs{
		Query: `"hello"`,
		Raw:   true,
	})
	if out.Error != "" {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	if got := out.Values[0]; got != "hello" {
		t.Fatalf("got %q, want bare hello", got)
	}
}

func TestRunQueryRawInput(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	out := runQuery(t, cs, runQueryArgs{
		Query:    `ascii_upcase`,
		Input:    "foo\nbar\n",
		RawInput: true,
	})
	if out.Error != "" {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	if out.Count != 2 || out.Values[0] != `"FOO"` || out.Values[1] != `"BAR"` {
		t.Fatalf("got %v, want FOO and BAR", out.Values)
	}
}

func TestRunQueryNullInputRawInputs(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	// -n with raw input: the program runs on null and reads the input through
	// the `inputs` builtin, which must still see the raw lines.
	out := runQuery(t, cs, runQueryArgs{
		Query:     `[inputs] | map(ascii_upcase)`,
		Input:     "foo\nbar\n",
		RawInput:  true,
		NullInput: true,
		Compact:   true,
	})
	if out.Error != "" {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	if got := strings.TrimSpace(out.Values[0]); got != `["FOO","BAR"]` {
		t.Fatalf("got %s, want [FOO,BAR]", got)
	}
}

func TestRunQueryParseError(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	res := callTool(t, cs, "run_query", runQueryArgs{Query: "foo | | bar"})
	if !res.IsError {
		t.Fatal("expected a tool error for an unparseable query")
	}
}

func TestRunQueryRuntimeError(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	res := callTool(t, cs, "run_query", runQueryArgs{
		Query: `1 + "x"`,
	})
	if !res.IsError {
		t.Fatal("expected a tool error for a runtime failure")
	}
	if !strings.Contains(contentText(res), "runtime") {
		t.Fatalf("expected kind in error, got %s", contentText(res))
	}
}

func TestRunQueryLimit(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	out := runQuery(t, cs, runQueryArgs{
		Query: `repeat(1)`,
		Limit: 5,
	})
	if !out.Truncated || out.Kind != "limit" {
		t.Fatalf("got truncated=%v kind=%q, want limit", out.Truncated, out.Kind)
	}
	if out.Count != 5 || len(out.Values) != 5 {
		t.Fatalf("got count=%d, want 5", out.Count)
	}
}

func TestRunQueryTimeout(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	// A single huge value with no intermediate output: the run must be stopped
	// by the deadline, not by a result limit.
	res := callTool(t, cs, "run_query", runQueryArgs{
		Query:     `[range(10000000000)]`,
		TimeoutMs: 50,
	})
	if !res.IsError {
		t.Fatal("expected a tool error when the run times out")
	}
	if !strings.Contains(contentText(res), "timeout") {
		t.Fatalf("expected kind in error, got %s", contentText(res))
	}
}

func contentText(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestRunQueryStateless(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	out := runQuery(t, cs, runQueryArgs{
		Query: `set_variable("x"; 42), get_variable("x"; {"ValueOnly": true})`,
	})
	if got := strings.TrimSpace(out.Values[len(out.Values)-1]); got != "42" {
		t.Fatalf("within-call session failed: got %s, want 42", got)
	}

	// The variable must not leak into the next call: a fresh session means the
	// lookup fails.
	res := callTool(t, cs, "run_query", runQueryArgs{Query: `get_variable("x"; {"ValueOnly": true})`})
	if !res.IsError {
		t.Fatal("variable leaked across calls; expected get_variable to fail")
	}
}

func TestListFunctions(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	res := callTool(t, cs, "list_functions", listFunctionsArgs{})
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	var out listFunctionsResult
	decodeStructured(t, res, &out)
	if out.Count == 0 || len(out.Functions) != out.Count {
		t.Fatalf("got count=%d len=%d, want matching non-zero", out.Count, len(out.Functions))
	}

	res = callTool(t, cs, "list_functions", listFunctionsArgs{Filter: "sha256"})
	var filtered listFunctionsResult
	decodeStructured(t, res, &filtered)
	for _, fn := range filtered.Functions {
		if !strings.Contains(fn.Name, "sha256") && !strings.Contains(fn.Category, "sha256") {
			t.Fatalf("filter returned %s which does not match", fn.Name)
		}
	}
	if filtered.Count < 1 {
		t.Fatal("expected at least sha256 in the filtered list")
	}
}

func TestValidateQuery(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	res := callTool(t, cs, "validate_query", validateQueryArgs{Query: `.foo | map(.bar)`})
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	var ok validateQueryResult
	decodeStructured(t, res, &ok)
	if !ok.OK || ok.Formatted == "" {
		t.Fatalf("expected ok with formatting, got %+v", ok)
	}

	res = callTool(t, cs, "validate_query", validateQueryArgs{Query: `foo | | bar`})
	var bad validateQueryResult
	decodeStructured(t, res, &bad)
	if bad.OK || bad.Error == "" {
		t.Fatalf("expected a parse failure, got %+v", bad)
	}
}

// TestConcurrentCalls exercises the engine's shared global session state under
// the go-sdk's asynchronous dispatch; run with -race.
func TestConcurrentCalls(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out := runQuery(t, cs, runQueryArgs{
				Query: ".n * 2",
				Input: `{"n": 1}`,
			})
			if got := strings.TrimSpace(out.Values[0]); got != "2" {
				t.Errorf("goroutine %d: got %s, want 2", i, got)
			}
		}(i)
	}
	wg.Wait()
}

func TestHTTPServer(t *testing.T) {
	server := NewServer("test")
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             ts.URL,
		HTTPClient:           ts.Client(),
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("http client connect: %v", err)
	}
	defer cs.Close()

	res := callTool(t, cs, "run_query", runQueryArgs{Query: `.`, Input: `{"a": 1}`, Compact: true})
	var out runQueryResult
	decodeStructured(t, res, &out)
	if got := strings.TrimSpace(out.Values[0]); got != `{"a":1}` {
		t.Fatalf("http run_query got %s, want {a:1}", got)
	}
}
