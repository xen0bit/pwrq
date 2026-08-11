package csv

import (
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// strInput resolves a string from the pipeline or first argument.
func strInput(v any, args []any, name string) (string, error) {
	inputVal, _, err := common.ParseFileArgs(v, args)
	if err != nil {
		return "", err
	}
	switch val := common.BindValue(inputVal).(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	default:
		return "", fmt.Errorf("%s: expected a string, got %T", name, inputVal)
	}
}

// RegisterTSVParse registers tsv_parse, a tab-separated document to an array of
// rows, honouring quoted fields.
func RegisterTSVParse() gojq.CompilerOption {
	return gojq.WithFunction("tsv_parse", 0, 1, func(v any, args []any) any {
		s, err := strInput(v, args, "tsv_parse")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		reader := csv.NewReader(strings.NewReader(s))
		reader.Comma = '\t'
		records, err := reader.ReadAll()
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("tsv_parse: %v", err), nil)
		}
		out := make([]any, len(records))
		for i, row := range records {
			items := make([]any, len(row))
			for j, cell := range row {
				items[j] = cell
			}
			out[i] = items
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterTSVStringify registers tsv_stringify, an array of rows to a
// tab-separated document, quoting cells that need it.
func RegisterTSVStringify() gojq.CompilerOption {
	return gojq.WithFunction("tsv_stringify", 0, 1, func(v any, args []any) any {
		input := common.BindValue(v)
		if len(args) > 0 {
			input = common.BindValue(args[0])
		}
		rows, ok := input.([]any)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("tsv_stringify: expected an array of rows, got %T", input), nil)
		}
		var out strings.Builder
		writer := csv.NewWriter(&out)
		writer.Comma = '\t'
		for _, row := range rows {
			rowArr, ok := row.([]any)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("tsv_stringify: expected an array of rows, got row %T", row), nil)
			}
			record := make([]string, len(rowArr))
			for i, cell := range rowArr {
				record[i] = cellString(cell)
			}
			if err := writer.Write(record); err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("tsv_stringify: %v", err), nil)
			}
		}
		writer.Flush()
		return common.MakeUDFSuccessResult(out.String(), nil)
	})
}

func cellString(v any) string {
	switch val := common.BindValue(v).(type) {
	case string:
		return val
	case nil:
		return ""
	default:
		return fmt.Sprint(val)
	}
}
