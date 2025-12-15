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
	var lastOutputType string

	// Create start node
	builder.WriteString("start: {\n  label: \"Start\"\n  shape: circle\n}\n\n")

	// Traverse the query AST
	lastOutputType, err := traverseQuery(query, &builder, &nodeCounter, &lastNodeID, "")
	if err != nil {
		return fmt.Errorf("failed to traverse query: %w", err)
	}

	// Add end node
	endNodeID := fmt.Sprintf("end_%d", nodeCounter)
	builder.WriteString(fmt.Sprintf("%s: {\n  label: \"End\"\n  shape: circle\n}\n\n", endNodeID))

	// Connect last node to end with type
	if lastNodeID != "start" {
		if lastOutputType != "" {
			builder.WriteString(fmt.Sprintf("%s -> %s: {\n  label: %q\n}\n", lastNodeID, endNodeID, lastOutputType))
		} else {
			builder.WriteString(fmt.Sprintf("%s -> %s\n", lastNodeID, endNodeID))
		}
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
// Returns the output type of this query node
func traverseQuery(query *gojq.Query, builder *strings.Builder, nodeCounter *int, lastNodeID *string, prevOutputType string) (string, error) {
	if query == nil {
		return "", nil
	}

	// Get the query operator
	op := query.Op

	// Create a node for this operation
	nodeID := fmt.Sprintf("node_%d", *nodeCounter)
	*nodeCounter++

	// Determine node label based on operation type and term
	label := getNodeLabel(query, op)

	// Infer output type for this node
	outputType := inferOutputType(query, op)

	// Create the node in D2 script
	// Format label to avoid D2 syntax issues with special characters
	builder.WriteString(fmt.Sprintf("%s: {\n  label: %s\n  shape: rectangle\n}\n\n", nodeID, formatD2Label(label)))

	// Connect from previous node with type information
	if *lastNodeID != "start" {
		// Use the previous node's output type as the edge label
		if prevOutputType != "" {
			builder.WriteString(fmt.Sprintf("%s -> %s: {\n  label: %q\n}\n", *lastNodeID, nodeID, prevOutputType))
		} else {
			builder.WriteString(fmt.Sprintf("%s -> %s\n", *lastNodeID, nodeID))
		}
	}

	*lastNodeID = nodeID

	// Recursively process query arguments
	// For pipe operations, process left then right sequentially
	if query.Op == gojq.OpPipe {
		// Left side feeds into right side
		var leftType string
		if query.Left != nil {
			var err error
			leftType, err = traverseQuery(query.Left, builder, nodeCounter, lastNodeID, prevOutputType)
			if err != nil {
				return "", err
			}
			// Connect left result to current node with type
			if *lastNodeID != nodeID {
				if leftType != "" {
					builder.WriteString(fmt.Sprintf("%s -> %s: {\n  label: %q\n}\n", *lastNodeID, nodeID, leftType))
				} else {
					builder.WriteString(fmt.Sprintf("%s -> %s\n", *lastNodeID, nodeID))
				}
				*lastNodeID = nodeID
			}
		}
		if query.Right != nil {
			// Right side receives output from left (or current node if no left)
			inputType := leftType
			if inputType == "" {
				inputType = outputType
			}
			rightType, err := traverseQuery(query.Right, builder, nodeCounter, lastNodeID, inputType)
			if err != nil {
				return "", err
			}
			// Connect current node to right result with type
			if *lastNodeID != nodeID {
				if outputType != "" {
					builder.WriteString(fmt.Sprintf("%s -> %s: {\n  label: %q\n}\n", nodeID, *lastNodeID, outputType))
				} else {
					builder.WriteString(fmt.Sprintf("%s -> %s\n", nodeID, *lastNodeID))
				}
			}
			return rightType, nil
		}
	} else {
		// For other operations, process left and right as separate branches
		if query.Left != nil {
			leftType, err := traverseQuery(query.Left, builder, nodeCounter, lastNodeID, prevOutputType)
			if err != nil {
				return "", err
			}
			// Connect back to current node
			if *lastNodeID != nodeID {
				if leftType != "" {
					builder.WriteString(fmt.Sprintf("%s -> %s: {\n  label: %q\n}\n", *lastNodeID, nodeID, leftType))
				} else {
					builder.WriteString(fmt.Sprintf("%s -> %s\n", *lastNodeID, nodeID))
				}
				*lastNodeID = nodeID
			}
		}

		if query.Right != nil {
			rightType, err := traverseQuery(query.Right, builder, nodeCounter, lastNodeID, prevOutputType)
			if err != nil {
				return "", err
			}
			// Connect back to current node
			if *lastNodeID != nodeID {
				if rightType != "" {
					builder.WriteString(fmt.Sprintf("%s -> %s: {\n  label: %q\n}\n", *lastNodeID, nodeID, rightType))
				} else {
					builder.WriteString(fmt.Sprintf("%s -> %s\n", *lastNodeID, nodeID))
				}
				*lastNodeID = nodeID
			}
		}
	}

	return outputType, nil
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

// formatD2Label formats a label for D2, escaping special characters
func formatD2Label(label string) string {
	// Replace $ with a safe representation to avoid D2 variable substitution
	// D2 interprets $ as variable substitution, so we'll replace it with a placeholder
	safeLabel := strings.ReplaceAll(label, "$", "_VAR_")
	// Also escape any newlines and ensure quotes are properly escaped
	safeLabel = strings.ReplaceAll(safeLabel, "\n", "\\n")
	return fmt.Sprintf("%q", safeLabel)
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
	// If there's a term, use it for the label
	if query.Term != nil {
		termLabel := getTermLabel(query.Term, query)
		if termLabel != "" {
			return termLabel
		}
	}

	// Check if this is an index operation on the query itself (like .[0:3])
	// This happens when the query has no term but has index operations in Left
	if query.Left != nil {
		if query.Left.Term != nil {
			// Check for suffixes on the left term
			if len(query.Left.Term.SuffixList) > 0 {
				suffixLabel := formatSuffixList(query.Left.Term.SuffixList)
				if suffixLabel != "" {
					// Combine with base term label
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
				suffixLabel := formatSuffixList(query.Right.Term.SuffixList)
				if suffixLabel != "" {
					baseLabel := getTermBaseLabel(query.Right.Term)
					if baseLabel != "" {
						return baseLabel + suffixLabel
					}
					return suffixLabel
				}
			}
		}
	}

	// If we still don't have a label, try to get more info from the query structure
	// This helps catch cases like .[0:3] that might be represented differently
	// Try the query's own string representation first
	queryStr := query.String()
	if slicePattern := extractSlicePattern(queryStr); slicePattern != "" {
		return "Slice " + slicePattern
	}

	// Also check Left and Right sides
	if query.Left != nil {
		leftStr := query.Left.String()
		if slicePattern := extractSlicePattern(leftStr); slicePattern != "" {
			return "Slice " + slicePattern
		}
	}
	if query.Right != nil {
		rightStr := query.Right.String()
		if slicePattern := extractSlicePattern(rightStr); slicePattern != "" {
			return "Slice " + slicePattern
		}
	}

	// Otherwise use the operator label (or empty if op is 0)
	opLabel := getOperationLabel(op)
	if opLabel == "" && queryStr != "" {
		// If no operator label and we have a query string, use a simplified version
		// Limit length to avoid overly long labels
		if len(queryStr) > 50 {
			return queryStr[:47] + "..."
		}
		return queryStr
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
	if len(term.SuffixList) > 0 {
		suffixLabel := formatSuffixList(term.SuffixList)
		if suffixLabel != "" {
			// Combine term label with suffix
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
				// Check for additional suffixes
				if len(term.SuffixList) > 0 {
					return "Slice " + sliceLabel + formatSuffixList(term.SuffixList)
				}
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
		// Fallback: try to extract from query string representation
		if query != nil {
			queryStr := query.String()
			if slicePattern := extractSlicePattern(queryStr); slicePattern != "" {
				return "Slice " + slicePattern
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
