package main

import (
	"path/filepath"
	"testing"

	"github.com/xen0bit/pwrq/pkg/ideengine"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// The workflow for a new cmdlet: document it with an example, and give the
// example a comment someone would write to ask for it in
// studies/scripts/intents.jsonl; `make d1.calibrate` then scores it with
// every other. These tests keep the two files in step.

func intents(t *testing.T) map[string][]string {
	t.Helper()
	_ = udf.DefaultRegistry()
	in, err := loadIntents(filepath.Join("studies", "scripts", "intents.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, i := range in {
		if i.Comment == "" {
			t.Errorf("an empty comment for %s", i.Example)
		}
		out[i.Example] = append(out[i.Example], i.Comment)
	}
	return out
}

// TestEveryExampleHasAnIntent: an example nobody can ask for is an example
// the suggestions are never tested on.
func TestEveryExampleHasAnIntent(t *testing.T) {
	have := intents(t)
	for _, r := range ideengine.ExampleRecipes() {
		if len(have[r.Source]) == 0 {
			t.Errorf("%s (%s) has no comment in studies/scripts/intents.jsonl", r.ID, r.Source)
		}
	}
}

// TestEveryIntentNamesAnExample: an example that changed leaves its old
// comment behind, asking for a query that no longer exists.
func TestEveryIntentNamesAnExample(t *testing.T) {
	have := intents(t)
	known := map[string]bool{}
	for _, r := range ideengine.ExampleRecipes() {
		known[r.Source] = true
	}
	for ex := range have {
		if !known[ex] {
			t.Errorf("intents.jsonl has a comment for %q, which is not a published example any more", ex)
		}
	}
}
