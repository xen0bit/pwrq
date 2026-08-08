package graph

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

// render parses a query and returns the D2 script for it.
func render(t *testing.T, src string) string {
	t.Helper()
	query, err := gojq.Parse(src)
	if err != nil {
		t.Fatalf("parsing %q: %v", src, err)
	}
	return RenderD2(query)
}

var labelPattern = regexp.MustCompile(`(?m)^\s*[\w.]+\s*:\s*"((?:[^"\\]|\\.)*)"`)

// labels returns every label the script assigns, unescaped.
func labels(script string) []string {
	var out []string
	for _, m := range labelPattern.FindAllStringSubmatch(script, -1) {
		label := strings.ReplaceAll(m[1], `\"`, `"`)
		label = strings.ReplaceAll(label, `\$`, "$")
		out = append(out, strings.ReplaceAll(label, `\\`, `\`))
	}
	return out
}

// mentions reports whether any label contains the given text.
func mentions(script, text string) bool {
	for _, label := range labels(script) {
		if strings.Contains(label, text) {
			return true
		}
	}
	return false
}

// requireMentions is the property the renderer exists to satisfy: a diagram
// that omits part of the query is showing the reader something untrue about
// what will run.
func requireMentions(t *testing.T, script string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !mentions(script, part) {
			t.Errorf("diagram never mentions %q\n--- script ---\n%s", part, script)
		}
	}
}

func TestRenderD2_PipelineIsTheSpine(t *testing.T) {
	script := render(t, ".a | .b | .c")

	requireMentions(t, script, "Start", "End", ".a", ".b", ".c")

	// Stages are chained in order.
	for _, edge := range []string{"start -> n1", "n1 -> n2", "n2 -> n3", "n3 -> end"} {
		if !strings.Contains(script, edge) {
			t.Errorf("expected edge %q\n--- script ---\n%s", edge, script)
		}
	}
}

// TestRenderD2_NothingIsLost is the regression guard. Each of these queries had
// a part the previous renderer dropped: the operands of a comparison, an
// object's shorthand keys, a unary operator's operand, the array construction
// that a later stage depends on.
func TestRenderD2_NothingIsLost(t *testing.T) {
	cases := []struct {
		query string
		parts []string
	}{
		{`select(.Length > 1000)`, []string{"select", ".Length", "1000"}},
		{`{Name, Size}`, []string{"Name", "Size"}},
		{`{Name, Hash: (.Path | sha256)}`, []string{"Name", "Hash", ".Path", "sha256"}},
		{`sort_by(-.Length)`, []string{"sort_by", ".Length"}},
		{`[.[] | select(.a)] | length`, []string{"Collect", "select", "length"}},
		{`.a and (.b or .c)`, []string{".a", ".b", ".c"}},
		{`if .n > 5 then "big" else "small" end`, []string{"if", "then", "else", ".n", "big", "small"}},
		{`try (cat("f") | fromjson) catch "bad"`, []string{"try", "catch", "cat", "fromjson", "bad"}},
		{`reduce .[] as $i (0; . + $i)`, []string{"reduce", "$i", "source", "init", "update"}},
		{`foreach .[] as $x (0; . + $x; .)`, []string{"foreach", "$x", "extract"}},
		{`. as $p | $p | cat`, []string{"as $p", "cat"}},
		{`get_childitem("."; {Recurse: true, Filter: "*.go"})`,
			[]string{"get_childitem", "Recurse", "true", "Filter", "*.go"}},
		{`.a[0].b[]?`, []string{".a[0].b[]?"}},
		{`map(select(.x | endswith(".go")))`, []string{"map", "select", ".x", "endswith"}},
		{`.[] |= (. * 2)`, []string{"2"}},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			requireMentions(t, render(t, tc.query), tc.parts...)
		})
	}
}

// TestRenderD2_StructureNests checks that a subquery appears inside the node it
// belongs to, not merely somewhere in the file.
func TestRenderD2_StructureNests(t *testing.T) {
	script := render(t, `[get_childitem(".") | select(.Length > 10) | {Name}] | sort_by(.Name)`)

	collectAt := strings.Index(script, "Collect [ ]")
	if collectAt < 0 {
		t.Fatalf("array construction is missing\n%s", script)
	}
	closeAt := strings.Index(script[collectAt:], "\n}")
	if closeAt < 0 {
		t.Fatalf("collect container is never closed\n%s", script)
	}
	inside := script[collectAt : collectAt+closeAt]

	for _, part := range []string{"get_childitem", "select", "Name"} {
		if !strings.Contains(inside, part) {
			t.Errorf("%q should be inside the collect container\n--- container ---\n%s", part, inside)
		}
	}
	// sort_by consumes the array, so it belongs outside it.
	if strings.Contains(inside, "sort_by") {
		t.Errorf("sort_by consumes the array and should sit outside it\n%s", script)
	}
}

// TestRenderD2_NoStrayIdentifierNodes guards a D2 scoping mistake: writing a
// dotted path inside a container declares that whole path *under* it, which
// produced boxes labelled "n1" and "n2" beside the real ones.
func TestRenderD2_NoStrayIdentifierNodes(t *testing.T) {
	script := render(t, `[get_childitem("."; {A: 1}) | {B: (.x | sha256)}]`)
	stray := regexp.MustCompile(`^n\d+$`)
	for _, label := range labels(script) {
		if stray.MatchString(label) {
			t.Errorf("label %q is an internal identifier, not part of the query\n%s", label, script)
		}
	}
}

// TestRenderD2_SimpleThingsStayInline keeps the diagram readable: expanding
// every path expression into its own box would show less than the query text.
func TestRenderD2_SimpleThingsStayInline(t *testing.T) {
	script := render(t, `select(.Length > 1000)`)
	if strings.Count(script, "shape: rectangle") != 1 {
		t.Errorf("a simple call should be one node, got:\n%s", script)
	}
}

func TestRenderD2_FuncDefsAreShownAsContext(t *testing.T) {
	script := render(t, `def double: . * 2; .a | double`)
	requireMentions(t, script, "Definitions", "double", ".a")
}

func TestRenderD2_EmptyAndTrivialQueries(t *testing.T) {
	if got := RenderD2(nil); !strings.Contains(got, "empty query") {
		t.Errorf("a nil query should render something explanatory, got %q", got)
	}
	requireMentions(t, render(t, "."), "Start", "End", ".")
}

// TestRenderD2_LabelsAreEscaped checks that query text D2 would read as syntax
// cannot break the script.
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

// TestGenerateGraph_WritesBothFormats covers the file-producing entry point.
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
	requireMentions(t, string(script), "get_childitem", "select", "Name", "sha256")

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

// TestRenderD2_EveryQueryCompiles is a broad check that no AST shape produces a
// script D2 cannot parse.
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
