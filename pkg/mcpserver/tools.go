package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// registerTools installs the server's three tools: run_query for evaluating
// programs, list_functions for discovering the cmdlet vocabulary, and
// validate_query for checking a program before running it.
func registerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_query",
		Description: "Evaluate a pwrq/jq query against input data. The query language is jq augmented with ~470 cmdlets for the filesystem, HTTP, crypto, hashes, encodings, compression, statistics and more (see list_functions). Input is a stream of JSON values unless rawInput is set. Returns one encoded value per query output.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args runQueryArgs) (*mcp.CallToolResult, runQueryResult, error) {
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
		Description: "List pwrq's user-defined functions (cmdlets). Each entry carries the name, argument arity, category, description and example invocations, so a caller can find what it needs and write a correct query.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args listFunctionsArgs) (*mcp.CallToolResult, listFunctionsResult, error) {
		res := listFunctions(args)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%d functions listed", res.Count)}},
		}, res, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "validate_query",
		Description: "Check whether a pwrq/jq query parses, and return the formatted program when it does. Cheap: use it to iterate on a query before running it.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args validateQueryArgs) (*mcp.CallToolResult, validateQueryResult, error) {
		res := validateQuery(args)
		if res.Error != "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "parse error: " + res.Error}},
			}, res, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: res.Formatted}},
		}, res, nil
	})
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
	}
	return strings.TrimSuffix(sb.String(), "\n")
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
func validateQuery(args validateQueryArgs) validateQueryResult {
	if strings.TrimSpace(args.Query) == "" {
		return validateQueryResult{}
	}
	query, err := gojq.Parse(args.Query)
	if err != nil {
		return validateQueryResult{Error: err.Error()}
	}
	return validateQueryResult{OK: true, Formatted: query.String()}
}
