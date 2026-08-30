package udf

import "github.com/xen0bit/pwrq/pkg/udf/discovery"

// The keys each cmdlet reads out of an options object.
//
// Twenty-five cmdlets are documented as taking `[options]`, and until this
// table existed that was the whole of what they said. The keys were real -
// invoke_web_request has honoured Headers since it was written - but they
// appeared on no surface a caller could read, so the only way to find one was
// to already know it. A wrong guess is not an error either: an unknown key is
// ignored in silence by most of these, and what comes back is the call made
// without it.
//
// That combination is worse than an undocumented feature. In a recorded
// session a model needed HTTP basic auth, guessed
// `{Authentication: {Basic: [...]}}`, watched it do nothing, and concluded in
// its summary to the user that "the pwrq runtime doesn't expose auth-option
// plumbing in this environment". The runtime does:
//
//	invoke_web_request("https://example.com/private";
//	                   {Headers: {Authorization: "Basic ..."}})
//
// works, and returns 200.
//
// # Why this is a table and not a declaration at the registration site
//
// Everywhere else in this package - Streaming, Shape, Input, Returns - the
// fact is declared where the cmdlet is registered, because that is the one
// place a declaration cannot be forgotten and cannot disagree with the code.
// Options do not have such a place. Each of these cmdlets parses its own
// options object by hand, in its own file, with its own switch; there is no
// chokepoint that sees the keys go by. A declaration at the registration site
// would therefore be exactly as separate from the parsing code as this table
// is, while being spread across twenty files instead of one.
//
// So it is a table, and the honest cost is that a renamed key here goes stale
// silently. TestDocumentedOptionsAreAccepted is the guard, for every cmdlet
// that can be driven without a network or a service manager: it passes each
// documented key and asserts the option took effect, so a rename fails here
// rather than in a caller's query.
var documentedOptionKeys = map[string][]discovery.Option{
	"invoke_web_request": {
		{Name: "Uri", Type: "string", Description: "the URL to request, if not given as the first argument"},
		{Name: "Method", Type: "string", Description: "the HTTP verb; GET by default"},
		{Name: "Headers", Type: "object", Description: "request headers as name/value pairs - this is where Authorization goes"},
		{Name: "Body", Type: "any", Description: "the request body; an object is sent as JSON"},
		{Name: "ContentType", Type: "string", Description: "the Content-Type header to send with the body"},
		{Name: "TimeoutSec", Type: "number", Description: "how long to wait for the response, in seconds; 30 by default"},
		{Name: "SkipSSLVerify", Type: "boolean", Description: "accept a certificate that does not verify"},
		{Name: "AllowAutoRedirect", Type: "boolean", Description: "follow redirects; true by default"},
		{Name: "MaximumRedirection", Type: "number", Description: "how many redirects to follow; 5 by default"},
		{Name: "OutFile", Type: "string", Description: "write the response body to this path"},
		{Name: "PassThru", Type: "boolean", Description: "return the response object even when OutFile was written"},
	},
	"invoke_rest_method": {
		{Name: "Uri", Type: "string", Description: "the URL to request, if not given as the first argument"},
		{Name: "Method", Type: "string", Description: "the HTTP verb; GET by default"},
		{Name: "Headers", Type: "object", Description: "request headers as name/value pairs - this is where Authorization goes"},
		{Name: "Body", Type: "any", Description: "the request body; an object is sent as JSON"},
		{Name: "ContentType", Type: "string", Description: "the Content-Type header to send with the body"},
		{Name: "TimeoutSec", Type: "number", Description: "how long to wait for the response, in seconds; 30 by default"},
		{Name: "SkipSSLVerify", Type: "boolean", Description: "accept a certificate that does not verify"},
		{Name: "AllowAutoRedirect", Type: "boolean", Description: "follow redirects; true by default"},
		{Name: "MaximumRedirection", Type: "number", Description: "how many redirects to follow; 5 by default"},
		{Name: "OutFile", Type: "string", Description: "write the response body to this path"},
		{Name: "PassThru", Type: "boolean", Description: "return the response object even when OutFile was written"},
	},
	"get_childitem": {
		{Name: "Path", Type: "string", Description: "where to list, if not given as the first argument"},
		{Name: "Filter", Type: "string", Description: "a wildcard the name must match, such as *.go"},
		{Name: "Recurse", Type: "boolean", Description: "descend into subdirectories"},
		{Name: "Force", Type: "boolean", Description: "include hidden entries"},
		{Name: "Name", Type: "string", Description: "return only entries with this name"},
		{Name: "Include", Type: "array", Description: "wildcards an entry must match to be listed"},
		{Name: "Exclude", Type: "array", Description: "wildcards that keep an entry out of the listing"},
		{Name: "Depth", Type: "number", Description: "how far to recurse; unlimited when absent"},
		{Name: "Directory", Type: "boolean", Description: "list directories only"},
		{Name: "File", Type: "boolean", Description: "list files only"},
	},
	"select_string": {
		{Name: "Include", Type: "string", Description: "a wildcard limiting which files are searched"},
		{Name: "Context", Type: "number", Description: "how many lines either side of a match to carry"},
		{Name: "CaseSensitive", Type: "boolean", Description: "match case exactly; case-insensitive by default"},
		{Name: "List", Type: "boolean", Description: "report only the first match in each file"},
	},
	"out_file": {
		{Name: "Append", Type: "boolean", Description: "add to the end of the file rather than replacing it"},
		{Name: "Encoding", Type: "string", Description: "the text encoding to write in; utf8 by default"},
		{Name: "Force", Type: "boolean", Description: "write even when the file is read-only"},
		{Name: "Value", Type: "any", Description: "what to write, if not taken from the pipeline"},
	},
	"add_content": {
		{Name: "Encoding", Type: "string", Description: "the text encoding to write in; utf8 by default"},
		{Name: "Force", Type: "boolean", Description: "write even when the file is read-only"},
		{Name: "Value", Type: "any", Description: "what to append, if not taken from the pipeline"},
	},
	"out_sqlite": {
		{Name: "Create", Type: "boolean", Description: "create the table when it does not exist"},
		{Name: "Truncate", Type: "boolean", Description: "empty the table before writing"},
	},
	"compare_object": {
		{Name: "IncludeEqual", Type: "boolean", Description: "report the values both sides share as well as the differences"},
		{Name: "ExcludeDifferent", Type: "boolean", Description: "report only what both sides share"},
		{Name: "Property", Type: "string", Description: "compare objects by this property rather than whole"},
	},
	// where_object and measure_object read their keys in lower case only,
	// where most of the cmdlets around them accept either case. Writing
	// {Sum: true} to measure_object is not an error: it returns a measurement
	// with no Sum in it, which is the failure this whole table exists to stop.
	"where_object": {
		{Name: "property", Type: "string", Description: "the property to test; lower case only"},
		{Name: "operator", Type: "string", Description: "eq, ne, gt, ge, lt, le, like, notlike, match, notmatch, contains or notcontains; lower case only"},
		{Name: "value", Type: "any", Description: "what to compare the property against; lower case only"},
		{Name: "script", Type: "string", Description: "a pwrq expression to filter by, instead of property and operator; lower case only"},
		{Name: "casesensitive", Type: "boolean", Description: "compare case exactly; lower case only"},
	},
	"measure_object": {
		{Name: "property", Type: "string", Description: "the property to measure; lower case only, and required before any of the statistics below"},
		{Name: "sum", Type: "boolean", Description: "include the total; lower case only"},
		{Name: "average", Type: "boolean", Description: "include the mean; lower case only"},
		{Name: "minimum", Type: "boolean", Description: "include the smallest value; lower case only"},
		{Name: "maximum", Type: "boolean", Description: "include the largest value; lower case only"},
	},
	"get_date": {
		{Name: "Format", Type: "string", Description: "a Go layout to render the date with"},
		{Name: "DisplayHint", Type: "string", Description: "Date, Time or DateTime, choosing which parts to show"},
		{Name: "Year", Type: "number", Description: "override the year"},
		{Name: "Month", Type: "number", Description: "override the month"},
		{Name: "Day", Type: "number", Description: "override the day"},
		{Name: "Hour", Type: "number", Description: "override the hour"},
		{Name: "Minute", Type: "number", Description: "override the minute"},
		{Name: "Second", Type: "number", Description: "override the second"},
	},
	"get_process": {
		{Name: "Name", Type: "string", Description: "list only processes with this name"},
		{Name: "Id", Type: "number", Description: "list only the process with this id"},
		{Name: "IncludeUserName", Type: "boolean", Description: "report the user each process runs as"},
	},
	"start_process": {
		{Name: "FilePath", Type: "string", Description: "the program to run, if not given as the first argument"},
		{Name: "ArgumentList", Type: "array", Description: "the arguments to pass"},
		{Name: "WorkingDirectory", Type: "string", Description: "the directory to run in"},
		{Name: "Environment", Type: "object", Description: "environment variables to set for the child"},
		{Name: "PassThru", Type: "boolean", Description: "return the process object"},
		{Name: "WindowStyle", Type: "string", Description: "how to show the window, on the platforms that have one"},
	},
	"stop_process": {
		{Name: "Name", Type: "string", Description: "stop processes with this name"},
		{Name: "Id", Type: "number", Description: "stop the process with this id"},
		{Name: "Force", Type: "boolean", Description: "kill rather than ask to terminate"},
	},
	"get_service": {
		{Name: "Name", Type: "string", Description: "list only services with this name"},
		{Name: "DisplayName", Type: "string", Description: "list only services with this display name"},
		{Name: "Exclude", Type: "string", Description: "a name to leave out of the listing"},
	},
	"start_service": {
		{Name: "Name", Type: "string", Description: "the service to start, if not given as the first argument"},
		{Name: "PassThru", Type: "boolean", Description: "return the service object"},
	},
	"stop_service": {
		{Name: "Name", Type: "string", Description: "the service to stop, if not given as the first argument"},
		{Name: "Force", Type: "boolean", Description: "stop it even when other services depend on it"},
		{Name: "PassThru", Type: "boolean", Description: "return the service object"},
	},
	"get_variable": {
		{Name: "Name", Type: "string", Description: "the variable to read, if not given as the first argument"},
		{Name: "ValueOnly", Type: "boolean", Description: "return the value rather than an object describing it"},
		{Name: "Scope", Type: "string", Description: "which scope to read from"},
		{Name: "Include", Type: "array", Description: "wildcards a name must match"},
		{Name: "Exclude", Type: "array", Description: "wildcards that keep a name out"},
	},
	"set_variable": {
		{Name: "Name", Type: "string", Description: "the variable to set, if not given as the first argument"},
		{Name: "Value", Type: "any", Description: "what to set it to"},
		{Name: "Description", Type: "string", Description: "a note stored alongside the variable"},
		{Name: "Option", Type: "string", Description: "None, ReadOnly, Constant or Private"},
		{Name: "Scope", Type: "string", Description: "which scope to set it in"},
		{Name: "PassThru", Type: "boolean", Description: "return the variable object"},
	},
	"remove_variable": {
		{Name: "Name", Type: "string", Description: "the variable to remove, if not given as the first argument"},
		{Name: "Scope", Type: "string", Description: "which scope to remove it from"},
		{Name: "Force", Type: "boolean", Description: "remove it even when it is read-only"},
		{Name: "Exclude", Type: "array", Description: "wildcards that keep a name from being removed"},
	},
	"copy_item": {
		{Name: "Path", Type: "string", Description: "what to copy, if not given as the first argument"},
		{Name: "Destination", Type: "string", Description: "where to copy it to"},
		{Name: "Recurse", Type: "boolean", Description: "copy a directory's contents too"},
		{Name: "Force", Type: "boolean", Description: "overwrite what is already there"},
		{Name: "Filter", Type: "string", Description: "a wildcard the name must match"},
		{Name: "Include", Type: "array", Description: "wildcards an entry must match to be copied"},
		{Name: "Exclude", Type: "array", Description: "wildcards that keep an entry from being copied"},
	},
	"move_item": {
		{Name: "Path", Type: "string", Description: "what to move, if not given as the first argument"},
		{Name: "Destination", Type: "string", Description: "where to move it to"},
		{Name: "Force", Type: "boolean", Description: "overwrite what is already there"},
	},
	// new_item also parses ErrorCode and resolve_path a Credential, and both
	// are then never read: the value lands in the options struct and nothing
	// consults it. They are left out deliberately. Listing a key that does
	// nothing is worse than listing none, because a caller who reads it will
	// believe it works and spend the round trip finding out otherwise - which
	// is the failure this whole table is here to prevent.
	"new_item": {
		{Name: "Path", Type: "string", Description: "what to create, if not given as the first argument"},
		{Name: "ItemType", Type: "string", Description: "File or Directory"},
		{Name: "Value", Type: "any", Description: "the contents to give a new file"},
		{Name: "Name", Type: "string", Description: "the name to create under Path"},
		{Name: "Force", Type: "boolean", Description: "replace what is already there"},
	},
	"resolve_path": {
		{Name: "Path", Type: "string", Description: "the path to resolve, if not given as the first argument"},
		{Name: "Literal", Type: "boolean", Description: "treat wildcards in the path as ordinary characters"},
	},
	"test_connection": {
		{Name: "Target", Type: "string", Description: "the host to reach, if not given as the first argument"},
		{Name: "Count", Type: "number", Description: "how many probes to send"},
		{Name: "TimeoutSeconds", Type: "number", Description: "how long to wait for each"},
		{Name: "TcpPort", Type: "number", Description: "probe this TCP port rather than sending an echo"},
		{Name: "HttpProbe", Type: "boolean", Description: "probe over HTTP"},
		{Name: "HttpsProbe", Type: "boolean", Description: "probe over HTTPS"},
		{Name: "SkipSSLVerify", Type: "boolean", Description: "accept a certificate that does not verify"},
		{Name: "ResolveDNS", Type: "boolean", Description: "resolve the name and report the addresses"},
		{Name: "BufferSize", Type: "number", Description: "how large a payload to send"},
		{Name: "TTL", Type: "number", Description: "the time-to-live to set on the probe"},
		{Name: "Quiet", Type: "boolean", Description: "return a boolean rather than an object"},
	},
}

// documentedOptions reports the keys a cmdlet reads out of an options object,
// for the catalogue to carry. A cmdlet that takes no options, or whose options
// nobody has written down, reports none rather than an empty guess.
func documentedOptions(name string) []discovery.Option {
	return documentedOptionKeys[name]
}
