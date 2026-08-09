package graph

import (
	"fmt"
	"sort"
	"strings"
)

// A diagram of a query is only worth more than the query text if it says
// something the text does not. Colour is where most of that extra meaning
// lives: at a glance, a coloured diagram separates the cmdlets you called from
// jq's own builtins, the data you constructed from the data you read, and the
// control flow from the straight line through it.
//
// The classes below are the vocabulary. Every node the renderer emits carries
// exactly one, so a legend drawn from this list explains the whole picture.
const (
	ClassTerminal  = "terminal"  // Start and End
	ClassCmdlet    = "cmdlet"    // a pwrq cmdlet: get_childitem, sha256, format_table
	ClassBuiltin   = "builtin"   // a jq builtin or a user definition: map, select, length
	ClassPath      = "path"      // navigation into the input: .a, .[], ..
	ClassLiteral   = "literal"   // a constant: 1, "text", true, $__loc__
	ClassConstruct = "construct" // data being built: {a: 1}, [ ... ]
	ClassControl   = "control"   // if, try, reduce, foreach, label
	ClassOperator  = "operator"  // a junction of two operands: +, ==, and, //
	ClassVariable  = "variable"  // a variable reference or a binding
	ClassDef       = "def"       // a user-defined function, shown as context
)

// ClassInfo describes one class for a legend.
type ClassInfo struct {
	Name        string
	Label       string
	Description string
}

// Classes lists the node classes in the order a legend should show them.
func Classes() []ClassInfo {
	return []ClassInfo{
		{ClassTerminal, "Start / End", "where the pipeline begins and ends"},
		{ClassPath, "Path", "navigation into the input: .a, .[], .."},
		{ClassCmdlet, "Cmdlet", "a pwrq cmdlet: get_childitem, sha256, format_table"},
		{ClassBuiltin, "Builtin", "a jq builtin or one of your own definitions"},
		{ClassOperator, "Operator", "a junction where two operands meet: +, ==, and, //"},
		{ClassConstruct, "Construct", "data being built: an object or an array"},
		{ClassControl, "Control flow", "if, try, reduce, foreach, label"},
		{ClassVariable, "Variable", "a binding or a reference to one: . as $x, $x"},
		{ClassLiteral, "Literal", "a constant value"},
		{ClassDef, "Definition", "a function you defined, shown as context"},
	}
}

// ClassStyle is the colour a class is drawn in.
type ClassStyle struct {
	Fill   string `json:"fill"`
	Stroke string `json:"stroke"`
	Font   string `json:"font"`
}

// Palette assigns a colour to every class.
type Palette map[string]ClassStyle

// darkPalette is tuned for D2's dark-mauve background: saturated enough to
// tell apart, dim enough that the labels stay the brightest thing on screen.
var darkPalette = Palette{
	ClassTerminal:  {"#2E3440", "#93A3BF", "#ECEFF4"},
	ClassPath:      {"#343156", "#A9A3EA", "#EDEBFF"},
	ClassCmdlet:    {"#1F3F6E", "#6E9BE0", "#EAF2FF"},
	ClassBuiltin:   {"#1B4A4E", "#4FD1C5", "#E3FFFB"},
	ClassOperator:  {"#2C333D", "#9AA5B1", "#E6EAF0"},
	ClassConstruct: {"#452747", "#D68FD6", "#FCEAFC"},
	ClassControl:   {"#54342A", "#E08A5F", "#FFEDE3"},
	ClassVariable:  {"#27412B", "#7FBF83", "#E9FBEA"},
	ClassLiteral:   {"#453520", "#D6A756", "#FFF6E5"},
	ClassDef:       {"#333845", "#7E8AA2", "#DDE3EC"},
}

// lightPalette is the same assignment of hue to meaning, inverted: pale fills
// under dark text, so the two themes are learnable as one scheme.
var lightPalette = Palette{
	ClassTerminal:  {"#E2E8F0", "#64748B", "#1F2933"},
	ClassPath:      {"#E6E4FB", "#6C63C7", "#241F5C"},
	ClassCmdlet:    {"#DCE9FF", "#3B76D0", "#10243F"},
	ClassBuiltin:   {"#D6F5F1", "#17A398", "#06302C"},
	ClassOperator:  {"#E9EDF2", "#7C8794", "#26303A"},
	ClassConstruct: {"#FADCF7", "#B75FB0", "#4A1345"},
	ClassControl:   {"#FDE3D3", "#D2733C", "#4E2410"},
	ClassVariable:  {"#DDF3DE", "#4C9A54", "#12331A"},
	ClassLiteral:   {"#FCEFD5", "#C08A2E", "#4A3406"},
	ClassDef:       {"#EBEEF3", "#808C9E", "#2A323C"},
}

// PaletteFor returns the palette for a theme name. Anything unrecognised gets
// the dark one, which is what the diagram has always been drawn in.
func PaletteFor(theme string) Palette {
	if strings.EqualFold(theme, "light") {
		return lightPalette
	}
	return darkPalette
}

// themeID maps a theme name onto the D2 theme that shares its background.
func themeID(theme string) int64 {
	if strings.EqualFold(theme, "light") {
		return 0 // neutral default
	}
	return 200 // dark mauve
}

// RenderOptions controls how a query is drawn.
//
// The zero value is the diagram pwrq has always produced: dark, laid out left
// to right by dagre, with cmdlets indistinguishable from builtins because
// nothing told the renderer which names are cmdlets.
type RenderOptions struct {
	// Cmdlets names the functions that come from pwrq rather than jq, so the
	// two can be coloured apart. The renderer cannot discover this itself: the
	// vocabulary depends on which registry the caller built, and the browser's
	// is deliberately smaller than the CLI's.
	Cmdlets map[string]bool

	// Theme is "dark" (default) or "light".
	Theme string

	// Direction is a D2 layout direction: right (default), down, left, up.
	Direction string

	// Layout is the engine: "dagre" (default) or "elk".
	Layout string

	// Sketch draws the diagram as if by hand.
	Sketch bool
}

func (o RenderOptions) direction() string {
	switch strings.ToLower(o.Direction) {
	case "down", "up", "left", "right":
		return strings.ToLower(o.Direction)
	default:
		return "right"
	}
}

func (o RenderOptions) layout() string {
	if strings.EqualFold(o.Layout, "elk") {
		return "elk"
	}
	return "dagre"
}

func (o RenderOptions) palette() Palette { return PaletteFor(o.Theme) }

// classDecls renders the D2 `classes` block. Styling by class rather than by
// node keeps the script readable and keeps the colour decisions in one place.
func classDecls(p Palette) string {
	names := make([]string, 0, len(p))
	for name := range p {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("classes: {\n")
	for _, name := range names {
		style := p[name]
		fmt.Fprintf(&b, "  %s: {\n", name)
		fmt.Fprintf(&b, "    style.fill: %q\n", style.Fill)
		fmt.Fprintf(&b, "    style.stroke: %q\n", style.Stroke)
		fmt.Fprintf(&b, "    style.font-color: %q\n", style.Font)
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}
