package censys

import (
	"fmt"
	"time"
)

// fetchPage runs one request and reports the objects it produced together with
// the cursor for the page after it. An empty cursor means there is no more.
type fetchPage func(token string) (batch []any, next string, err error)

// walkPages follows an endpoint's cursor.
//
// limit caps how many requests are made; 0 means "keep going until the API
// stops handing out a cursor". Defaulting to one page rather than to all of
// them is deliberate: these calls cost credits, and a query that quietly turned
// into two hundred requests would be an expensive surprise.
func walkPages(op, start string, limit int, fetch fetchPage) ([]any, error) {
	if limit < 0 {
		return nil, fmt.Errorf("%s: Pages must not be negative, got %d", op, limit)
	}

	var out []any
	token := start
	for page := 1; ; page++ {
		batch, next, err := fetch(token)
		if err != nil {
			return nil, err
		}
		out = append(out, batch...)

		// A cursor that does not advance would loop forever, so treat a
		// repeated token as the end rather than trusting the server.
		if next == "" || next == token {
			return out, nil
		}
		if limit > 0 && page >= limit {
			return out, nil
		}
		token = next
	}
}

// parseTime reads an RFC3339 timestamp option.
//
// The API rejects a bare date, so a caller who writes "2026-01-01" gets told
// what is missing here rather than as an opaque 400 three layers down.
func parseTime(op, name, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("%s: %s must be an RFC3339 timestamp such as 2026-01-01T00:00:00Z, got %q", op, name, value)
	}
	return &t, nil
}

// optString returns a pointer for a set option and nil for an unset one, which
// is how the SDK distinguishes "no filter" from "filter on the empty string".
func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// optInt is optString for a page size or a port: zero means unset.
func optInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

// optInt64 is optInt for the request fields the SDK types as int64.
func optInt64(n int) *int64 {
	if n == 0 {
		return nil
	}
	v := int64(n)
	return &v
}
