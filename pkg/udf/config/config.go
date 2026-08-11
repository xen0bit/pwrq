// Package config parses and renders the text formats configuration and logs
// actually arrive in: INI files, .env / Java properties, and logfmt key=value
// lines. Each is a pure string-in, object-out (and back) transform.
package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every config cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterIniParse(),
		RegisterIniStringify(),
		RegisterPropertiesParse(),
		RegisterPropertiesStringify(),
		RegisterLogfmtParse(),
		RegisterLogfmtStringify(),
	}
}

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

// objInput resolves an object from the pipeline or first argument.
func objInput(v any, args []any, name string) (map[string]any, error) {
	inputVal := v
	if len(args) > 0 {
		inputVal = args[0]
	}
	m, ok := common.BindValue(inputVal).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected an object, got %T", name, inputVal)
	}
	return m, nil
}

// RegisterIniParse registers ini_parse, an INI document to an object of
// sections. Lines outside a section land under the empty key.
func RegisterIniParse() gojq.CompilerOption {
	return gojq.WithFunction("ini_parse", 0, 1, func(v any, args []any) any {
		s, err := strInput(v, args, "ini_parse")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		root := make(map[string]any)
		section := ""
		for _, raw := range strings.Split(s, "\n") {
			line := strings.TrimSpace(raw)
			if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				section = strings.TrimSpace(line[1 : len(line)-1])
				if _, ok := root[section].(map[string]any); !ok {
					root[section] = make(map[string]any)
				}
				continue
			}
			key, val, ok := cutUnescaped(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			val = unquote(strings.TrimSpace(val))
			if section == "" {
				root[key] = val
				continue
			}
			sectionMap := root[section].(map[string]any)
			sectionMap[key] = val
		}
		return common.MakeUDFSuccessResult(root, nil)
	})
}

// RegisterIniStringify registers ini_stringify, an object to an INI document.
// Top-level keys are emitted first, then one [section] per nested object.
func RegisterIniStringify() gojq.CompilerOption {
	return gojq.WithFunction("ini_stringify", 0, 1, func(v any, args []any) any {
		m, err := objInput(v, args, "ini_stringify")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		var out strings.Builder
		keys := sortedKeys(m)
		for _, key := range keys {
			val := m[key]
			if _, ok := val.(map[string]any); ok {
				continue
			}
			fmt.Fprintf(&out, "%s=%s\n", key, scalarString(val))
		}
		for _, key := range keys {
			sub, ok := m[key].(map[string]any)
			if !ok {
				continue
			}
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			fmt.Fprintf(&out, "[%s]\n", key)
			for _, sk := range sortedKeys(sub) {
				fmt.Fprintf(&out, "%s=%s\n", sk, scalarString(sub[sk]))
			}
		}
		return common.MakeUDFSuccessResult(out.String(), nil)
	})
}

// RegisterPropertiesParse registers properties_parse, a Java-properties or
// .env document to an object. Comments (# or !), escaped separators and
// line continuations are handled.
func RegisterPropertiesParse() gojq.CompilerOption {
	return gojq.WithFunction("properties_parse", 0, 1, func(v any, args []any) any {
		s, err := strInput(v, args, "properties_parse")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		out := make(map[string]any)
		var pending strings.Builder
		for _, raw := range strings.Split(s, "\n") {
			line := strings.TrimRight(raw, "\r")
			if pending.Len() == 0 {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
					continue
				}
				pending.WriteString(line)
			} else {
				pending.WriteString(line)
			}
			if strings.HasSuffix(strings.TrimRight(pending.String(), " \t"), "\\") {
				// A trailing backslash continues the logical line onto the
				// next physical one; drop the marker and keep building.
				pending.Reset()
				pending.WriteString(strings.TrimSuffix(strings.TrimRight(line, " \t"), "\\"))
				continue
			}
			key, val, ok := cutProperty(pending.String())
			pending.Reset()
			if !ok {
				continue
			}
			key = unescapeProperty(strings.TrimSpace(key))
			val = unescapeProperty(strings.TrimSpace(val))
			out[key] = val
		}
		if pending.Len() > 0 {
			if key, val, ok := cutProperty(pending.String()); ok {
				out[unescapeProperty(strings.TrimSpace(key))] = unescapeProperty(strings.TrimSpace(val))
			}
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// cutProperty splits a properties line on the first unescaped '=' or ':'.
func cutProperty(line string) (key, val string, ok bool) {
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '=', ':':
			return line[:i], line[i+1:], true
		}
	}
	// A bare key is allowed and means an empty value.
	if key := strings.TrimSpace(line); key != "" {
		return key, "", true
	}
	return "", "", false
}

func unescapeProperty(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			out.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case 'r':
			out.WriteByte('\r')
		case 'u':
			if i+4 < len(s) {
				hex := s[i+1 : i+5]
				var r rune
				if _, err := fmt.Sscanf(hex, "%04x", &r); err == nil {
					out.WriteRune(r)
					i += 4
					continue
				}
			}
			out.WriteByte('u')
		default:
			out.WriteByte(s[i])
		}
	}
	return out.String()
}

// RegisterPropertiesStringify registers properties_stringify, an object to a
// .env / properties document.
func RegisterPropertiesStringify() gojq.CompilerOption {
	return gojq.WithFunction("properties_stringify", 0, 1, func(v any, args []any) any {
		m, err := objInput(v, args, "properties_stringify")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		var out strings.Builder
		for _, key := range sortedKeys(m) {
			fmt.Fprintf(&out, "%s=%s\n", key, scalarString(m[key]))
		}
		return common.MakeUDFSuccessResult(out.String(), nil)
	})
}

// RegisterLogfmtParse registers logfmt_parse, a logfmt line to an object.
// Quoted values stay strings; unquoted true/false/null and numbers get their
// JSON types; a bare key is true.
func RegisterLogfmtParse() gojq.CompilerOption {
	return gojq.WithFunction("logfmt_parse", 0, 1, func(v any, args []any) any {
		s, err := strInput(v, args, "logfmt_parse")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		out, err := logfmtParse(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("logfmt_parse: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

func logfmtParse(s string) (map[string]any, error) {
	out := make(map[string]any)
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		for i < len(s) && s[i] != ' ' && s[i] != '\t' && s[i] != '=' {
			i++
		}
		key := s[start:i]
		if key == "" {
			return nil, fmt.Errorf("empty key at position %d", i)
		}
		if i >= len(s) || s[i] != '=' {
			out[key] = true
			continue
		}
		i++
		if i < len(s) && s[i] == '"' {
			i++
			var val strings.Builder
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				val.WriteByte(s[i])
				i++
			}
			if i >= len(s) {
				return nil, fmt.Errorf("unterminated quoted value for %q", key)
			}
			i++ // closing quote
			out[key] = val.String()
			continue
		}
		start = i
		for i < len(s) && s[i] != ' ' && s[i] != '\t' {
			i++
		}
		raw := s[start:i]
		switch raw {
		case "true":
			out[key] = true
		case "false":
			out[key] = false
		case "null":
			out[key] = nil
		default:
			if n, ok := parseNumber(raw); ok {
				out[key] = n
			} else {
				out[key] = raw
			}
		}
	}
	return out, nil
}

func parseNumber(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	var n float64
	if _, err := fmt.Sscanf(s, "%g", &n); err != nil {
		return 0, false
	}
	// Reject tokens that a number parse would only partially consume.
	if _, err := strconv.ParseFloat(s, 64); err != nil {
		return 0, false
	}
	return n, true
}

// RegisterLogfmtStringify registers logfmt_stringify, an object to a logfmt
// line. Values needing spaces or special characters are quoted.
func RegisterLogfmtStringify() gojq.CompilerOption {
	return gojq.WithFunction("logfmt_stringify", 0, 1, func(v any, args []any) any {
		m, err := objInput(v, args, "logfmt_stringify")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		var out strings.Builder
		for _, key := range sortedKeys(m) {
			if out.Len() > 0 {
				out.WriteByte(' ')
			}
			out.WriteString(key)
			val := m[key]
			if b, ok := val.(bool); ok {
				if !b {
					out.WriteString("=false")
				}
				continue
			}
			if val == nil {
				continue
			}
			out.WriteByte('=')
			out.WriteString(logfmtValue(val))
		}
		return common.MakeUDFSuccessResult(out.String(), nil)
	})
}

func logfmtValue(v any) string {
	switch val := common.BindValue(v).(type) {
	case string:
		if val == "" || strings.ContainsAny(val, " \t\"=") {
			return `"` + strings.ReplaceAll(val, `"`, `\"`) + `"`
		}
		return val
	default:
		return scalarString(v)
	}
}

// cutUnescaped splits on the first unescaped separator.
func cutUnescaped(s, sep string) (before, after string, ok bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if strings.HasPrefix(s[i:], sep) {
			return s[:i], s[i+len(sep):], true
		}
	}
	return s, "", false
}

func unquote(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

func scalarString(v any) string {
	switch val := common.BindValue(v).(type) {
	case string:
		return val
	case nil:
		return ""
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		if f, ok := common.ToFloat64(val); ok {
			return fmt.Sprintf("%g", f)
		}
		if b, err := json.Marshal(val); err == nil {
			return string(b)
		}
		return fmt.Sprint(val)
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
