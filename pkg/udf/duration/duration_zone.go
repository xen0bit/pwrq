// Time zones and date formatting: moving an instant between zones and writing
// it out in a chosen layout.
package duration

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// namedLayouts are the formats worth a name rather than a Go reference-time
// incantation. A layout that is not one of these is passed to Go as-is, so
// "2006-01-02" still works for anyone who knows the convention.
var namedLayouts = map[string]string{
	"rfc3339":  time.RFC3339,
	"rfc3339n": time.RFC3339Nano,
	"rfc1123":  time.RFC1123,
	"rfc822":   time.RFC822,
	"iso":      "2006-01-02T15:04:05Z07:00",
	"date":     "2006-01-02",
	"time":     "15:04:05",
	"datetime": "2006-01-02 15:04:05",
	"kitchen":  time.Kitchen,
	"stamp":    time.Stamp,
	"unix":     "unix",
	"http":     http1123,
}

const http1123 = "Mon, 02 Jan 2006 15:04:05 GMT"

func layoutFor(name string) string {
	if l, ok := namedLayouts[strings.ToLower(name)]; ok {
		return l
	}
	return name
}

// zoneOf resolves an IANA zone name, plus the two spellings people reach for
// that are not in the database.
func zoneOf(name string) (*time.Location, error) {
	switch strings.ToLower(name) {
	case "utc", "z":
		return time.UTC, nil
	case "local":
		return time.Local, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("unknown time zone %q: expected an IANA name like Europe/London", name)
	}
	return loc, nil
}

// zoneNames lists the zones this machine can resolve, by walking the tzdata
// directory. Go exposes no enumeration API — LoadLocation answers about one
// name at a time — so the directory is the only source of truth, and a build
// with no system tzdata correctly reports only the two zones it really has.
func zoneNames() []string {
	roots := []string{"/usr/share/zoneinfo", "/usr/lib/locale/TZ", "/usr/share/lib/zoneinfo"}
	if tz := os.Getenv("ZONEINFO"); tz != "" {
		roots = append([]string{tz}, roots...)
	}
	var names []string
	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return nil
			}
			// Skip the non-zone files that share the directory, and the
			// legacy flat names that are not Area/Location.
			if strings.HasSuffix(rel, ".tab") || strings.HasSuffix(rel, ".list") ||
				rel == "leapseconds" || rel == "tzdata.zi" || rel == "posixrules" {
				return nil
			}
			if !strings.Contains(rel, string(os.PathSeparator)) {
				return nil
			}
			// right/ and posix/ hold leap-second and POSIX variants of every
			// zone. They resolve, but listing them doubles the output with
			// names nobody means to pick.
			if strings.HasPrefix(rel, "right"+string(os.PathSeparator)) ||
				strings.HasPrefix(rel, "posix"+string(os.PathSeparator)) {
				return nil
			}
			names = append(names, filepath.ToSlash(rel))
			return nil
		})
		if len(names) > 0 {
			break
		}
	}
	names = append(names, "UTC", "Local")
	sort.Strings(names)
	return names
}

// RegisterToTimezone registers to_timezone, the same instant expressed in
// another zone.
//
// The returned object carries the offset and abbreviation alongside the
// formatted time, because "is this 09:00 UTC or 09:00 local" is exactly the
// question the cmdlet exists to answer, and a bare string does not say.
func RegisterToTimezone() gojq.CompilerOption {
	return common.WithFunction("to_timezone", 1, 2, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		t, err := parseTime(in)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("to_timezone: %v", err), nil)
		}
		name, err := common.BindString(rest[0], "zone")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("to_timezone: %v", err), nil)
		}
		loc, err := zoneOf(name)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("to_timezone: %v", err), nil)
		}
		local := t.In(loc)
		abbr, offset := local.Zone()
		return common.MakeUDFSuccessResult(map[string]any{
			psobject.PSTypeNameKey: "System.DateTimeOffset",
			"DateTime":             local.Format(time.RFC3339),
			"Timezone":             loc.String(),
			"Abbreviation":         abbr,
			"OffsetSeconds":        offset,
			"Offset":               local.Format("-07:00"),
			"Timestamp":            local.Unix(),
			"IsDST":                local.IsDST(),
		}, nil)
	})
}

// RegisterFormatDate registers format_date, an instant written out in a
// layout, optionally in a given zone.
func RegisterFormatDate() gojq.CompilerOption {
	return common.WithFunction("format_date", 1, 3, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 2)
		t, err := parseTime(in)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("format_date: %v", err), nil)
		}
		layout, err := common.BindString(rest[0], "layout")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("format_date: %v", err), nil)
		}
		if len(rest) > 1 {
			name, err := common.BindString(rest[1], "zone")
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("format_date: %v", err), nil)
			}
			loc, err := zoneOf(name)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("format_date: %v", err), nil)
			}
			t = t.In(loc)
		}
		l := layoutFor(layout)
		if l == "unix" {
			return common.MakeUDFSuccessResult(t.Unix(), nil)
		}
		if l == http1123 {
			return common.MakeUDFSuccessResult(t.UTC().Format(http1123), nil)
		}
		return common.MakeUDFSuccessResult(t.Format(l), nil)
	})
}

// RegisterParseDate registers parse_date, a string read with an explicit
// layout, optionally interpreted in a given zone.
//
// This is the inverse of format_date and the reason both exist: jq can render
// an epoch with strftime but cannot read "03/04/2026" back, and whether that is
// March or April is a question only the caller's layout answers.
func RegisterParseDate() gojq.CompilerOption {
	return common.WithFunction("parse_date", 1, 3, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 2)
		s, ok := common.BindValue(in).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("parse_date: expected a string, got %T", common.BindValue(in)), nil)
		}
		layout, err := common.BindString(rest[0], "layout")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("parse_date: %v", err), nil)
		}
		loc := time.UTC
		if len(rest) > 1 {
			name, err := common.BindString(rest[1], "zone")
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("parse_date: %v", err), nil)
			}
			if loc, err = zoneOf(name); err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("parse_date: %v", err), nil)
			}
		}
		t, err := time.ParseInLocation(layoutFor(layout), s, loc)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("parse_date: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(t.Format(time.RFC3339), nil)
	})
}

// RegisterListTimezones registers list_timezones, the zone names this build
// can resolve, filtered by a substring.
//
// Without it the only way to find out whether "Europe/Kyiv" or "Europe/Kiev"
// resolves on a given machine is to guess and read the error.
func RegisterListTimezones() gojq.CompilerOption {
	return common.WithFunction("list_timezones", 0, 1, func(v any, args []any) any {
		filter := ""
		in, rest := common.SplitInput(v, args, 0)
		if len(rest) > 0 {
			s, err := common.BindString(rest[0], "filter")
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("list_timezones: %v", err), nil)
			}
			filter = s
		} else if s, ok := common.BindValue(in).(string); ok {
			filter = s
		}
		filter = strings.ToLower(filter)
		out := []any{}
		for _, name := range zoneNames() {
			if filter == "" || strings.Contains(strings.ToLower(name), filter) {
				out = append(out, name)
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}
