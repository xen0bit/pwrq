package llm

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/xen0bit/pwrq/pkg/core/typed"
)

// defaultMaxCalls is how many model calls one pwrq process will make before it
// refuses to make more.
//
// This is the censys paging rule applied to tokens. `map(invoke_llm(...))` over
// a file with 50,000 rows is 50,000 billed requests, and the query that does it
// by accident looks exactly like the query that does it on purpose. A ceiling
// that errors — naming the variable that raises it — costs a caller one run;
// no ceiling costs them the whole bill.
const defaultMaxCalls = 100

var (
	usageMu sync.Mutex
	usage   struct {
		Calls        int
		CacheHits    int
		InputTokens  int
		OutputTokens int
		Cost         float64
		// costKnown records whether any call was priced. Without it a run
		// with no prices supplied would report a confident 0.
		costKnown bool
	}
)

// chargeCall counts a call against the ceiling, before it is made.
func chargeCall(op string, o options) error {
	limit, err := callLimit(op, o)
	if err != nil {
		return err
	}

	usageMu.Lock()
	defer usageMu.Unlock()
	if limit > 0 && usage.Calls >= limit {
		return fmt.Errorf("%s: this process has made %d model calls, which is the ceiling; raise it with %s=<n> or {MaxCalls: n}, or set %s=0 to remove it",
			op, usage.Calls, EnvMaxCalls, EnvMaxCalls)
	}
	usage.Calls++
	return nil
}

// callLimit resolves the ceiling: the option, then the environment, then the
// default. Zero means unbounded, which is a thing a caller can ask for and not
// a thing they get by accident.
func callLimit(op string, o options) (int, error) {
	if o.MaxCalls > 0 {
		return o.MaxCalls, nil
	}
	if o.MaxCalls < 0 {
		return 0, fmt.Errorf("%s: MaxCalls must not be negative, got %d", op, o.MaxCalls)
	}
	if raw := os.Getenv(EnvMaxCalls); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("%s: %s must be a non-negative integer, got %q", op, EnvMaxCalls, raw)
		}
		return n, nil
	}
	return defaultMaxCalls, nil
}

// recordUsage adds one completed call to the running totals.
//
// Prices are the caller's to supply, per million tokens. A table of per-model
// prices compiled into the binary would be stale the week after it shipped,
// and a confidently wrong cost is worse than no cost at all.
func recordUsage(resp *response, o options) {
	usageMu.Lock()
	defer usageMu.Unlock()
	usage.InputTokens += resp.InputTokens
	usage.OutputTokens += resp.OutputTokens
	if o.PriceInput > 0 || o.PriceOutput > 0 {
		usage.Cost += float64(resp.InputTokens)*o.PriceInput/1e6 + float64(resp.OutputTokens)*o.PriceOutput/1e6
		usage.costKnown = true
	}
}

func recordCacheHit() {
	usageMu.Lock()
	defer usageMu.Unlock()
	usage.CacheHits++
}

// usageObject is what get_llm_usage reports.
func usageObject() map[string]any {
	usageMu.Lock()
	defer usageMu.Unlock()
	out := map[string]any{
		"Calls":        usage.Calls,
		"CacheHits":    usage.CacheHits,
		"InputTokens":  usage.InputTokens,
		"OutputTokens": usage.OutputTokens,
		"TotalTokens":  usage.InputTokens + usage.OutputTokens,
		"Cost":         nil,

		typed.TypeKey: "Pwrq.LLM.Usage",
	}
	if usage.costKnown {
		out["Cost"] = usage.Cost
	}
	return out
}

// resetUsage exists for the tests, which share a process and would otherwise
// see each other's totals.
func resetUsage() {
	usageMu.Lock()
	defer usageMu.Unlock()
	usage.Calls = 0
	usage.CacheHits = 0
	usage.InputTokens = 0
	usage.OutputTokens = 0
	usage.Cost = 0
	usage.costKnown = false
}
