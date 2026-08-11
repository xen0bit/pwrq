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
