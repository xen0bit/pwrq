package ideengine

import (
	"context"
	"encoding/json"
	"fmt"
)

// Call is the engine as the browser page sees it: a method name and a JSON
// request in, a JSON response out. Both browser hosts - the WASM worker and
// the native server - hand strings to this and nothing else, so the page has
// one protocol whichever engine answers it, and adding a capability here needs
// no change to either host.
//
// A reply is JSON in every case, because the page has no other channel: a
// reply it cannot parse is indistinguishable from a crash.
func (e *Engine) Call(ctx context.Context, method, request string) string {
	switch method {
	case "validate":
		return handle(request, e.Validate, func(err string) ValidateResponse { return ValidateResponse{Error: err} })
	case "run":
		return handle(request, func(req RunRequest) RunResponse { return e.Run(ctx, req) },
			func(err string) RunResponse { return RunResponse{Error: err, Kind: "request"} })
	case "diagram":
		return handle(request, e.Diagram, func(err string) DiagramResponse { return DiagramResponse{Error: err} })
	case "format":
		return handle(request, func(req FormatRequest) FormatResponse { return Format(req.Query) },
			func(err string) FormatResponse { return FormatResponse{Error: err} })
	case "minify":
		return handle(request, func(req FormatRequest) FormatResponse { return Minify(req.Query) },
			func(err string) FormatResponse { return FormatResponse{Error: err} })
	case "inline":
		return handle(request, func(req InlineRequest) InlineResponse { return Inline(req.Query) },
			func(err string) InlineResponse { return InlineResponse{Error: err} })
	case "catalog":
		return Marshal(e.Catalog())
	default:
		return Marshal(errorResponse{Error: fmt.Sprintf("unknown method %q", method)})
	}
}

// errorResponse is the shape a reply degrades to when there is no method to
// give it a better one.
type errorResponse struct {
	Error string `json:"error"`
}

// handle decodes a request, answers it, and encodes the answer. A request that
// cannot be read is answered in the method's own response shape, so the page
// finds the error where it looks for one.
func handle[Req, Resp any](request string, answer func(Req) Resp, malformed func(string) Resp) string {
	var req Req
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return Marshal(malformed("malformed request: " + err.Error()))
	}
	return Marshal(answer(req))
}

// Marshal renders a response, falling back to a hand-built error object so the
// page always receives JSON.
func Marshal(v any) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		return `{"error":"failed to encode response: ` + jsonEscape(err.Error()) + `"}`
	}
	return string(encoded)
}

func jsonEscape(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		return "encoding error"
	}
	return string(encoded[1 : len(encoded)-1])
}
