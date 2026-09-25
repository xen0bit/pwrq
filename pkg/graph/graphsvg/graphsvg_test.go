package graphsvg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/graph"
)

// render parses a query and returns the D2 script for it.
func render(t *testing.T, src string) string {
	t.Helper()
	query, err := gojq.Parse(src)
	if err != nil {
		t.Fatalf("parsing %q: %v", src, err)
	}
	return graph.RenderD2(query)
}

func TestRenderD2_LabelsAreEscaped(t *testing.T) {
	for _, src := range []string{`"a \"quoted\" string"`, `"back\\slash"`, `{"k: v": 1}`} {
		t.Run(src, func(t *testing.T) {
			script := render(t, src)
			if _, err := renderSVG(script); err != nil {
				t.Errorf("script does not compile: %v\n--- script ---\n%s", err, script)
			}
		})
	}
}

func TestGenerateGraph_WritesBothFormats(t *testing.T) {
	query, err := gojq.Parse(`[get_childitem(".") | select(.Length > 10) | {Name, H: (.p | sha256)}] | .[0:3]`)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	d2Path := filepath.Join(dir, "out.d2")
	if err := GenerateGraph(query, d2Path); err != nil {
		t.Fatalf("GenerateGraph(.d2): %v", err)
	}
	script, err := os.ReadFile(d2Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"get_childitem", "select", "Name", "sha256"} {
		if !strings.Contains(string(script), part) {
			t.Errorf("the D2 file never mentions %q", part)
		}
	}

	svgPath := filepath.Join(dir, "out.svg")
	if err := GenerateGraph(query, svgPath); err != nil {
		t.Fatalf("GenerateGraph(.svg): %v", err)
	}
	svg, err := os.ReadFile(svgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(svg), "<svg") {
		t.Error("output is not an SVG document")
	}
	// The labels have to survive into the rendered image, not just the script.
	for _, part := range []string{"get_childitem", "sha256", "Collect"} {
		if !strings.Contains(string(svg), part) {
			t.Errorf("rendered SVG never shows %q", part)
		}
	}
}

func TestGenerateGraph_RejectsUnknownFormat(t *testing.T) {
	query, _ := gojq.Parse(".")
	err := GenerateGraph(query, filepath.Join(t.TempDir(), "out.png"))
	if err == nil {
		t.Fatal("expected an error for an unsupported format")
	}
	if !strings.Contains(err.Error(), ".d2") || !strings.Contains(err.Error(), ".svg") {
		t.Errorf("the error should name the supported formats, got: %v", err)
	}
}

func TestGenerateSVG(t *testing.T) {
	query, err := gojq.Parse(`.a | select(.b > 1) | {c}`)
	if err != nil {
		t.Fatal(err)
	}
	svg, err := GenerateSVG(query)
	if err != nil {
		t.Fatalf("GenerateSVG: %v", err)
	}
	if !strings.Contains(svg, "<svg") {
		t.Error("output is not an SVG document")
	}
}

func TestRenderD2_EveryQueryCompiles(t *testing.T) {
	queries := []string{
		".", "..", ".a.b.c", ".[]", ".[1:3]", ".a?",
		"1, 2, 3", "[1,2,3]", "{}", "{a: 1}", "$__loc__",
		`"interp \(.a) here"`, "@base64 \"x\"", "-.a", "not",
		".a // .b", ".a as [$x, $y] | $x",
		"label $out | .[] | if . then ., break $out else . end",
		`[limit(3; repeat(1))]`,
		`get_childitem("."; {Recurse: true}) | where_object(.; {script: ".a > 1"})`,
		`reduce (.[] | select(.n)) as $i ({}; .[$i.k] = $i.v)`,
		`try error("x") catch .`,
		`def f(g): g | g; f(.a)`,
	}
	for _, src := range queries {
		t.Run(src, func(t *testing.T) {
			script := render(t, src)
			if _, err := renderSVG(script); err != nil {
				t.Errorf("script does not compile: %v\n--- script ---\n%s", err, script)
			}
		})
	}
}
