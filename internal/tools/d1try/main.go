// Command d1try asks the IDE's suggestion engine, with the real d1-2B
// ranker, what comes next in a query: a quick look at what an editor would
// show, without one. The cursor is where ‸ is, else the end.
//
//	go run ./internal/tools/d1try '# list images that contain a shrimp'
//	go run ./internal/tools/d1try -input '[{"a":1}]' 'map(.‸)'
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/xen0bit/pwrq/pkg/d1"
	"github.com/xen0bit/pwrq/pkg/d1/suggest"
	"github.com/xen0bit/pwrq/pkg/ideengine"
	"github.com/xen0bit/pwrq/pkg/udf"
)

func main() {
	input := flag.String("input", "", "a sample of the query's input")
	auto := flag.Bool("auto", false, "ask as typing does, not as Ctrl-Space does")
	flag.Parse()
	r := suggest.New(d1.OptionsFromEnv())
	eng := ideengine.New(ideengine.Config{Registry: udf.DefaultRegistry(), Ranker: r})
	start := time.Now()
	if _, err := d1.Shared(context.Background(), d1.OptionsFromEnv()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "loaded in %.1fs\n", time.Since(start).Seconds())
	for _, q := range flag.Args() {
		q = strings.ReplaceAll(q, `\n`, "\n")
		cursor := -1
		if i := strings.Index(q, "‸"); i >= 0 {
			cursor = len([]rune(q[:i]))
			q = strings.Replace(q, "‸", "", 1)
		}
		req := ideengine.SuggestRequest{Query: q, Cursor: cursor, Input: *input, Explicit: !*auto}
		sc := eng.SuggestContext(req)
		resp := eng.Suggest(context.Background(), req)
		if a, err := r.Answer(context.Background(), sc); err == nil {
			fmt.Printf("slot answers %q\n", a.Slots)
		}
		fmt.Printf("=== %q\nposition %s, lead %q, %d candidates, slots %q\n", q, resp.Position, sc.Lead, len(sc.Candidates), sc.SlotQuestions)
		for _, it := range resp.Items {
			fmt.Printf("  %5.1f%%  %-8s %s\n", 100*it.Probability, it.Kind, it.Display)
		}
		if resp.Error != "" {
			fmt.Println("  error:", resp.Error)
		}
		fmt.Printf("ghost (%.2fs, %s):\n%s\n", resp.Seconds, resp.Status.Device, resp.Ghost)
	}
}
