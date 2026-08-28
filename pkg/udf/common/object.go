package common

import (
	"fmt"

	"github.com/xen0bit/pwrq/pkg/core/typed"
)

// EnsureObject wraps a value in a typed object if it isn't already, so UDFs can
// work uniformly with cmdlet output and plain JSON.
//
// Any JSON object is a valid object - a hand-written {"Name":"x"} binds the
// same way a cmdlet's output does. Objects carrying PwrqType keep their type.
func EnsureObject(v any) *typed.Object {
	if v == nil {
		return nil
	}

	if obj, ok := v.(*typed.Object); ok {
		return obj
	}

	if m, ok := v.(map[string]any); ok {
		if obj, err := typed.FromMap(m); err == nil {
			return obj
		}
	}

	return typed.New(v)
}

// TryEnsureObject is like EnsureObject but reports conversion failures.
func TryEnsureObject(v any) (*typed.Object, error) {
	if v == nil {
		return nil, nil
	}

	if obj, ok := v.(*typed.Object); ok {
		return obj, nil
	}

	if m, ok := v.(map[string]any); ok {
		obj, err := typed.FromMap(m)
		if err != nil {
			return nil, fmt.Errorf("malformed object's wire form: %w", err)
		}
		return obj, nil
	}

	return typed.New(v), nil
}

// MakeObjectResult converts a typed object to its JSON wire form.
func MakeObjectResult(obj *typed.Object) any {
	if obj == nil {
		return nil
	}
	return obj.ToJSON()
}

// MakeObjectErrorResult reports a cmdlet failure as a jq error, naming the
// PowerShell type that was being produced.
func MakeObjectErrorResult(err error, typeName string) any {
	if typeName == "" {
		return err
	}
	return fmt.Errorf("%s: %w", typeName, err)
}

// GetPwrqType extracts the PowerShell type name from a value.
func TypeNameOf(v any) string {
	return typed.TypeOf(v)
}

// AddMemberTo adds a member to a typed object, creating one if necessary.
func AddMemberTo(v any, name string, memberType typed.MemberType, value any) *typed.Object {
	obj := EnsureObject(v)
	obj.AddMember(name, memberType, value)
	return obj
}

// AddNoteProperty adds a NoteProperty to a typed object.
func AddNoteProperty(v any, name string, value any) *typed.Object {
	return AddMemberTo(v, name, typed.MemberTypeNoteProperty, value)
}

// ConvertObject converts a typed object to a different type.
func ConvertObject(v any, targetType string) (*typed.Object, error) {
	obj := EnsureObject(v)
	return typed.ConvertValue(obj, targetType)
}
