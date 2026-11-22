package suggest

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/d1"
	"github.com/xen0bit/pwrq/pkg/ideengine"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// realRanker runs the real model when PWRQ_TEST_D1_DIR and
// PWRQ_TEST_ORT_LIB name it; CI skips these.
func realRanker(t *testing.T) *ideengine.Engine {
	t.Helper()
	dir, lib := os.Getenv("PWRQ_TEST_D1_DIR"), os.Getenv("PWRQ_TEST_ORT_LIB")
	if dir == "" || lib == "" {
		t.Skip("set PWRQ_TEST_D1_DIR and PWRQ_TEST_ORT_LIB to run the model")
	}
	opts := d1.Options{ModelDir: dir, ORTLib: lib, Device: "cpu"}
	if _, err := d1.Shared(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	return ideengine.New(ideengine.Config{Registry: udf.DefaultRegistry(), Ranker: New(opts)})
}

// TestACommentWritesTheQuery is the editors' promise, on the real model: a
// comment, misspelt as people write them, becomes the query it asks for,
// shown unasked.
func TestACommentWritesTheQuery(t *testing.T) {
	e := realRanker(t)
	for _, c := range []struct{ comment, want []string }{
		{[]string{"# Use a decision model to list images in the current directory that contain a shimp"},
			[]string{`get_childitem(".")`, `Does the image contain shrimp?`, `| .Name`}},
		{[]string{"# find files bigger than 50MB in ~/Downloads"},
			[]string{`get_childitem("~/Downloads"; {Recurse: true})`, `select(.Length > 52428800)`}},
		{[]string{"# show the 5 processes using the most memory"},
			[]string{`[get_process]`, `.[:5]`}},
	} {
		query := strings.Join(c.comment, "\n")
		resp := e.Suggest(context.Background(), ideengine.SuggestRequest{Query: query, Cursor: -1})
		if !strings.HasPrefix(resp.Ghost, "\n") {
			t.Errorf("%s: no ghost (items %+v)", query, resp.Items)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(resp.Ghost, w) {
				t.Errorf("%s: ghost lacks %q:\n%s", query, w, resp.Ghost)
			}
		}
	}
}
