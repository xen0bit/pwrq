package duration

import "github.com/xen0bit/pwrq/pkg/core/shape"

// ZonedTimeShape is an instant expressed in a named time zone.
var ZonedTimeShape = shape.Fixed("Pwrq.ZonedDateTime",
	shape.Prop("DateTime", shape.String, "the instant in the target zone, RFC 3339"),
	shape.Prop("Timezone", shape.String, "IANA zone name the instant was converted to"),
	shape.Prop("Abbreviation", shape.String, "zone abbreviation in effect at that instant, such as EDT"),
	shape.Prop("Offset", shape.String, "offset from UTC, formatted as ±HH:MM"),
	shape.Prop("OffsetSeconds", shape.Number, "offset from UTC in seconds"),
	shape.Prop("Timestamp", shape.Number, "seconds since the Unix epoch; unaffected by the zone"),
	shape.Prop("IsDST", shape.Boolean, "whether daylight saving was in effect at that instant"),
)
