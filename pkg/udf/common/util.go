package common

import (
	"fmt"
	"strings"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// pathProperties are the properties that identify a filesystem location, in the
// order PowerShell's FileSystem provider would bind them. Name is deliberately
// absent: it is relative and cannot locate a file on its own.
var pathProperties = []string{psobject.PSPathKey, "FullName", "Path"}

// BindValue resolves pipeline input to the value a UDF should operate on.
//
// A scalar binds directly. Cmdlet output that wraps a scalar - a PSObject whose
// PSPath carries the real value - binds to that scalar, which is what lets one
// cmdlet's output feed the next. An ordinary JSON object binds as itself: the
// object cmdlets (where_object, sort_object, group_object) need the whole
// object, and collapsing it to some property would silently discard the rest.
func BindValue(v any) any {
	switch val := v.(type) {
	case *psobject.PSObject:
		if _, isMap := val.Value.(map[string]any); !isMap && val.Value != nil {
			return psobject.NormalizeJSON(val.Value)
		}
		return psobject.NormalizeJSON(val.Value)
	case map[string]any:
		// Only unwrap objects that are demonstrably a cmdlet's scalar output.
		if _, typed := val[psobject.PSTypeNameKey]; typed {
			if s, ok := val[psobject.PSPathKey].(string); ok {
				return s
			}
		}
		return val
	default:
		return v
	}
}

// BindPath resolves pipeline input to a filesystem path, following PowerShell's
// ByValue-then-ByPropertyName binding. This is what lets `find(".") | cat` (a
// stream of strings) and `get_childitem(".") | cat` (a stream of FileInfo
// objects) both work without the caller reaching for a property.
func BindPath(v any) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case *psobject.PSObject:
		if s, ok := val.Value.(string); ok {
			return s, true
		}
		for _, name := range pathProperties {
			if got, err := val.GetPropertyValue(name); err == nil {
				if s, ok := got.(string); ok && s != "" {
					return s, true
				}
			}
		}
	case map[string]any:
		for _, name := range pathProperties {
			if s, ok := val[name].(string); ok && s != "" {
				return s, true
			}
		}
	}
	return "", false
}

// BindString resolves pipeline input to a string parameter, reporting a usable
// error rather than silently binding the wrong thing.
func BindString(v any, param string) (string, error) {
	if s, ok := BindPath(v); ok {
		return s, nil
	}
	bound := BindValue(v)
	if bound == nil {
		return "", fmt.Errorf("cannot bind null to parameter %q", param)
	}
	return "", fmt.Errorf("cannot bind %T to string parameter %q", bound, param)
}

// NormalizeToSlice converts pipeline input to a slice the object cmdlets can
// iterate. A single object is a one-element pipeline, matching PowerShell,
// where every value is a pipeline of length one.
func NormalizeToSlice(v any) []any {
	switch val := v.(type) {
	case nil:
		return []any{}
	case []any:
		return val
	default:
		return []any{val}
	}
}

// PreserveTypeName carries the PowerShell type of an input object onto a value
// derived from it, so that a projection of a FileInfo still reports what it came
// from. Values derived from untyped JSON stay untyped.
func PreserveTypeName(original, result any) any {
	typeName := psobject.ExtractTypeName(original)
	if typeName == "" || !psobject.IsPSObject(original) {
		return result
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		return result
	}
	if _, exists := resultMap[psobject.PSTypeNameKey]; !exists {
		resultMap[psobject.PSTypeNameKey] = typeName
	}
	return resultMap
}

// ExtractPropertyByPath reads a property using dot notation ("Address.City"),
// resolving PSObject members as well as plain map keys.
func ExtractPropertyByPath(value any, path string) (any, error) {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return value, nil
	}

	current := value
	for _, part := range strings.Split(path, ".") {
		if current == nil {
			return nil, fmt.Errorf("cannot access property %q on nil", part)
		}

		switch v := current.(type) {
		case map[string]any:
			got, ok := v[part]
			if !ok {
				return nil, fmt.Errorf("property %q not found", part)
			}
			current = got
		case *psobject.PSObject:
			got, err := v.GetPropertyValue(part)
			if err == nil {
				current = got
				continue
			}
			valMap, ok := v.Value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("property %q not found", part)
			}
			got, ok = valMap[part]
			if !ok {
				return nil, fmt.Errorf("property %q not found", part)
			}
			current = got
		default:
			return nil, fmt.Errorf("cannot access property %q on type %T", part, current)
		}
	}

	return current, nil
}
