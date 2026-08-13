package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
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

// TestRunQuerySlurp pins jq -s, which the server advertises in its tool
// schema. A slurped run is one input, so `length` counts the values rather
// than being applied to each of them.
func TestRunQuerySlurp(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	out := runQuery(t, cs, runQueryArgs{Query: "length", Input: "1 2 3", Slurp: true, Compact: true})
	if out.Error != "" {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	if out.Count != 1 || strings.TrimSpace(out.Values[0]) != "3" {
		t.Fatalf("got %d values %q, want [3]", out.Count, out.Values)
	}

	// -R -s is the whole input as one string, trailing newline included.
	raw := runQuery(t, cs, runQueryArgs{Query: ".", Input: "a\nb\n", RawInput: true, Slurp: true, Compact: true})
	if raw.Count != 1 || raw.Values[0] != `"a\nb\n"` {
		t.Fatalf("got %d values %q, want [\"a\\nb\\n\"]", raw.Count, raw.Values)
	}

	// -n -s: the program runs on null, and `inputs` sees the slurped array as
	// the single value it now is.
	null := runQuery(t, cs, runQueryArgs{Query: "[inputs]", Input: "1 2 3", Slurp: true, NullInput: true, Compact: true})
	if null.Count != 1 || strings.TrimSpace(null.Values[0]) != "[[1,2,3]]" {
		t.Fatalf("got %q, want [[[1,2,3]]]", null.Values)
	}
}

// TestRunQueryEncodingMatchesCLI pins the values gojq itself would print.
// encoding/json is not a substitute: it escapes <, > and & into \u sequences,
// and refuses NaN and infinity where jq prints null and the largest float.
func TestRunQueryEncodingMatchesCLI(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	cases := []struct{ name, query, input, want string }{
		{"html", ".", `"<a href=\"x\">&"`, `"<a href=\"x\">&"`},
		{"nan", "nan", "", "null"},
		{"infinite", "infinite", "", "1.7976931348623157e+308"},
		{"bignum", ".", "123456789012345678901234567890", "123456789012345678901234567890"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := runQuery(t, cs, runQueryArgs{Query: c.query, Input: c.input, Compact: true})
			if out.Error != "" {
				t.Fatalf("unexpected error: %s", out.Error)
			}
			if got := strings.TrimSpace(out.Values[0]); got != c.want {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
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

// TestToolResultsCarryText pins the content blocks. A client is only obliged
// to show the model the content of a result, and several drop
// structuredContent entirely, so a tool whose answer lives only in the
// structured result answers nobody.
func TestToolResultsCarryText(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	cases := []struct {
		tool     string
		args     any
		contains []string
	}{
		{"run_query", runQueryArgs{Query: ".a", Input: `{"a": 42}`}, []string{"42"}},
		{"run_query", runQueryArgs{Query: "empty"}, []string{"no output"}},
		{"validate_query", validateQueryArgs{Query: ".a"}, []string{".a"}},
		// The catalogue itself, not a count of it: a filtered list carries the
		// name, the arity, the category and the examples.
		{"list_functions", listFunctionsArgs{Filter: "sha256"}, []string{"sha256/0", "[Hash]", "e.g."}},
		// Unfiltered it is still the catalogue, with the count as a header.
		{"list_functions", listFunctionsArgs{}, []string{"pass filter to narrow", "sha256/0", "get_childitem"}},
		{"list_functions", listFunctionsArgs{Filter: "no-such-cmdlet"}, []string{"no functions match"}},
	}
	for _, c := range cases {
		t.Run(c.tool+"/"+fmt.Sprint(c.args), func(t *testing.T) {
			text := contentText(callTool(t, cs, c.tool, c.args))
			if strings.TrimSpace(text) == "" {
				t.Fatal("result has no text content")
			}
			for _, want := range c.contains {
				if !strings.Contains(text, want) {
					t.Errorf("text content lacks %q; got:\n%s", want, truncate(text))
				}
			}
		})
	}
}

func truncate(s string) string {
	if len(s) > 400 {
		return s[:400] + "..."
	}
	return s
}

// TestInputSchemasArePortable pins the shape of what we advertise. A client
// hands the input schema to the model provider as the function's parameters,
// and the stricter providers reject a type union such as ["null", "array"]
// with a 400 that takes the whole request down with it.
func TestInputSchemasArePortable(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(res.Tools) != 3 {
		t.Fatalf("got %d tools, want 3", len(res.Tools))
	}

	for _, tool := range res.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			var schema map[string]any
			encoded, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatalf("marshal input schema: %v", err)
			}
			if err := json.Unmarshal(encoded, &schema); err != nil {
				t.Fatalf("decode input schema: %v", err)
			}
			if schema["type"] != "object" {
				t.Errorf("root type is %v, want object", schema["type"])
			}
			// Kept, not dropped: OpenAI's strict function calling requires it,
			// and arguments are normalized before validation so a model that
			// invents a property is not punished for it.
			if schema["additionalProperties"] != false {
				t.Errorf(`additionalProperties is %v, want false`, schema["additionalProperties"])
			}
			walkSchema(schema, func(node map[string]any) {
				if types, ok := node["type"].([]any); ok {
					t.Errorf("type union %v; a single type is what strict providers accept", types)
				}
			})
		})
	}
}

// walkSchema calls f on every subschema of a decoded JSON Schema.
func walkSchema(node map[string]any, f func(map[string]any)) {
	f(node)
	for _, child := range node {
		switch child := child.(type) {
		case map[string]any:
			// Either a subschema or a map of them; both are worth descending.
			if _, isSchema := child["type"]; isSchema {
				walkSchema(child, f)
				continue
			}
			for _, grandchild := range child {
				if sub, ok := grandchild.(map[string]any); ok {
					walkSchema(sub, f)
				}
			}
		case []any:
			for _, elem := range child {
				if sub, ok := elem.(map[string]any); ok {
					walkSchema(sub, f)
				}
			}
		}
	}
}

// TestArgumentCoercion covers what a language model actually sends, as opposed
// to what the schema asked for. None of these should fail the call.
func TestArgumentCoercion(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	cases := []struct {
		name string
		args map[string]any
		want string
	}{{
		// The data itself where JSON text was asked for. This is the single
		// most common thing a model does with run_query.
		name: "object input",
		args: map[string]any{"query": ".a", "input": map[string]any{"a": 1}},
		want: "1",
	}, {
		name: "array input",
		args: map[string]any{"query": ".[0]", "input": []any{7, 8}},
		want: "7",
	}, {
		name: "quoted number",
		args: map[string]any{"query": "1, 2", "limit": "5", "timeoutMs": "5000"},
		want: "1\n2",
	}, {
		name: "quoted boolean",
		args: map[string]any{"query": `"x"`, "raw": "true"},
		want: "x",
	}, {
		// An optional the model mentioned and then declined to use.
		name: "explicit null",
		args: map[string]any{"query": "1", "input": nil, "args": nil},
		want: "1",
	}, {
		// A flag that does not exist. One invented property should not cost
		// the model the whole call.
		name: "invented property",
		args: map[string]any{"query": "1", "pretty": true},
		want: "1",
	}, {
		// Coercion reaches into arrays: a named argument's value is JSON text,
		// and a model will send the number.
		name: "named argument value",
		args: map[string]any{"query": "$x + 1", "args": []any{map[string]any{"name": "x", "value": 41}}},
		want: "42",
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := callTool(t, cs, "run_query", c.args)
			if res.IsError {
				t.Fatalf("tool error: %s", contentText(res))
			}
			if got := contentText(res); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestArgumentCoercionStopsAtTheAmbiguous pins the other half: what cannot be
// read as the declared type is still rejected, so the model gets told.
func TestArgumentCoercionStopsAtTheAmbiguous(t *testing.T) {
	server := NewServer("test")
	cs := newTestClient(t, server)

	cases := map[string]map[string]any{
		"unparseable number": {"query": "1", "limit": "ten"},
		"missing required":   {"input": "1"},
		// A bare string where a list of named arguments was declared: there is
		// no obvious reading of it, so it is not invented.
		"wrong type": {"query": "1", "args": "x"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			res := callTool(t, cs, "run_query", args)
			if !res.IsError {
				t.Fatalf("expected a tool error, got %q", contentText(res))
			}
		})
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

// TestServeHTTPRefusesOpenPort pins the one thing that separates a local tool
// from a remote shell: run_query can read files and run commands, so a bind
// that is reachable from the network needs a secret to gate it.
func TestServeHTTPRefusesOpenPort(t *testing.T) {
	t.Setenv(TokenEnv, "")

	err := ServeHTTP(":0", "test")
	if err == nil {
		t.Fatal("expected a refusal for an unauthenticated non-loopback bind")
	}
	if !strings.Contains(err.Error(), TokenEnv) {
		t.Fatalf("the refusal should name %s, got: %v", TokenEnv, err)
	}
}

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:0": true,
		"[::1]:0":     true,
		":0":          false,
		"0.0.0.0:0":   false,
	}
	for addr, want := range cases {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			t.Skipf("cannot listen on %s here: %v", addr, err)
		}
		got := isLoopback(listener.Addr())
		_ = listener.Close()
		if got != want {
			t.Errorf("isLoopback(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestRequireBearer(t *testing.T) {
	reached := false
	handler := requireBearer("s3cret", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	cases := []struct {
		name   string
		header string
		status int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"bare token", "s3cret", http.StatusUnauthorized},
		{"correct", "Bearer s3cret", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reached = false
			req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			res, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			_ = res.Body.Close()
			if res.StatusCode != c.status {
				t.Fatalf("status %d, want %d", res.StatusCode, c.status)
			}
			if reached != (c.status == http.StatusOK) {
				t.Fatalf("handler reached = %v at status %d", reached, res.StatusCode)
			}
		})
	}
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
	defer func() { _ = cs.Close() }()

	res := callTool(t, cs, "run_query", runQueryArgs{Query: `.`, Input: `{"a": 1}`, Compact: true})
	var out runQueryResult
	decodeStructured(t, res, &out)
	if got := strings.TrimSpace(out.Values[0]); got != `{"a":1}` {
		t.Fatalf("http run_query got %s, want {a:1}", got)
	}
}
