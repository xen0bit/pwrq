package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/itchyny/gojq"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xen0bit/pwrq/pkg/jqfmt"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// registerTools installs the server's three tools: run_query for evaluating
// programs, list_functions for discovering the cmdlet vocabulary, and
// validate_query for checking a program before running it.
//
// Every tool sets Content as well as returning a typed result. The structured
// result is the same data in a form a client can decode, but a client is only
// obliged to show the model the content blocks, and several - Open WebUI among
// them - drop structuredContent entirely. So the text has to carry the answer
// rather than a summary of it.
func registerTools(server *mcp.Server, logger *slog.Logger) {
	schemas := map[string]*jsonschema.Schema{
		"run_query":      portableSchema[runQueryArgs](),
		"list_functions": portableSchema[listFunctionsArgs](),
		"validate_query": portableSchema[validateQueryArgs](),
	}
	server.AddReceivingMiddleware(normalizeArguments(schemas))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_query",
		Description: "Evaluate a pwrq/jq query against input data. The query language is jq augmented with ~470 cmdlets for the filesystem, HTTP, crypto, hashes, encodings, compression, statistics and more (see list_functions). Input is a stream of JSON values unless rawInput is set. Returns one encoded value per query output.",
		InputSchema: schemas["run_query"],
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args runQueryArgs) (result *mcp.CallToolResult, structured runQueryResult, err error) {
		started := time.Now()
		// Logged last, so a panic recovered below is reflected in the line.
		defer func() {
			attrs := []any{
				"tool", "run_query",
				"query", truncateForLog(args.Query),
				"count", structured.Count,
				"kind", structured.Kind,
				"truncated", structured.Truncated,
				"duration", time.Since(started).Round(time.Millisecond).String(),
			}
			level := slog.LevelInfo
			if err != nil {
				level = slog.LevelError
				attrs = append(attrs, "error", err.Error())
			}
			logger.Log(ctx, level, "tool call", attrs...)
		}()

		// A query can panic in a cmdlet or in gojq itself. The HTTP transport
		// has no recover anywhere in its dispatch path, so a panic in this
		// handler would take the whole server down. Turn it into a tool error
		// instead: the agent asked for a result and gets told the run failed.
		defer func() {
			if r := recover(); r != nil {
				result, structured, err = nil, runQueryResult{}, fmt.Errorf("run_query panicked: %v", r)
			}
		}()

		e := getEngine()
		e.execMu.Lock()
		res := e.execute(args)
		e.execMu.Unlock()

		// A run that produced nothing and failed is a genuine tool error. One
		// that produced partial output or was cut off by a limit is still a
		// successful call: the values are useful and the result reports why
		// it stopped.
		if res.Error != "" && res.Count == 0 && !res.Truncated {
			return nil, runQueryResult{}, fmt.Errorf("%s: %s", res.Kind, res.Error)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: summarize(res)}},
		}, res, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_functions",
		Description: "List pwrq's user-defined functions (cmdlets). Each entry carries the name, argument arity, category, description and example invocations, so a caller can find what it needs and write a correct query. Pass filter to narrow the list: unfiltered it is long.",
		InputSchema: schemas["list_functions"],
	}, func(_ context.Context, _ *mcp.CallToolRequest, args listFunctionsArgs) (*mcp.CallToolResult, listFunctionsResult, error) {
		started := time.Now()
		res := listFunctions(args)
		logger.Info("tool call",
			"tool", "list_functions",
			"filter", args.Filter,
			"count", res.Count,
			"duration", time.Since(started).Round(time.Millisecond).String(),
		)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: describeFunctions(args, res)}},
		}, res, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "validate_query",
		Description: "Check whether a pwrq/jq query parses, and return the formatted program when it does. Cheap: use it to iterate on a query before running it.",
		InputSchema: schemas["validate_query"],
	}, func(_ context.Context, _ *mcp.CallToolRequest, args validateQueryArgs) (*mcp.CallToolResult, validateQueryResult, error) {
		started := time.Now()
		res := validateQuery(args)
		logger.Info("tool call",
			"tool", "validate_query",
			"query", truncateForLog(args.Query),
			"ok", res.OK,
			"duration", time.Since(started).Round(time.Millisecond).String(),
		)
		if !res.OK {
			// Not a tool error: "this does not parse" is the answer the tool
			// was asked for, and the caller wants it as an answer rather than
			// as a failure it has to recover from.
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: res.Error}},
			}, res, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: res.Formatted}},
		}, res, nil
	})
}

// truncateForLog caps a query or filter before it is logged, so a program the
// model wrote cannot balloon the log line. The cut is made on a rune boundary:
// half a UTF-8 sequence would render as a replacement character and make the
// logged query harder to recognise than the truncation already does.
func truncateForLog(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// summarize renders a run's output as plain text for the LLM, one result per
// line, with a note when the run was cut short.
func summarize(res runQueryResult) string {
	var sb strings.Builder
	for _, v := range res.Values {
		sb.WriteString(v)
		sb.WriteString("\n")
	}
	switch {
	case res.Truncated:
		fmt.Fprintf(&sb, "(%s)", res.Error)
	case res.Error != "":
		fmt.Fprintf(&sb, "(%s: %s)", res.Kind, res.Error)
	case res.Count == 0:
		// A query can succeed and emit nothing - `empty`, or a filter that
		// matched no input. Say so, rather than handing back a blank result
		// that reads like the tool failed.
		sb.WriteString("(no output)")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

// examplesUpTo is the size of catalogue below which describeFunctions prints
// each function's examples. Unfiltered the catalogue runs to several hundred
// entries, and the examples would then be most of what the model reads.
const examplesUpTo = 40

// describeFunctions renders the cmdlet catalogue as text, one function per
// line: name, arity, category and description, plus examples when the list is
// short enough to afford them.
func describeFunctions(args listFunctionsArgs, res listFunctionsResult) string {
	filter := strings.TrimSpace(args.Filter)
	if res.Count == 0 {
		return fmt.Sprintf("no functions match filter %q", filter)
	}

	var sb strings.Builder
	if filter == "" {
		fmt.Fprintf(&sb, "%d functions (pass filter to narrow the list)\n", res.Count)
	} else {
		fmt.Fprintf(&sb, "%d functions matching %q\n", res.Count, filter)
	}

	withExamples := res.Count <= examplesUpTo
	for _, fn := range res.Functions {
		fmt.Fprintf(&sb, "%s%s [%s] %s\n", fn.Name, arity(fn), fn.Category, fn.Description)
		if withExamples {
			for _, example := range fn.Examples {
				fmt.Fprintf(&sb, "    e.g. %s\n", example)
			}
		}
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

// arity renders how many arguments a function takes, in the jq notation the
// model will write: /0, or /1-2 for a range.
func arity(fn functionInfo) string {
	if fn.MinArgs == fn.MaxArgs {
		return fmt.Sprintf("/%d", fn.MinArgs)
	}
	return fmt.Sprintf("/%d-%d", fn.MinArgs, fn.MaxArgs)
}

// listFunctions returns the documented cmdlet catalog, filtered by a substring
// of the name or category. The metadata is the same table the CLI's --udf-list
// prints, so what the model sees matches what it can call.
func listFunctions(args listFunctionsArgs) listFunctionsResult {
	filter := strings.TrimSpace(args.Filter)
	out := make([]functionInfo, 0)
	for _, m := range udf.GetFunctionMetadata() {
		if filter != "" &&
			!strings.Contains(m.Name, filter) &&
			!strings.Contains(m.Category, filter) {
			continue
		}
		out = append(out, functionInfo{
			Name:        m.Name,
			MinArgs:     m.MinArgs,
			MaxArgs:     m.MaxArgs,
			Category:    m.Category,
			Description: m.Description,
			Examples:    m.Examples,
		})
	}
	return listFunctionsResult{Functions: out, Count: len(out)}
}

// validateQuery parses a program and reports whether it is well-formed.
//
// A valid query comes back laid out by jqfmt - the same formatting the browser
// IDE offers - rather than as the canonical single line. A model that asked
// whether its query parses is about to read the answer, and a pipeline broken
// one stage per line is what shows it the shape of what it wrote.
func validateQuery(args validateQueryArgs) validateQueryResult {
	if strings.TrimSpace(args.Query) == "" {
		return validateQueryResult{Error: "query is empty"}
	}
	query, err := gojq.Parse(args.Query)
	if err != nil {
		return validateQueryResult{Error: err.Error()}
	}
	return validateQueryResult{OK: true, Formatted: jqfmt.Format(query)}
}
