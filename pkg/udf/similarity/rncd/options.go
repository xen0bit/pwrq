package rncd

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strings"

	"github.com/xen0bit/pwrq/pkg/core/pipeline"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// bindOptions reads a cmdlet's trailing options object, rejecting any name the
// options struct does not declare.
//
// BindParameters on its own ignores what it does not recognize, which is fine
// for a flag that only affects presentation and wrong here: a misspelled
// {Alpah: 0.9} would score the corpus with the default weights and return
// numbers the caller would read as the ones they asked for.
func bindOptions(op string, arg any, target any) error {
	value := common.BindValue(arg)
	if value == nil {
		return nil
	}
	opts, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: options must be an object, got %T", op, value)
	}

	known := paramNames(target)
	normalized := make(map[string]any, len(opts))
	for k, v := range opts {
		if _, ok := known[strings.ToLower(k)]; !ok {
			return fmt.Errorf("%s: unknown option %q; expected one of %s",
				op, k, strings.Join(sortedNames(known), ", "))
		}
		normalized[k] = normalizeNumber(v)
	}
	if err := pipeline.BindParameters(normalized, target); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// paramNames maps an options struct's declared names, lowercased for the
// case-insensitive match, to the spelling to suggest in an error.
func paramNames(target any) map[string]string {
	names := make(map[string]string)
	t := reflect.TypeOf(target)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return names
	}
	for i := range t.NumField() {
		declared := strings.TrimSpace(strings.Split(t.Field(i).Tag.Get("param"), ",")[0])
		if declared != "" {
			names[strings.ToLower(declared)] = declared
		}
	}
	return names
}

func sortedNames(known map[string]string) []string {
	names := make([]string, 0, len(known))
	for _, declared := range known {
		names = append(names, declared)
	}
	sort.Strings(names)
	return names
}

// normalizeNumber converts the number representations the pipeline carries
// into the ones the parameter binder converts from. The CLI decodes with
// UseNumber, so a value read out of a JSON document arrives as a json.Number
// and would otherwise fail to bind to an int.
func normalizeNumber(v any) any {
	switch n := v.(type) {
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
		if f, err := n.Float64(); err == nil {
			return f
		}
		return n.String()
	case *big.Int:
		if n.IsInt64() {
			return int(n.Int64())
		}
		return n.String()
	default:
		return v
	}
}
