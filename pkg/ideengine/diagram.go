package ideengine

import (
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/graph"
)

// DiagramRequest asks for a query's flow diagram.
type DiagramRequest struct {
	Query     string `json:"query"`
	Theme     string `json:"theme"`
	Layout    string `json:"layout"`
	Direction string `json:"direction"`
	Sketch    bool   `json:"sketch"`
	// D2 asks for the script alongside the picture, which is what makes the
	// diagram editable elsewhere rather than a dead end.
	D2 bool `json:"d2"`
}

// DiagramResponse carries the rendered diagram.
type DiagramResponse struct {
	SVG    string `json:"svg,omitempty"`
	Script string `json:"script,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Diagram draws a query's structure, coloured by this engine's vocabulary so
// a cmdlet it can run is drawn as one.
//
// The script is always returned when the engine has no image renderer, since
// it is then the whole diagram; otherwise only when the request asks for it.
func (e *Engine) Diagram(req DiagramRequest) DiagramResponse {
	if strings.TrimSpace(req.Query) == "" {
		return DiagramResponse{Error: "query is empty"}
	}

	query, err := gojq.Parse(req.Query)
	if err != nil {
		return DiagramResponse{Error: err.Error()}
	}

	// The user's own query is drawn, not the alias-expanded one: the diagram
	// should show what was written.
	opts := graph.RenderOptions{
		Cmdlets:   e.cmdlets,
		Theme:     req.Theme,
		Layout:    req.Layout,
		Direction: req.Direction,
		Sketch:    req.Sketch,
	}

	resp := DiagramResponse{}
	if req.D2 || e.config.RenderSVG == nil {
		resp.Script = graph.RenderD2Opts(query, opts)
	}
	if e.config.RenderSVG == nil {
		return resp
	}
	svg, err := e.config.RenderSVG(query, opts)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	resp.SVG = svg
	return resp
}
