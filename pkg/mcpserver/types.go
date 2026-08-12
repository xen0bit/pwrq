package mcpserver

// runQueryArgs is the parameter set of the run_query tool. It mirrors what the
// CLI accepts, so any jq/pwrq invocation can be reproduced over MCP: the query,
// the input and the presentation flags.
//
// The struct doubles as the tool's JSON Schema. Fields marked omitempty are
// optional; everything else a client must provide. The `jsonschema` tag
// becomes the property description the model sees.
type runQueryArgs struct {
	// Query is the program to evaluate. It is the full pwrq language: jq
	// augmented with the registered cmdlets and aliases.
	Query string `json:"query" jsonschema:"the pwrq/jq program to evaluate; may call any registered cmdlet or alias"`
	// Input is the data the program runs against: a stream of JSON values, raw
	// text lines when RawInput is set, or empty for a single null input.
	Input string `json:"input,omitempty" jsonschema:"input data: a stream of JSON values, raw text when rawInput is set, or empty for a single null input"`
	// RawInput reads Input as lines of raw text rather than JSON (jq -R).
	RawInput bool `json:"rawInput,omitempty" jsonschema:"read input as raw lines of text instead of JSON (jq -R)"`
	// Slurp reads the whole input as a single array (jq -s).
	Slurp bool `json:"slurp,omitempty" jsonschema:"read all inputs into a single array (jq -s)"`
	// NullInput runs the program on null, ignoring the input (jq -n).
	NullInput bool `json:"nullInput,omitempty" jsonschema:"ignore input and run the query on null (jq -n)"`
	// Raw outputs string results without quotes (jq -r).
	Raw bool `json:"raw,omitempty" jsonschema:"output string values without quotes (jq -r)"`
	// Compact outputs each result on a single line (jq -c).
	Compact bool `json:"compact,omitempty" jsonschema:"output each result on one line (jq -c)"`
	// Indent is the number of spaces of indentation when not compact.
	Indent int `json:"indent,omitempty" jsonschema:"spaces of indentation when not compact"`
	// Limit caps the number of results. Zero means the default; it is clamped.
	Limit int `json:"limit,omitempty" jsonschema:"maximum number of results, default 1000, capped at 100000"`
	// TimeoutMs caps how long the run may take. Zero means the default; it is clamped.
	TimeoutMs int `json:"timeoutMs,omitempty" jsonschema:"timeout in milliseconds, default 30000"`
	// Args binds named variables reachable from the query as $name, like jq --argjson.
	Args []namedArg `json:"args,omitempty" jsonschema:"named variables bound like jq --argjson, reachable as $name"`
}

// namedArg binds one variable. Value is JSON text, so any value can be bound.
type namedArg struct {
	Name  string `json:"name" jsonschema:"variable name, with or without the leading dollar"`
	Value string `json:"value" jsonschema:"JSON text bound to the variable"`
}

// runQueryResult is what a run produced. Output and error are not exclusive: a
// query that emits ten values and then fails has told you something about all
// eleven, so both are reported.
type runQueryResult struct {
	// Values are the encoded results, one per query output.
	Values []string `json:"values"`
	// Count is the number of results emitted.
	Count int `json:"count"`
	// Truncated reports that a limit or byte cap stopped the run early.
	Truncated bool `json:"truncated"`
	// Error is the failure message, if the run did not complete cleanly.
	Error string `json:"error,omitempty"`
	// Kind classifies the failure: parse, compile, args, input, runtime,
	// timeout, limit or halt.
	Kind string `json:"kind,omitempty"`
	// ElapsedMs is how long the run took.
	ElapsedMs float64 `json:"elapsedMs"`
}

// listFunctionsArgs filters the cmdlet catalog.
type listFunctionsArgs struct {
	// Filter is a substring matched against a function's name or category.
	// Empty lists everything.
	Filter string `json:"filter,omitempty" jsonschema:"optional substring matched against function name or category; empty lists everything"`
}

// functionInfo is one documented cmdlet, as the catalog reports it.
type functionInfo struct {
	Name        string   `json:"name"`
	MinArgs     int      `json:"minArgs"`
	MaxArgs     int      `json:"maxArgs"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
}

// listFunctionsResult is the full or filtered cmdlet catalog.
type listFunctionsResult struct {
	Functions []functionInfo `json:"functions"`
	Count     int            `json:"count"`
}

// validateQueryArgs asks whether a query parses.
type validateQueryArgs struct {
	Query string `json:"query" jsonschema:"the pwrq/jq program to parse"`
}

// validateQueryResult reports the answer, and the query laid out over multiple
// lines when it is valid.
type validateQueryResult struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Formatted string `json:"formatted,omitempty"`
}
