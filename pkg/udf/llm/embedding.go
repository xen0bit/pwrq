package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterInvokeEmbedding registers invoke_embedding, which turns text into the
// vector a model represents its meaning by.
//
// It returns the vector itself rather than an object around it, for the same
// reason invoke_llm returns the completion: the vector is the value the next
// stage wants. Paired with cosine_similarity it makes semantic search an
// ordinary jq pipeline —
//
//	[$docs[] | {text: ., v: invoke_embedding(.)}] as $index
//	| invoke_embedding($question) as $q
//	| [$index[] | {text, score: cosine_similarity(.v; $q)}] | sort_by(-.score)
//
// which is the same shape as the string-similarity cmdlets pwrq already has,
// over a comparison they cannot make.
func RegisterInvokeEmbedding() gojq.CompilerOption {
	const op = "invoke_embedding"
	return common.WithFunction(op, 0, 2, func(v any, args []any) any {
		c, err := parseCall(op, v, args)
		if err != nil {
			return err
		}
		p, err := c.options.resolve(op)
		if err != nil {
			return err
		}
		if p.dialect != dialectOpenAI {
			return fmt.Errorf("%s: provider %q has no embeddings API; use openai, ollama or an openai-compatible server", op, p.name)
		}
		if err := c.options.requireKey(op, p); err != nil {
			return err
		}

		// A string embeds to one vector and an array to one vector per
		// element, in order. Anything else is a mistake worth naming: an
		// object has no text to embed, and guessing which field held it is
		// the kind of guess that fails silently.
		var (
			texts  []string
			single bool
		)
		switch in := common.BindValue(c.input).(type) {
		case string:
			texts, single = []string{in}, true
		case []any:
			for i, item := range in {
				s, err := common.BindString(common.BindValue(item), "Input")
				if err != nil {
					return fmt.Errorf("%s: element %d: %w", op, i, err)
				}
				texts = append(texts, s)
			}
		default:
			return fmt.Errorf("%s: expected a string or an array of strings, got %T", op, in)
		}
		if len(texts) == 0 {
			return []any{}
		}

		if err := chargeCall(op, c.options); err != nil {
			return err
		}
		vectors, tokens, err := embed(context.Background(), op, texts, c.options)
		if err != nil {
			return err
		}
		recordUsage(&response{InputTokens: tokens}, c.options)

		if single {
			return vectors[0]
		}
		out := make([]any, len(vectors))
		for i, vec := range vectors {
			out[i] = vec
		}
		return out
	})
}

func embed(ctx context.Context, op string, texts []string, o options) ([][]any, int, error) {
	body, err := json.Marshal(map[string]any{"model": o.modelID(), "input": texts})
	if err != nil {
		return nil, 0, fmt.Errorf("%s: encoding request: %w", op, err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, o.timeout())
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, o.BaseUrl+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if o.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.ApiKey)
	}
	debugf("POST %s/embeddings (%d input(s))", o.BaseUrl, len(texts))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if reqCtx.Err() != nil {
			return nil, 0, fmt.Errorf("%s: request timed out after %s", op, o.timeout())
		}
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: reading response: %w", op, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("%s: %s returned %s: %s", op, o.Model, resp.Status, apiErrorMessage(payload))
	}

	var decoded struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, 0, fmt.Errorf("%s: decoding response: %w", op, err)
	}
	if len(decoded.Data) != len(texts) {
		return nil, 0, fmt.Errorf("%s: asked for %d embeddings and got %d", op, len(texts), len(decoded.Data))
	}
	// The API documents index rather than order, so honour it: a vector
	// matched to the wrong text is a failure nothing downstream can detect.
	sort.Slice(decoded.Data, func(i, j int) bool { return decoded.Data[i].Index < decoded.Data[j].Index })

	vectors := make([][]any, len(decoded.Data))
	for i, item := range decoded.Data {
		vec := make([]any, len(item.Embedding))
		for j, f := range item.Embedding {
			vec[j] = f
		}
		vectors[i] = vec
	}
	return vectors, decoded.Usage.PromptTokens, nil
}
