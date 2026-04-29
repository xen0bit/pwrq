// Package pipeline provides PowerShell-style pipeline infrastructure for pwrq.
package pipeline

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

// OutputWriter handles writing objects to the pipeline output.
// It supports different output formats similar to PowerShell's formatting system.
type OutputWriter struct {
	writer    io.Writer
	formatter Formatter
}

// Formatter defines the interface for output formatters.
type Formatter interface {
	Format(any) ([]byte, error)
}

// JSONFormatter formats objects as JSON.
type JSONFormatter struct {
	Indent int
}

// Format implements the Formatter interface.
func (f *JSONFormatter) Format(obj any) ([]byte, error) {
	if f.Indent > 0 {
		return json.MarshalIndent(obj, "", string(make([]byte, f.Indent)))
	}
	return json.Marshal(obj)
}

// TableFormatter formats objects as a table (simplified).
type TableFormatter struct {
	Properties []string
}

// Format implements the Formatter interface.
func (f *TableFormatter) Format(obj any) ([]byte, error) {
	var result string

	// Collect all rows (handle both single object and slice of objects)
	var rows []map[string]string
	val := reflect.ValueOf(obj)

	if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
		// Multiple objects - each is a row
		for i := 0; i < val.Len(); i++ {
			row := extractProperties(val.Index(i).Interface(), f.Properties)
			rows = append(rows, row)
		}
	} else {
		// Single object - one row
		row := extractProperties(obj, f.Properties)
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return []byte(""), nil
	}

	// Determine columns: use f.Properties order if specified, otherwise sort alphabetically
	var columns []string
	if len(f.Properties) > 0 {
		// Use specified properties order
		columns = make([]string, len(f.Properties))
		copy(columns, f.Properties)
	} else {
		// Collect all unique column names from all rows
		columnSet := make(map[string]bool)
		for _, row := range rows {
			for col := range row {
				columnSet[col] = true
			}
		}
		columns = make([]string, 0, len(columnSet))
		for col := range columnSet {
			columns = append(columns, col)
		}
		// Sort columns alphabetically for deterministic output
		sort.Strings(columns)
	}

	// Calculate column widths (start with header width)
	widths := make(map[string]int)
	for _, col := range columns {
		widths[col] = len(col)
	}

	// Expand widths based on values
	for _, row := range rows {
		for _, col := range columns {
			if val, ok := row[col]; ok && len(val) > widths[col] {
				widths[col] = len(val)
			}
		}
	}

	// Build header line
	var header string
	for i, col := range columns {
		if i > 0 {
			header += " "
		}
		header += fmt.Sprintf("%-*s", widths[col], col)
	}
	result += header + "\n"

	// Build separator line
	var separator string
	for i, col := range columns {
		if i > 0 {
			separator += " "
		}
		separator += strings.Repeat("-", widths[col])
	}
	result += separator + "\n"

	// Build data rows
	for _, row := range rows {
		var line string
		for i, col := range columns {
			if i > 0 {
				line += " "
			}
			line += fmt.Sprintf("%-*s", widths[col], row[col])
		}
		result += line + "\n"
	}

	return []byte(result), nil
}

// extractProperties extracts property values from an object as strings.
// Supports case-insensitive property matching when properties are specified.
func extractProperties(obj any, properties []string) map[string]string {
	result := make(map[string]string)
	val := reflect.ValueOf(obj)

	if len(properties) == 0 {
		// Extract all properties
		switch val.Kind() {
		case reflect.Map:
			for _, key := range val.MapKeys() {
				keyStr := fmt.Sprintf("%v", key.Interface())
				result[keyStr] = fmt.Sprintf("%v", val.MapIndex(key).Interface())
			}
		case reflect.Struct:
			for i := 0; i < val.NumField(); i++ {
				field := val.Type().Field(i)
				result[field.Name] = fmt.Sprintf("%v", val.Field(i).Interface())
			}
		default:
			result["Value"] = fmt.Sprintf("%v", obj)
		}
	} else {
		// Extract specified properties with case-insensitive matching
		for _, prop := range properties {
			value := getPropertyCaseInsensitive(val, prop)
			result[prop] = fmt.Sprintf("%v", value)
		}
	}

	return result
}

// getPropertyCaseInsensitive gets a property value with case-insensitive matching.
// It first tries exact match, then falls back to case-insensitive comparison.
func getPropertyCaseInsensitive(obj reflect.Value, name string) any {
	// Try exact match first
	if value := getProperty(obj, name); value != nil {
		return value
	}

	// Case-insensitive search for maps
	if obj.Kind() == reflect.Map {
		nameLower := strings.ToLower(name)
		for _, key := range obj.MapKeys() {
			if strings.ToLower(fmt.Sprintf("%v", key.Interface())) == nameLower {
				return obj.MapIndex(key).Interface()
			}
		}
	}

	// Case-insensitive search for structs
	if obj.Kind() == reflect.Struct {
		nameLower := strings.ToLower(name)
		for i := 0; i < obj.NumField(); i++ {
			if strings.ToLower(obj.Type().Field(i).Name) == nameLower {
				return obj.Field(i).Interface()
			}
		}
	}

	return nil
}

// ListFormatter formats objects as a list of properties.
type ListFormatter struct {
	Properties []string
}

// Format implements the Formatter interface.
func (f *ListFormatter) Format(obj any) ([]byte, error) {
	var result string
	val := reflect.ValueOf(obj)

	// Handle slices - format each object separately
	if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
		for i := 0; i < val.Len(); i++ {
			if i > 0 {
				result += "\n"
			}
			result += formatAsList(val.Index(i).Interface(), f.Properties)
		}
	} else {
		result = formatAsList(obj, f.Properties)
	}

	return []byte(result), nil
}

// formatAsList formats a single object as a property list.
func formatAsList(obj any, properties []string) string {
	var result string
	val := reflect.ValueOf(obj)

	if len(properties) == 0 {
		// Show all properties
		switch val.Kind() {
		case reflect.Map:
			for _, key := range val.MapKeys() {
				keyStr := fmt.Sprintf("%v", key.Interface())
				valStr := fmt.Sprintf("%v", val.MapIndex(key).Interface())
				result += fmt.Sprintf("%-20s : %s\n", keyStr, valStr)
			}
		case reflect.Struct:
			for i := 0; i < val.NumField(); i++ {
				field := val.Type().Field(i)
				result += fmt.Sprintf("%-20s : %s\n", field.Name, val.Field(i).Interface())
			}
		default:
			result += fmt.Sprintf("%v\n", obj)
		}
	} else {
		// Show specified properties
		for _, prop := range properties {
			value := getProperty(val, prop)
			result += fmt.Sprintf("%-20s : %s\n", prop, value)
		}
	}

	return result
}

// NewOutputWriter creates a new output writer with the specified formatter.
func NewOutputWriter(writer io.Writer, formatter Formatter) *OutputWriter {
	return &OutputWriter{
		writer:    writer,
		formatter: formatter,
	}
}

// Write writes an object to the output.
func (ow *OutputWriter) Write(obj any) error {
	if ow.formatter == nil {
		ow.formatter = &JSONFormatter{}
	}

	data, err := ow.formatter.Format(obj)
	if err != nil {
		return err
	}

	_, err = ow.writer.Write(data)
	if err != nil {
		return err
	}

	_, err = ow.writer.Write([]byte{'\n'})
	return err
}

// PipelineOutput is a simple function-based output writer for use with CmdletBase.
type PipelineOutput struct {
	outputs []any
}

// NewPipelineOutput creates a new pipeline output collector.
func NewPipelineOutput() *PipelineOutput {
	return &PipelineOutput{
		outputs: make([]any, 0),
	}
}

// Write adds an object to the output collection.
func (po *PipelineOutput) Write(obj any) {
	po.outputs = append(po.outputs, obj)
}

// Outputs returns all collected outputs.
func (po *PipelineOutput) Outputs() []any {
	return po.outputs
}

// CreateOutputWriter creates a function suitable for CmdletBase.OutputWriter.
func (po *PipelineOutput) CreateOutputWriter() func(any) {
	return func(obj any) {
		po.Write(obj)
	}
}

// FormatObject formats a single object for output.
func FormatObject(obj any, format string) ([]byte, error) {
	switch format {
	case "json":
		return json.Marshal(obj)
	case "table":
		// Simplified - just return JSON for now
		return json.Marshal(obj)
	case "list":
		// Simplified - just return JSON for now
		return json.Marshal(obj)
	default:
		return json.Marshal(obj)
	}
}

// FormatList formats an object as a property list.
func FormatList(obj any, properties []string) string {
	var result string
	objValue := reflectValue(obj)

	if len(properties) == 0 {
		// Show all properties
		switch objValue.Kind() {
		case reflect.Map:
			for _, key := range objValue.MapKeys() {
				result += fmt.Sprintf("%-20v : %v\n", key.Interface(), objValue.MapIndex(key).Interface())
			}
		case reflect.Struct:
			for i := 0; i < objValue.NumField(); i++ {
				field := objValue.Type().Field(i)
				result += fmt.Sprintf("%-20v : %v\n", field.Name, objValue.Field(i).Interface())
			}
		}
	} else {
		// Show specified properties
		for _, prop := range properties {
			value := getProperty(objValue, prop)
			result += fmt.Sprintf("%-20v : %v\n", prop, value)
		}
	}

	return result
}

// reflectValue gets the reflect.Value of an interface.
func reflectValue(obj any) reflect.Value {
	if obj == nil {
		return reflect.ValueOf(nil)
	}
	return reflect.ValueOf(obj)
}

// getProperty gets a property value from an object.
func getProperty(obj reflect.Value, name string) any {
	switch obj.Kind() {
	case reflect.Map:
		if val := obj.MapIndex(reflect.ValueOf(name)); val.IsValid() {
			return val.Interface()
		}
	case reflect.Struct:
		for i := 0; i < obj.NumField(); i++ {
			if obj.Type().Field(i).Name == name {
				return obj.Field(i).Interface()
			}
		}
	}
	return nil
}
