package shape

import (
	"fmt"
	"sort"
	"sync"
)

// Why a declaration and an emitted object disagreed.
const (
	// ReasonMissing is a declared, non-optional property the cmdlet did not
	// emit. Either the cmdlet stopped emitting it or the property should have
	// been declared optional.
	ReasonMissing = "declared but not emitted"
	// ReasonUndeclared is a key the cmdlet emitted that the shape does not
	// mention. Only a Fixed shape reports this: for the other kinds, keys the
	// declaration does not name are the whole point.
	ReasonUndeclared = "emitted but not declared"
)

// Discrepancy is one disagreement between what a shape declares and what a
// cmdlet built through it.
type Discrepancy struct {
	Shape    string
	Property string
	Reason   string
}

func (d Discrepancy) String() string {
	return fmt.Sprintf("%s: %s %s", d.Shape, d.Property, d.Reason)
}

// Discrepancies are recorded rather than returned because a documentation bug
// must not become a query failure. The alternative — Build returning an error —
// would mean a shape declaration that had fallen behind the code would break
// every query that touched the cmdlet, which is a far worse outcome than a
// catalogue that is briefly wrong.
//
// Recording them makes the check possible without that risk: the test suite
// exercises the cmdlets, and a test then asserts this table is empty. The
// reconciliation is therefore against what the cmdlets actually emit, not
// against a second hand-written list that could drift in its own direction.
var (
	discrepancyMu sync.Mutex
	discrepancies = make(map[Discrepancy]int)
)

func record(d Discrepancy) {
	discrepancyMu.Lock()
	defer discrepancyMu.Unlock()
	discrepancies[d]++
}

// Discrepancies reports every disagreement recorded so far, sorted so a test
// failure reads the same way twice.
func Discrepancies() []Discrepancy {
	discrepancyMu.Lock()
	defer discrepancyMu.Unlock()

	out := make([]Discrepancy, 0, len(discrepancies))
	for d := range discrepancies {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Shape != out[j].Shape {
			return out[i].Shape < out[j].Shape
		}
		if out[i].Property != out[j].Property {
			return out[i].Property < out[j].Property
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

// ResetDiscrepancies clears the table, so a test that means to provoke one can
// start from a known state.
func ResetDiscrepancies() {
	discrepancyMu.Lock()
	defer discrepancyMu.Unlock()
	discrepancies = make(map[Discrepancy]int)
}
