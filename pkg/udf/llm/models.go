package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterGetModel registers get_llm_model, which asks the provider what it
// serves.
//
// This matters most for the case pwrq is likeliest to meet: a local server with
// a dozen models loaded, where the name is whatever the person who downloaded
// it called the file. Guessing that name from documentation is not possible,
// and a 404 from an inference server rarely says which names would have worked.
func RegisterGetModel() gojq.CompilerOption {
	const op = "get_llm_model"
	return common.WithIterFunction(op, 0, 1, func(v any, args []any) gojq.Iter {
		var rawOpts map[string]any
		if len(args) == 1 {
			var err error
			if rawOpts, err = optionsArg(op, args[0]); err != nil {
				return gojq.NewIter(err)
			}
		}
		o := defaults()
		if err := bindOptions(op, rawOpts, &o); err != nil {
			return gojq.NewIter(err)
		}
		// Listing needs a provider and an endpoint but not a model, and the
		// natural way to ask is to name only the provider. A placeholder
		// stands in for the half of the address that is not being used, so
		// every other cmdlet's resolution still applies unchanged.
		// The environment fallback is applied here rather than left to
		// resolve, because the normalization below has to happen after it:
		// PWRQ_LLM_MODEL may name a full model, and listing only needs the
		// provider out of it.
		model := o.Model
		if model == "" {
			model = os.Getenv(EnvModel)
		}
		o.Model = listingModel(model)
		if o.Model == "" {
			return gojq.NewIter(fmt.Errorf("%s: no provider; pass {Model: \"anthropic\"} or set %s", op, EnvModel))
		}
		p, err := o.resolve(op)
		if err != nil {
			return gojq.NewIter(err)
		}
		if err := o.requireKey(op, p); err != nil {
			return gojq.NewIter(err)
		}

		models, err := listModels(context.Background(), op, o, p)
		if err != nil {
			return gojq.NewIter(err)
		}
		return common.SliceIter(models)
	})
}

// listingModel completes a provider-only address. "anthropic", "anthropic/"
// and "anthropic/claude-sonnet-4-5" all name the same endpoint to list.
func listingModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	provider, rest, found := strings.Cut(model, "/")
	if !found || rest == "" {
		return provider + "/-"
	}
	return model
}

func listModels(ctx context.Context, op string, o options, p provider) ([]any, error) {
	path := "/v1/models"
	if p.dialect == dialectOpenAI {
		path = "/models"
	}

	reqCtx, cancel := context.WithTimeout(ctx, o.timeout())
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, o.BaseUrl+path, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	switch p.dialect {
	case dialectAnthropic:
		req.Header.Set("x-api-key", o.ApiKey)
		req.Header.Set("anthropic-version", anthropicVersion)
	case dialectOpenAI:
		if o.ApiKey != "" {
			req.Header.Set("Authorization", "Bearer "+o.ApiKey)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: reading response: %w", op, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s returned %s: %s", op, p.name, resp.Status, apiErrorMessage(payload))
	}

	var decoded struct {
		Data []struct {
			ID          string `json:"id"`
			OwnedBy     string `json:"owned_by"`
			DisplayName string `json:"display_name"`
			Created     int64  `json:"created"`
			CreatedAt   string `json:"created_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("%s: decoding response: %w", op, err)
	}

	models := make([]any, 0, len(decoded.Data))
	for _, m := range decoded.Data {
		entry := map[string]any{
			// Model is the name spelled the way every other cmdlet wants it,
			// so `get_llm_model | .Model` feeds straight into {Model: ...}.
			"Model":    p.name + "/" + m.ID,
			"Id":       m.ID,
			"Provider": p.name,
			"OwnedBy":  nil,
			"Created":  nil,

			psobject.PSTypeNameKey: "Pwrq.LLM.Model",
		}
		if m.OwnedBy != "" {
			entry["OwnedBy"] = m.OwnedBy
		}
		if m.DisplayName != "" {
			entry["DisplayName"] = m.DisplayName
		}
		if m.CreatedAt != "" {
			entry["Created"] = m.CreatedAt
		} else if m.Created > 0 {
			entry["Created"] = m.Created
		}
		models = append(models, entry)
	}
	sort.Slice(models, func(i, j int) bool {
		a, _ := models[i].(map[string]any)["Id"].(string)
		b, _ := models[j].(map[string]any)["Id"].(string)
		return a < b
	})
	return models, nil
}
