package logfile

import "github.com/xen0bit/pwrq/pkg/core/shape"

// MatchInfo is one matching line.
//
// Path and PSPath are the same string, and both are kept: Path is what
// PowerShell's Select-String reports, and PSPath is what a downstream cmdlet
// binds to when the match is piped onward.
var MatchInfo = shape.Fixed("Microsoft.PowerShell.Commands.MatchInfo",
	shape.Prop("Path", shape.String, "file the match was found in"),
	shape.OptProp("LineNumber", shape.Number, "one-based line number; absent in list mode"),
	shape.OptProp("Line", shape.String, "the whole matching line; absent in list mode"),
	shape.OptProp("Match", shape.String, "the part of the line the pattern matched; absent in list mode"),
	shape.OptProp("PSPath", shape.String, "the path, as the bindable value"),
	shape.OptProp("Before", shape.Array, "lines before the match; present only when context was asked for"),
	shape.OptProp("After", shape.Array, "lines after the match; present only when context was asked for"),
).Note("in list mode only Path is reported, since the question there is which files matched rather than where")
