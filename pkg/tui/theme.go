package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/xen0bit/pwrq/pkg/graph"
)

// Styles for text in an editor or the output. A style is a small integer so a
// line can be drawn in runs of the same one; styleError is a flag on top of
// any of the others.
const (
	styleNone = iota
	styleComment
	styleString
	styleNumber
	styleKeyword
	styleCmdlet
	styleBuiltin
	styleVariable
	styleField
	styleFormat
	stylePunct
	styleKey
	styleCursor

	styleError = 1 << 8
)

var kindStyles = map[string]int{
	tokComment:  styleComment,
	tokString:   styleString,
	tokInterp:   stylePunct,
	tokNumber:   styleNumber,
	tokKeyword:  styleKeyword,
	tokLiteral:  styleKeyword,
	tokCmdlet:   styleCmdlet,
	tokBuiltin:  styleBuiltin,
	tokVariable: styleVariable,
	tokField:    styleField,
	tokFormat:   styleFormat,
	tokPunct:    stylePunct,
	tokKey:      styleKey,
}

// The page's syntax colours (pkg/web/src/css/app.css), light and dark, so a
// query looks the same in either editor.
var syntaxColours = map[int]lipgloss.AdaptiveColor{
	styleComment:  {Light: "#7c8794", Dark: "#6d7686"},
	styleString:   {Light: "#1f7a4d", Dark: "#8fd6a6"},
	styleNumber:   {Light: "#a35b00", Dark: "#e0b166"},
	styleKeyword:  {Light: "#9b2fa8", Dark: "#d79be6"},
	styleCmdlet:   {Light: "#2a5db0", Dark: "#7fb0ff"},
	styleBuiltin:  {Light: "#0f7c74", Dark: "#59d9cd"},
	styleVariable: {Light: "#b1600a", Dark: "#f0b27a"},
	styleField:    {Light: "#3f3aa8", Dark: "#b3aef5"},
	styleFormat:   {Light: "#b1600a", Dark: "#f0b27a"},
	stylePunct:    {Light: "#6b7480", Dark: "#8d97a6"},
	styleKey:      {Light: "#2a5db0", Dark: "#7fb0ff"},
}

var (
	colourAccent = lipgloss.AdaptiveColor{Light: "#2a5db0", Dark: "#7fb0ff"}
	colourFaint  = lipgloss.AdaptiveColor{Light: "#8a929c", Dark: "#5f6773"}
	colourMuted  = lipgloss.AdaptiveColor{Light: "#5b6470", Dark: "#9aa3af"}
	colourDanger = lipgloss.AdaptiveColor{Light: "#c0392b", Dark: "#ff7b72"}
	colourOK     = lipgloss.AdaptiveColor{Light: "#1f7a4d", Dark: "#8fd6a6"}
	colourWarn   = lipgloss.AdaptiveColor{Light: "#a35b00", Dark: "#e0b166"}
	colourSelect = lipgloss.AdaptiveColor{Light: "#dce9ff", Dark: "#1f3f6e"}
)

// theme holds every style the UI draws with, made from one renderer so that
// colour follows the terminal the UI is actually drawn on - which, when
// stdout is a pipe, is not stdout.
type theme struct {
	r      *lipgloss.Renderer
	styles map[int]lipgloss.Style

	gutter, gutterActive       lipgloss.Style
	border, borderFocused      lipgloss.Style
	title, titleFocused        lipgloss.Style
	tab, tabActive             lipgloss.Style
	faint, muted, bold, accent lipgloss.Style
	ok, warn, danger           lipgloss.Style
	selected                   lipgloss.Style
	key                        lipgloss.Style
	classes                    map[string]lipgloss.Style
}

func newTheme(r *lipgloss.Renderer) *theme {
	s := func() lipgloss.Style { return r.NewStyle() }
	th := &theme{
		r:             r,
		styles:        map[int]lipgloss.Style{},
		gutter:        s().Foreground(colourFaint),
		gutterActive:  s().Foreground(colourMuted),
		border:        s().Foreground(colourFaint),
		borderFocused: s().Foreground(colourAccent),
		title:         s().Foreground(colourMuted),
		titleFocused:  s().Foreground(colourAccent).Bold(true),
		tab:           s().Foreground(colourMuted),
		tabActive:     s().Foreground(colourAccent).Bold(true).Underline(true),
		faint:         s().Foreground(colourFaint),
		muted:         s().Foreground(colourMuted),
		bold:          s().Bold(true),
		accent:        s().Foreground(colourAccent),
		ok:            s().Foreground(colourOK),
		warn:          s().Foreground(colourWarn),
		danger:        s().Foreground(colourDanger),
		selected:      s().Background(colourSelect),
		key:           s().Foreground(colourAccent).Bold(true),
		classes:       map[string]lipgloss.Style{},
	}

	// The outline is coloured by the diagram's own palette, so a node is the
	// same colour in the terminal as in the picture.
	dark, light := graph.PaletteFor("dark"), graph.PaletteFor("light")
	for _, class := range graph.Classes() {
		th.classes[class.Name] = s().Foreground(lipgloss.AdaptiveColor{
			Light: light[class.Name].Stroke,
			Dark:  dark[class.Name].Stroke,
		})
	}
	return th
}

func (th *theme) kindStyle(kind string) int { return kindStyles[kind] }

// styleFor builds, once, the lipgloss style for a text style.
func (th *theme) styleFor(style int) lipgloss.Style {
	if cached, ok := th.styles[style]; ok {
		return cached
	}
	st := th.r.NewStyle()
	base := style &^ styleError
	switch base {
	case styleCursor:
		st = st.Reverse(true)
	case styleComment:
		st = st.Foreground(syntaxColours[base]).Italic(true)
	case styleKeyword, styleCmdlet:
		st = st.Foreground(syntaxColours[base]).Bold(true)
	default:
		if colour, ok := syntaxColours[base]; ok {
			st = st.Foreground(colour)
		}
	}
	if style&styleError != 0 {
		st = st.Underline(true).Foreground(colourDanger)
	}
	th.styles[style] = st
	return st
}

func (th *theme) class(name string) lipgloss.Style {
	if st, ok := th.classes[name]; ok {
		return st
	}
	return th.muted
}
