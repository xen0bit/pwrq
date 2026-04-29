package common

import (
	"fmt"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// EnsurePSObject wraps a value in a PSObject if it isn't already.
// This allows UDFs to work with both raw values and PSObject-wrapped values.
//
// Behavior:
//   - If v is already a *PSObject, returns it unchanged
//   - If v is a map with PSObject shape ({_val, _meta}), converts it (ignoring malformed maps)
//   - If v is nil, returns nil (caller must handle nil checks)
//   - Otherwise, wraps v in a new PSObject with automatic type detection
//
// Note: This function returns nil for malformed PSObject maps rather than panicking.
// For error-aware conversion, use TryEnsurePSObject instead.
func EnsurePSObject(v any) *psobject.PSObject {
	if v == nil {
		return nil
	}

	// Already a PSObject - return as-is
	if psobj, ok := v.(*psobject.PSObject); ok {
		return psobj
	}

	// Check for PSObject-like map
	if m, ok := v.(map[string]any); ok {
		// Validate it has the basic PSObject shape before attempting conversion
		if psobject.IsPSObject(m) {
			psobj, err := psobject.FromMap(m)
			if err == nil {
				return psobj
			}
			// Malformed PSObject map - fall through to wrap as-is
		}
	}

	// Wrap raw value in a new PSObject
	return psobject.NewPSObject(v)
}

// TryEnsurePSObject is like EnsurePSObject but returns an error for malformed inputs.
// Use this when you need to handle conversion failures explicitly.
func TryEnsurePSObject(v any) (*psobject.PSObject, error) {
	if v == nil {
		return nil, nil
	}

	// Already a PSObject - return as-is
	if psobj, ok := v.(*psobject.PSObject); ok {
		return psobj, nil
	}

	// Check for PSObject-like map
	if m, ok := v.(map[string]any); ok {
		if psobject.IsPSObject(m) {
			psobj, err := psobject.FromMap(m)
			if err != nil {
				return nil, fmt.Errorf("malformed PSObject map: %w", err)
			}
			return psobj, nil
		}
	}

	// Wrap raw value in a new PSObject
	return psobject.NewPSObject(v), nil
}

// ExtractPSValue extracts the underlying value from a PSObject or PSObject-like map.
// Falls back to ExtractUDFValue for backward compatibility.
func ExtractPSValue(v any) any {
	if psobj, ok := v.(*psobject.PSObject); ok {
		return psobj.Value
	}
	if m, ok := v.(map[string]any); ok {
		if psobject.IsPSObject(m) {
			psobj, _ := psobject.FromMap(m)
			return psobj.Value
		}
	}
	// Fall back to old UDF extraction
	return ExtractUDFValue(v)
}

// MakePSObjectResult creates a UDF result object from a PSObject.
// Returns {_val: value, _meta: {type: typeName, members: {...}}}
func MakePSObjectResult(psobj *psobject.PSObject) map[string]any {
	if psobj == nil {
		return MakeUDFErrorResult(nil, map[string]any{"error": "nil PSObject"})
	}
	return psobj.ToMap()
}

// MakePSObjectErrorResult creates a UDF error result with PSObject metadata.
func MakePSObjectErrorResult(err error, typeName string) map[string]any {
	meta := map[string]any{
		"type": typeName,
		"error": err.Error(),
	}
	return MakeUDFErrorResult(err, meta)
}

// GetPSTypeName extracts the type name from a value, handling PSObject wrappers.
func GetPSTypeName(v any) string {
	if psobj, ok := v.(*psobject.PSObject); ok {
		return psobj.TypeName
	}
	if m, ok := v.(map[string]any); ok {
		if psobject.IsPSObject(m) {
			return psobject.ExtractTypeName(m)
		}
	}
	return psobject.ExtractTypeName(v)
}

// AddMemberToPSObject adds a member to a PSObject, creating one if necessary.
func AddMemberToPSObject(v any, name string, memberType psobject.MemberType, value any) *psobject.PSObject {
	psobj := EnsurePSObject(v)
	psobj.AddMember(name, memberType, value)
	return psobj
}

// AddNoteProperty adds a NoteProperty to a PSObject.
func AddNoteProperty(v any, name string, value any) *psobject.PSObject {
	return AddMemberToPSObject(v, name, psobject.MemberTypeNoteProperty, value)
}

// ConvertPSObject converts a PSObject to a different type.
func ConvertPSObject(v any, targetType string) (*psobject.PSObject, error) {
	psobj := EnsurePSObject(v)
	return psobject.ConvertValue(psobj, targetType)
}
