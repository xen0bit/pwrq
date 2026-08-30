package discovery

import "github.com/xen0bit/pwrq/pkg/core/shape"

// CommandInfoShape is one entry in the cmdlet catalogue.
//
// It describes get_command, which is itself the catalogue, so this is the one
// shape that has to describe the thing describing everything else. Shape and
// TypeName are the properties this change added: the first is the summary a
// caller reads, the second is the key they look the full property list up by.
var CommandInfoShape = shape.Fixed("Pwrq.Command",
	shape.Prop("Name", shape.String, "the callable name"),
	shape.Prop("Aliases", shape.Array, "other names that reach the same cmdlet"),
	shape.Prop("Category", shape.String, "the vocabulary group it belongs to"),
	shape.Prop("Description", shape.String, "one line on what it does"),
	shape.Prop("Examples", shape.Array, "invocations that work as written"),
	shape.Prop("MinArgs", shape.Number, "fewest arguments it accepts"),
	shape.Prop("MaxArgs", shape.Number, "most arguments it accepts"),
	shape.Prop("Available", shape.Boolean, "whether this build can actually run it"),
	shape.Prop("Streaming", shape.Boolean, "whether it emits a stream rather than one value"),
	shape.Prop("Output", shape.String, "what Streaming means for the caller, in words"),
	shape.Prop("Input", shape.String, "where it reads its input from, or \"\" when it has not said"),
	shape.Prop("Shape", shape.String, "one-line summary of the object it emits, or \"\" when it emits none"),
	shape.Prop("TypeName", shape.String, "the PwrqType its output carries, or \"\"; the key to look this shape up by"),
	shape.Prop("Returns", shape.String, "how its output is encoded when that is not obvious, or \"\""),
	shape.Prop("Options", shape.Array, "the keys it reads out of an options object, each {Name, Type, Description}"),
)
