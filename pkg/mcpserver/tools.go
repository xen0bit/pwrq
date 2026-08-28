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
		Description: "Evaluate a pwrq/jq query against input data. The query language is jq augmented with ~470 cmdlets for the filesystem, HTTP, crypto, hashes, encodings, compression, statistics and more (see list_functions). Input is a stream of JSON values unless rawInput is set. Returns one encoded value per query output, plus a description of the shape those values had - which keys the objects carried - so a follow-up query can be written without reading every result.",
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
			return nil, runQueryResult{}, fmt.Errorf("%s: %s", res.Kind, explainRunFailure(res, args.Query))
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: summarize(res)}},
		}, res, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_functions",
		Description: "List pwrq's user-defined functions (cmdlets). Each entry carries the name, argument arity, category, description, aliases and example invocations, plus what the cmdlet emits: whether it streams (and so must be collected with [...]), how its output is encoded when that is not obvious, the keys it reads out of an options object, and the shape and property names of the object it returns. Pass filter to narrow the list: unfiltered it is long. The filter is case-insensitive and matches names, aliases and categories first, falling back to descriptions when nothing is named that way, and to the nearest names when nothing matches at all.",
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
		Description: "Check whether a pwrq/jq query parses and compiles against the cmdlet vocabulary, and return the formatted program when it does. A query this accepts will run: unresolvable cmdlet names and wrong arities are caught here, not at run time. Cheap: use it to iterate on a query before running it. Pass the same args you would pass to run_query, so a query that reads $name still compiles.",
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
	// The shape goes before the outcome line so that a truncated run reads as
	// "here is what all of it looks like, and here is why it stopped". It is
	// only worth printing when there is more than one value: for a single
	// result the value above already is the shape.
	if res.Shape != "" && res.Count > 1 {
		fmt.Fprintf(&sb, "-- %s\n", res.Shape)
	}
	// Said separately from the outcome line, because a cut value is not a
	// stopped run: the query produced everything it was going to, and only the
	// rendering of some of it was capped. A caller that conflates the two
	// re-runs a query that had already finished.
	if res.Elided > 0 {
		fmt.Fprintf(&sb, "-- %d of %d results were larger than the byte budget and are shown in part; "+
			"slice them in the query, or raise maxBytes\n", res.Elided, res.Count)
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
		if len(res.Suggestions) == 0 {
			return fmt.Sprintf("no functions match %q, and nothing is named close to it", filter)
		}
		// A dead end is the one answer a caller cannot act on, so it never
		// ends here: the nearest names are what turns "guess again" into a
		// next call.
		return fmt.Sprintf("no functions match %q; closest are %s",
			filter, strings.Join(res.Suggestions, ", "))
	}

	var sb strings.Builder
	switch {
	case filter == "":
		fmt.Fprintf(&sb, "%d functions (pass filter to narrow the list)\n", res.Count)
	case res.Matched == matchedDescription:
		// Said out loud, because these matched on prose rather than on a name
		// and a caller who thinks otherwise will trust the list too far.
		fmt.Fprintf(&sb, "%d functions whose description mentions %q; none is named or categorised that\n",
			res.Count, filter)
	default:
		fmt.Fprintf(&sb, "%d functions matching %q\n", res.Count, filter)
	}

	withExamples := res.Count <= examplesUpTo
	for _, fn := range res.Functions {
		fmt.Fprintf(&sb, "%s%s [%s] %s\n", fn.Name, arity(fn), fn.Category, fn.Description)
		// The cardinality is printed for every entry, however long the list,
		// because it is the fact that decides whether the caller writes
		// brackets - and getting it wrong is the most common way a query
		// fails. The shape and the output encoding are printed on the same
		// terms: a caller who does not know that zlib_compress returns hex
		// writes a pipeline that cannot work and blames the wrong stage.
		if fn.Streaming {
			sb.WriteString("    streams: collect with [...] before length, map or sort_by\n")
		}
		if fn.Returns != "" {
			fmt.Fprintf(&sb, "    returns %s\n", fn.Returns)
		}
		if fn.Shape != "" {
			fmt.Fprintf(&sb, "    emits %s\n", fn.Shape)
		}
		if withExamples {
			if fn.Input != "" {
				fmt.Fprintf(&sb, "    input: %s\n", fn.Input)
			}
			for _, o := range fn.Options {
				fmt.Fprintf(&sb, "    option %s (%s): %s\n", o.Name, o.Type, o.Description)
			}
			if len(fn.Aliases) > 0 {
				fmt.Fprintf(&sb, "    also called %s\n", strings.Join(fn.Aliases, ", "))
			}
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

// listFunctions returns the documented cmdlet catalog, filtered by a search
// term matched against the name, alias and category first and the description
// second. findFunctions has the reasoning; this is the plumbing.
//
// It reads discovery.Catalog rather than the raw metadata table, which is the
// same catalogue get_command and get_help serve. That is the point: the three
// surfaces now answer from one source, so a model over MCP is told what a
// terminal user is told rather than a subset of it.
func listFunctions(args listFunctionsArgs) listFunctionsResult {
	// The registry publishes the catalogue as it is built, so make sure it has
	// been. Every other entry point has already done this, but a caller that
	// only ever lists functions would otherwise see an empty catalogue.
	udf.DefaultRegistry()

	found, matched, suggestions := findFunctions(strings.TrimSpace(args.Filter))
	if found == nil {
		found = make([]functionInfo, 0)
	}
	return listFunctionsResult{
		Functions:   found,
		Count:       len(found),
		Matched:     matched,
		Suggestions: suggestions,
	}
}

// validateQuery parses a program, compiles it against the cmdlet vocabulary,
// and reports whether it is well-formed.
//
// It used to stop after parsing, and that made its answer worth less than it
// looked. Parsing accepts any name at any arity - `notacmdlet(1;2;3)` parses
// perfectly - so "validates cleanly" meant only "is grammatical", and every
// unresolvable call still failed at run time. A model that had just been told
// its query was fine would then reasonably read the run failure as being about
// something else. Compiling here costs a few milliseconds and moves that whole
// class of failure to the tool whose entire purpose is to catch it.
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
		return validateQueryResult{Error: err.Error(), Stage: stageParse}
	}

	formatted := jqfmt.Format(query)
	if err := compileForValidation(query, args.Args); err != nil {
		return validateQueryResult{
			Error: explainCompileError(err.Error(), args.Query),
			Stage: stageCompile,
			// The layout goes back even for a query that does not compile: the
			// caller is about to edit it, and a failure at this stage is about
			// one call in an otherwise well-formed program.
			Formatted: formatted,
		}
	}
	return validateQueryResult{OK: true, Formatted: formatted, Stage: stageCompile}
}

// compileForValidation compiles a parsed query against the engine's vocabulary
// exactly as a run would, so the two cannot disagree about what resolves.
func compileForValidation(query *gojq.Query, args []namedArg) error {
	e := getEngine()

	// The alias definitions are prepended by the runner on every run, so a
	// query calling `gci` compiles there. Prepending them here too is what
	// makes this check answer the same question the run will ask. Parse
	// returns a fresh tree each call, so mutating it affects nobody.
	if len(e.runner.AliasDefs) > 0 {
		query.FuncDefs = append(append([]*gojq.FuncDef{}, e.runner.AliasDefs...), query.FuncDefs...)
	}

	options := append([]gojq.CompilerOption{}, e.runner.Options...)
	// A query that reads $name compiles only if the name is bound, so the
	// caller passes the same args they would pass to run_query. Only the names
	// matter here; the values are never evaluated.
	if names := variableNames(args); len(names) > 0 {
		options = append(options, gojq.WithVariables(names))
	}

	_, err := gojq.Compile(query, options...)
	return err
}

// variableNames renders the caller's named args the way gojq wants them, with
// the leading dollar it may or may not have been given.
func variableNames(args []namedArg) []string {
	names := make([]string, 0, len(args))
	for _, arg := range args {
		name := strings.TrimSpace(arg.Name)
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "$") {
			name = "$" + name
		}
		names = append(names, name)
	}
	return names
}
