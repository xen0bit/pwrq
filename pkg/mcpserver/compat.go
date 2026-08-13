package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file is about the far end of the wire rather than about pwrq. Two facts
// of life shape it.
//
// A tool's input schema does not stop at the MCP client. Clients such as Open
// WebUI hand it to the model provider verbatim as the function's parameters,
// and the stricter providers reject constructs the Go SDK emits by default -
// notably the nullable type union ["null", "array"] that reflection produces
// for a Go slice, which Gemini refuses with a 400. portableSchema files those
// corners off.
//
// At the other end of a tool call is a language model, which will now and then
// send an object where the schema asked for a string, a quoted number where it
// asked for an integer, or an explicit null for an optional it decided to
// mention. Rejecting the call outright teaches the model little, so
// normalizeArguments coerces what is unambiguous before validation sees it.

// portableSchema infers a tool's input schema from its argument struct and
// removes "null" from the type unions reflection produces: a nil slice encodes
// as JSON null, so []namedArg is described as ["null", "array"], and a union is
// exactly what a strict tool-calling API rejects. Dropping "null" from what is
// advertised is safe because normalizeArguments removes null-valued properties
// before the schema is applied.
//
// Only input schemas are treated this way. An output schema is validated
// against what the tool actually returned, where a nil slice really is null.
func portableSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](&jsonschema.ForOptions{})
	if err != nil {
		panic(fmt.Sprintf("mcpserver: cannot infer an input schema for %T: %v", *new(T), err))
	}
	dropNullTypes(schema)
	return schema
}

// dropNullTypes rewrites every ["null", X] in the tree as plain X.
func dropNullTypes(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if len(s.Types) > 0 {
		kept := slices.DeleteFunc(slices.Clone(s.Types), func(t string) bool { return t == "null" })
		if len(kept) == 1 {
			s.Type, s.Types = kept[0], nil
		} else if len(kept) > 0 {
			s.Types = kept
		}
	}
	for _, p := range s.Properties {
		dropNullTypes(p)
	}
	for _, d := range s.Defs {
		dropNullTypes(d)
	}
	for _, sub := range slices.Concat(s.AllOf, s.AnyOf, s.OneOf, s.PrefixItems) {
		dropNullTypes(sub)
	}
	dropNullTypes(s.Items)
	dropNullTypes(s.AdditionalProperties)
}

// normalizeArguments is receiving middleware that reshapes the arguments of a
// tools/call towards what the called tool's schema declares, before the SDK
// validates them. It only ever makes an unambiguous change; anything it cannot
// make sense of is passed through so that validation rejects it with a message
// the model can read and retry against.
func normalizeArguments(schemas map[string]*jsonschema.Schema) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if call, ok := req.(*mcp.CallToolRequest); ok && call.Params != nil {
				if schema := schemas[call.Params.Name]; schema != nil {
					if args, changed := normalizeValue(schema, call.Params.Arguments); changed {
						call.Params.Arguments = args
					}
				}
			}
			return next(ctx, method, req)
		}
	}
}

// normalizeValue coerces one JSON value towards the type schema declares, and
// reports whether it changed anything.
func normalizeValue(schema *jsonschema.Schema, raw json.RawMessage) (json.RawMessage, bool) {
	if schema == nil || len(bytes.TrimSpace(raw)) == 0 {
		return raw, false
	}

	switch schemaType(schema) {
	case "object":
		return normalizeObject(schema, raw)

	case "array":
		return normalizeArray(schema, raw)

	case "string":
		// A model with data to hand tends to send it as itself rather than as
		// the JSON text the tool asked for: {"a": 1} where "{\"a\":1}" was
		// wanted. The text of what it sent is what the tool would have run on.
		if jsonKind(raw) == "string" {
			return raw, false
		}
		var buf bytes.Buffer
		if err := json.Compact(&buf, raw); err != nil {
			return raw, false
		}
		encoded, err := json.Marshal(buf.String())
		if err != nil {
			return raw, false
		}
		return encoded, true

	case "integer", "number":
		// A number that arrived quoted, as models routinely send them.
		if unquoted, ok := unquote(raw); ok {
			if _, err := strconv.ParseFloat(unquoted, 64); err == nil {
				return json.RawMessage(unquoted), true
			}
		}

	case "boolean":
		if unquoted, ok := unquote(raw); ok {
			switch strings.ToLower(unquoted) {
			case "true":
				return json.RawMessage("true"), true
			case "false":
				return json.RawMessage("false"), true
			}
		}
	}
	return raw, false
}

// normalizeObject walks an object's properties: it drops the nulls a model
// writes for optionals it chose not to use, drops properties the schema does
// not allow rather than failing the whole call over one invented flag, and
// normalizes the rest against their own schemas.
func normalizeObject(schema *jsonschema.Schema, raw json.RawMessage) (json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return raw, false
	}

	closed := isFalseSchema(schema.AdditionalProperties)
	changed := false
	for name, value := range fields {
		prop, known := schema.Properties[name]
		switch {
		case jsonKind(value) == "null":
			delete(fields, name)
			changed = true
		case !known:
			if closed {
				delete(fields, name)
				changed = true
			}
		default:
			if normalized, ok := normalizeValue(prop, value); ok {
				fields[name] = normalized
				changed = true
			}
		}
	}
	if !changed {
		return raw, false
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return raw, false
	}
	return encoded, true
}

// normalizeArray normalizes each element against the array's item schema, which
// is how a named argument's value gets the same treatment as a top-level one.
func normalizeArray(schema *jsonschema.Schema, raw json.RawMessage) (json.RawMessage, bool) {
	if schema.Items == nil {
		return raw, false
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return raw, false
	}
	changed := false
	for i, elem := range elems {
		if normalized, ok := normalizeValue(schema.Items, elem); ok {
			elems[i] = normalized
			changed = true
		}
	}
	if !changed {
		return raw, false
	}
	encoded, err := json.Marshal(elems)
	if err != nil {
		return raw, false
	}
	return encoded, true
}

// schemaType is the single type a schema declares, or "" when it declares none
// or several. A union is left to validation: guessing which member was meant
// would be a coercion this package cannot justify.
func schemaType(s *jsonschema.Schema) string {
	if len(s.Types) > 0 {
		return ""
	}
	return s.Type
}

// isFalseSchema reports whether a subschema rejects everything, which is how
// `"additionalProperties": false` is represented.
func isFalseSchema(s *jsonschema.Schema) bool {
	if s == nil {
		return false
	}
	encoded, err := json.Marshal(s)
	return err == nil && string(encoded) == "false"
}

// jsonKind names the JSON type of an encoded value, cheaply, from its first
// byte.
func jsonKind(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	switch raw[0] {
	case '"':
		return "string"
	case '{':
		return "object"
	case '[':
		return "array"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// unquote returns the contents of a JSON string, trimmed, and whether the value
// was a string at all.
func unquote(raw json.RawMessage) (string, bool) {
	if jsonKind(raw) != "string" {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return strings.TrimSpace(s), true
}
