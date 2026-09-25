package graph

import (
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

func outline(t *testing.T, src string, cmdlets ...string) []*OutlineNode {
	t.Helper()
	query, err := gojq.Parse(src)
	if err != nil {
		t.Fatalf("parsing %q: %v", src, err)
	}
	opts := RenderOptions{Cmdlets: map[string]bool{}}
	for _, name := range cmdlets {
		opts.Cmdlets[name] = true
	}
	return Outline(query, opts)
}

// flatten renders a tree one node per line, indented by depth, as
// "label [class]".
func flatten(nodes []*OutlineNode, depth int, out *[]string) {
	for _, node := range nodes {
		*out = append(*out, strings.Repeat("  ", depth)+node.Label+" ["+node.Class+"]")
		flatten(node.Children, depth+1, out)
	}
}

func TestOutlineFollowsThePipeline(t *testing.T) {
	var lines []string
	flatten(outline(t, `.a | sha256 | select(. != "")`, "sha256"), 0, &lines)

	var order []string
	for _, line := range lines {
		for _, want := range []string{".a", "sha256", "select"} {
			if strings.Contains(line, want) && !contains(order, want) {
				order = append(order, want)
			}
		}
	}
	if strings.Join(order, ",") != ".a,sha256,select" {
		t.Errorf("stages out of order: %v\n%s", order, strings.Join(lines, "\n"))
	}
	if !containsLine(lines, "sha256 ["+ClassCmdlet+"]") {
		t.Errorf("sha256 should be coloured as a cmdlet:\n%s", strings.Join(lines, "\n"))
	}
}

// TestOutlineAgreesWithTheScript is the property Outline exists for: every
// label the diagram draws, the outline lists.
func TestOutlineAgreesWithTheScript(t *testing.T) {
	src := `def f(g): g; [.[] | {k: f(.a)} | if .k then "y" else "n" end] | reduce .[] as $x (0; . + 1)`
	query, _ := gojq.Parse(src)
	script := RenderD2Opts(query, RenderOptions{})

	var lines []string
	flatten(outline(t, src), 0, &lines)
	joined := strings.Join(lines, "\n")
	for _, label := range labels(script) {
		// The class declarations quote their colours, which are not labels.
		if label == "Start" || label == "End" || strings.HasPrefix(label, "#") {
			continue
		}
		if !strings.Contains(joined, label) {
			t.Errorf("the diagram draws %q but the outline does not list it\n%s", label, joined)
		}
	}
}

func TestOutlineNestsContainers(t *testing.T) {
	nodes := outline(t, `def f: .; [.[] | f]`)
	if len(nodes) == 0 || nodes[0].Class != ClassDef || len(nodes[0].Children) != 1 {
		t.Fatalf("definitions should come first, holding one definition: %+v", nodes)
	}
	for _, node := range nodes {
		if node.Class == ClassTerminal {
			t.Errorf("start and end markers should be left out, got %q", node.Label)
		}
	}
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func containsLine(lines []string, s string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == s {
			return true
		}
	}
	return false
}
