// Package aggregate provides PowerShell-style grouping and summarising over
// arrays of JSON objects: Group-Object, Measure-Object and pivot tables as pure
// functions. Keys are property names, matched case-insensitively, and values
// are grouped by their JSON identity so numbers and strings never mix.
package aggregate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every aggregation cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterGroupByKey(),
		RegisterCountBy(),
		RegisterSumBy(),
		RegisterAvgBy(),
		RegisterIndexBy(),
		RegisterValueCounts(),
		RegisterSummarizeBy(),
		RegisterPivot(),
		RegisterUnpivot(),
		RegisterTopBy(),
		RegisterBottomBy(),
	}
}

// rowsInput separates the array of rows from a cmdlet's operands, following
// common.SplitInput: the explicit input is the leading argument at the cmdlet's
// maximum arity, and is never guessed at by inspecting operand types.
func rowsInput(v any, args []any, operands int) (rows []any, rest []any) {
	in, rest := common.SplitInput(v, args, operands)
	return common.NormalizeToSlice(common.BindValue(in)), rest
}

// keyArg reads operand i as a property name.
func keyArg(fn string, args []any, i int) (string, error) {
	if i >= len(args) {
		return "", fmt.Errorf("%s: a key property name is required", fn)
	}
	s, ok := common.BindValue(args[i]).(string)
	if !ok {
		return "", fmt.Errorf("%s: key must be a string, got %T", fn, common.BindValue(args[i]))
	}
	return s, nil
}

// rowValue reads a property from a row, following dot paths like
// "address.city", and matching property names case-insensitively.
func rowValue(row any, key string) (any, error) {
	if got, err := common.ExtractPropertyByPath(row, key); err == nil {
		return got, nil
	}
	if m, ok := common.BindValue(row).(map[string]any); ok {
		for name, val := range m {
			if stringsEqualFold(name, key) {
				return val, nil
			}
		}
	}
	return nil, fmt.Errorf("property %q not found on %T", key, row)
}

func stringsEqualFold(a, b string) bool {
	return len(a) == len(b) && toLower(a) == toLower(b)
}

func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

// groupAndValue reads the grouping key and the numeric column from positional
// operands. When the column is omitted the grouping key is itself the column,
// which is what makes sum_by("amount") total that property.
func groupAndValue(args []any) (groupKey, valueKey string) {
	if len(args) == 0 {
		return "", ""
	}
	groupKey, _ = common.BindValue(args[0]).(string)
	if len(args) > 1 {
		if s, ok := common.BindValue(args[1]).(string); ok {
			return groupKey, s
		}
	}
	return groupKey, groupKey
}

// keyString renders a value as a stable object key. Strings stay bare so the
// output reads naturally; everything else is a JSON rendering, so a number 1
// and a string "1" stay distinct.
func keyString(v any) string {
	switch val := v.(type) {
	case nil:
		return "null"
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case json.Number:
		return val.String()
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	case int:
		return strconv.Itoa(val)
	}
	if f, ok := common.ToFloat64(v); ok {
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%T:%v", v, v)
	}
	return string(b)
}

// numValue coerces a row property to a number.
func numValue(row any, key string) (float64, error) {
	got, err := rowValue(row, key)
	if err != nil {
		return 0, err
	}
	f, ok := common.ToFloat64(got)
	if !ok {
		return 0, fmt.Errorf("property %q is not a number (%T)", key, got)
	}
	return f, nil
}

// RegisterGroupByKey registers group_by_key, buckets an array of objects by a
// property into {keyvalue: [rows...]}.
func RegisterGroupByKey() gojq.CompilerOption {
	return gojq.WithFunction("group_by_key", 1, 2, func(v any, args []any) any {
		arr, rest := rowsInput(v, args, 1)
		key, err := keyArg("group_by_key", rest, 0)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		buckets := make(map[string][]any)
		for _, row := range arr {
			got, err := rowValue(row, key)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("group_by_key: %v", err), nil)
			}
			ks := keyString(got)
			buckets[ks] = append(buckets[ks], row)
		}
		out := make(map[string]any, len(buckets))
		for ks, rows := range buckets {
			out[ks] = rows
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterCountBy registers count_by, a property's value to a row count.
func RegisterCountBy() gojq.CompilerOption {
	return gojq.WithFunction("count_by", 1, 2, func(v any, args []any) any {
		arr, rest := rowsInput(v, args, 1)
		key, err := keyArg("count_by", rest, 0)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		counts := make(map[string]any)
		for _, row := range arr {
			got, err := rowValue(row, key)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("count_by: %v", err), nil)
			}
			ks := keyString(got)
			counts[ks] = intToAny(intVal(counts[ks]) + 1)
		}
		return common.MakeUDFSuccessResult(counts, nil)
	})
}

func intVal(v any) int {
	if f, ok := common.ToFloat64(v); ok {
		return int(f)
	}
	return 0
}

func intToAny(n int) any { return n }

// RegisterSumBy registers sum_by, a numeric column summed per key value:
// sum_by(arr; "dept"; "salary") -> {eng: 200, ops: 80}. Without a column the
// key itself is summed, so sum_by(arr; "amount") totals amount per value.
func RegisterSumBy() gojq.CompilerOption {
	return gojq.WithFunction("sum_by", 1, 3, func(v any, args []any) any {
		arr, rest := rowsInput(v, args, 2)
		groupKey, valueKey := groupAndValue(rest)
		if groupKey == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("sum_by: a key property name is required"), nil)
		}
		type acc struct{ sum float64 }
		accs := make(map[string]*acc)
		order := []string{}
		for _, row := range arr {
			got, err := rowValue(row, groupKey)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("sum_by: %v", err), nil)
			}
			ks := keyString(got)
			if _, seen := accs[ks]; !seen {
				accs[ks] = &acc{}
				order = append(order, ks)
			}
			f, err := numValue(row, valueKey)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("sum_by: %v", err), nil)
			}
			accs[ks].sum += f
		}
		out := make(map[string]any, len(order))
		for _, ks := range order {
			out[ks] = accs[ks].sum
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterAvgBy registers avg_by, a numeric column averaged per key value.
func RegisterAvgBy() gojq.CompilerOption {
	return gojq.WithFunction("avg_by", 1, 3, func(v any, args []any) any {
		arr, rest := rowsInput(v, args, 2)
		groupKey, valueKey := groupAndValue(rest)
		if groupKey == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("avg_by: a key property name is required"), nil)
		}
		type acc struct{ sum, n float64 }
		accs := make(map[string]*acc)
		order := []string{}
		for _, row := range arr {
			got, err := rowValue(row, groupKey)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("avg_by: %v", err), nil)
			}
			ks := keyString(got)
			if _, seen := accs[ks]; !seen {
				accs[ks] = &acc{}
				order = append(order, ks)
			}
			f, err := numValue(row, valueKey)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("avg_by: %v", err), nil)
			}
			accs[ks].sum += f
			accs[ks].n++
		}
		out := make(map[string]any, len(order))
		for _, ks := range order {
			out[ks] = accs[ks].sum / accs[ks].n
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterIndexBy registers index_by, the first row seen per key value.
func RegisterIndexBy() gojq.CompilerOption {
	return gojq.WithFunction("index_by", 1, 2, func(v any, args []any) any {
		arr, rest := rowsInput(v, args, 1)
		key, err := keyArg("index_by", rest, 0)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		index := make(map[string]any)
		for _, row := range arr {
			got, err := rowValue(row, key)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("index_by: %v", err), nil)
			}
			ks := keyString(got)
			if _, seen := index[ks]; !seen {
				index[ks] = row
			}
		}
		return common.MakeUDFSuccessResult(index, nil)
	})
}

// RegisterValueCounts registers value_counts, whole-value frequencies of an
// array as {value: count}.
func RegisterValueCounts() gojq.CompilerOption {
	return gojq.WithFunction("value_counts", 0, 1, func(v any, args []any) any {
		arr, _ := rowsInput(v, args, 0)
		counts := make(map[string]any)
		order := []string{}
		for _, item := range arr {
			ks := keyString(common.BindValue(item))
			if _, seen := counts[ks]; !seen {
				order = append(order, ks)
			}
			counts[ks] = intVal(counts[ks]) + 1
		}
		out := make(map[string]any, len(order))
		for _, ks := range order {
			out[ks] = counts[ks]
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterSummarizeBy registers summarize_by, one summary object per key value
// with count, sum, average, minimum and maximum over a numeric column:
// summarize_by(arr; "dept"; "salary"). Without a column the key itself is the
// column.
func RegisterSummarizeBy() gojq.CompilerOption {
	return gojq.WithFunction("summarize_by", 1, 3, func(v any, args []any) any {
		arr, rest := rowsInput(v, args, 2)
		groupKey, valueKey := groupAndValue(rest)
		if groupKey == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("summarize_by: a key property name is required"), nil)
		}
		type acc struct {
			key any
			sum float64
			min float64
			max float64
			n   float64
		}
		accs := make(map[string]*acc)
		order := []string{}
		for _, row := range arr {
			got, err := rowValue(row, groupKey)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("summarize_by: %v", err), nil)
			}
			f, err := numValue(row, valueKey)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("summarize_by: %v", err), nil)
			}
			ks := keyString(got)
			a, seen := accs[ks]
			if !seen {
				a = &acc{key: got, min: f, max: f}
				accs[ks] = a
				order = append(order, ks)
			}
			a.sum += f
			if f < a.min {
				a.min = f
			}
			if f > a.max {
				a.max = f
			}
			a.n++
		}
		out := make([]any, 0, len(order))
		for _, ks := range order {
			a := accs[ks]
			out = append(out, map[string]any{
				"key":   a.key,
				"count": int(a.n),
				"sum":   a.sum,
				"avg":   a.sum / a.n,
				"min":   a.min,
				"max":   a.max,
			})
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// options reads a parameter object like {rows: "dept", cols: "year",
// values: "amount"} into a map of string options.
// options reads an options object from a positional operand.
func options(fn string, args []any, i int) (map[string]string, error) {
	opts := make(map[string]string)
	if i >= len(args) {
		return nil, fmt.Errorf("%s: an options object is required", fn)
	}
	m, ok := common.BindValue(args[i]).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: options must be an object, got %T", fn, common.BindValue(args[i]))
	}
	for k, val := range m {
		if s, ok := val.(string); ok {
			opts[toLower(k)] = s
		}
	}
	return opts, nil
}

// RegisterPivot registers pivot, building a row×column table from an array of
// objects: {rows: "dept", cols: "year", values: "amount"} turns
// [{dept:"eng",year:2020,amount:10}, ...] into {eng: {2020: 10, ...}, ...}.
func RegisterPivot() gojq.CompilerOption {
	return gojq.WithFunction("pivot", 1, 2, func(v any, args []any) any {
		arr, rest := rowsInput(v, args, 1)
		opts, err := options("pivot", rest, 0)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		rowKey, colKey, valKey := opts["rows"], opts["cols"], opts["values"]
		if rowKey == "" || colKey == "" || valKey == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("pivot: {rows, cols, values} are all required"), nil)
		}
		table := make(map[string]any)
		for _, row := range arr {
			rv, err := rowValue(row, rowKey)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("pivot: %v", err), nil)
			}
			cv, err := rowValue(row, colKey)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("pivot: %v", err), nil)
			}
			vv, err := numValue(row, valKey)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("pivot: %v", err), nil)
			}
			rk := keyString(rv)
			rowMap, ok := table[rk].(map[string]any)
			if !ok {
				rowMap = make(map[string]any)
				table[rk] = rowMap
			}
			rowMap[keyString(cv)] = vv
		}
		return common.MakeUDFSuccessResult(table, nil)
	})
}

// RegisterUnpivot registers unpivot, the inverse of pivot: it melts selected
// value columns of an array of objects into {key, value} rows. Options:
// {cols: ["2020","2021"], id: "name"} keeps the id property alongside the
// melted key and value.
func RegisterUnpivot() gojq.CompilerOption {
	return gojq.WithFunction("unpivot", 1, 2, func(v any, args []any) any {
		arr, rest := rowsInput(v, args, 1)
		var cols []string
		idKey := ""
		for _, a := range rest {
			switch val := common.BindValue(a).(type) {
			case map[string]any:
				if list, ok := val["cols"].([]any); ok {
					for _, c := range list {
						if s, ok := c.(string); ok {
							cols = append(cols, s)
						}
					}
				}
				if s, ok := val["id"].(string); ok {
					idKey = s
				}
			case []any:
				for _, c := range val {
					if s, ok := c.(string); ok {
						cols = append(cols, s)
					}
				}
			}
		}
		if len(cols) == 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("unpivot: {cols: [...]} is required"), nil)
		}
		out := []any{}
		for _, row := range arr {
			m, ok := common.BindValue(row).(map[string]any)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("unpivot: expected an array of objects, got %T", row), nil)
			}
			for _, col := range cols {
				val, exists := m[col]
				if !exists {
					val = nil
				}
				melted := map[string]any{"key": col, "value": val}
				if idKey != "" {
					if idVal, err := rowValue(row, idKey); err == nil {
						melted[idKey] = idVal
					}
				}
				out = append(out, melted)
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterTopBy registers top_by, the n rows with the largest value of a
// numeric property, sorted descending.
func RegisterTopBy() gojq.CompilerOption {
	return gojq.WithFunction("top_by", 1, 3, func(v any, args []any) any {
		return rankedBy("top_by", v, args, false)
	})
}

// RegisterBottomBy registers bottom_by, the n rows with the smallest value of
// a numeric property, sorted ascending.
func RegisterBottomBy() gojq.CompilerOption {
	return gojq.WithFunction("bottom_by", 1, 3, func(v any, args []any) any {
		return rankedBy("bottom_by", v, args, true)
	})
}

func rankedBy(fn string, v any, args []any, ascending bool) any {
	arr, rest := rowsInput(v, args, 2)
	key, err := keyArg(fn, rest, 0)
	if err != nil {
		return common.MakeUDFErrorResult(err, nil)
	}
	n := 1
	if len(rest) > 1 {
		cnt, ok := common.ToInt(common.BindValue(rest[1]))
		if !ok || cnt < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: count must be a non-negative integer, got %v", fn, rest[1]), nil)
		}
		n = cnt
	}
	type scored struct {
		value float64
		row   any
	}
	scoredList := make([]scored, 0, len(arr))
	for _, row := range arr {
		f, err := numValue(row, key)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("rankedBy: %v", err), nil)
		}
		scoredList = append(scoredList, scored{value: f, row: row})
	}
	sort.SliceStable(scoredList, func(i, j int) bool {
		if ascending {
			return scoredList[i].value < scoredList[j].value
		}
		return scoredList[i].value > scoredList[j].value
	})
	if n > len(scoredList) {
		n = len(scoredList)
	}
	if n < 0 {
		n = 0
	}
	out := make([]any, n)
	for i := 0; i < n; i++ {
		out[i] = scoredList[i].row
	}
	return common.MakeUDFSuccessResult(out, nil)
}
