package json

import "github.com/xen0bit/pwrq/pkg/core/shape"

var (
	// EditedDocument is the caller's document with one edit applied, so it has
	// the caller's keys. Saying this explicitly matters more than it looks:
	// these cmdlets return the *whole* document rather than the edited value,
	// which is the mistake a caller makes when they assume otherwise and
	// pipe the result straight into a field access.
	EditedDocument = shape.Derived("the whole input document with the edit applied, not just the edited value")

	// ParsedQueryString has one key per parameter.
	ParsedQueryString = shape.Derived("one key per query parameter, holding its value, or an array of values when the parameter repeats")
)
