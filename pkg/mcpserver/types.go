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
//
// Every field carries a jsonschema tag, because these become the tool's
// advertised outputSchema and a model reads it. Without them the schema is
// typed but unexplained - `"count": {"type": "integer"}` - and values, which is
// an array of JSON *text* rather than of plain strings, invites exactly the
// wrong reading.
type runQueryResult struct {
	// Values are the encoded results, one per query output.
	Values []string `json:"values" jsonschema:"the results, one per query output, each encoded as JSON text that must be parsed again to get the value"`
	// Count is the number of results emitted.
	Count int `json:"count" jsonschema:"how many results the query emitted"`
	// Truncated reports that a limit or byte cap stopped the run early.
	Truncated bool `json:"truncated" jsonschema:"true when a result limit or byte cap stopped the run before the query finished producing"`
	// Error is the failure message, if the run did not complete cleanly.
	Error string `json:"error,omitempty" jsonschema:"why the run did not complete cleanly; a run can emit values and then fail, so this can be set alongside results"`
	// Kind classifies the failure: parse, compile, args, input, runtime,
	// timeout, limit or halt.
	Kind string `json:"kind,omitempty" jsonschema:"what kind of failure occurred: parse, compile, args, input, runtime, timeout, limit or halt"`
	// Shape describes the values that came back, so a caller can write the
	// next stage without reading all of them.
	Shape string `json:"shape,omitempty" jsonschema:"the shape of the values that were produced: how many, what kind, and which keys the objects carried"`
	// ElapsedMs is how long the run took.
	ElapsedMs float64 `json:"elapsedMs" jsonschema:"how long the run took, in milliseconds"`
}

// listFunctionsArgs filters the cmdlet catalog.
type listFunctionsArgs struct {
	// Filter is a substring matched against a function's name or category.
	// Empty lists everything.
	Filter string `json:"filter,omitempty" jsonschema:"optional substring matched against function name or category; empty lists everything"`
}

// functionInfo is one documented cmdlet, as the catalog reports it.
//
// The last five fields are what this catalog used to drop on the floor. pwrq
// knew all of them - get_help prints them, get_command carries them as data -
// but list_functions was built from the raw metadata table rather than from the
// catalogue discovery assembles, so a model over MCP saw a name, an arity and a
// sentence, and had to run a probe query to learn anything about the output.
type functionInfo struct {
	Name        string   `json:"name" jsonschema:"the cmdlet's name, as it is called in a query"`
	MinArgs     int      `json:"minArgs" jsonschema:"fewest arguments the cmdlet accepts"`
	MaxArgs     int      `json:"maxArgs" jsonschema:"most arguments the cmdlet accepts"`
	Category    string   `json:"category" jsonschema:"the vocabulary group the cmdlet belongs to"`
	Description string   `json:"description" jsonschema:"one line on what the cmdlet does"`
	Examples    []string `json:"examples,omitempty" jsonschema:"invocations that work as written"`
	// Aliases are the other names that reach this cmdlet, such as gci and dir
	// for get_childitem.
	Aliases []string `json:"aliases,omitempty" jsonschema:"other names that call the same cmdlet"`
	// Streaming decides whether the caller must collect with [...], which is
	// the single fact callers most often get wrong.
	Streaming bool `json:"streaming" jsonschema:"true when the cmdlet emits a stream of values: collect it with [...] before using length, map or sort_by"`
	// Output says what Streaming means, in words, because a bare boolean does
	// not tell a model what to write.
	Output string `json:"output" jsonschema:"what the cmdlet's cardinality means for the caller, in words"`
	// Input says where the cmdlet reads its input from, for the cmdlets that
	// take it either from the pipeline or as a leading argument.
	Input string `json:"input,omitempty" jsonschema:"where the cmdlet reads its input from; empty when the cmdlet has not said"`
	// Shape summarises the object the cmdlet emits, and TypeName is the key to
	// look the full property list up by.
	Shape    string `json:"shape,omitempty" jsonschema:"summary of the object the cmdlet emits and the keys it carries; empty when it does not emit an object"`
	TypeName string `json:"typeName,omitempty" jsonschema:"the PSTypeName the cmdlet's output carries, which identifies its shape"`
}

// listFunctionsResult is the full or filtered cmdlet catalog.
type listFunctionsResult struct {
	Functions []functionInfo `json:"functions" jsonschema:"the matching cmdlets"`
	Count     int            `json:"count" jsonschema:"how many cmdlets matched"`
}

// validateQueryArgs asks whether a query parses.
type validateQueryArgs struct {
	Query string `json:"query" jsonschema:"the pwrq/jq program to parse"`
}

// validateQueryResult reports the answer, and the query laid out over multiple
// lines when it is valid.
type validateQueryResult struct {
	OK        bool   `json:"ok" jsonschema:"true when the query parses"`
	Error     string `json:"error,omitempty" jsonschema:"why the query does not parse"`
	Formatted string `json:"formatted,omitempty" jsonschema:"the query laid out over several lines, one pipeline stage per line"`
}
