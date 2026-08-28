package variables

import "github.com/xen0bit/pwrq/pkg/core/shape"

// VariableShape is one session variable.
var VariableShape = shape.Fixed("System.Management.Automation.PSVariable",
	shape.Prop("Name", shape.String, "variable name, without the leading dollar"),
	shape.Prop("Value", shape.Any, "whatever the variable holds"),
	shape.Prop("Options", shape.String, "None, ReadOnly or Constant"),
	shape.Prop("Scope", shape.String, "the scope the variable lives in"),
	shape.OptProp("Description", shape.String, "the description, when one was set"),
)
