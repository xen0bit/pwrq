package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// schemaValidator turns a caller's JSON Schema into something that can both be
// sent to a provider and used to check what comes back.
//
// Checking matters more than sending. Both providers can be asked for JSON,
// neither guarantees it satisfies the schema, and a pipeline that receives an
// object missing the field it is about to group by fails somewhere else
// entirely. Validating here means the failure names the schema.
type schemaValidator struct {
	// schema is the caller's document, passed to the provider unchanged.
	schema map[string]any
	// resolved is the same document compiled for validation.
	resolved *jsonschema.Resolved
}

func compileSchema(op string, doc map[string]any) (*schemaValidator, error) {
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("%s: Schema is not valid JSON: %w", op, err)
	}
	var parsed jsonschema.Schema
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		return nil, fmt.Errorf("%s: Schema is not a valid JSON Schema: %w", op, err)
	}
	resolved, err := parsed.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("%s: Schema is not a valid JSON Schema: %w", op, err)
	}
	return &schemaValidator{schema: doc, resolved: resolved}, nil
}

// decode parses the model's answer and validates it.
func (v *schemaValidator) decode(op, content string) (any, error) {
	var value any
	if err := json.Unmarshal([]byte(stripFence(content)), &value); err != nil {
		return nil, fmt.Errorf("%s: the model did not return JSON: %w", op, err)
	}
	if err := v.resolved.Validate(value); err != nil {
		return nil, fmt.Errorf("%s: the model's JSON does not satisfy Schema: %w", op, err)
	}
	return value, nil
}

// stripFence removes a markdown code fence around a JSON answer.
//
// Models wrap JSON in ```json ... ``` often enough that refusing it would make
// the schema path fail for a reason the caller cannot act on, and unwrapping
// is unambiguous: a fenced block is not itself valid JSON.
func stripFence(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	trimmed = strings.TrimPrefix(trimmed, "```")
	if newline := strings.IndexByte(trimmed, '\n'); newline >= 0 {
		// Drop the language tag on the opening fence, if there is one.
		if tag := strings.TrimSpace(trimmed[:newline]); tag == "" || !strings.ContainsAny(tag, "{}[]\"") {
			trimmed = trimmed[newline+1:]
		}
	}
	if end := strings.LastIndex(trimmed, "```"); end >= 0 {
		trimmed = trimmed[:end]
	}
	return strings.TrimSpace(trimmed)
}
