// Package pipeline provides PowerShell-style pipeline infrastructure for pwrq.
package pipeline

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ParameterAttribute defines metadata for cmdlet parameters.
// This is similar to PowerShell's [Parameter()] attribute.
type ParameterAttribute struct {
	// Position specifies the positional order of the parameter.
	// -1 means the parameter must be named.
	Position int

	// Mandatory indicates whether the parameter is required.
	Mandatory bool

	// ValueFromPipeline indicates whether the parameter accepts pipeline input.
	ValueFromPipeline bool

	// ValueFromPipelineByPropertyName indicates whether the parameter accepts
	// pipeline input by property name matching.
	ValueFromPipelineByPropertyName bool

	// HelpMessage provides help text for the parameter.
	HelpMessage string
}

// Parameter binds a value to a parameter with validation.
func Parameter(name string, value any, target reflect.Value, attr ParameterAttribute) error {
	// Check if value is nil and parameter is mandatory
	if value == nil && attr.Mandatory {
		return fmt.Errorf("parameter %q is mandatory", name)
	}

	if value == nil {
		return nil
	}

	// Type conversion if needed
	if !target.IsValid() {
		return fmt.Errorf("invalid target for parameter %q", name)
	}

	targetType := target.Type()
	valueType := reflect.TypeOf(value)

	// If types match, set directly
	if valueType.AssignableTo(targetType) {
		target.Set(reflect.ValueOf(value))
		return nil
	}

	// Try to convert common types
	converted, err := convertValue(value, targetType)
	if err != nil {
		return fmt.Errorf("cannot convert parameter %q from %v to %v: %v",
			name, valueType, targetType, err)
	}

	target.Set(converted)
	return nil
}

// convertValue attempts to convert a value to the target type.
func convertValue(value any, targetType reflect.Type) (reflect.Value, error) {
	if value == nil {
		return reflect.Zero(targetType), nil
	}

	valueType := reflect.TypeOf(value)

	// Handle string conversions
	if targetType.Kind() == reflect.String {
		return reflect.ValueOf(fmt.Sprintf("%v", value)), nil
	}

	// Handle numeric conversions
	switch targetType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var num int64
		switch v := value.(type) {
		case int:
			num = int64(v)
		case int8:
			num = int64(v)
		case int16:
			num = int64(v)
		case int32:
			num = int64(v)
		case int64:
			num = v
		case float64:
			num = int64(v)
		case string:
			_, err := fmt.Sscanf(v, "%d", &num)
			if err != nil {
				return reflect.Zero(targetType), err
			}
		default:
			return reflect.Zero(targetType), fmt.Errorf("cannot convert %v to int", valueType)
		}
		return reflect.ValueOf(num).Convert(targetType), nil

	case reflect.Float32, reflect.Float64:
		var num float64
		switch v := value.(type) {
		case float32:
			num = float64(v)
		case float64:
			num = v
		case int:
			num = float64(v)
		case int64:
			num = float64(v)
		case string:
			_, err := fmt.Sscanf(v, "%f", &num)
			if err != nil {
				return reflect.Zero(targetType), err
			}
		default:
			return reflect.Zero(targetType), fmt.Errorf("cannot convert %v to float", valueType)
		}
		return reflect.ValueOf(num).Convert(targetType), nil

	case reflect.Bool:
		switch v := value.(type) {
		case bool:
			return reflect.ValueOf(v), nil
		case string:
			switch strings.ToLower(v) {
			case "true", "1", "yes":
				return reflect.ValueOf(true), nil
			case "false", "0", "no":
				return reflect.ValueOf(false), nil
			}
			return reflect.Zero(targetType), fmt.Errorf("cannot convert %q to bool", v)
		}

	case reflect.Slice:
		// JSON arrays arrive as []any, so a parameter declared as []string has
		// to convert element-wise. A single value binds as a one-element list,
		// which is how PowerShell treats a scalar passed to an array parameter.
		elemType := targetType.Elem()
		items, ok := value.([]any)
		if !ok {
			items = []any{value}
		}
		result := reflect.MakeSlice(targetType, 0, len(items))
		for i, item := range items {
			converted, err := convertValue(item, elemType)
			if err != nil {
				return reflect.Zero(targetType), fmt.Errorf("element %d: %w", i, err)
			}
			result = reflect.Append(result, converted)
		}
		return result, nil
	}

	return reflect.Zero(targetType), fmt.Errorf("no conversion from %v to %v", valueType, targetType)
}

// BindParameters binds parameters from a map to a struct using reflection.
// The struct should have tags like `param:"name"` or `param:"name,pos=0"`.
// Tags support: param:"name" param:"name,pos=0" param:"name,mandatory" param:"name,pos=0,mandatory"
// 
// positionalParams allows binding positional parameters by order.
// For example, if a cmdlet has positional params at positions 0 and 1,
// passing positionalParams = []any{"val0", "val1"} will bind them in order.
func BindParameters(params map[string]any, target any, positionalParams ...any) error {
	if params == nil {
		params = make(map[string]any)
	}

	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Pointer {
		return fmt.Errorf("target must be a pointer")
	}
	targetElem := targetValue.Elem()
	if targetElem.Kind() != reflect.Struct {
		return fmt.Errorf("target must be a pointer to a struct")
	}

	targetType := targetElem.Type()

	// Build position map: position -> field index
	positionMap := make(map[int]int)
	// Build name map: parameter name -> field index
	nameMap := make(map[string]int)
	// Track mandatory fields
	mandatoryFields := make(map[int]string) // field index -> parameter name
	// Track which fields have been bound
	boundFields := make(map[int]bool)

	// Parse struct tags to build maps
	for i := 0; i < targetType.NumField(); i++ {
		field := targetType.Field(i)
		tag := field.Tag.Get("param")
		if tag == "" {
			continue
		}

		parts := strings.Split(tag, ",")
		if len(parts) == 0 {
			continue
		}

		paramName := strings.TrimSpace(parts[0])
		position := -1
		mandatory := false

		// Parse additional tag options
		for _, part := range parts[1:] {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "pos=") {
				posStr := strings.TrimPrefix(part, "pos=")
				pos, err := strconv.Atoi(posStr)
				if err == nil {
					position = pos
				}
			} else if part == "mandatory" {
				mandatory = true
			}
		}

		// PowerShell parameter names are case-insensitive, so -recurse and
		// -Recurse are the same parameter.
		nameMap[strings.ToLower(paramName)] = i
		if position >= 0 {
			positionMap[position] = i
		}
		if mandatory {
			mandatoryFields[i] = paramName
		}
	}

	// Step 1: Bind positional parameters first (in order)
	for pos, value := range positionalParams {
		if value == nil {
			continue
		}
		if fieldIdx, ok := positionMap[pos]; ok {
			field := targetElem.Field(fieldIdx)
			if field.CanSet() {
				paramName := ""
				// Find the parameter name for this field
				for name, idx := range nameMap {
					if idx == fieldIdx {
						paramName = name
						break
					}
				}
				attr := ParameterAttribute{
					Position: pos,
					Mandatory: false,
				}
				if err := Parameter(paramName, value, field, attr); err != nil {
					return err
				}
				boundFields[fieldIdx] = true
			}
		} else {
			// Positional parameter provided but no field defined for this position
			// Check if it exceeds the maximum defined position
			maxPos := -1
			for p := range positionMap {
				if p > maxPos {
					maxPos = p
				}
			}
			if pos > maxPos {
				return fmt.Errorf("positional parameter at index %d exceeds maximum defined position %d", pos, maxPos)
			}
		}
	}

	// Step 2: Bind named parameters (don't override already-bound positional params)
	for paramName, value := range params {
		if fieldIdx, ok := nameMap[strings.ToLower(paramName)]; ok {
			// Skip if already bound by positional parameter
			if boundFields[fieldIdx] {
				continue
			}
			field := targetElem.Field(fieldIdx)
			if field.CanSet() {
				attr := ParameterAttribute{
					Position:  -1,
					Mandatory: false,
				}
				if err := Parameter(paramName, value, field, attr); err != nil {
					return err
				}
				boundFields[fieldIdx] = true
			}
		}
	}

	// Step 3: Validate mandatory parameters
	for fieldIdx, paramName := range mandatoryFields {
		if !boundFields[fieldIdx] {
			return fmt.Errorf("missing mandatory parameter: %s", paramName)
		}
		field := targetElem.Field(fieldIdx)
		// Check if field is still zero value (was bound but with nil/zero value)
		if field.Interface() == reflect.Zero(field.Type()).Interface() {
			return fmt.Errorf("missing mandatory parameter: %s", paramName)
		}
	}

	return nil
}
