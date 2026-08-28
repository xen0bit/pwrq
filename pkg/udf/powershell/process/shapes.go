package process

import "github.com/xen0bit/pwrq/pkg/core/shape"

// ProcessInfoShape is one running process.
//
// get_process is one of the two cmdlets the README leads with, and until this
// declaration it emitted an object with no type name and no documented
// properties: a caller had to run it once and read the keys off the result.
var ProcessInfoShape = shape.Fixed("Pwrq.Process",
	shape.Prop("Id", shape.Number, "process id"),
	shape.Prop("Name", shape.String, "executable name, without the directory"),
	shape.Prop("Path", shape.String, "full path to the executable, or \"\" when it could not be read"),
	shape.Prop("CPU", shape.Number, "percent of a core, as the platform reports it"),
	shape.Prop("WorkingSet", shape.Number, "resident memory in bytes"),
	shape.Prop("VirtualMemory", shape.Number, "virtual memory in bytes"),
	shape.Prop("StartTime", shape.String, "RFC 3339 timestamp"),
	shape.Prop("PriorityClass", shape.String, "scheduling priority, as a name"),
	shape.Prop("Threads", shape.Number, "thread count"),
	shape.Prop("Handles", shape.Number, "open handle count; always 0 on Unix, which does not report it"),
	shape.Prop("ResponseTime", shape.String, "duration since the process started, as a Go duration string"),
	shape.Prop("TotalProcessorTime", shape.String, "CPU time used, as a Go duration string"),
	shape.Prop("UserProcessorTime", shape.String, "CPU time used in user mode, as a Go duration string"),
	shape.OptProp("UserName", shape.String, "owner; present only when the IncludeUserName option is set"),
)
