package location

import "github.com/xen0bit/pwrq/pkg/core/shape"

// CurrentLocationShape is the working directory.
//
// It is a smaller object than resolve_path's System.IO.PathInfo despite the
// similar name: this one reports where the session is, and carries no path that
// was resolved from an argument.
var CurrentLocationShape = shape.Fixed("System.Management.Automation.PathInfo",
	shape.Prop("Path", shape.String, "the current working directory"),
	shape.Prop("Provider", shape.String, "the PowerShell provider, always FileSystem here"),
	shape.OptProp("Drive", shape.String, "the drive the location sits on, when one is set"),
)
