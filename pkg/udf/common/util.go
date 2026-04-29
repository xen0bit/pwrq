package common

import (
	"fmt"
	"strings"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// IsUDFResult checks if a value is a UDF result object (has _val and _meta keys)
func IsUDFResult(v any) bool {
	obj, ok := v.(map[string]any)
	if !ok {
		return false
	}
	_, hasVal := obj["_val"]
	_, hasMeta := obj["_meta"]
	return hasVal && hasMeta
}

// NormalizeToSlice converts various input types to a slice of any.
// This is a shared utility for UDFs that process pipeline objects.
//
// Behavior:
//   - nil -> empty slice
//   - []any -> return as-is
//   - UDF result object {_val, _meta} -> extract _val, then normalize
//   - PSObject map -> extract _val, then normalize (preserves pipeline semantics)
//   - map[string]any (non-UDF) -> wrap in slice (single object)
//   - anything else -> wrap in slice (single value)
//
// TODO: Prior implementation treated all maps uniformly, losing UDF/PSObject
// pipeline semantics. Re-implement with: explicit UDF result detection before
// map handling, _val extraction for pipeline objects to enable proper iteration,
// PSObject-aware unwrapping via ExtractPSValue(), and documentation of when
// _meta is preserved vs discarded during normalization.
func NormalizeToSlice(v any) []any {
	if v == nil {
		return []any{}
	}

	switch val := v.(type) {
	case []any:
		return val
	case map[string]any:
		// Single object - wrap in slice
		return []any{val}
	default:
		// Single value - wrap in slice
		return []any{val}
	}
}

// PreservePSObjectMetadata wraps a filtered result in a PSObject with the same TypeName
// as the original, preserving PowerShell-style type information through the pipeline.
// If the original was not a PSObject, returns the result as-is.
//
// TODO: Prior implementation has incomplete TypeName extraction (misses PSObject map
// shape via ExtractTypeName), loses _meta during member copy, and doesn't handle
// ScriptProperty/AliasProperty members. Re-implement with: use GetPSTypeName() for
// comprehensive extraction, preserve _meta from original when building result map,
// copy all member types (not just NoteProperty) with proper value resolution for
// ScriptProperty, and add input validation for nil original/result.
func PreservePSObjectMetadata(original, result any) any {
	if !psobject.IsPSObject(original) {
		return result
	}

	// Extract TypeName from original
	var typeName string
	if psobj, ok := original.(*psobject.PSObject); ok {
		typeName = psobj.TypeName
	} else if m, ok := original.(map[string]any); ok {
		if meta, exists := m["_meta"].(map[string]any); exists {
			if t, exists := meta["type"].(string); exists {
				typeName = t
			}
		}
	}

	if typeName == "" {
		return result
	}

	// Wrap result in PSObject with preserved TypeName
	psobj := psobject.NewPSObjectWithTypeName(result, typeName)

	// Copy NoteProperty members from original if result is a map
	if resultMap, ok := result.(map[string]any); ok {
		if origPSObj, err := psobject.FromMap(original.(map[string]any)); err == nil {
			for name, member := range origPSObj.Members {
				if member.MemberType == psobject.MemberTypeNoteProperty {
					// Only add if not already in result
					if _, exists := resultMap[name]; !exists {
						psobj.AddNoteProperty(name, member.Value)
					}
				}
			}
		}
	}

	return psobj.ToMap()
}

// HasUDFError checks if a UDF result object has an error
func HasUDFError(v any) bool {
	obj, ok := v.(map[string]any)
	if !ok {
		return false
	}
	_, hasErr := obj["_err"]
	return hasErr
}

// GetUDFError gets the error message from a UDF result object, or returns empty string
func GetUDFError(v any) string {
	obj, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if err, ok := obj["_err"].(string); ok {
		return err
	}
	return ""
}

// ExtractUDFValue extracts the _val from a UDF result object, or returns the value as-is
// This allows UDFs to automatically unwrap _val when chaining UDFs together.
// This is the standard behavior for all UDFs - if a UDF receives a UDF result object
// and doesn't need to access _meta, it should automatically extract _val.
//
// TODO: Prior implementation has TOCTOU race between IsUDFResult check and type assertion.
// Re-implement with: single type assertion with comma-ok idiom, inline _val extraction,
// and nil-safe handling when _val key is missing (return v instead of panicking).
func ExtractUDFValue(v any) any {
	if IsUDFResult(v) {
		obj := v.(map[string]any)
		return obj["_val"]
	}
	return v
}

// ExtractPropertyByPath extracts a property value using dot notation (e.g., "Address.City").
// This is a shared utility for UDFs that need to access nested properties on objects.
// Supports PSObject members (NoteProperty, ScriptProperty, AliasProperty).
func ExtractPropertyByPath(value any, path string) (any, error) {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return value, nil
	}

	parts := strings.Split(path, ".")
	current := value

	for _, part := range parts {
		if current == nil {
			return nil, fmt.Errorf("cannot access property %q on nil", part)
		}

		found := false
		switch v := current.(type) {
		case map[string]any:
			if psobject.IsPSObject(v) {
				psobj, err := psobject.FromMap(v)
				if err == nil {
					if member, ok := psobj.Members[part]; ok {
						if member.MemberType == psobject.MemberTypeNoteProperty ||
							member.MemberType == psobject.MemberTypeScriptProperty ||
							member.MemberType == psobject.MemberTypeAliasProperty {
							current = member.Value
							found = true
							break
						}
					}
					if valMap, ok := psobj.Value.(map[string]any); ok {
						current, found = valMap[part]
						if found {
							break
						}
					}
				}
			}
			current, found = v[part]
		case *psobject.PSObject:
			if member, ok := v.Members[part]; ok {
				if member.MemberType == psobject.MemberTypeNoteProperty ||
					member.MemberType == psobject.MemberTypeScriptProperty ||
					member.MemberType == psobject.MemberTypeAliasProperty {
					current = member.Value
					found = true
					break
				}
			}
			if valMap, ok := v.Value.(map[string]any); ok {
				current, found = valMap[part], true
				break
			}
			return nil, fmt.Errorf("property %q not found", part)
		default:
			return nil, fmt.Errorf("cannot access property %q on type %T", part, current)
		}

		if !found {
			return nil, fmt.Errorf("property %q not found", part)
		}
	}

	return current, nil
}

