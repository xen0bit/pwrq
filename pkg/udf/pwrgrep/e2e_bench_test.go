package pwrgrep_test

import (
	"context"
	"testing"
	"time"

	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// A whole rule, run the way a caller runs one: over a tree, through the query
// engine, from scan_ast to report. The operator benchmarks say what each stage
// costs; this says what the stages add up to, which is the number that decides
// whether pwrgrep is usable on a repository.

func benchQuery(b *testing.B, query string) {
	b.Helper()
	runner := &queryrun.Runner{Options: udf.DefaultRegistry().Options()}
	run := func() int {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		res := runner.Run(ctx, &queryrun.Request{Query: query, NullInput: true, Compact: true})
		if res.Error != "" {
			b.Fatalf("%s\n  %s", query, res.Error)
		}
		return len(res.Values)
	}
	// The first run compiles the patterns and decodes the grammars, which is
	// paid once per process and is not what a repeated search costs.
	run()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		run()
	}
}

// The rule shape a corpus is mostly made of: a set of calls, narrowed by what
// else the file contains, reported.
func BenchmarkRuleInFilesWith(b *testing.B) {
	benchQuery(b, `
		["md5.New()", "md5.Sum($$$A)", "sha1.New()", "sha1.Sum($$$A)"] as $calls
		| ["\"crypto/md5\"", "\"crypto/sha1\""] as $imports
		| "../../.." | scan_ast("*.go"; $calls + $imports) as $all
		| ($all | of($calls) | in_files_with($all | of($imports)))
		| finding("go-weak-hash"; "this hash is not collision resistant")
		| report | length`)
}

// The other half: a span inside a span, which is the operator that has to know
// where every match in the other list is.
func BenchmarkRuleWithin(b *testing.B) {
	benchQuery(b, `
		["fmt.Errorf($$$A)"] as $calls
		| ["func $F($$$P) error { $$$B }"] as $funcs
		| "../../.." | scan_ast("*.go"; $calls + $funcs) as $all
		| ($all | of($calls) | within($all | of($funcs)))
		| finding("go-errorf-in-error-func"; "formatted error")
		| report | length`)
}

// Searching the tree with no combining at all, which is the floor the two
// above are measured against.
func BenchmarkScanAst(b *testing.B) {
	benchQuery(b, `"../../.." | scan_ast("*.go"; ["md5.New()", "md5.Sum($$$A)", "sha1.New()", "sha1.Sum($$$A)"]) | length`)
}
