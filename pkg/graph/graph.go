package graph

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2oracle"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2target"
	d2log "oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

// GenerateGraph creates a D2 diagram representing the flow of a jq query
func GenerateGraph(query *gojq.Query, outputPath string) error {
	// Resolve absolute output path
	outputPath, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("failed to resolve output path: %w", err)
	}

	// Create context with a logger to suppress D2 library warnings
	// Use a no-op logger to silence the warnings while still allowing D2 to function
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx = d2log.With(ctx, logger)

	// Start with an empty graph (following d2oracle pattern from blog post)
	_, graph, err := d2lib.Compile(ctx, "", nil, nil)
	if err != nil {
		return fmt.Errorf("failed to initialize graph: %w", err)
	}

	nodeCounter := 0
	lastNodeID := "start"
	var lastOutputType string
	boardPath := []string{} // Empty board path for root level

	// Create start node using d2oracle
	graph, startKey, err := d2oracle.Create(graph, boardPath, "start")
	if err != nil {
		return fmt.Errorf("failed to create start node: %w", err)
	}
	shapeCircle := "circle"
	labelStart := "Start"
	graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.shape", startKey), nil, &shapeCircle)
	if err != nil {
		return fmt.Errorf("failed to set start node shape: %w", err)
	}
	graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", startKey), nil, &labelStart)
	if err != nil {
		return fmt.Errorf("failed to set start node label: %w", err)
	}

	// Traverse the query AST and build graph programmatically
	lastOutputType, graph, err = traverseQueryWithOracle(query, graph, boardPath, &nodeCounter, &lastNodeID, "")
	if err != nil {
		return fmt.Errorf("failed to traverse query: %w", err)
	}

	// Add end node
	endNodeID := fmt.Sprintf("end_%d", nodeCounter)
	graph, endKey, err := d2oracle.Create(graph, boardPath, endNodeID)
	if err != nil {
		return fmt.Errorf("failed to create end node: %w", err)
	}
	labelEnd := "End"
	graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.shape", endKey), nil, &shapeCircle)
	if err != nil {
		return fmt.Errorf("failed to set end node shape: %w", err)
	}
	graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", endKey), nil, &labelEnd)
	if err != nil {
		return fmt.Errorf("failed to set end node label: %w", err)
	}

	// Connect last node to end with type
	if lastNodeID != "start" {
		edgeKey := fmt.Sprintf("%s -> %s", lastNodeID, endNodeID)
		graph, _, err = d2oracle.Create(graph, boardPath, edgeKey)
		if err != nil {
			return fmt.Errorf("failed to create end edge: %w", err)
		}
		if lastOutputType != "" {
			formattedType := formatEdgeLabel(lastOutputType)
			if formattedType != "" {
				graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", edgeKey), nil, &formattedType)
				if err != nil {
					return fmt.Errorf("failed to set end edge label: %w", err)
				}
			}
		}
	}

	// Format the graph AST to D2 script
	d2Script := d2format.Format(graph.AST)

	// Check output file extension
	ext := strings.ToLower(filepath.Ext(outputPath))

	switch ext {
	case ".d2":
		// Write plain D2 script text without directives to avoid creating nodes
		// Users can add directives manually if needed
		return os.WriteFile(outputPath, []byte(d2Script), 0644)

	case ".svg":
		// For SVG, prepend directives for layout direction
		// Theme will be set via RenderOpts to avoid creating a node
		// Layout is needed for compilation
		svgD2Script := "direction: right\nlayout: dagre\n" + d2Script

		// Render to SVG using the D2 library
		// Set up text measurement ruler for D2 compilation
		ruler, err := textmeasure.NewRuler()
		if err != nil {
			// Save D2 script for debugging
			d2OutputPath := outputPath[:len(outputPath)-len(ext)] + ".d2"
			os.WriteFile(d2OutputPath, []byte(svgD2Script), 0644)
			return fmt.Errorf("failed to create text ruler: %w\nD2 script saved to: %s", err, d2OutputPath)
		}

		// Compile the D2 script with layout and ruler (following blog post pattern)
		// Use ELK layout which supports container-to-descendant edges
		layoutStr := "dagre"
		compileOpts := &d2lib.CompileOptions{
			Layout: &layoutStr,
			Ruler:  ruler,
			LayoutResolver: func(engine string) (d2graph.LayoutGraph, error) {
				if engine == "elk" {
					return d2elklayout.DefaultLayout, nil
				}
				if engine == "dagre" {
					return d2dagrelayout.DefaultLayout, nil
				}
				return nil, fmt.Errorf("unknown layout engine: %s", engine)
			},
		}
		diagram, _, err := d2lib.Compile(ctx, svgD2Script, compileOpts, nil)
		if err != nil {
			// Save D2 script for debugging
			d2OutputPath := outputPath[:len(outputPath)-len(ext)] + ".d2"
			os.WriteFile(d2OutputPath, []byte(svgD2Script), 0644)
			return fmt.Errorf("failed to compile D2 diagram: %w\nD2 script saved to: %s", err, d2OutputPath)
		}

		// Remove directive nodes (theme, layout, layout.dir, direction) from the diagram
		// These are created when we add directives to the script, but we don't want them rendered
		if diagram != nil {
			// Filter out nodes named "theme", "layout", "layout.dir", and "direction" from top-level shapes
			var filteredShapes []d2target.Shape
			for _, shape := range diagram.Shapes {
				if shape.ID != "theme" && shape.ID != "layout" && shape.ID != "layout.dir" && shape.ID != "direction" {
					filteredShapes = append(filteredShapes, shape)
				}
			}
			diagram.Shapes = filteredShapes
		}

		// Render to SVG (following blog post pattern)
		// Use ThemeID for dark-mauve theme (theme ID 200 according to D2 docs)
		pad := int64(d2svg.DEFAULT_PADDING)
		themeID := int64(200) // dark-mauve theme
		svgBytes, err := d2svg.Render(diagram, &d2svg.RenderOpts{
			Pad:     &pad,
			ThemeID: &themeID,
		})
		if err != nil {
			// Save D2 script for debugging
			d2OutputPath := outputPath[:len(outputPath)-len(ext)] + ".d2"
			os.WriteFile(d2OutputPath, []byte(d2Script), 0644)
			return fmt.Errorf("failed to render D2 diagram to SVG: %w\nD2 script saved to: %s", err, d2OutputPath)
		}

		// Write SVG to file
		return os.WriteFile(outputPath, svgBytes, 0644)

	default:
		return fmt.Errorf("unsupported output format: %s (supported formats: .d2, .svg)", ext)
	}
}

// traverseQueryWithOracle recursively traverses the jq query AST and builds D2 nodes using d2oracle
// Returns the output type, updated graph, and error
func traverseQueryWithOracle(query *gojq.Query, graph *d2graph.Graph, boardPath []string, nodeCounter *int, lastNodeID *string, prevOutputType string) (string, *d2graph.Graph, error) {
	if query == nil {
		return "", graph, nil
	}
	if graph == nil {
		return "", nil, fmt.Errorf("graph is nil at start of traversal")
	}

	op := query.Op

	// Handle operators using switch
	switch op {
	case gojq.OpPipe:
		// Pipe operations: process left, then right (no pipe node created)
		return handlePipeOperation(query, graph, boardPath, nodeCounter, lastNodeID, prevOutputType)
	}

	// Handle term types using switch
	if query.Term != nil {
		switch query.Term.Type {
		case gojq.TermTypeQuery:
			// Unwrap query term and recurse
			if query.Term.Query != nil {
				return traverseQueryWithOracle(query.Term.Query, graph, boardPath, nodeCounter, lastNodeID, prevOutputType)
			}
		case gojq.TermTypeFunc:
			// Function calls create containers
			if query.Term.Func != nil {
				return traverseFunction(query, graph, boardPath, nodeCounter, lastNodeID, prevOutputType)
			}
		case gojq.TermTypeObject:
			// Object literals create containers with key containers
			if query.Term.Object != nil {
				return traverseObjectLiteral(query, graph, boardPath, nodeCounter, lastNodeID, prevOutputType)
			}
		case gojq.TermTypeArray:
			// Array literals - traverse the array query
			if query.Term.Array != nil && query.Term.Array.Query != nil {
				return traverseQueryWithOracle(query.Term.Array.Query, graph, boardPath, nodeCounter, lastNodeID, prevOutputType)
			}
		}
	}

	// For other operations, create a regular node
	return handleRegularNode(query, op, graph, boardPath, nodeCounter, lastNodeID, prevOutputType)
}

// handlePipeOperation processes pipe operations (no pipe node, just edges)
func handlePipeOperation(query *gojq.Query, graph *d2graph.Graph, boardPath []string, nodeCounter *int, lastNodeID *string, prevOutputType string) (string, *d2graph.Graph, error) {
	var leftType string
	var err error

	// Process left side
	if query.Left != nil {
		leftType, graph, err = traverseQueryWithOracle(query.Left, graph, boardPath, nodeCounter, lastNodeID, prevOutputType)
		if err != nil {
			return "", graph, err
		}
	}

	// Process right side with left's output as input
	if query.Right != nil {
		inputType := leftType
		if inputType == "" && query.Left != nil {
			inputType = inferOutputType(query.Left, query.Left.Op)
		}
		rightType, graph, err := traverseQueryWithOracle(query.Right, graph, boardPath, nodeCounter, lastNodeID, inputType)
		if err != nil {
			return "", graph, err
		}
		return rightType, graph, nil
	}

	return leftType, graph, nil
}

// handleRegularNode creates a regular node (non-container, non-pipe)
func handleRegularNode(query *gojq.Query, op gojq.Operator, graph *d2graph.Graph, boardPath []string, nodeCounter *int, lastNodeID *string, prevOutputType string) (string, *d2graph.Graph, error) {
	nodeID := fmt.Sprintf("node_%d", *nodeCounter)
	*nodeCounter++

	label := getNodeLabel(query, op)
	outputType := inferOutputType(query, op)

	// Create node
	var err error
	graph, _, err = d2oracle.Create(graph, boardPath, nodeID)
	if err != nil {
		return "", graph, fmt.Errorf("failed to create node %s: %w", nodeID, err)
	}

	// Set node properties
	shapeRect := "rectangle"
	graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.shape", nodeID), nil, &shapeRect)
	if err != nil {
		return "", graph, fmt.Errorf("failed to set node shape: %w", err)
	}
	formattedLabel := formatD2LabelForOracle(label)
	graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", nodeID), nil, &formattedLabel)
	if err != nil {
		return "", graph, fmt.Errorf("failed to set node label: %w", err)
	}

	// Connect from previous node
	if err := connectNodeFromPrevious(graph, boardPath, *lastNodeID, nodeID, prevOutputType); err != nil {
		return "", graph, err
	}

	*lastNodeID = nodeID

	// Process children recursively (if not a slice to avoid duplicates)
	if !strings.HasPrefix(label, "Slice ") {
		if query.Left != nil {
			leftType, graph, err := traverseQueryWithOracle(query.Left, graph, boardPath, nodeCounter, lastNodeID, prevOutputType)
			if err != nil {
				return "", graph, err
			}
			// Connect back if needed
			if *lastNodeID != nodeID {
				if err := connectNodeFromPrevious(graph, boardPath, *lastNodeID, nodeID, leftType); err != nil {
					return "", graph, err
				}
			}
		}
		if query.Right != nil {
			rightType, graph, err := traverseQueryWithOracle(query.Right, graph, boardPath, nodeCounter, lastNodeID, prevOutputType)
			if err != nil {
				return "", graph, err
			}
			// Connect back if needed
			if *lastNodeID != nodeID {
				if err := connectNodeFromPrevious(graph, boardPath, *lastNodeID, nodeID, rightType); err != nil {
					return "", graph, err
				}
			}
		}
	}

	return outputType, graph, nil
}

// connectNodeFromPrevious creates an edge from previous node (or start) to current node
func connectNodeFromPrevious(graph *d2graph.Graph, boardPath []string, lastNodeID, nodeID, edgeType string) error {
	var fromID string
	if lastNodeID == "start" {
		fromID = "start"
	} else {
		fromID = lastNodeID
	}

	edgeKey := fmt.Sprintf("%s -> %s", fromID, nodeID)
	var err error
	graph, _, err = d2oracle.Create(graph, boardPath, edgeKey)
	if err != nil {
		return fmt.Errorf("failed to create edge: %w", err)
	}

	if edgeType != "" && fromID != "start" {
		formattedType := formatEdgeLabel(edgeType)
		if formattedType != "" {
			graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", edgeKey), nil, &formattedType)
			if err != nil {
				return fmt.Errorf("failed to set edge label: %w", err)
			}
		}
	}
	return nil
}

// traverseFunction handles ALL function calls by creating a container and exploding the function's arguments
func traverseFunction(query *gojq.Query, graph *d2graph.Graph, boardPath []string, nodeCounter *int, lastNodeID *string, prevOutputType string) (string, *d2graph.Graph, error) {
	if query == nil || query.Term == nil || query.Term.Func == nil {
		return "", graph, fmt.Errorf("traverseFunction called on non-function")
	}

	funcName := query.Term.Func.Name
	if funcName == "" {
		return "", graph, fmt.Errorf("traverseFunction called on function with no name")
	}

	// Create a container node for the function
	funcNodeID := fmt.Sprintf("node_%d", *nodeCounter)
	*nodeCounter++

	var err error
	graph, _, err = d2oracle.Create(graph, boardPath, funcNodeID)
	if err != nil {
		return "", graph, fmt.Errorf("failed to create function container node %s: %w", funcNodeID, err)
	}

	// Set container properties - format function name with parentheses
	labelFunc := fmt.Sprintf("%s()", funcName)
	graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", funcNodeID), nil, &labelFunc)
	if err != nil {
		return "", graph, fmt.Errorf("failed to set function container label: %w", err)
	}

	// Connect from previous node
	if *lastNodeID != "start" {
		edgeKey := fmt.Sprintf("%s -> %s", *lastNodeID, funcNodeID)
		graph, _, err = d2oracle.Create(graph, boardPath, edgeKey)
		if err != nil {
			return "", graph, fmt.Errorf("failed to create edge to function container: %w", err)
		}
		if prevOutputType != "" {
			formattedType := formatEdgeLabel(prevOutputType)
			if formattedType != "" {
				graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", edgeKey), nil, &formattedType)
				if err != nil {
					return "", graph, fmt.Errorf("failed to set edge label: %w", err)
				}
			}
		}
	} else {
		// Connect from start node to the first function
		edgeKey := fmt.Sprintf("start -> %s", funcNodeID)
		graph, _, err = d2oracle.Create(graph, boardPath, edgeKey)
		if err != nil {
			return "", graph, fmt.Errorf("failed to create start edge: %w", err)
		}
	}

	// Traverse the function's arguments
	// Create child nodes inside the container using D2's dot notation (container.child)
	childCounter := 0
	// Start with "start" so we don't create an edge from container to first child
	childLastNodeID := "start"

	// Traverse all function arguments
	for i, arg := range query.Term.Func.Args {
		if arg != nil {
			// Traverse the argument, creating nodes inside the function container
			// This will recursively handle nested functions
			_, graph, err = traverseInContainer(arg, graph, boardPath, funcNodeID, &childCounter, &childLastNodeID, prevOutputType)
			if err != nil {
				return "", graph, fmt.Errorf("failed to traverse function argument %d: %w", i, err)
			}
		}
	}

	// The function container itself represents the output node
	*lastNodeID = funcNodeID

	// Infer output type for the function
	outputType := inferOutputType(query, query.Op)
	return outputType, graph, nil
}

// traverseObjectLiteral handles object literals by creating a container and traversing their values
func traverseObjectLiteral(query *gojq.Query, graph *d2graph.Graph, boardPath []string, nodeCounter *int, lastNodeID *string, prevOutputType string) (string, *d2graph.Graph, error) {
	if query == nil || query.Term == nil || query.Term.Object == nil {
		return "", graph, fmt.Errorf("traverseObjectLiteral called on non-object")
	}

	// Create a container node for the object
	objNodeID := fmt.Sprintf("node_%d", *nodeCounter)
	*nodeCounter++

	var err error
	graph, _, err = d2oracle.Create(graph, boardPath, objNodeID)
	if err != nil {
		return "", graph, fmt.Errorf("failed to create object container node %s: %w", objNodeID, err)
	}

	// Set container properties - use a label that shows it's an object
	labelObj := getTermLabel(query.Term, query)
	graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", objNodeID), nil, &labelObj)
	if err != nil {
		return "", graph, fmt.Errorf("failed to set object container label: %w", err)
	}

	// Connect from previous node
	if *lastNodeID != "start" {
		edgeKey := fmt.Sprintf("%s -> %s", *lastNodeID, objNodeID)
		graph, _, err = d2oracle.Create(graph, boardPath, edgeKey)
		if err != nil {
			return "", graph, fmt.Errorf("failed to create edge to object container: %w", err)
		}
		if prevOutputType != "" {
			formattedType := formatEdgeLabel(prevOutputType)
			if formattedType != "" {
				graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", edgeKey), nil, &formattedType)
				if err != nil {
					return "", graph, fmt.Errorf("failed to set edge label: %w", err)
				}
			}
		}
	} else {
		// Connect from start node to the first object
		edgeKey := fmt.Sprintf("start -> %s", objNodeID)
		graph, _, err = d2oracle.Create(graph, boardPath, edgeKey)
		if err != nil {
			return "", graph, fmt.Errorf("failed to create start edge: %w", err)
		}
	}

	// Traverse the object's key-value pairs
	// Each key-value pair gets its own container to show independence
	childCounter := 0

	for _, kv := range query.Term.Object.KeyVals {
		if kv.Val != nil {
			// Get the key name for the container label
			keyName := ""
			if kv.KeyQuery != nil {
				keyName = kv.KeyQuery.String()
				if len(keyName) > 20 {
					keyName = keyName[:17] + "..."
				}
			} else if kv.Key != "" {
				keyName = kv.Key
			}
			if keyName == "" {
				keyName = fmt.Sprintf("key_%d", childCounter)
			}

			// Create a container for this key-value pair
			keyContainerID := fmt.Sprintf("%s.child_%d", objNodeID, childCounter)
			childCounter++

			graph, _, err = d2oracle.Create(graph, boardPath, keyContainerID)
			if err != nil {
				return "", graph, fmt.Errorf("failed to create key container node: %w", err)
			}

			// Set container label to the key name
			graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", keyContainerID), nil, &keyName)
			if err != nil {
				return "", graph, fmt.Errorf("failed to set key container label: %w", err)
			}

			// Traverse the value query inside this key's container (independent of other keys)
			keyChildCounter := 0
			keyLastNodeID := "start"
			_, graph, err = traverseInContainer(kv.Val, graph, boardPath, keyContainerID, &keyChildCounter, &keyLastNodeID, prevOutputType)
			if err != nil {
				return "", graph, fmt.Errorf("failed to traverse object value: %w", err)
			}
		}
	}

	// The object container itself represents the output node
	*lastNodeID = objNodeID

	// Infer output type for the object
	outputType := inferOutputType(query, query.Op)
	return outputType, graph, nil
}

// traverseObjectLiteralInContainer handles object literals inside a container
func traverseObjectLiteralInContainer(query *gojq.Query, graph *d2graph.Graph, boardPath []string, containerID string, childCounter *int, lastNodeID *string, prevOutputType string) (string, *d2graph.Graph, error) {
	if query == nil || query.Term == nil || query.Term.Object == nil {
		return "", graph, fmt.Errorf("traverseObjectLiteralInContainer called on non-object")
	}

	// Create a nested container node for the object inside the parent container
	objNodeID := fmt.Sprintf("%s.child_%d", containerID, *childCounter)
	*childCounter++

	var err error
	graph, _, err = d2oracle.Create(graph, boardPath, objNodeID)
	if err != nil {
		return "", graph, fmt.Errorf("failed to create nested object container node: %w", err)
	}

	// Set container properties
	labelObj := getTermLabel(query.Term, query)
	graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", objNodeID), nil, &labelObj)
	if err != nil {
		return "", graph, fmt.Errorf("failed to set nested object container label: %w", err)
	}

	// Connect from previous node (but not from container - containment is sufficient)
	if *lastNodeID != "start" && *lastNodeID != containerID {
		edgeKey := fmt.Sprintf("%s -> %s", *lastNodeID, objNodeID)
		graph, _, err = d2oracle.Create(graph, boardPath, edgeKey)
		if err != nil {
			return "", graph, fmt.Errorf("failed to create edge to nested object container: %w", err)
		}
		if prevOutputType != "" {
			formattedType := formatEdgeLabel(prevOutputType)
			if formattedType != "" {
				graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", edgeKey), nil, &formattedType)
				if err != nil {
					return "", graph, fmt.Errorf("failed to set edge label: %w", err)
				}
			}
		}
	}

	// Traverse the object's key-value pairs
	// Each key-value pair gets its own container to show independence
	nestedChildCounter := 0

	for _, kv := range query.Term.Object.KeyVals {
		if kv.Val != nil {
			// Get the key name for the container label
			keyName := ""
			if kv.KeyQuery != nil {
				keyName = kv.KeyQuery.String()
				if len(keyName) > 20 {
					keyName = keyName[:17] + "..."
				}
			} else if kv.Key != "" {
				keyName = kv.Key
			}
			if keyName == "" {
				keyName = fmt.Sprintf("key_%d", nestedChildCounter)
			}

			// Create a container for this key-value pair
			keyContainerID := fmt.Sprintf("%s.child_%d", objNodeID, nestedChildCounter)
			nestedChildCounter++

			graph, _, err = d2oracle.Create(graph, boardPath, keyContainerID)
			if err != nil {
				return "", graph, fmt.Errorf("failed to create nested key container node: %w", err)
			}

			// Set container label to the key name
			graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", keyContainerID), nil, &keyName)
			if err != nil {
				return "", graph, fmt.Errorf("failed to set nested key container label: %w", err)
			}

			// Traverse the value query inside this key's container (independent of other keys)
			keyChildCounter := 0
			keyLastNodeID := "start"
			_, graph, err = traverseInContainer(kv.Val, graph, boardPath, keyContainerID, &keyChildCounter, &keyLastNodeID, prevOutputType)
			if err != nil {
				return "", graph, fmt.Errorf("failed to traverse nested object value: %w", err)
			}
		}
	}

	// Update lastNodeID to point to the nested object container
	*lastNodeID = objNodeID

	// Infer output type for the object
	outputType := inferOutputType(query, query.Op)
	return outputType, graph, nil
}

// traverseInContainer traverses a query and creates nodes inside a container using dot notation
// It creates nodes with IDs like "containerID.child_0", "containerID.child_1", etc.
// This handles nested functions recursively - if a child is a function, it creates a nested container
func traverseInContainer(query *gojq.Query, graph *d2graph.Graph, boardPath []string, containerID string, childCounter *int, lastNodeID *string, prevOutputType string) (string, *d2graph.Graph, error) {
	if query == nil {
		return "", graph, nil
	}

	op := query.Op

	// Handle pipe operations using switch
	pipeQuery := findPipeQuery(query, op)
	if pipeQuery != nil {
		return handlePipeInContainer(pipeQuery, graph, boardPath, containerID, childCounter, lastNodeID, prevOutputType)
	}

	// Handle term types using switch
	if query.Term != nil {
		switch query.Term.Type {
		case gojq.TermTypeQuery:
			// Unwrap query term and recurse
			if query.Term.Query != nil {
				return traverseInContainer(query.Term.Query, graph, boardPath, containerID, childCounter, lastNodeID, prevOutputType)
			}
		case gojq.TermTypeObject:
			// Object literals create containers with key containers
			if query.Term.Object != nil {
				return traverseObjectLiteralInContainer(query, graph, boardPath, containerID, childCounter, lastNodeID, prevOutputType)
			}
		case gojq.TermTypeFunc:
			// Function calls create nested containers
			if query.Term.Func != nil {
				return handleFunctionInContainer(query, graph, boardPath, containerID, childCounter, lastNodeID, prevOutputType)
			}
		}
	}

	// For other operations, create a regular child node
	return handleRegularNodeInContainer(query, op, graph, boardPath, containerID, childCounter, lastNodeID, prevOutputType)
}

// Helper functions for container traversal

// findPipeQuery finds a pipe query in the query tree
func findPipeQuery(query *gojq.Query, op gojq.Operator) *gojq.Query {
	if op == gojq.OpPipe {
		return query
	}
	if query.Left != nil && query.Left.Op == gojq.OpPipe {
		return query.Left
	}
	if query.Right != nil && query.Right.Op == gojq.OpPipe {
		return query.Right
	}
	return nil
}

// handlePipeInContainer processes pipe operations inside containers
func handlePipeInContainer(pipeQuery *gojq.Query, graph *d2graph.Graph, boardPath []string, containerID string, childCounter *int, lastNodeID *string, prevOutputType string) (string, *d2graph.Graph, error) {
	var leftType string
	var err error

	if pipeQuery.Left != nil {
		leftType, graph, err = traverseInContainer(pipeQuery.Left, graph, boardPath, containerID, childCounter, lastNodeID, prevOutputType)
		if err != nil {
			return "", graph, err
		}
	}

	if pipeQuery.Right != nil {
		inputType := leftType
		if inputType == "" && pipeQuery.Left != nil {
			inputType = inferOutputType(pipeQuery.Left, pipeQuery.Left.Op)
		}
		rightType, graph, err := traverseInContainer(pipeQuery.Right, graph, boardPath, containerID, childCounter, lastNodeID, inputType)
		if err != nil {
			return "", graph, err
		}
		return rightType, graph, nil
	}

	return leftType, graph, nil
}

// handleFunctionInContainer processes function calls inside containers
func handleFunctionInContainer(query *gojq.Query, graph *d2graph.Graph, boardPath []string, containerID string, childCounter *int, lastNodeID *string, prevOutputType string) (string, *d2graph.Graph, error) {
	funcName := query.Term.Func.Name
	if funcName == "" {
		return "", graph, fmt.Errorf("function has no name")
	}

	// Create nested function container
	nestedFuncNodeID := fmt.Sprintf("%s.child_%d", containerID, *childCounter)
	*childCounter++

	var err error
	graph, _, err = d2oracle.Create(graph, boardPath, nestedFuncNodeID)
	if err != nil {
		return "", graph, fmt.Errorf("failed to create nested function container: %w", err)
	}

	labelFunc := fmt.Sprintf("%s()", funcName)
	graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", nestedFuncNodeID), nil, &labelFunc)
	if err != nil {
		return "", graph, fmt.Errorf("failed to set nested function container label: %w", err)
	}

	// Connect from previous (but not from container itself)
	if *lastNodeID != "start" && *lastNodeID != containerID {
		edgeKey := fmt.Sprintf("%s -> %s", *lastNodeID, nestedFuncNodeID)
		graph, _, err = d2oracle.Create(graph, boardPath, edgeKey)
		if err != nil {
			return "", graph, fmt.Errorf("failed to create edge to nested function: %w", err)
		}
		if prevOutputType != "" {
			formattedType := formatEdgeLabel(prevOutputType)
			if formattedType != "" {
				graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", edgeKey), nil, &formattedType)
				if err != nil {
					return "", graph, fmt.Errorf("failed to set edge label: %w", err)
				}
			}
		}
	}

	// Traverse function arguments
	nestedChildCounter := 0
	nestedLastNodeID := "start"
	for i, arg := range query.Term.Func.Args {
		if arg != nil {
			_, graph, err = traverseInContainer(arg, graph, boardPath, nestedFuncNodeID, &nestedChildCounter, &nestedLastNodeID, prevOutputType)
			if err != nil {
				return "", graph, fmt.Errorf("failed to traverse nested function argument %d: %w", i, err)
			}
		}
	}

	*lastNodeID = nestedFuncNodeID
	outputType := inferOutputType(query, query.Op)
	return outputType, graph, nil
}

// handleRegularNodeInContainer creates a regular node inside a container
func handleRegularNodeInContainer(query *gojq.Query, op gojq.Operator, graph *d2graph.Graph, boardPath []string, containerID string, childCounter *int, lastNodeID *string, prevOutputType string) (string, *d2graph.Graph, error) {
	childNodeID := fmt.Sprintf("%s.child_%d", containerID, *childCounter)
	*childCounter++

	label := getNodeLabel(query, op)
	outputType := inferOutputType(query, op)

	// Create node
	var err error
	graph, _, err = d2oracle.Create(graph, boardPath, childNodeID)
	if err != nil {
		return "", graph, fmt.Errorf("failed to create child node: %w", err)
	}

	// Set node properties
	shapeRect := "rectangle"
	graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.shape", childNodeID), nil, &shapeRect)
	if err != nil {
		return "", graph, fmt.Errorf("failed to set child node shape: %w", err)
	}
	formattedLabel := formatD2LabelForOracle(label)
	graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", childNodeID), nil, &formattedLabel)
	if err != nil {
		return "", graph, fmt.Errorf("failed to set child node label: %w", err)
	}

	// Connect from previous (but not from container itself)
	if *lastNodeID != "start" && *lastNodeID != containerID {
		edgeKey := fmt.Sprintf("%s -> %s", *lastNodeID, childNodeID)
		graph, _, err = d2oracle.Create(graph, boardPath, edgeKey)
		if err != nil {
			return "", graph, fmt.Errorf("failed to create child edge: %w", err)
		}
		if prevOutputType != "" {
			formattedType := formatEdgeLabel(prevOutputType)
			if formattedType != "" {
				graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", edgeKey), nil, &formattedType)
				if err != nil {
					return "", graph, fmt.Errorf("failed to set child edge label: %w", err)
				}
			}
		}
	}

	*lastNodeID = childNodeID

	// Process children recursively (if not a slice)
	if !strings.HasPrefix(label, "Slice ") {
		if query.Left != nil {
			leftType, graph, err := traverseInContainer(query.Left, graph, boardPath, containerID, childCounter, lastNodeID, prevOutputType)
			if err != nil {
				return "", graph, err
			}
			if *lastNodeID != childNodeID {
				edgeKey := fmt.Sprintf("%s -> %s", *lastNodeID, childNodeID)
				graph, _, err = d2oracle.Create(graph, boardPath, edgeKey)
				if err != nil {
					return "", graph, fmt.Errorf("failed to create left branch edge: %w", err)
				}
				if leftType != "" {
					formattedType := formatEdgeLabel(leftType)
					if formattedType != "" {
						graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", edgeKey), nil, &formattedType)
						if err != nil {
							return "", graph, fmt.Errorf("failed to set left branch edge label: %w", err)
						}
					}
				}
			}
		}
		if query.Right != nil {
			rightType, graph, err := traverseInContainer(query.Right, graph, boardPath, containerID, childCounter, lastNodeID, prevOutputType)
			if err != nil {
				return "", graph, err
			}
			if *lastNodeID != childNodeID {
				edgeKey := fmt.Sprintf("%s -> %s", *lastNodeID, childNodeID)
				graph, _, err = d2oracle.Create(graph, boardPath, edgeKey)
				if err != nil {
					return "", graph, fmt.Errorf("failed to create right branch edge: %w", err)
				}
				if rightType != "" {
					formattedType := formatEdgeLabel(rightType)
					if formattedType != "" {
						graph, err = d2oracle.Set(graph, boardPath, fmt.Sprintf("%s.label", edgeKey), nil, &formattedType)
						if err != nil {
							return "", graph, fmt.Errorf("failed to set right branch edge label: %w", err)
						}
					}
				}
			}
		}
	}

	return outputType, graph, nil
}

// formatD2LabelForOracle formats a label for use with d2oracle.Set
func formatD2LabelForOracle(label string) string {
	// Replace $ with _VAR_ to avoid D2 variable substitution
	safeLabel := strings.ReplaceAll(label, "$", "_VAR_")
	// Don't remove quotes - they're part of the string representation
	// The quotes are needed for proper display of string literals
	return safeLabel
}

// formatEdgeLabel formats a label for use on edges, avoiding reserved keywords
func formatEdgeLabel(label string) string {
	// D2 has reserved keywords that can't be used in edge labels
	// Common ones: array, object, string, number, boolean, null, true, false
	// If the label is a reserved keyword, return empty string to skip the label
	reservedKeywords := map[string]bool{
		"array": true, "object": true, "string": true, "number": true,
		"boolean": true, "bool": true, "null": true, "true": true, "false": true,
	}

	// Remove quotes if present
	cleanLabel := strings.Trim(label, "\"")
	cleanLabel = strings.TrimSpace(cleanLabel)

	// Check if it's a reserved keyword (case-insensitive)
	if reservedKeywords[strings.ToLower(cleanLabel)] {
		// Return empty string to skip setting the label for reserved keywords
		// This avoids D2 compilation errors
		return ""
	}

	return cleanLabel
}

// formatIndexBound formats an index bound (start or end) for display
func formatIndexBound(query *gojq.Query) string {
	if query == nil {
		return ""
	}
	// Try to extract a simple numeric value
	if query.Term != nil && query.Term.Type == gojq.TermTypeNumber {
		if query.Term.Number != "" {
			return query.Term.Number
		}
	}
	// Fallback to string representation
	return query.String()
}

// getTermBaseLabel gets the base label for a term without suffixes
func getTermBaseLabel(term *gojq.Term) string {
	if term == nil {
		return ""
	}
	switch term.Type {
	case gojq.TermTypeIdentity:
		return "."
	case gojq.TermTypeRecurse:
		return ".."
	case gojq.TermTypeNull:
		return "null"
	case gojq.TermTypeTrue:
		return "true"
	case gojq.TermTypeFalse:
		return "false"
	case gojq.TermTypeNumber:
		if term.Number != "" {
			return term.Number
		}
		return "number"
	case gojq.TermTypeString:
		if term.Str != nil {
			return fmt.Sprintf("%q", term.Str.Str)
		}
		return "string"
	default:
		return ""
	}
}

// formatSuffixList formats a list of suffixes (like multiple index operations)
func formatSuffixList(suffixes []*gojq.Suffix) string {
	var parts []string
	for _, suffix := range suffixes {
		if suffix.Index != nil {
			if suffix.Index.IsSlice {
				start := formatIndexBound(suffix.Index.Start)
				end := formatIndexBound(suffix.Index.End)
				if start == "" && end == "" {
					parts = append(parts, "[:]")
				} else if start == "" {
					parts = append(parts, fmt.Sprintf("[:%s]", end))
				} else if end == "" {
					parts = append(parts, fmt.Sprintf("[%s:]", start))
				} else {
					parts = append(parts, fmt.Sprintf("[%s:%s]", start, end))
				}
			} else if suffix.Index.Name != "" {
				parts = append(parts, fmt.Sprintf(".%s", suffix.Index.Name))
			} else if suffix.Index.Str != nil {
				parts = append(parts, fmt.Sprintf("[%q]", suffix.Index.Str.Str))
			}
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "")
	}
	return ""
}

// getOperationLabel returns a human-readable label for a gojq operation
func getOperationLabel(op gojq.Operator) string {
	switch op {
	case gojq.OpPipe:
		return "Pipe (|)"
	case gojq.OpComma:
		return "Comma (,)"
	case gojq.OpAdd:
		return "Add (+)"
	case gojq.OpSub:
		return "Subtract (-)"
	case gojq.OpMul:
		return "Multiply (*)"
	case gojq.OpDiv:
		return "Divide (/)"
	case gojq.OpMod:
		return "Modulo (%)"
	case gojq.OpEq:
		return "Equal (==)"
	case gojq.OpNe:
		return "Not Equal (!=)"
	case gojq.OpGt:
		return "Greater Than (>)"
	case gojq.OpLt:
		return "Less Than (<)"
	case gojq.OpGe:
		return "Greater or Equal (>=)"
	case gojq.OpLe:
		return "Less or Equal (<=)"
	case gojq.OpAnd:
		return "And (and)"
	case gojq.OpOr:
		return "Or (or)"
	case gojq.OpAlt:
		return "Alternative (//)"
	case gojq.OpAssign:
		return "Assign (=)"
	case gojq.OpModify:
		return "Modify (|=)"
	case gojq.OpUpdateAdd:
		return "Update Add (+=)"
	case gojq.OpUpdateSub:
		return "Update Subtract (-=)"
	case gojq.OpUpdateMul:
		return "Update Multiply (*=)"
	case gojq.OpUpdateDiv:
		return "Update Divide (/=)"
	case gojq.OpUpdateMod:
		return "Update Modulo (%=)"
	case gojq.OpUpdateAlt:
		return "Update Alternative (//=)"
	default:
		if op == 0 {
			// Op 0 means no operator - this is often a query wrapper
			// The actual operation is in the term, so return empty
			// to let getNodeLabel handle it via the term
			return ""
		}
		return fmt.Sprintf("Op(%d)", op)
	}
}

// getNodeLabel returns a label for a query node, combining operator and term info
func getNodeLabel(query *gojq.Query, op gojq.Operator) string {
	// For pipe operations, always return "Pipe (|)" - don't check terms or Left/Right
	// The slice will be detected when we process the right side of the pipe
	if op == gojq.OpPipe {
		return "Pipe (|)"
	}

	// If there's a term, use it for the label
	if query.Term != nil {
		termLabel := getTermLabel(query.Term, query)
		if termLabel != "" {
			// If we got a label from the term (including slices), return it immediately
			// Don't check Left/Right to avoid duplicates
			return termLabel
		}
	}

	// Check if this is an index operation on the query itself (like .[0:3])
	// This happens when the query has no term but has index operations in Left
	// Only check Left/Right if we didn't already get a label from the term
	// NOTE: This should rarely be needed since slices are usually in SuffixList or Term.Index
	// We only check this as a fallback when query.Term is nil
	if query.Term == nil && query.Left != nil {
		if query.Left.Term != nil {
			// Check for suffixes on the left term
			if len(query.Left.Term.SuffixList) > 0 {
				// Check if any suffix is a slice
				hasSlice := false
				for _, suffix := range query.Left.Term.SuffixList {
					if suffix.Index != nil && suffix.Index.IsSlice {
						hasSlice = true
						break
					}
				}
				suffixLabel := formatSuffixList(query.Left.Term.SuffixList)
				if suffixLabel != "" {
					if hasSlice {
						// For slices, return "Slice [0:3]" format
						return "Slice " + suffixLabel
					}
					// Combine with base term label for non-slice suffixes
					baseLabel := getTermBaseLabel(query.Left.Term)
					if baseLabel != "" {
						return baseLabel + suffixLabel
					}
					return suffixLabel
				}
			}
			// Also check if the left term itself is an index with slice
			if query.Left.Term.Type == gojq.TermTypeIndex && query.Left.Term.Index != nil {
				if query.Left.Term.Index.IsSlice {
					start := formatIndexBound(query.Left.Term.Index.Start)
					end := formatIndexBound(query.Left.Term.Index.End)
					if start == "" && end == "" {
						return "Slice [:]"
					} else if start == "" {
						return fmt.Sprintf("Slice [:%s]", end)
					} else if end == "" {
						return fmt.Sprintf("Slice [%s:]", start)
					} else {
						return fmt.Sprintf("Slice [%s:%s]", start, end)
					}
				}
			}
		}
		// Also check Right side for index operations
		if query.Right != nil && query.Right.Term != nil {
			if query.Right.Term.Type == gojq.TermTypeIndex && query.Right.Term.Index != nil {
				if query.Right.Term.Index.IsSlice {
					start := formatIndexBound(query.Right.Term.Index.Start)
					end := formatIndexBound(query.Right.Term.Index.End)
					if start == "" && end == "" {
						return "Slice [:]"
					} else if start == "" {
						return fmt.Sprintf("Slice [:%s]", end)
					} else if end == "" {
						return fmt.Sprintf("Slice [%s:]", start)
					} else {
						return fmt.Sprintf("Slice [%s:%s]", start, end)
					}
				}
			}
			if len(query.Right.Term.SuffixList) > 0 {
				// Check if any suffix is a slice
				hasSlice := false
				for _, suffix := range query.Right.Term.SuffixList {
					if suffix.Index != nil && suffix.Index.IsSlice {
						hasSlice = true
						break
					}
				}
				suffixLabel := formatSuffixList(query.Right.Term.SuffixList)
				if suffixLabel != "" {
					if hasSlice {
						// For slices, return "Slice [0:3]" format
						return "Slice " + suffixLabel
					}
					baseLabel := getTermBaseLabel(query.Right.Term)
					if baseLabel != "" {
						return baseLabel + suffixLabel
					}
					return suffixLabel
				}
			}
		}
	}

	// Don't use string representation fallback for slices - it causes duplicates
	// Only detect slices from the actual AST structure above

	// Otherwise use the operator label (or empty if op is 0)
	opLabel := getOperationLabel(op)
	if opLabel == "" {
		// If no operator label, try query string as last resort (but avoid slices)
		queryStr := query.String()
		if queryStr != "" && !strings.Contains(queryStr, "[") {
			// Only use query string if it doesn't contain brackets (to avoid slice detection)
			if len(queryStr) > 50 {
				return queryStr[:47] + "..."
			}
			return queryStr
		}
	}
	return opLabel
}

// extractSlicePattern tries to extract slice notation from a string
func extractSlicePattern(s string) string {
	// Look for patterns like [0:3], [:3], [0:], [:]
	start := strings.Index(s, "[")
	if start == -1 {
		return ""
	}
	end := strings.Index(s[start:], "]")
	if end == -1 {
		return ""
	}
	end += start
	slicePart := s[start : end+1]
	// Check if it contains a colon (indicating a slice)
	if strings.Contains(slicePart, ":") {
		return slicePart
	}
	return ""
}

// getTermLabel extracts a label from a Term, including function arguments
func getTermLabel(term *gojq.Term, query *gojq.Query) string {
	if term == nil {
		return ""
	}

	// Check for suffixes first (like .[0:3] where . is identity and [0:3] is a suffix)
	// But skip if the term itself is already an Index type (to avoid duplicate slice detection)
	if len(term.SuffixList) > 0 && term.Type != gojq.TermTypeIndex {
		suffixLabel := formatSuffixList(term.SuffixList)
		if suffixLabel != "" {
			// Check if the suffix contains a slice - if so, format it with "Slice " prefix
			hasSlice := false
			for _, suffix := range term.SuffixList {
				if suffix.Index != nil && suffix.Index.IsSlice {
					hasSlice = true
					break
				}
			}
			if hasSlice {
				// For slices, return "Slice [0:3]" format (without the "." prefix)
				return "Slice " + suffixLabel
			}
			// Combine term label with suffix for non-slice suffixes
			termBase := ""
			switch term.Type {
			case gojq.TermTypeIdentity:
				termBase = "."
			case gojq.TermTypeRecurse:
				termBase = ".."
			default:
				// For other types, get the base label
				termBase = getTermBaseLabel(term)
			}
			if termBase != "" {
				return termBase + suffixLabel
			}
			return suffixLabel
		}
	}

	switch term.Type {
	case gojq.TermTypeIdentity:
		return "Identity (.)"
	case gojq.TermTypeRecurse:
		return "Recurse (..)"
	case gojq.TermTypeNull:
		return "Null"
	case gojq.TermTypeTrue:
		return "True"
	case gojq.TermTypeFalse:
		return "False"
	case gojq.TermTypeIndex:
		if term.Index != nil {
			// Handle slice operations like [0:3]
			if term.Index.IsSlice {
				start := formatIndexBound(term.Index.Start)
				end := formatIndexBound(term.Index.End)
				sliceLabel := ""
				if start == "" && end == "" {
					sliceLabel = "[:]"
				} else if start == "" {
					sliceLabel = fmt.Sprintf("[:%s]", end)
				} else if end == "" {
					sliceLabel = fmt.Sprintf("[%s:]", start)
				} else {
					sliceLabel = fmt.Sprintf("[%s:%s]", start, end)
				}
				// Don't add additional suffixes to slice labels - they're already part of the slice
				// This prevents duplicate slice detection
				return "Slice " + sliceLabel
			}
			// Handle object indexing
			if term.Index.Name != "" {
				indexLabel := fmt.Sprintf(".%s", term.Index.Name)
				// Check for additional suffixes
				if len(term.SuffixList) > 0 {
					return indexLabel + formatSuffixList(term.SuffixList)
				}
				return indexLabel
			}
			// Handle string indexing
			if term.Index.Str != nil {
				indexLabel := fmt.Sprintf("[%q]", term.Index.Str.Str)
				// Check for additional suffixes
				if len(term.SuffixList) > 0 {
					return indexLabel + formatSuffixList(term.SuffixList)
				}
				return indexLabel
			}
		}
		// Even if Index is nil, check for suffixes (slices can be in suffixes)
		if len(term.SuffixList) > 0 {
			suffixLabel := formatSuffixList(term.SuffixList)
			if suffixLabel != "" {
				return suffixLabel
			}
		}
		return "Index"
	case gojq.TermTypeFunc:
		if term.Func != nil {
			// Format function with arguments
			funcName := term.Func.Name
			if len(term.Func.Args) > 0 {
				args := formatFuncArgs(term.Func.Args)
				return fmt.Sprintf("%s(%s)", funcName, args)
			}
			return funcName
		}
		return "Function"
	case gojq.TermTypeArray:
		if term.Array != nil && term.Array.Query != nil {
			// Check if array contains function calls
			q := term.Array.Query
			if q.Term != nil && q.Term.Type == gojq.TermTypeFunc {
				if q.Term.Func != nil {
					funcName := q.Term.Func.Name
					if len(q.Term.Func.Args) > 0 {
						args := formatFuncArgs(q.Term.Func.Args)
						return fmt.Sprintf("[%s(%s)]", funcName, args)
					}
					return fmt.Sprintf("[%s()]", funcName)
				}
			}
			// Also check if the query itself is a pipe or other operation containing a function
			// This handles cases like [find(...) | something]
			if q.Term == nil && (q.Left != nil || q.Right != nil) {
				// Try to find a function in the query structure
				if q.Left != nil && q.Left.Term != nil && q.Left.Term.Type == gojq.TermTypeFunc {
					if q.Left.Term.Func != nil {
						funcName := q.Left.Term.Func.Name
						if len(q.Left.Term.Func.Args) > 0 {
							args := formatFuncArgs(q.Left.Term.Func.Args)
							return fmt.Sprintf("[%s(%s)]", funcName, args)
						}
						return fmt.Sprintf("[%s()]", funcName)
					}
				}
			}
		}
		return "Array"
	case gojq.TermTypeObject:
		if term.Object != nil && len(term.Object.KeyVals) > 0 {
			// Check if object contains function calls in values
			var keyValLabels []string
			for _, kv := range term.Object.KeyVals {
				if kv.Val != nil {
					// Use helper function to find function in query (handles pipes, parentheses, etc.)
					funcName, funcArgs := findFuncInQuery(kv.Val)

					if funcName != "" {
						keyName := ""
						// Key can be a string or a query
						if kv.KeyQuery != nil {
							// Key might be a query
							keyName = kv.KeyQuery.String()
							if len(keyName) > 20 {
								keyName = keyName[:17] + "..."
							}
						} else if kv.Key != "" {
							keyName = kv.Key
						}
						if len(funcArgs) > 0 {
							args := formatFuncArgs(funcArgs)
							if keyName != "" {
								keyValLabels = append(keyValLabels, fmt.Sprintf("%s: %s(%s)", keyName, funcName, args))
							} else {
								keyValLabels = append(keyValLabels, fmt.Sprintf("%s(%s)", funcName, args))
							}
						} else {
							if keyName != "" {
								keyValLabels = append(keyValLabels, fmt.Sprintf("%s: %s()", keyName, funcName))
							} else {
								keyValLabels = append(keyValLabels, fmt.Sprintf("%s()", funcName))
							}
						}
					}
				}
			}
			if len(keyValLabels) > 0 {
				// If we have function labels, show them
				if len(keyValLabels) == 1 {
					return fmt.Sprintf("{%s}", keyValLabels[0])
				}
				return fmt.Sprintf("{%s, ...}", keyValLabels[0])
			}
		}
		return "Object"
	case gojq.TermTypeNumber:
		if term.Number != "" {
			return fmt.Sprintf("Number: %s", term.Number)
		}
		return "Number"
	case gojq.TermTypeUnary:
		if term.Unary != nil {
			return fmt.Sprintf("Unary: %s", term.Unary.Op)
		}
		return "Unary"
	case gojq.TermTypeFormat:
		return fmt.Sprintf("Format: %s", term.Format)
	case gojq.TermTypeString:
		if term.Str != nil {
			return fmt.Sprintf("String: %q", term.Str.Str)
		}
		return "String"
	case gojq.TermTypeIf:
		return "If"
	case gojq.TermTypeTry:
		return "Try"
	case gojq.TermTypeReduce:
		return "Reduce"
	case gojq.TermTypeForeach:
		return "Foreach"
	case gojq.TermTypeLabel:
		return "Label"
	case gojq.TermTypeBreak:
		return "Break"
	case gojq.TermTypeQuery:
		return "Query"
	default:
		return fmt.Sprintf("Term(%d)", term.Type)
	}
}

// findFuncInQuery recursively searches a query for a function call
// Returns the function name and args if found, empty string and nil otherwise
func findFuncInQuery(q *gojq.Query) (string, []*gojq.Query) {
	if q == nil {
		return "", nil
	}
	// Check direct function call
	if q.Term != nil && q.Term.Type == gojq.TermTypeFunc {
		if q.Term.Func != nil {
			return q.Term.Func.Name, q.Term.Func.Args
		}
	}
	// Check pipe operations - function is usually on the left
	if q.Op == gojq.OpPipe && q.Left != nil {
		if name, args := findFuncInQuery(q.Left); name != "" {
			return name, args
		}
	}
	// Check Left side (for parentheses or other wrappers)
	if q.Left != nil {
		if name, args := findFuncInQuery(q.Left); name != "" {
			return name, args
		}
	}
	// Check Right side (less common but possible)
	if q.Right != nil {
		if name, args := findFuncInQuery(q.Right); name != "" {
			return name, args
		}
	}
	return "", nil
}

// formatFuncArgs formats function arguments as a string
func formatFuncArgs(args []*gojq.Query) string {
	if len(args) == 0 {
		return ""
	}

	var parts []string
	for _, arg := range args {
		parts = append(parts, formatQueryArg(arg))
	}
	return strings.Join(parts, ", ")
}

// formatQueryArg formats a single query argument as a string
func formatQueryArg(query *gojq.Query) string {
	if query == nil {
		return ""
	}

	// Try to extract a simple value from the query
	if query.Term != nil {
		switch query.Term.Type {
		case gojq.TermTypeString:
			if query.Term.Str != nil {
				return fmt.Sprintf("%q", query.Term.Str.Str)
			}
		case gojq.TermTypeNumber:
			if query.Term.Number != "" {
				return query.Term.Number
			}
		case gojq.TermTypeTrue:
			return "true"
		case gojq.TermTypeFalse:
			return "false"
		case gojq.TermTypeNull:
			return "null"
		case gojq.TermTypeIdentity:
			return "."
		case gojq.TermTypeIndex:
			if query.Term.Index != nil {
				if query.Term.Index.Name != "" {
					return query.Term.Index.Name
				}
				if query.Term.Index.Str != nil {
					return fmt.Sprintf("%q", query.Term.Index.Str.Str)
				}
			}
		}
	}

	// Fallback: use string representation
	return query.String()
}

// inferOutputType infers the output type of a query operation
func inferOutputType(query *gojq.Query, op gojq.Operator) string {
	if query == nil {
		return ""
	}

	// Check term type first
	if query.Term != nil {
		switch query.Term.Type {
		case gojq.TermTypeString:
			return "string"
		case gojq.TermTypeNumber:
			return "number"
		case gojq.TermTypeTrue, gojq.TermTypeFalse:
			return "boolean"
		case gojq.TermTypeNull:
			return "null"
		case gojq.TermTypeArray:
			return "array"
		case gojq.TermTypeObject:
			return "object"
		case gojq.TermTypeFunc:
			// Try to infer from function name
			if query.Term.Func != nil {
				name := query.Term.Func.Name
				// Common functions that return strings
				if strings.HasSuffix(name, "_encode") || strings.HasSuffix(name, "_decode") ||
					strings.HasPrefix(name, "base") || strings.HasPrefix(name, "hex") ||
					name == "cat" || name == "tee" || name == "sh" {
					return "string"
				}
				// Hash functions return strings
				if name == "md5" || name == "sha1" || name == "sha256" || name == "sha512" ||
					strings.HasPrefix(name, "sha") {
					return "string"
				}
				// Functions that return numbers
				if name == "length" || name == "keys" {
					return "number"
				}
			}
		}
	}

	// Infer from operator
	switch op {
	case gojq.OpAdd, gojq.OpSub, gojq.OpMul, gojq.OpDiv, gojq.OpMod:
		return "number"
	case gojq.OpEq, gojq.OpNe, gojq.OpGt, gojq.OpLt, gojq.OpGe, gojq.OpLe, gojq.OpAnd, gojq.OpOr:
		return "boolean"
	}

	return ""
}
