package main

import (
	"fmt"
	"syscall/js"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/graph"
)

func main() {
	// Expose functions to JavaScript
	js.Global().Set("validateQuery", js.FuncOf(validateQuery))
	js.Global().Set("createSVG", js.FuncOf(createSVG))

	// Keep the program running
	select {}
}

// validateQuery validates a jq query string
// Returns: {ok: boolean, err: string}
func validateQuery(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return map[string]interface{}{
			"ok":  false,
			"err": "validateQuery requires 1 argument: query string",
		}
	}

	queryStr := args[0].String()
	if queryStr == "" {
		return map[string]interface{}{
			"ok":  false,
			"err": "query string cannot be empty",
		}
	}

	_, err := gojq.Parse(queryStr)
	if err != nil {
		return map[string]interface{}{
			"ok":  false,
			"err": err.Error(),
		}
	}

	return map[string]interface{}{
		"ok":  true,
		"err": "",
	}
}

// createSVG creates an SVG from a jq query string
// Returns: {svg: string, err: string}
func createSVG(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return map[string]interface{}{
			"svg": "",
			"err": "createSVG requires 1 argument: query string",
		}
	}

	queryStr := args[0].String()
	if queryStr == "" {
		return map[string]interface{}{
			"svg": "",
			"err": "query string cannot be empty",
		}
	}

	// Parse the query
	query, err := gojq.Parse(queryStr)
	if err != nil {
		return map[string]interface{}{
			"svg": "",
			"err": fmt.Sprintf("failed to parse query: %v", err),
		}
	}

	// Generate SVG using the graph package
	svg, err := graph.GenerateSVG(query)
	if err != nil {
		return map[string]interface{}{
			"svg": "",
			"err": err.Error(),
		}
	}

	return map[string]interface{}{
		"svg": svg,
		"err": "",
	}
}
