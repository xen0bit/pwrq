// Package collection provides the data-structure utilities jq leaves to the
// caller: reshaping arrays, reshaping objects, and set arithmetic.
package collection

import (
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every collection cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterChunks(),
		RegisterDedupe(),
		RegisterDeepMerge(),
		RegisterPrune(),
		RegisterFlattenKeys(),
		RegisterUnflattenKeys(),
		RegisterZipArrays(),
		RegisterRotate(),
		RegisterTopN(),
		RegisterInterleave(),
		// Sets, slicing and lookups
		RegisterIntersection(),
		RegisterUnion(),
		RegisterDifference(),
		RegisterSymmetricDifference(),
		RegisterAllEqual(),
		RegisterContainsDuplicates(),
		RegisterCartesian(),
		RegisterColumn(),
		RegisterLookup(),
		RegisterCompareObject(),
		RegisterNaturalSort(),
		RegisterRenameKeys(),
		RegisterWindows(),
	}
}

// arrInput resolves the array a cmdlet operates on, along with its remaining
// operands. See common.SplitInput for the binding rule: the explicit input is
// the leading argument at the cmdlet's maximum arity, and never inferred from
// the operands' types.
func arrInput(v any, args []any, operands int, fn string) ([]any, []any, error) {
	return common.ArrayInput(v, args, operands, fn)
}

// The cmdlets below resolve their array with arrInput, so their input comes
// from the pipeline or from the leading argument. Declaring that here keeps the
// catalogue's input form in step with the binding the helper applies at run
// time (arrInput runs inside each closure, where it cannot declare itself).
func init() {
	for _, name := range []string{
		"chunks", "windows", "rotate", "zip_arrays", "interleave", "column",
		"top_n", "natural_sort", "all_equal", "contains_duplicates", "dedupe",
		"lookup",
	} {
		common.DeclareInput(name, common.InputPipeline)
	}
}
