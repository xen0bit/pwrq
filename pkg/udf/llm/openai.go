package llm

import (
	"encoding/json"
	"fmt"
)

// openAIRequest encodes a chat completion. This dialect is a family rather
// than a vendor: Ollama, vLLM, OpenRouter, Groq and LM Studio all speak it, and
// which one a call reaches is decided by the base URL alone.
func openAIRequest(messages []message, o options, validator *schemaValidator) (string, []byte, error) {
	// The wire shape of a turn happens to match message field for field, so a
	// conversion carries it rather than a rebuild.
	type msg message

	turns := make([]msg, 0, len(messages)+1)
	if o.System != "" {
		turns = append(turns, msg{Role: "system", Content: o.System})
	}
	for _, m := range messages {
		turns = append(turns, msg(m))
	}

	body := map[string]any{
		"model":       o.modelID(),
		"messages":    turns,
		"max_tokens":  o.MaxTokens,
		"temperature": o.Temperature,
	}
	if o.TopP > 0 {
		body["top_p"] = o.TopP
	}
	if len(o.StopAt) > 0 {
		body["stop"] = o.StopAt
	}
	if validator != nil {
		// strict is deliberately off. It requires every property to be
		// required and additionalProperties to be false, which rejects
		// perfectly ordinary schemas; pwrq validates the answer itself, so a
		// schema the server would have refused still works here.
		body["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "result",
				"schema": validator.schema,
				"strict": false,
			},
		}
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return "", nil, fmt.Errorf("encoding request: %w", err)
	}
	return "/chat/completions", encoded, nil
}

func openAIResponse(payload []byte) (*response, error) {
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
				// Local runtimes — LM Studio, Ollama, vLLM — put a reasoning
				// model's chain of thought here rather than in the content.
				// Reporting it separately means .Content stays the answer
				// while the reasoning is still there to read.
				Reasoning string `json:"reasoning_content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return nil, fmt.Errorf("the model returned no choices")
	}
	choice := decoded.Choices[0]
	if choice.Message.Content == "" {
		// A reasoning model can spend the whole budget thinking and return
		// nothing, which looks like a broken provider until you know that is
		// what happened.
		if choice.FinishReason == "length" {
			return nil, errTruncatedReply
		}
		return nil, fmt.Errorf("the model returned no content (finish_reason %q)", choice.FinishReason)
	}
	return &response{
		Content:      choice.Message.Content,
		Reasoning:    choice.Message.Reasoning,
		StopReason:   choice.FinishReason,
		InputTokens:  decoded.Usage.PromptTokens,
		OutputTokens: decoded.Usage.CompletionTokens,
	}, nil
}
