// Compare-Object: what differs between two collections.
package collection

import (
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterCompareObject registers compare_object, PowerShell's Compare-Object:
// the values that differ between a reference collection and a difference
// collection, each tagged with the side it came from.
//
// deep_diff already answers "what changed in this object". This answers the
// other question — "what is in one list and not the other" — which the set
// cmdlets can only answer one direction at a time, and without saying how many
// times a value occurred.
//
//	compare_object(["a","b"]; ["b","c"])
//	  [{InputObject: "a", SideIndicator: "<="},
//	   {InputObject: "c", SideIndicator: "=>"}]
//
// With {IncludeEqual: true} the matches come too, tagged "==", which is what
// makes it useful for reconciling two exports rather than just diffing them.
//
// The options object is only available in the explicit form: the difference
// collection is one operand, so at two arguments the leading one is always the
// reference. Piping a reference and passing options at once is an error rather
// than a guess.
func RegisterCompareObject() gojq.CompilerOption {
	common.DeclareInput("compare_object", common.InputPipeline)
	return common.WithFunctionOf("compare_object", 1, 3, ComparisonShape.Each(), func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		reference, err := common.BindArray(in, "compare_object")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		difference, err := common.BindArray(rest[0], "compare_object")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		includeEqual, excludeDifferent, property, err := compareOptions(rest[1:])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("compare_object: %v", err), nil)
		}

		// Compare by a property when asked, so two lists of objects can be
		// reconciled on an id rather than on whole-value identity.
		identity := func(item any) (string, error) {
			if property == "" {
				return keyOf(item), nil
			}
			m, ok := common.BindValue(item).(map[string]any)
			if !ok {
				return "", fmt.Errorf("compare_object: expected objects when comparing by property %q, got %T", property, common.BindValue(item))
			}
			val, exists := m[property]
			if !exists {
				return "", fmt.Errorf("compare_object: property %q not found on a row", property)
			}
			return keyOf(val), nil
		}

		// Counts, not sets: a value present twice on the left and once on the
		// right leaves one unmatched, and dropping that is a wrong answer.
		refCount := map[string]int{}
		refFirst := map[string]any{}
		for _, item := range reference {
			k, err := identity(item)
			if err != nil {
				return common.MakeUDFErrorResult(err, nil)
			}
			refCount[k]++
			if _, seen := refFirst[k]; !seen {
				refFirst[k] = item
			}
		}
		diffCount := map[string]int{}
		diffFirst := map[string]any{}
		for _, item := range difference {
			k, err := identity(item)
			if err != nil {
				return common.MakeUDFErrorResult(err, nil)
			}
			diffCount[k]++
			if _, seen := diffFirst[k]; !seen {
				diffFirst[k] = item
			}
		}

		row := func(item any, side string) any {
			return ComparisonShape.Build(map[string]any{
				"InputObject":   item,
				"SideIndicator": side,
			})
		}

		out := []any{}
		// Reference order first, then the difference-only values, so the
		// output reads in the order the caller supplied. Matches are consumed
		// as they are found, so two "a" on the left against one on the right
		// reports the surplus rather than treating both sides as sets.
		unmatched := make(map[string]int, len(diffCount))
		for k, n := range diffCount {
			unmatched[k] = n
		}
		for _, item := range reference {
			k, _ := identity(item)
			if unmatched[k] > 0 {
				unmatched[k]--
				if includeEqual {
					out = append(out, row(item, "=="))
				}
				continue
			}
			if !excludeDifferent {
				out = append(out, row(item, "<="))
			}
		}
		if !excludeDifferent {
			surplus := make(map[string]int, len(refCount))
			for k, n := range diffCount {
				if extra := n - refCount[k]; extra > 0 {
					surplus[k] = extra
				}
			}
			for _, item := range difference {
				k, _ := identity(item)
				if surplus[k] > 0 {
					surplus[k]--
					out = append(out, row(item, "=>"))
				}
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

func compareOptions(args []any) (includeEqual, excludeDifferent bool, property string, err error) {
	if len(args) == 0 {
		return false, false, "", nil
	}
	m, ok := common.BindValue(args[0]).(map[string]any)
	if !ok {
		return false, false, "", fmt.Errorf("options must be an object, got %T", common.BindValue(args[0]))
	}
	for k, val := range m {
		switch lower(k) {
		case "includeequal":
			b, ok := val.(bool)
			if !ok {
				return false, false, "", fmt.Errorf("IncludeEqual must be a boolean")
			}
			includeEqual = b
		case "excludedifferent":
			b, ok := val.(bool)
			if !ok {
				return false, false, "", fmt.Errorf("ExcludeDifferent must be a boolean")
			}
			excludeDifferent = b
		case "property":
			s, ok := val.(string)
			if !ok {
				return false, false, "", fmt.Errorf("expected a string for Property")
			}
			property = s
		default:
			return false, false, "", fmt.Errorf("unknown option %q", k)
		}
	}
	return includeEqual, excludeDifferent, property, nil
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
