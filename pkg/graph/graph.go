package graph

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
)

// GenerateGraph creates a D2 diagram representing the flow of a jq query
func GenerateGraph(query *gojq.Query, outputPath string) error {
	// Build D2 script by traversing the query AST
	var builder strings.Builder
	builder.WriteString("title: Query Flow\n\n")

	nodeCounter := 0
	lastNodeID := "start"

	// Create start node
	builder.WriteString("start: {\n  label: \"Start\"\n  shape: circle\n}\n\n")

	// Traverse the query AST
	err := traverseQuery(query, &builder, &nodeCounter, &lastNodeID)
	if err != nil {
		return fmt.Errorf("failed to traverse query: %w", err)
	}

	// Add end node
	endNodeID := fmt.Sprintf("end_%d", nodeCounter)
	builder.WriteString(fmt.Sprintf("%s: {\n  label: \"End\"\n  shape: circle\n}\n\n", endNodeID))

	// Connect last node to end
	if lastNodeID != "start" {
		builder.WriteString(fmt.Sprintf("%s -> %s\n", lastNodeID, endNodeID))
	}

	// Write D2 script to temporary file
	tmpFile, err := os.CreateTemp("", "pwrq_graph_*.d2")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	d2ScriptPath := tmpFile.Name()
	defer func() {
		// Only remove temp file if rendering succeeded
		if _, err := os.Stat(d2ScriptPath); err == nil {
			os.Remove(d2ScriptPath)
		}
	}()

	_, err = tmpFile.WriteString(builder.String())
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write D2 script: %w", err)
	}
	tmpFile.Close()

	// Use d2 CLI to render to PNG
	outputPath, err = filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("failed to resolve output path: %w", err)
	}

	// Check if d2 is available
	_, err = exec.LookPath("d2")
	if err != nil {
		// Save D2 script next to output file for manual rendering
		d2OutputPath := outputPath[:len(outputPath)-4] + ".d2"
		if err := os.WriteFile(d2OutputPath, []byte(builder.String()), 0644); err == nil {
			return fmt.Errorf("d2 CLI not found in PATH. Install d2 from https://d2lang.com/tour/install/\nD2 script saved to: %s\nYou can render it manually with: d2 %s %s", d2OutputPath, d2OutputPath, outputPath)
		}
		return fmt.Errorf("d2 CLI not found in PATH. Install d2 from https://d2lang.com/tour/install/")
	}

	cmd := exec.Command("d2", d2ScriptPath, outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Save D2 script for debugging
		d2OutputPath := outputPath[:len(outputPath)-4] + ".d2"
		os.WriteFile(d2OutputPath, []byte(builder.String()), 0644)
		return fmt.Errorf("failed to render D2 diagram: %w\nOutput: %s\nD2 script saved to: %s", err, string(output), d2OutputPath)
	}

	return nil
}

// traverseQuery recursively traverses the jq query AST and builds D2 nodes
func traverseQuery(query *gojq.Query, builder *strings.Builder, nodeCounter *int, lastNodeID *string) error {
	if query == nil {
		return nil
	}

	// Get the query operator
	op := query.Op

	// Create a node for this operation
	nodeID := fmt.Sprintf("node_%d", *nodeCounter)
	*nodeCounter++

	// Determine node label based on operation type and term
	label := getNodeLabel(query, op)

	// Create the node in D2 script
	builder.WriteString(fmt.Sprintf("%s: {\n  label: %q\n  shape: rectangle\n}\n\n", nodeID, label))

	// Connect from previous node
	if *lastNodeID != "start" {
		builder.WriteString(fmt.Sprintf("%s -> %s\n", *lastNodeID, nodeID))
	}

	*lastNodeID = nodeID

	// Recursively process query arguments
	// For pipe operations, process left then right sequentially
	if query.Op == gojq.OpPipe {
		// Left side feeds into right side
		if query.Left != nil {
			err := traverseQuery(query.Left, builder, nodeCounter, lastNodeID)
			if err != nil {
				return err
			}
			// Connect left result to current node
			if *lastNodeID != nodeID {
				builder.WriteString(fmt.Sprintf("%s -> %s\n", *lastNodeID, nodeID))
				*lastNodeID = nodeID
			}
		}
		if query.Right != nil {
			err := traverseQuery(query.Right, builder, nodeCounter, lastNodeID)
			if err != nil {
				return err
			}
			// Connect current node to right result
			if *lastNodeID != nodeID {
				builder.WriteString(fmt.Sprintf("%s -> %s\n", nodeID, *lastNodeID))
			}
		}
	} else {
		// For other operations, process left and right as separate branches
		if query.Left != nil {
			err := traverseQuery(query.Left, builder, nodeCounter, lastNodeID)
			if err != nil {
				return err
			}
			// Connect back to current node
			if *lastNodeID != nodeID {
				builder.WriteString(fmt.Sprintf("%s -> %s\n", *lastNodeID, nodeID))
				*lastNodeID = nodeID
			}
		}

		if query.Right != nil {
			err := traverseQuery(query.Right, builder, nodeCounter, lastNodeID)
			if err != nil {
				return err
			}
			// Connect back to current node
			if *lastNodeID != nodeID {
				builder.WriteString(fmt.Sprintf("%s -> %s\n", *lastNodeID, nodeID))
				*lastNodeID = nodeID
			}
		}
	}

	return nil
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
			return "Query"
		}
		return fmt.Sprintf("Op(%d)", op)
	}
}

// getNodeLabel returns a label for a query node, combining operator and term info
func getNodeLabel(query *gojq.Query, op gojq.Operator) string {
	// If there's a term, use it for the label
	if query.Term != nil {
		termLabel := getTermLabel(query.Term)
		if termLabel != "" {
			return termLabel
		}
	}

	// Otherwise use the operator label
	return getOperationLabel(op)
}

// getTermLabel extracts a label from a Term
func getTermLabel(term *gojq.Term) string {
	if term == nil {
		return ""
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
			if term.Index.Name != "" {
				return fmt.Sprintf("Index: %s", term.Index.Name)
			}
			if term.Index.Str != nil {
				return fmt.Sprintf("Index: %q", term.Index.Str.Str)
			}
		}
		return "Index"
	case gojq.TermTypeFunc:
		if term.Func != nil {
			return fmt.Sprintf("Function: %s", term.Func.Name)
		}
		return "Function"
	case gojq.TermTypeArray:
		return "Array"
	case gojq.TermTypeObject:
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
