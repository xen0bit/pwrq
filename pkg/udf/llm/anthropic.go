package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// anthropicVersion is the API version header the Messages API requires. It is
// a date, and pinning it is the point: the server changes behaviour when the
// header changes, not when the client does.
const anthropicVersion = "2023-06-01"

// structuredToolName is the tool a schema call is forced through. The Messages
// API has no response_format, so a tool whose input_schema is the caller's
// schema is how a typed answer is asked for — and the model then cannot reply
// with prose by mistake.
const structuredToolName = "emit_result"

func anthropicRequest(messages []message, o options, validator *schemaValidator) (string, []byte, error) {
	type content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type msg struct {
		Role    string    `json:"role"`
		Content []content `json:"content"`
	}

	body := map[string]any{
		"model":      o.modelID(),
		"max_tokens": o.MaxTokens,
	}

	turns := make([]msg, 0, len(messages))
	for _, m := range messages {
		turns = append(turns, msg{Role: m.Role, Content: []content{{Type: "text", Text: m.Content}}})
	}
	body["messages"] = turns

	if o.System != "" {
		body["system"] = o.System
	}
	// Temperature is always sent: the default is 0, and a pipeline that cannot
	// be re-run is a pipeline whose output nobody can check.
	body["temperature"] = o.Temperature
	if o.TopP > 0 {
		body["top_p"] = o.TopP
	}
	if len(o.StopAt) > 0 {
		body["stop_sequences"] = o.StopAt
	}

	if validator != nil {
		body["tools"] = []any{map[string]any{
			"name":         structuredToolName,
			"description":  "Return the result. Every field must satisfy the schema.",
			"input_schema": validator.schema,
		}}
		body["tool_choice"] = map[string]any{"type": "tool", "name": structuredToolName}
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return "", nil, fmt.Errorf("encoding request: %w", err)
	}
	return "/v1/messages", encoded, nil
}

func anthropicResponse(payload []byte, structured bool) (*response, error) {
	var decoded struct {
		Content []struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			Thinking string          `json:"thinking"`
			Name     string          `json:"name"`
			Input    json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var text, reasoning strings.Builder
	for _, block := range decoded.Content {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "thinking":
			reasoning.WriteString(block.Thinking)
		case "tool_use":
			// A forced tool call carries the answer as its input, which is
			// already JSON — so the schema path and the text path hand the
			// same kind of string to the validator.
			if structured && block.Name == structuredToolName {
				text.Write(block.Input)
			}
		}
	}

	if text.Len() == 0 {
		if decoded.StopReason == "max_tokens" {
			return nil, errTruncatedReply
		}
		return nil, fmt.Errorf("the model returned no content (stop_reason %q)", decoded.StopReason)
	}
	return &response{
		Content:      text.String(),
		Reasoning:    reasoning.String(),
		StopReason:   decoded.StopReason,
		InputTokens:  decoded.Usage.InputTokens,
		OutputTokens: decoded.Usage.OutputTokens,
	}, nil
}
