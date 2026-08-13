// Package formatting provides PowerShell-style formatting cmdlets.
// This file implements Format-Table functionality for displaying objects
// in a tabular format with columns and rows.
package formatting

import (
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// FormatTableOptions holds options for the format_table function
type FormatTableOptions struct {
	Property         []string // Properties to display as columns (empty means all)
	CaseSensitive    bool     // Whether property matching is case-sensitive
	AutoSize         bool     // Calculate optimal column widths (-AutoSize)
	HideTableHeaders bool     // Hide column headers (-HideTableHeaders)
	Depth            int      // Maximum depth for nested object formatting (0 = unlimited)
}

// FormattedTable represents a formatted table output
type FormattedTable struct {
	Headers          []string   // Column headers
	Rows             [][]string // Row data (formatted strings)
	Widths           []int      // Column widths (populated when AutoSize is true)
	RowCount         int        // Number of data rows
	HideTableHeaders bool       // Whether to hide headers in output
}

// RegisterFormatTable registers the format_table function with gojq
// Supports PowerShell-style parameters:
//   - format_table(objects) - format all properties as table
//   - format_table(objects; {property: "Name"}) - format specific property as column
//   - format_table(objects; {property: ["Name", "Length"]}) - format multiple columns
//   - format_table(objects; {property: ["Name", "Length"], autosize: true})
//   - format_table(objects; {property: ["Name", "Length"], hidetableheaders: true})
//
// Usage: format_table(objects) or format_table(objects; options)
func RegisterFormatTable() gojq.CompilerOption {
	return common.WithFunction("format_table", 1, 2, func(input any, args []any) any {
		// Parse arguments
		objects, opts, err := ParseFormatTableArgs(args)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		// Format objects as table
		formatted, err := formatTable(objects, opts)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		// A formatter's output is text. Returning the rendered string keeps it
		// printable (`pwrq -r 'format_table(.)'`) and pipeable into any jq
		// string builtin, instead of leaking a Go struct into the value space.
		return FormatTableToString(formatted)
	})
}

// formatTable formats objects as a table with columns and rows
func formatTable(objects []any, opts FormatTableOptions) (*FormattedTable, error) {
	if len(objects) == 0 {
		return &FormattedTable{
			Headers:          []string{},
			Rows:             [][]string{},
			Widths:           []int{},
			RowCount:         0,
			HideTableHeaders: opts.HideTableHeaders,
		}, nil
	}

	// Get all available properties from the first object to determine columns
	firstObj := objects[0]
	allProps := getAvailableProperties(firstObj)

	// Determine which properties to display as columns
	var columns []string
	if len(opts.Property) == 0 {
		// No property specified - use all available properties
		columns = allProps
	} else {
		// Match patterns against available properties
		columns = matchProperties(allProps, opts.Property, opts.CaseSensitive)
	}

	if len(columns) == 0 {
		return &FormattedTable{
			Headers:          []string{},
			Rows:             [][]string{},
			Widths:           []int{},
			RowCount:         len(objects),
			HideTableHeaders: opts.HideTableHeaders,
		}, nil
	}

	// Build the table
	table := &FormattedTable{
		Headers:          make([]string, len(columns)),
		Rows:             make([][]string, 0, len(objects)),
		Widths:           make([]int, len(columns)),
		RowCount:         len(objects),
		HideTableHeaders: opts.HideTableHeaders,
	}

	// Set headers
	for i, col := range columns {
		table.Headers[i] = col
		table.Widths[i] = len(col) // Initialize width to header length
	}

	// Build rows
	for _, obj := range objects {
		row := make([]string, len(columns))
		for i, col := range columns {
			value := getPropertyValue(obj, col)
			formattedValue := formatValue(value, opts.Depth, 0)
			row[i] = formattedValue

			// Track maximum width for each column
			if len(formattedValue) > table.Widths[i] {
				table.Widths[i] = len(formattedValue)
			}
		}
		table.Rows = append(table.Rows, row)
	}

	// Widths always fit their content. Narrowing them back to the header
	// length without -AutoSize - which is what this used to do - made any value
	// longer than its header overflow its column and shift every column after
	// it, so the header row and the data rows did not line up at all.
	return table, nil
}

// ParseFormatTableArgs parses arguments for the format_table function
func ParseFormatTableArgs(args []any) ([]any, FormatTableOptions, error) {
	opts := FormatTableOptions{
		CaseSensitive:    false,
		AutoSize:         false,
		HideTableHeaders: false,
		Depth:            0,
	}

	if len(args) == 0 {
		return []any{}, opts, fmt.Errorf("format_table: requires objects argument")
	}

	// First argument is objects
	var objects []any
	inputVal := common.BindValue(args[0])
	objects = common.NormalizeToSlice(inputVal)

	// Validate input
	if len(objects) == 0 && inputVal != nil {
		objects = []any{inputVal}
	}

	// Parse options if present
	if len(args) > 1 {
		if optsMap, ok := args[1].(map[string]any); ok {
			if propVal, exists := optsMap["property"]; exists {
				switch p := propVal.(type) {
				case string:
					opts.Property = []string{p}
				case []any:
					opts.Property = make([]string, 0, len(p))
					for _, item := range p {
						if str, ok := item.(string); ok {
							opts.Property = append(opts.Property, str)
						}
					}
				}
			}
			if csVal, exists := optsMap["casesensitive"]; exists {
				if csBool, ok := csVal.(bool); ok {
					opts.CaseSensitive = csBool
				}
			}
			if autoVal, exists := optsMap["autosize"]; exists {
				if autoBool, ok := autoVal.(bool); ok {
					opts.AutoSize = autoBool
				}
			}
			if hideVal, exists := optsMap["hidetableheaders"]; exists {
				if hideBool, ok := hideVal.(bool); ok {
					opts.HideTableHeaders = hideBool
				}
			}
			if depthVal, exists := optsMap["depth"]; exists {
				if depthInt, ok := depthVal.(float64); ok {
					opts.Depth = int(depthInt)
				} else if depthInt, ok := depthVal.(int); ok {
					opts.Depth = depthInt
				}
			}
		}
	}

	return objects, opts, nil
}

// FormatTableToString converts a formatted table to a PowerShell-style string representation
func FormatTableToString(table *FormattedTable) string {
	if table == nil || len(table.Headers) == 0 {
		return ""
	}

	var sb strings.Builder

	// Calculate column widths if not already set
	widths := make([]int, len(table.Headers))
	if len(table.Widths) == len(table.Headers) {
		copy(widths, table.Widths)
	} else {
		for i, header := range table.Headers {
			widths[i] = len(header)
		}
		for _, row := range table.Rows {
			for i, cell := range row {
				if len(cell) > widths[i] {
					widths[i] = len(cell)
				}
			}
		}
	}

	// Build header line
	if !table.HideTableHeaders {
		sb.WriteString("  ")
		for i, header := range table.Headers {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(padRight(header, widths[i]))
		}
		sb.WriteString("\n")

		// Build separator line
		sb.WriteString("  ")
		for i := range table.Headers {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(strings.Repeat("-", widths[i]))
		}
		sb.WriteString("\n")
	}

	// Build data rows
	for _, row := range table.Rows {
		sb.WriteString("  ")
		for i, cell := range row {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(padRight(cell, widths[i]))
		}
		sb.WriteString("\n")
	}

	return strings.TrimSuffix(sb.String(), "\n")
}

// GetTableHeaders extracts headers from a formatted table
func GetTableHeaders(table any) []string {
	if t, ok := table.(*FormattedTable); ok {
		return t.Headers
	}
	if m, ok := table.(map[string]any); ok {
		if headersVal, exists := m["Headers"]; exists {
			if headers, ok := headersVal.([]any); ok {
				result := make([]string, len(headers))
				for i, h := range headers {
					if str, ok := h.(string); ok {
						result[i] = str
					}
				}
				return result
			}
		}
	}
	return []string{}
}

// GetTableRows extracts rows from a formatted table
func GetTableRows(table any) [][]string {
	if t, ok := table.(*FormattedTable); ok {
		return t.Rows
	}
	if m, ok := table.(map[string]any); ok {
		if rowsVal, exists := m["Rows"]; exists {
			if rows, ok := rowsVal.([]any); ok {
				result := make([][]string, len(rows))
				for i, row := range rows {
					if rowSlice, ok := row.([]any); ok {
						result[i] = make([]string, len(rowSlice))
						for j, cell := range rowSlice {
							if str, ok := cell.(string); ok {
								result[i][j] = str
							}
						}
					}
				}
				return result
			}
		}
	}
	return [][]string{}
}

// GetTableRowCount returns the number of rows in a formatted table
func GetTableRowCount(table any) int {
	if t, ok := table.(*FormattedTable); ok {
		return t.RowCount
	}
	if m, ok := table.(map[string]any); ok {
		if countVal, exists := m["RowCount"]; exists {
			if count, ok := countVal.(int); ok {
				return count
			}
		}
	}
	return 0
}
