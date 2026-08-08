package common

import (
	"fmt"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// EnsurePSObject wraps a value in a PSObject if it isn't already, so UDFs can
// work uniformly with cmdlet output and plain JSON.
//
// Any JSON object is a valid PSObject - a hand-written {"Name":"x"} binds the
// same way a cmdlet's output does. Objects carrying PSTypeName keep their type.
func EnsurePSObject(v any) *psobject.PSObject {
	if v == nil {
		return nil
	}

	if psobj, ok := v.(*psobject.PSObject); ok {
		return psobj
	}

	if m, ok := v.(map[string]any); ok {
		if psobj, err := psobject.FromMap(m); err == nil {
			return psobj
		}
	}

	return psobject.NewPSObject(v)
}

// TryEnsurePSObject is like EnsurePSObject but reports conversion failures.
func TryEnsurePSObject(v any) (*psobject.PSObject, error) {
	if v == nil {
		return nil, nil
	}

	if psobj, ok := v.(*psobject.PSObject); ok {
		return psobj, nil
	}

	if m, ok := v.(map[string]any); ok {
		psobj, err := psobject.FromMap(m)
		if err != nil {
			return nil, fmt.Errorf("malformed PSObject map: %w", err)
		}
		return psobj, nil
	}

	return psobject.NewPSObject(v), nil
}

// MakePSObjectResult converts a PSObject to its JSON wire form.
func MakePSObjectResult(psobj *psobject.PSObject) any {
	if psobj == nil {
		return nil
	}
	return psobj.ToJSON()
}

// MakePSObjectErrorResult reports a cmdlet failure as a jq error, naming the
// PowerShell type that was being produced.
func MakePSObjectErrorResult(err error, typeName string) any {
	if typeName == "" {
		return err
	}
	return fmt.Errorf("%s: %w", typeName, err)
}

// GetPSTypeName extracts the PowerShell type name from a value.
func GetPSTypeName(v any) string {
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
