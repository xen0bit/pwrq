package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// item is one row of a list.
type item struct {
	title  string
	detail string
	// kind is a short tag drawn beside the title: cmdlet, example, saved.
	kind string
	// search is extra text the filter matches but the row does not show.
	search string
	// header marks a row that labels the rows below it; it is never
	// selected.
	header bool
	data   any
}

// list is a filterable, scrollable list: the catalog, the examples, the
// history and the palette are all one. Typing filters it.
type list struct {
	items  []item
	filter string
	shown  []int
	sel    int
	top    int
}

func newList(items []item) *list {
	l := &list{}
	l.setItems(items)
	return l
}

func (l *list) setItems(items []item) {
	l.items = items
	l.refilter()
}

// refilter keeps the rows every word of the filter appears in. A row whose
// title starts with the filter comes first, since that is usually the name
// being typed.
func (l *list) refilter() {
	words := strings.Fields(strings.ToLower(l.filter))
	var prefixed, rest []int
	for i, it := range l.items {
		if it.header {
			if len(words) == 0 {
				rest = append(rest, i)
			}
			continue
		}
		hay := strings.ToLower(it.title + " " + it.detail + " " + it.kind + " " + it.search)
		matched := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		if len(words) > 0 && strings.HasPrefix(strings.ToLower(it.title), words[0]) {
			prefixed = append(prefixed, i)
		} else {
			rest = append(rest, i)
		}
	}
	l.shown = append(prefixed, rest...)
	l.sel, l.top = 0, 0
	l.skipHeaders(1)
}

func (l *list) skipHeaders(dir int) {
	for l.sel >= 0 && l.sel < len(l.shown) && l.items[l.shown[l.sel]].header {
		l.sel += dir
	}
	if l.sel < 0 || l.sel >= len(l.shown) {
		l.sel = max(0, min(l.sel, len(l.shown)-1))
		if dir > 0 {
			l.skipHeaders(-1)
		}
	}
}

// selected is the highlighted row, if there is one.
func (l *list) selected() (item, bool) {
	if l.sel < 0 || l.sel >= len(l.shown) {
		return item{}, false
	}
	it := l.items[l.shown[l.sel]]
	return it, !it.header
}

func (l *list) move(n int) {
	if len(l.shown) == 0 {
		return
	}
	dir := 1
	if n < 0 {
		dir = -1
	}
	l.sel = max(0, min(l.sel+n, len(l.shown)-1))
	if l.items[l.shown[l.sel]].header {
		before := l.sel
		l.skipHeaders(dir)
		if l.items[l.shown[l.sel]].header {
			l.sel = before
			l.skipHeaders(-dir)
		}
	}
}

// handleKey applies navigation and filter keys, reporting whether it used
// the key. Enter and the like are the caller's.
func (l *list) handleKey(msg tea.KeyMsg, height int) bool {
	switch msg.Type {
	case tea.KeyRunes:
		if msg.Alt {
			return false
		}
		l.filter += string(msg.Runes)
		l.refilter()
		return true
	case tea.KeySpace:
		l.filter += " "
		l.refilter()
		return true
	}
	switch msg.String() {
	case "up", "ctrl+p":
		l.move(-1)
	case "down", "ctrl+n":
		l.move(1)
	case "pgup":
		l.move(-max(1, height-1))
	case "pgdown":
		l.move(max(1, height-1))
	case "home":
		l.sel = 0
		l.skipHeaders(1)
	case "end":
		l.sel = len(l.shown) - 1
		l.skipHeaders(-1)
	case "backspace", "ctrl+h":
		if l.filter == "" {
			return false
		}
		runes := []rune(l.filter)
		l.filter = string(runes[:len(runes)-1])
		l.refilter()
	case "ctrl+u":
		l.filter = ""
		l.refilter()
	default:
		return false
	}
	return true
}

// render draws the rows that fit, each exactly width cells, keeping the
// selection in view.
func (l *list) render(th *theme, width, height int, focused bool, row func(it item, width int) string) []string {
	if l.sel < l.top {
		l.top = l.sel
	}
	if l.sel >= l.top+height {
		l.top = l.sel - height + 1
	}
	l.top = max(0, l.top)

	out := make([]string, 0, height)
	for i := l.top; i < l.top+height && i < len(l.shown); i++ {
		it := l.items[l.shown[i]]
		var line string
		if it.header {
			line = th.bold.Render(fit(it.title, width))
		} else {
			line = row(it, width)
		}
		line = pad(line, width)
		if i == l.sel && focused && !it.header {
			line = th.selected.Render(ansi.Strip(line))
		}
		out = append(out, line)
	}
	if len(l.shown) == 0 && height > 0 {
		out = append(out, th.faint.Render(fit("  nothing matches", width)))
	}
	for len(out) < height {
		out = append(out, strings.Repeat(" ", width))
	}
	return out
}

// fit cuts plain text to width cells, marking the cut.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

// pad cuts or fills a styled line to exactly width cells.
func pad(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		return ansi.Truncate(s, width, "…")
	}
	return s + strings.Repeat(" ", width-w)
}
