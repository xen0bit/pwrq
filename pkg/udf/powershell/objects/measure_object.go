// Package objects provides PowerShell-style object manipulation cmdlets.
// This file implements Measure-Object functionality for measuring properties
// of pipeline objects (count, sum, average, min, max).
package objects

import (
	"fmt"
	"strconv"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// MeasureObjectOptions holds options for the measure_object function
type MeasureObjectOptions struct {
	Property      string // Property to measure (empty means count objects)
	Sum           bool   // Calculate sum of property values
	Average       bool   // Calculate average of property values
	Minimum       bool   // Find minimum value
	Maximum       bool   // Find maximum value
	CaseSensitive bool   // Whether string comparison is case-sensitive
}

// MeasurementResult holds the results of a measurement operation
type MeasurementResult struct {
	Count   int     `json:"count"`
	Sum     float64 `json:"sum,omitempty"`
	Average float64 `json:"average,omitempty"`
	Minimum any     `json:"minimum,omitempty"`
	Maximum any     `json:"maximum,omitempty"`
}

// RegisterMeasureObject registers the measure_object function with gojq
// Supports PowerShell-style parameters:
//   - measure_object(objects) - just count
//   - measure_object(objects; {property: "Length", sum: true})
//   - measure_object(objects; {property: "Size", average: true, minimum: true, maximum: true})
//
// Usage: measure_object(objects) or measure_object(objects; options)
func RegisterMeasureObject() gojq.CompilerOption {
	return common.WithFunctionOf("measure_object", 1, 2, MeasureInfoShape, func(input any, args []any) any {
		// Parse arguments
		objects, opts, err := ParseMeasureObjectArgs(args)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		// Perform measurement
		result, err := measureObject(objects, opts)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		// Format result as PSObject with proper type information
		output := formatMeasurementResult(result, opts)

		return output
	})
}

// measureObject performs the measurement on the given objects
func measureObject(objects []any, opts MeasureObjectOptions) (*MeasurementResult, error) {
	// Input validation - explicit nil check
	if objects == nil {
		return &MeasurementResult{Count: 0}, nil
	}

	if len(objects) == 0 {
		return &MeasurementResult{Count: 0}, nil
	}

	result := &MeasurementResult{
		Count:   0,
		Sum:     0,
		Average: 0,
		Minimum: nil,
		Maximum: nil,
	}

	var errors []string
	hasNumericValues := false

	for _, obj := range objects {
		result.Count++

		// If no property specified, just count objects
		if opts.Property == "" {
			continue
		}

		// Extract property value for measurement
		propValue, err := extractPropertyForMeasurement(obj, opts.Property)
		if err != nil {
			// Collect error for skipped item (PowerShell-style $ERROR stream)
			errors = append(errors, fmt.Sprintf("object %d: %v", result.Count, err))
			continue
		}

		// Convert to float64 for numeric operations
		numVal, err := convertToFloat64(propValue)
		if err != nil {
			// Skip non-numeric values silently (PowerShell behavior)
			errors = append(errors, fmt.Sprintf("object %d: non-numeric value %T", result.Count, propValue))
			continue
		}

		hasNumericValues = true

		// Calculate sum
		if opts.Sum {
			result.Sum += numVal
		}

		// Track minimum
		if opts.Minimum {
			if result.Minimum == nil || numVal < result.Minimum.(float64) {
				result.Minimum = numVal
			}
		}

		// Track maximum
		if opts.Maximum {
			if result.Maximum == nil || numVal > result.Maximum.(float64) {
				result.Maximum = numVal
			}
		}
	}

	// Calculate average if requested and we have numeric values
	if opts.Average && hasNumericValues && result.Count > 0 {
		// Count how many objects contributed to sum for average calculation
		numericCount := 0
		sumForAvg := 0.0
		for _, obj := range objects {
			if opts.Property == "" {
				continue
			}
			propValue, err := extractPropertyForMeasurement(obj, opts.Property)
			if err != nil {
				continue
			}
			numVal, err := convertToFloat64(propValue)
			if err != nil {
				continue
			}
			numericCount++
			sumForAvg += numVal
		}
		if numericCount > 0 {
			result.Average = sumForAvg / float64(numericCount)
		}
	}

	// If no property was specified, we only do counting
	if opts.Property == "" {
		return result, nil
	}

	// If property was specified but no numeric values found, return zeros
	if !hasNumericValues && opts.Property != "" {
		if len(errors) > 0 {
			// Return result with errors noted (caller can inspect)
			return result, nil
		}
	}

	return result, nil
}

// extractPropertyForMeasurement extracts a property value from an object for measurement
func extractPropertyForMeasurement(obj any, property string) (any, error) {
	// Extract the underlying value from PSObject if present
	value := common.BindValue(obj)

	return common.ExtractPropertyByPath(value, property)
}

// convertToFloat64 converts various numeric types to float64.
//
// The numeric cases live in common.ToFloat64 because they are not obvious:
// numbers piped in from stdin are json.Number, not float64, so measuring a
// property of real input used to report a sum of zero.
func convertToFloat64(v any) (float64, error) {
	if f, ok := common.ToFloat64(v); ok {
		return f, nil
	}
	switch val := v.(type) {
	case string:
		// Try to parse string as number
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot convert string %q to number", val)
		}
		return f, nil
	case bool:
		// PowerShell treats $true as 1, $false as 0
		if val {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("cannot convert type %T to number", v)
	}
}

// formatMeasurementResult formats the measurement result as a PSObject
// Returns a properly wrapped PSObject with TypeName "Microsoft.PowerShell.Commands.GenericMeasureInfo"
func formatMeasurementResult(result *MeasurementResult, opts MeasureObjectOptions) map[string]any {
	// Build the value map with all requested measurements
	valueMap := make(map[string]any)
	valueMap["Count"] = result.Count

	if opts.Property != "" {
		if opts.Sum {
			valueMap["Sum"] = result.Sum
		}
		if opts.Average {
			valueMap["Average"] = result.Average
		}
		if opts.Minimum {
			valueMap["Minimum"] = result.Minimum
		}
		if opts.Maximum {
			valueMap["Maximum"] = result.Maximum
		}
	}

	// The shape supplies the type name, so it is written down once, beside
	// the property list it goes with.
	psobj := psobject.NewPSObject(valueMap)

	// Add NoteProperties for all measurement values
	psobj.AddNoteProperty("Count", result.Count)
	if opts.Property != "" {
		if opts.Sum {
			psobj.AddNoteProperty("Sum", result.Sum)
		}
		if opts.Average {
			psobj.AddNoteProperty("Average", result.Average)
		}
		if opts.Minimum {
			psobj.AddNoteProperty("Minimum", result.Minimum)
		}
		if opts.Maximum {
			psobj.AddNoteProperty("Maximum", result.Maximum)
		}
	}

	return MeasureInfoShape.Build(psobj.ToMap())
}

// ParseMeasureObjectArgs parses arguments for the measure_object function
func ParseMeasureObjectArgs(args []any) ([]any, MeasureObjectOptions, error) {
	opts := MeasureObjectOptions{
		CaseSensitive: false,
	}

	if len(args) == 0 {
		return []any{}, opts, fmt.Errorf("measure_object: requires objects argument")
	}

	// First argument is objects
	var objects []any
	inputVal := common.BindValue(args[0])
	objects = common.NormalizeToSlice(inputVal)

	// Parse options if present
	if len(args) > 1 {
		if optsMap, ok := args[1].(map[string]any); ok {
			if propVal, exists := optsMap["property"]; exists {
				if propStr, ok := propVal.(string); ok {
					opts.Property = propStr
				}
			}
			if sumVal, exists := optsMap["sum"]; exists {
				if sumBool, ok := sumVal.(bool); ok {
					opts.Sum = sumBool
				}
			}
			if avgVal, exists := optsMap["average"]; exists {
				if avgBool, ok := avgVal.(bool); ok {
					opts.Average = avgBool
				}
			}
			if minVal, exists := optsMap["minimum"]; exists {
				if minBool, ok := minVal.(bool); ok {
					opts.Minimum = minBool
				}
			}
			if maxVal, exists := optsMap["maximum"]; exists {
				if maxBool, ok := maxVal.(bool); ok {
					opts.Maximum = maxBool
				}
			}
			if csVal, exists := optsMap["casesensitive"]; exists {
				if csBool, ok := csVal.(bool); ok {
					opts.CaseSensitive = csBool
				}
			}
		}
	}

	return objects, opts, nil
}

// GetMeasurementCount returns the count from a measurement result
func GetMeasurementCount(result any) int {
	if m, ok := result.(map[string]any); ok {
		if count, ok := m["Count"].(int); ok {
			return count
		}
	}
	return 0
}

// GetMeasurementSum returns the sum from a measurement result
func GetMeasurementSum(result any) float64 {
	if m, ok := result.(map[string]any); ok {
		if sum, ok := m["Sum"].(float64); ok {
			return sum
		}
	}
	return 0
}

// GetMeasurementAverage returns the average from a measurement result
func GetMeasurementAverage(result any) float64 {
	if m, ok := result.(map[string]any); ok {
		if avg, ok := m["Average"].(float64); ok {
			return avg
		}
	}
	return 0
}

// GetMeasurementMinimum returns the minimum from a measurement result
func GetMeasurementMinimum(result any) any {
	if m, ok := result.(map[string]any); ok {
		return m["Minimum"]
	}
	return nil
}

// GetMeasurementMaximum returns the maximum from a measurement result
func GetMeasurementMaximum(result any) any {
	if m, ok := result.(map[string]any); ok {
		return m["Maximum"]
	}
	return nil
}
