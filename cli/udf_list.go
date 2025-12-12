package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xen0bit/pwrq/pkg/udf"
)

func (cli *cli) printUDFList() {
	metadata := udf.GetFunctionMetadata()
	
	// Group by category
	categories := make(map[string][]udf.FunctionMetadata)
	for _, meta := range metadata {
		categories[meta.Category] = append(categories[meta.Category], meta)
	}
	
	// Sort categories
	categoryOrder := []string{
		"File Operations",
		"Encoding",
		"Compression",
		"String",
		"Hash",
		"HMAC",
		"Timestamp",
		"JSON",
		"CSV",
		"XML",
		"Entropy",
	}
	
	// Collect all categories
	allCategories := make(map[string]bool)
	for _, meta := range metadata {
		allCategories[meta.Category] = true
	}
	
	// Add any categories not in the predefined order
	for cat := range allCategories {
		found := false
		for _, orderedCat := range categoryOrder {
			if cat == orderedCat {
				found = true
				break
			}
		}
		if !found {
			categoryOrder = append(categoryOrder, cat)
		}
	}
	
	fmt.Fprintf(cli.outStream, "Available User-Defined Functions (UDFs)\n\n")
	fmt.Fprintf(cli.outStream, "Total: %d functions\n\n", len(metadata))
	
	for _, category := range categoryOrder {
		funcs, ok := categories[category]
		if !ok {
			continue
		}
		
		// Sort functions within category by name
		sort.Slice(funcs, func(i, j int) bool {
			return funcs[i].Name < funcs[j].Name
		})
		
		fmt.Fprintf(cli.outStream, "%s:\n", category)
		fmt.Fprintf(cli.outStream, "%s\n", strings.Repeat("-", len(category)+1))
		
		for _, meta := range funcs {
			// Build argument signature
			var argSig strings.Builder
			if meta.MinArgs == 0 && meta.MaxArgs == 0 {
				argSig.WriteString("()")
			} else if meta.MinArgs == meta.MaxArgs {
				argSig.WriteString(fmt.Sprintf("(%d args)", meta.MinArgs))
			} else {
				argSig.WriteString(fmt.Sprintf("(%d-%d args)", meta.MinArgs, meta.MaxArgs))
			}
			
			fmt.Fprintf(cli.outStream, "  %-25s %-15s %s\n", meta.Name, argSig.String(), meta.Description)
			
			// Print examples if available
			if len(meta.Examples) > 0 {
				for _, example := range meta.Examples {
					fmt.Fprintf(cli.outStream, "    Example: %s\n", example)
				}
			}
		}
		
		fmt.Fprintf(cli.outStream, "\n")
	}
	
	fmt.Fprintf(cli.outStream, "Note: Most functions support an optional 'file' boolean argument.\n")
	fmt.Fprintf(cli.outStream, "      When true, the input is treated as a file path to operate on.\n")
	fmt.Fprintf(cli.outStream, "      Example: base64_encode(true) reads from a file.\n")
}

