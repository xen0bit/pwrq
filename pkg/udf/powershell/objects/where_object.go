// Package objects provides PowerShell-style object manipulation cmdlets.
// This file implements Where-Object functionality for filtering pipeline objects
// based on property conditions or script block expressions.
package objects

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// FilterOperator enumerates the comparison operators supported by Where-Object
type FilterOperator int

const (
	FilterOperatorEq          FilterOperator = iota // -eq (equal)
	FilterOperatorNe                                // -ne (not equal)
	FilterOperatorGt                                // -gt (greater than)
	FilterOperatorGe                                // -ge (greater or equal)
	FilterOperatorLt                                // -lt (less than)
	FilterOperatorLe                                // -le (less or equal)
	FilterOperatorLike                              // -like (wildcard match)
	FilterOperatorNotLike                           // -notlike (wildcard non-match)
	FilterOperatorMatch                             // -match (regex match)
	FilterOperatorNotMatch                          // -notmatch (regex non-match)
	FilterOperatorContains                          // -contains (contains value)
	FilterOperatorNotContains                       // -notcontains (does not contain)
)

// String returns the PowerShell-style name for the operator
func (op FilterOperator) String() string {
	switch op {
	case FilterOperatorEq:
		return "eq"
	case FilterOperatorNe:
		return "ne"
	case FilterOperatorGt:
		return "gt"
	case FilterOperatorGe:
		return "ge"
	case FilterOperatorLt:
		return "lt"
	case FilterOperatorLe:
		return "le"
	case FilterOperatorLike:
		return "like"
	case FilterOperatorNotLike:
		return "notlike"
	case FilterOperatorMatch:
		return "match"
	case FilterOperatorNotMatch:
		return "notmatch"
	case FilterOperatorContains:
		return "contains"
	case FilterOperatorNotContains:
		return "notcontains"
	default:
		return "unknown"
	}
}

// WhereObjectOptions holds options for the where_object function
type WhereObjectOptions struct {
	// ScriptBlock is a jq filter expression (e.g. ".Age > 18"). Any jq
	// expression is valid; it is compiled once and run against each object.
	ScriptBlock string
	// Property is the property name to filter on (simplified syntax)
	Property string
	// Operator is the comparison operator (simplified syntax)
	Operator FilterOperator
	// Value is the value to compare against (simplified syntax)
	Value any
	// CaseSensitive indicates whether matching is case-sensitive
	CaseSensitive bool
}

// RegisterWhereObject registers the where_object function with gojq
// Supports PowerShell-style parameters:
//   - Script block: where_object(objects; {script: ".Age > 18"})
//   - Simplified: where_object(objects; {property: "Age", operator: "gt", value: 18})
//
// Usage: where_object(objects) or where_object(objects; options)
func RegisterWhereObject() gojq.CompilerOption {
	return common.WithFunction("where_object", 1, 2, func(input any, args []any) any {
		// Parse arguments
		objects, opts, err := ParseWhereObjectArgs(args)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		// Filter objects
		filtered, err := whereObject(objects, opts)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		return filtered
	})
}

// parseOperator converts a string to FilterOperator
func parseOperator(s string) FilterOperator {
	// Normalize: remove leading dashes, convert to lowercase
	s = strings.TrimPrefix(s, "-")
	s = strings.ToLower(s)

	switch s {
	case "eq", "equals", "equal":
		return FilterOperatorEq
	case "ne", "notequals", "not_equal":
		return FilterOperatorNe
	case "gt", "greaterthan", "greater":
		return FilterOperatorGt
	case "ge", "greaterorequal", "greater_or_equal":
		return FilterOperatorGe
	case "lt", "lessthan", "less":
		return FilterOperatorLt
	case "le", "lessorequal", "less_or_equal":
		return FilterOperatorLe
	case "like":
		return FilterOperatorLike
	case "notlike", "not_like":
		return FilterOperatorNotLike
	case "match", "regex":
		return FilterOperatorMatch
	case "notmatch", "not_match", "notregex":
		return FilterOperatorNotMatch
	case "contains":
		return FilterOperatorContains
	case "notcontains", "not_contains":
		return FilterOperatorNotContains
	default:
		return FilterOperatorEq
	}
}

// evaluateCondition evaluates whether an object matches the filter condition
func evaluateCondition(obj any, opts WhereObjectOptions, block *common.ScriptBlock) (bool, error) {
	if block != nil {
		return evaluateScriptBlock(obj, block)
	}

	// Simplified syntax mode - property + operator + value
	// Validate that property is specified
	if opts.Property == "" {
		return false, fmt.Errorf("where_object: requires either 'script' or 'property' option")
	}
	return evaluatePropertyCondition(obj, opts.Property, opts.Operator, opts.Value, opts.CaseSensitive)
}

// evaluateScriptBlock evaluates a jq expression against an object.
func evaluateScriptBlock(obj any, block *common.ScriptBlock) (bool, error) {
	return block.EvalBool(obj)
}

// extractPropertyByPath extracts a property value using dot notation
func extractPropertyByPath(value any, path string) (any, error) {
	path = strings.TrimSpace(path)

	// Remove leading dot
	path = strings.TrimPrefix(path, ".")

	if path == "" {
		return value, nil
	}

	// Split by dots for nested access
	parts := strings.Split(path, ".")
	current := value

	for _, part := range parts {
		if current == nil {
			return nil, fmt.Errorf("cannot access property %q on nil", part)
		}

		found := false
		switch v := current.(type) {
		case map[string]any:
			// Check for wire form
			if typed.Is(v) {
				obj, err := typed.FromMap(v)
				if err == nil {
					// Try to get from members first
					if member, ok := obj.Members[part]; ok {
						if member.MemberType == typed.MemberTypeNoteProperty {
							current = member.Value
							found = true
							break
						}
					}
					// Fall back to value map
					if valMap, ok := obj.Value.(map[string]any); ok {
						current, found = valMap[part]
						if found {
							break
						}
					}
				}
			}
			// Regular map access
			current, found = v[part]
		case *typed.Object:
			// Direct object access
			if member, ok := v.Members[part]; ok {
				if member.MemberType == typed.MemberTypeNoteProperty {
					current = member.Value
					found = true
					break
				}
			}
			// Try the underlying value
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

// applyOperator applies a comparison operator to two values
func applyOperator(left any, op FilterOperator, right any, caseSensitive bool) (any, error) {
	switch op {
	case FilterOperatorEq:
		return compareValues(left, right) == 0, nil
	case FilterOperatorNe:
		return compareValues(left, right) != 0, nil
	case FilterOperatorGt:
		return compareValues(left, right) > 0, nil
	case FilterOperatorGe:
		return compareValues(left, right) >= 0, nil
	case FilterOperatorLt:
		return compareValues(left, right) < 0, nil
	case FilterOperatorLe:
		return compareValues(left, right) <= 0, nil
	case FilterOperatorLike:
		return matchWildcard(left, right, caseSensitive), nil
	case FilterOperatorNotLike:
		return !matchWildcard(left, right, caseSensitive), nil
	case FilterOperatorMatch:
		return matchRegex(left, right, caseSensitive)
	case FilterOperatorNotMatch:
		matches, err := matchRegex(left, right, caseSensitive)
		return !matches, err
	case FilterOperatorContains:
		return containsValue(left, right), nil
	case FilterOperatorNotContains:
		return !containsValue(left, right), nil
	default:
		return false, fmt.Errorf("unsupported operator: %s", op)
	}
}

// compareValues compares two values, returning -1, 0, or 1
func compareValues(left, right any) int {
	// Handle nil
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}

	// Convert both to comparable types
	leftNum, leftIsNum := toNumber(left)
	rightNum, rightIsNum := toNumber(right)

	if leftIsNum && rightIsNum {
		if leftNum < rightNum {
			return -1
		}
		if leftNum > rightNum {
			return 1
		}
		return 0
	}

	// String comparison
	leftStr := fmt.Sprintf("%v", left)
	rightStr := fmt.Sprintf("%v", right)

	if leftStr < rightStr {
		return -1
	}
	if leftStr > rightStr {
		return 1
	}
	return 0
}

// toNumber attempts to convert a value to float64.
//
// The numeric cases are common.ToFloat64's, which covers json.Number - the type
// every number piped in from stdin actually has. Without it, sort_object
// compared numbers as strings, so 9 sorted after 100.
func toNumber(v any) (float64, bool) {
	if f, ok := common.ToFloat64(v); ok {
		return f, true
	}
	switch val := v.(type) {
	case string:
		if num, err := strconv.ParseFloat(val, 64); err == nil {
			return num, true
		}
	case bool:
		if val {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// matchWildcard performs wildcard matching (PowerShell -like operator)
func matchWildcard(left, right any, caseSensitive bool) bool {
	leftStr := fmt.Sprintf("%v", left)
	rightStr := fmt.Sprintf("%v", right)

	if !caseSensitive {
		leftStr = strings.ToLower(leftStr)
		rightStr = strings.ToLower(rightStr)
	}

	// Convert PowerShell wildcard pattern to regex
	// * matches any sequence, ? matches single char, [abc] matches character class
	// First, handle character classes by protecting them
	result := ""
	i := 0
	for i < len(rightStr) {
		ch := rightStr[i]
		switch ch {
		case '*':
			result += ".*"
		case '?':
			result += "."
		case '[':
			// Find closing bracket
			j := i + 1
			for j < len(rightStr) && rightStr[j] != ']' {
				j++
			}
			if j < len(rightStr) {
				// Include the bracket expression as-is in regex
				result += rightStr[i : j+1]
				i = j
			} else {
				// No closing bracket, treat as literal
				result += regexp.QuoteMeta(string(ch))
			}
		default:
			result += regexp.QuoteMeta(string(ch))
		}
		i++
	}
	pattern := "^" + result + "$"

	matched, _ := regexp.MatchString(pattern, leftStr)
	return matched
}

// matchRegex performs regex matching (PowerShell -match operator)
func matchRegex(left, right any, caseSensitive bool) (bool, error) {
	leftStr := fmt.Sprintf("%v", left)
	patternStr := fmt.Sprintf("%v", right)

	var re *regexp.Regexp
	var err error

	if caseSensitive {
		re, err = regexp.Compile(patternStr)
	} else {
		re, err = regexp.Compile("(?i)" + patternStr)
	}

	if err != nil {
		return false, fmt.Errorf("invalid regex pattern %q: %w", patternStr, err)
	}

	return re.MatchString(leftStr), nil
}

// containsValue checks if left contains right
func containsValue(left, right any) bool {
	leftStr := fmt.Sprintf("%v", left)
	rightStr := fmt.Sprintf("%v", right)

	return strings.Contains(leftStr, rightStr)
}

// toBool converts a value to boolean
func toBool(v any) bool {
	if v == nil {
		return false
	}

	switch val := v.(type) {
	case bool:
		return val
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return reflect.ValueOf(v).Int() != 0
	case float32, float64:
		return reflect.ValueOf(v).Float() != 0
	case string:
		return val != "" && strings.ToLower(val) != "false" && val != "0"
	default:
		return true
	}
}

// evaluatePropertyCondition evaluates a property-based condition
func evaluatePropertyCondition(obj any, property string, op FilterOperator, value any, caseSensitive bool) (bool, error) {
	// Extract the property value from the object
	propValue, err := extractPropertyByPath(obj, property)
	if err != nil {
		return false, fmt.Errorf("failed to get property %q: %w", property, err)
	}

	// Apply the operator
	result, err := applyOperator(propValue, op, value, caseSensitive)
	if err != nil {
		return false, err
	}

	return toBool(result), nil
}

// whereObject is the internal implementation for testing
func whereObject(objects []any, opts WhereObjectOptions) ([]any, error) {
	// Input validation
	if objects == nil {
		return []any{}, nil
	}

	if len(objects) == 0 {
		return []any{}, nil
	}

	// Validate that either ScriptBlock or Property is specified
	if opts.ScriptBlock == "" && opts.Property == "" {
		return nil, fmt.Errorf("where_object: requires either 'script' or 'property' option")
	}

	// Compile the script block once rather than per object.
	var block *common.ScriptBlock
	if opts.ScriptBlock != "" {
		compiled, err := common.CompileScriptBlock(opts.ScriptBlock)
		if err != nil {
			return nil, fmt.Errorf("where_object: %w", err)
		}
		block = compiled
	}

	result := make([]any, 0, len(objects))

	for _, obj := range objects {
		matches, err := evaluateCondition(obj, opts, block)
		if err != nil {
			return nil, err
		}

		if matches {
			// Where-Object is a filter: matching objects pass through as they
			// arrived, type and all.
			result = append(result, obj)
		}
	}

	return result, nil
}

// ParseWhereObjectArgs parses arguments for testing
func ParseWhereObjectArgs(args []any) ([]any, WhereObjectOptions, error) {
	opts := WhereObjectOptions{}

	if len(args) == 0 {
		return []any{}, opts, fmt.Errorf("where_object: requires objects argument")
	}

	// First argument is objects
	var objects []any
	inputVal := common.BindValue(args[0])
	objects = common.NormalizeToSlice(inputVal)

	// Parse options if present
	if len(args) > 1 {
		if optsMap, ok := args[1].(map[string]any); ok {
			if script, exists := optsMap["script"]; exists {
				if s, ok := script.(string); ok {
					opts.ScriptBlock = s
				}
			}
			if prop, exists := optsMap["property"]; exists {
				if p, ok := prop.(string); ok {
					opts.Property = p
				}
			}
			if op, exists := optsMap["operator"]; exists {
				if s, ok := op.(string); ok {
					opts.Operator = parseOperator(s)
				}
			}
			if val, exists := optsMap["value"]; exists {
				opts.Value = val
			}
			if cs, exists := optsMap["casesensitive"]; exists {
				if b, ok := cs.(bool); ok {
					opts.CaseSensitive = b
				}
			}
		}
	}

	return objects, opts, nil
}
