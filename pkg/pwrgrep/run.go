package pwrgrep

import (
	"context"
	"fmt"
	"sync"

	"github.com/itchyny/gojq"
)

// A rule is a pwrq query, so running one means compiling it against the same
// vocabulary a query typed at the prompt gets. That vocabulary is the cmdlet
// registry, which imports this package, so it arrives through UseVocabulary
// rather than through an import - the same shape common.SetScriptBlockOptions
// uses, and for the same reason.
var (
	vocabularyMu sync.RWMutex
	vocabulary   []gojq.CompilerOption
	compiled     sync.Map // rule path -> *gojq.Code
)

// UseVocabulary supplies the compiler options rules are built with. The
// registry calls it once it has assembled them.
func UseVocabulary(options []gojq.CompilerOption) {
	vocabularyMu.Lock()
	defer vocabularyMu.Unlock()
	vocabulary = options
	// A rule compiled against the old vocabulary would keep calling it.
	compiled.Range(func(key, _ any) bool { compiled.Delete(key); return true })
}

// Compile turns a rule into runnable code, once per rule.
//
// Compiling is where a generated corpus fails: a translator that emits
// something which is not a query emits it hundreds of times. So this is also
// what a test uses to check the corpus without running it - the queries are
// compiled and not run, because running one means walking a tree.
func (r *Rule) Compile() (*gojq.Code, error) {
	if code, ok := compiled.Load(r.Path); ok {
		return code.(*gojq.Code), nil
	}
	query, err := gojq.Parse(r.Query)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", r.Path, err)
	}
	vocabularyMu.RLock()
	options := vocabulary
	vocabularyMu.RUnlock()

	code, err := gojq.Compile(query, options...)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", r.Path, err)
	}
	compiled.Store(r.Path, code)
	return code, nil
}

// Run searches one path with one rule and returns what it reported.
//
// A rule ends in `report`, which collects its findings into one array, so what
// comes back here is that array's elements: one value per finding, in the
// order the rule put them in.
func (r *Rule) Run(ctx context.Context, root string) ([]any, error) {
	code, err := r.Compile()
	if err != nil {
		return nil, err
	}
	var out []any
	iter := code.RunWithContext(ctx, root)
	for {
		v, ok := iter.Next()
		if !ok {
			return out, nil
		}
		switch value := v.(type) {
		case error:
			return nil, fmt.Errorf("%s: %v", r.Path, value)
		case []any:
			out = append(out, value...)
		default:
			// A rule that ends in something other than report emits its
			// findings one at a time, which is just as good an answer.
			out = append(out, value)
		}
	}
}

// forgetCompiled drops every compiled rule, so a file that changed on disk is
// recompiled rather than answered from the copy read first. Rules calls it
// after a reload; nothing else should need to.
func forgetCompiled() {
	compiled.Range(func(key, _ any) bool { compiled.Delete(key); return true })
}
