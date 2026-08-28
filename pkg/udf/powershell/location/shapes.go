package location

import "github.com/xen0bit/pwrq/pkg/core/shape"

// CurrentLocationShape is the working directory.
//
// It is a smaller object than resolve_path's Pwrq.Path.Info despite the
// similar name: this one reports where the session is, and carries no path that
// was resolved from an argument.
var CurrentLocationShape = shape.Fixed("Pwrq.Location",
	shape.Prop("Path", shape.String, "the current working directory"),
	shape.Prop("Provider", shape.String, "which namespace the path lives in, always FileSystem here"),
	shape.OptProp("Drive", shape.String, "the drive the location sits on, when one was asked for"),
	shape.OptProp("StackName", shape.String, "the location stack this came from, when one was named"),
)
