package service

import "github.com/xen0bit/pwrq/pkg/core/shape"

// ServiceShape is one system service.
//
// The three optional properties are genuinely conditional rather than merely
// sometimes-empty: the cmdlet adds each only when there is something to say, so
// a caller has to guard them. Saying so is most of the value of declaring the
// shape at all.
var ServiceShape = shape.Fixed("Pwrq.Service",
	shape.Prop("Name", shape.String, "service name, as the manager knows it"),
	shape.Prop("DisplayName", shape.String, "human-readable name"),
	shape.Prop("Status", shape.String, "Running, Stopped, or another manager state"),
	shape.Prop("StartType", shape.String, "Automatic, Manual or Disabled"),
	shape.Prop("CanStop", shape.Boolean, "whether the service accepts a stop request"),
	shape.Prop("CanPause", shape.Boolean, "whether the service accepts a pause request"),
	shape.Prop("CanShutdown", shape.Boolean, "whether the service is notified at shutdown"),
	shape.Prop("MachineName", shape.String, "host the service runs on"),
	shape.Prop("ServiceType", shape.String, "how the service is hosted"),
	shape.OptProp("DependentServices", shape.Array, "names of services that depend on this one; absent when none"),
	shape.OptProp("ServicesDependedOn", shape.Array, "names of services this one depends on; absent when none"),
	shape.OptProp("ProcessId", shape.Number, "pid of the running service; absent when it is not running"),
)
