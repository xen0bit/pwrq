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
	// MaxBytes caps how large one rendered value may be, which Limit does not:
	// a single fetched document is one result. Zero means the default.
	MaxBytes int `json:"maxBytes,omitempty" jsonschema:"maximum size of any one result in bytes, default 8192, capped at 1048576; a larger value is cut with a marker saying how much was dropped, so raise this only when you mean to read the whole thing"`
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
	Values []string `json:"values" jsonschema:"the results, one per query output, each encoded as JSON text that must be parsed again to get the value; a value the maxBytes cap cut ends with a marker and is no longer valid JSON"`
	// Count is the number of results emitted.
	Count int `json:"count" jsonschema:"how many results the query emitted"`
	// Truncated reports that a limit or byte cap stopped the run early.
	Truncated bool `json:"truncated" jsonschema:"true when a result limit or byte cap stopped the run before the query finished producing"`
	// Elided is how many individual values the per-value cap cut. A different
	// event from Truncated: every value was produced, some are shown in part.
	Elided int `json:"elided,omitempty" jsonschema:"how many individual results were too large for maxBytes and are shown in part; the query still produced all of them"`
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
	// Filter is a substring matched case-insensitively against a function's
	// name, aliases and category, then against its description, then against
	// nothing at all - at which point the nearest names come back instead.
	// Empty lists everything.
	Filter string `json:"filter,omitempty" jsonschema:"optional case-insensitive substring matched against function name, aliases and category, falling back to descriptions when nothing is named that way and to the nearest names when nothing matches; empty lists everything"`
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
	// Returns says how the output is encoded, for the cmdlets whose result is
	// a text rendering of bytes rather than the bytes themselves.
	Returns string `json:"returns,omitempty" jsonschema:"how the cmdlet's output is encoded, when that is not obvious from the name; empty when it needs no explaining"`
	// Options are the keys the cmdlet reads out of an options object.
	Options []optionInfo `json:"options,omitempty" jsonschema:"the keys the cmdlet reads out of an options object; an unlisted key is ignored in silence"`
	// Shape summarises the object the cmdlet emits, and TypeName is the key to
	// look the full property list up by.
	Shape    string `json:"shape,omitempty" jsonschema:"summary of the object the cmdlet emits and the keys it carries; empty when it does not emit an object"`
	TypeName string `json:"typeName,omitempty" jsonschema:"the PwrqType the cmdlet's output carries, which identifies its shape"`
}

// optionInfo is one key a cmdlet reads out of an options object.
type optionInfo struct {
	Name        string `json:"name" jsonschema:"the key as it is written in the object, in its own casing"`
	Type        string `json:"type" jsonschema:"the JSON type the value must have: string, number, boolean, object or array"`
	Description string `json:"description" jsonschema:"what the option does"`
}

// listFunctionsResult is the full or filtered cmdlet catalog.
//
// Matched and Suggestions exist because a search can succeed in more than one
// way, and the difference matters to whoever reads the answer. A cmdlet that
// is *named* for what you asked and one that merely *mentions* it are not the
// same claim, and a search that found neither has something better to offer
// than an empty list.
type listFunctionsResult struct {
	Functions []functionInfo `json:"functions" jsonschema:"the matching cmdlets"`
	Count     int            `json:"count" jsonschema:"how many cmdlets matched"`
	// Matched says which tier of the search answered.
	Matched string `json:"matched,omitempty" jsonschema:"how the filter matched: name, when it matched a name, alias or category; description, when nothing was named that way and these merely mention it; none, when nothing matched at all"`
	// Suggestions are the nearest names when nothing matched.
	Suggestions []string `json:"suggestions,omitempty" jsonschema:"names, aliases and categories closest to a filter that matched nothing"`
}

// validateQueryArgs asks whether a query parses and compiles.
type validateQueryArgs struct {
	Query string `json:"query" jsonschema:"the pwrq/jq program to parse and compile"`
	// Args names the variables the query may read, so one that mentions $name
	// compiles here as it would in a run. Only the names are used.
	Args []namedArg `json:"args,omitempty" jsonschema:"the same named variables you would pass to run_query, so a query reading $name compiles; only the names are used"`
}

// Validation stages. A query that does not parse is not a program at all; one
// that parses but does not compile is a program calling something that is not
// there, which is a different mistake with a different fix.
const (
	stageParse   = "parse"
	stageCompile = "compile"
)

// validateQueryResult reports the answer, and the query laid out over multiple
// lines - whether or not it was valid, since a compile failure is about one
// call inside an otherwise well-formed program and the caller is about to edit
// it.
type validateQueryResult struct {
	OK        bool   `json:"ok" jsonschema:"true when the query parses and compiles, which means it will run"`
	Error     string `json:"error,omitempty" jsonschema:"why the query is not valid"`
	Stage     string `json:"stage,omitempty" jsonschema:"how far the query got: parse, when it is not grammatical; compile, when it is grammatical but calls something that is not defined"`
	Formatted string `json:"formatted,omitempty" jsonschema:"the query laid out over several lines, one pipeline stage per line"`
}
