package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/xen0bit/pwrq/pkg/graph"
	"github.com/xen0bit/pwrq/pkg/ideengine"
)

// wideWidth is the width from which the input sits beside the output rather
// than behind a tab.
const wideWidth = 100

type layout struct {
	wide            bool
	queryH, bottomH int
	leftW, rightW   int
}

// layout divides the screen: a header, the query sized to its text (up to a
// third of the body), the panes below, and two lines of status.
func (m *model) layout() layout {
	body := max(8, m.height-3)
	content := max(2, min(m.query.Lines(), max(2, body/3)))
	l := layout{queryH: content + 2}
	l.bottomH = max(4, body-l.queryH)
	l.wide = m.width >= wideWidth
	if l.wide {
		l.leftW = max(30, m.width*2/5)
		l.rightW = m.width - l.leftW
	} else {
		l.rightW = m.width
	}
	return l
}

func (m *model) queryHeight() int { return m.layout().queryH - 2 }
func (m *model) paneHeight() int  { return m.layout().bottomH - 2 }

func (m *model) View() string {
	if m.width < 40 || m.height < 12 {
		return "The terminal is too small for pwrq's TUI (it needs 40×12).\nCtrl-C quits."
	}
	l := m.layout()

	lines := []string{m.header()}
	lines = append(lines, m.queryBox(l)...)

	if l.wide {
		left := m.paneBox(paneLeft, leftTabs, l.leftW, l.bottomH)
		right := m.paneBox(paneRight, rightTabs, l.rightW, l.bottomH)
		for i := range left {
			lines = append(lines, left[i]+right[i])
		}
	} else {
		p := paneRight
		if m.focus == paneLeft {
			p = paneLeft
		}
		lines = append(lines, m.paneBox(p, append(append([]tab{}, rightTabs...), leftTabs...), m.width, l.bottomH)...)
	}
	lines = append(lines, m.statusLine(), m.hintLine())

	if m.complete != nil && m.focus == paneQuery {
		m.drawCompletion(lines, l)
	}
	switch m.overlay {
	case overlayPalette:
		m.drawPalette(lines)
	case overlayPrompt:
		m.drawPrompt(lines)
	case overlayKeys:
		m.drawKeys(lines)
	}

	if m.osc != "" {
		lines[0] = m.osc + lines[0]
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// frame

func (m *model) header() string {
	th := m.th
	left := th.key.Render(" pwrq ") + th.faint.Render("·") + " "
	switch {
	case m.opts.InputLabel != "":
		left += th.muted.Render("input: " + m.opts.InputLabel)
	case m.input.Empty():
		left += th.faint.Render("no input")
	default:
		left += th.muted.Render("input: typed here")
	}
	// Where a query runs is the one thing worth always saying: here, as
	// this user, with everything that implies.
	cwd := m.cwd
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(cwd, home) {
		cwd = "~" + cwd[len(home):]
	}
	right := th.faint.Render("runs here as " + m.userName + " in " + cwd + " ")
	return joinEnds(left, right, m.width)
}

// joinEnds puts left and right at the two ends of a line, cutting the end
// of right when they do not both fit.
func joinEnds(left, right string, width int) string {
	lw, rw := ansi.StringWidth(left), ansi.StringWidth(right)
	if lw >= width {
		return pad(left, width)
	}
	if lw+rw+1 > width {
		right = ansi.Truncate(right, width-lw-1, "…")
		rw = ansi.StringWidth(right)
	}
	return left + strings.Repeat(" ", max(0, width-lw-rw)) + right
}

// box draws content inside a rounded border with a title in its top edge
// and a note at the right of it.
func (m *model) box(title, note string, content []string, w, h int, focused bool) []string {
	bs := m.th.border
	if focused {
		bs = m.th.borderFocused
	}
	inner := w - 2
	titleW, noteW := ansi.StringWidth(title), ansi.StringWidth(note)
	if titleW+noteW+3 > inner {
		note, noteW = "", 0
	}
	if titleW+2 > inner {
		title = ansi.Truncate(title, max(0, inner-2), "…")
		titleW = ansi.StringWidth(title)
	}
	fill := max(0, inner-2-titleW-noteW)
	top := bs.Render("╭─") + title + bs.Render(strings.Repeat("─", fill)) + note + bs.Render("─╮")
	out := []string{pad(top, w)}
	side := bs.Render("│")
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(content) {
			line = content[i]
		}
		out = append(out, side+pad(line, inner)+side)
	}
	out = append(out, bs.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return out
}

func (m *model) queryBox(l layout) []string {
	focused := m.focus == paneQuery && m.overlay == overlayNone
	title := m.th.title.Render(" Query ")
	if focused {
		title = m.th.titleFocused.Render(" Query ")
	}
	note := ""
	if m.stale() {
		note = m.th.faint.Render(" edited since the last run ")
	}
	content := m.query.Render(m.th, m.width-2, l.queryH-2, focused, true, m.queryKindsFunc(), m.errorSpan())
	return m.box(title, note, content, m.width, l.queryH, focused)
}

func (m *model) paneBox(p pane, tabs []tab, w, h int) []string {
	focused := m.focus == p && m.overlay == overlayNone
	shown := m.visibleTab(p)
	var parts []string
	for _, t := range tabs {
		name := tabNames[t]
		if t == tabArgs {
			name += argsNote(parseArgs(m.args.Value()))
		}
		switch {
		case t == shown && focused:
			parts = append(parts, m.th.tabActive.Render(name))
		case t == shown:
			parts = append(parts, m.th.bold.Render(name))
		default:
			parts = append(parts, m.th.tab.Render(name))
		}
	}
	title := " " + strings.Join(parts, m.th.faint.Render(" · ")) + " "
	content := m.paneContent(shown, w-2, h-2, focused)
	return m.box(title, "", content, w, h, focused)
}

// ---------------------------------------------------------------------------
// panes

func (m *model) paneContent(t tab, w, h int, focused bool) []string {
	switch t {
	case tabInput:
		return m.inputContent(w, h, focused)
	case tabArgs:
		return m.argsContent(w, h, focused)
	case tabOutput:
		return m.outputContent(w, h)
	case tabDiagram:
		return m.diagramContent(w, h)
	case tabCatalog:
		return m.catalogContent(w, h, focused)
	case tabExamples:
		return m.listContent(m.examplesList, "Enter loads the example", w, h, focused, m.exampleRow)
	case tabHistory:
		return m.listContent(m.historyList, "Enter loads it · Del deletes a saved one", w, h, focused, m.historyRow)
	}
	return nil
}

func (m *model) inputContent(w, h int, focused bool) []string {
	if m.input.Lines() == 1 && m.input.Line(0) == "" {
		return m.placeholder(m.input, w, h, focused,
			"Type or paste JSON here - a stream of values, as a file would hold.",
			"Or pipe it in:  … | pwrq --tui '.query'")
	}
	var kinds Kinds
	if !m.set.rawInput {
		kinds = func(_ int, line []rune) []string { return perRune(line, TokenizeJSON(line)) }
	}
	return m.input.Render(m.th, w, h, focused, true, kinds, nil)
}

func (m *model) argsContent(w, h int, focused bool) []string {
	if m.args.Lines() == 1 && m.args.Line(0) == "" {
		return m.placeholder(m.args, w, h, focused,
			"One variable per line, as --argjson binds it:",
			"  limit = 1000        name = \"ada\"")
	}
	kinds := func(_ int, line []rune) []string {
		eq := -1
		for i, r := range line {
			if r == '=' {
				eq = i
				break
			}
		}
		out := make([]string, len(line))
		if eq < 0 {
			for i := range out {
				out[i] = tokVariable
			}
			return out
		}
		for i := 0; i < eq; i++ {
			out[i] = tokVariable
		}
		out[eq] = tokPunct
		copy(out[eq+1:], perRune(line[eq+1:], TokenizeJSON(line[eq+1:])))
		return out
	}
	return m.args.Render(m.th, w, h, focused, false, kinds, nil)
}

// placeholder shows what an empty editor is for, with the cursor still in
// it.
func (m *model) placeholder(e *Editor, w, h int, focused bool, lines ...string) []string {
	out := e.Render(m.th, w, h, focused, false, nil, nil)
	for i, text := range lines {
		if i+1 < len(out) {
			out[i+1] = m.th.faint.Render(fit(text, w))
		}
	}
	return out
}

func colourJSON(line []rune) []string { return perRune(line, TokenizeJSON(line)) }

// perRune spreads tokens over the runes they cover.
func perRune(line []rune, tokens []Token) []string {
	out := make([]string, len(line))
	for _, token := range tokens {
		for i := token.Start; i < token.End && i < len(out); i++ {
			out[i] = token.Kind
		}
	}
	return out
}

func (m *model) outputContent(w, h int) []string {
	th := m.th
	if m.last == nil && !m.running {
		return []string{
			th.muted.Render(fit("Nothing has run yet. Ctrl-R runs the query.", w)),
			"",
			th.faint.Render(fit("Queries run here, as you, only when you ask:", w)),
			th.faint.Render(fit("editing validates the query but never runs it.", w)),
		}
	}
	out := []string{m.outputSummary(w)}
	height := h - 1
	m.outTop = max(0, min(m.outTop, len(m.outLines)-height))
	for i := m.outTop; i < m.outTop+height && i < len(m.outLines); i++ {
		line := m.outLines[i]
		switch {
		case line.head:
			out = append(out, th.bold.Render(fit("── "+line.text+" ──", w)))
		case line.debug:
			out = append(out, th.faint.Render(renderText(th, line.text, nil, m.outLeft, w, -1, nil, 0)))
		case m.lastRaw:
			out = append(out, renderText(th, line.text, nil, m.outLeft, w, -1, nil, 0))
		default:
			out = append(out, renderText(th, line.text, colourJSON, m.outLeft, w, -1, nil, 0))
		}
	}
	return out
}

func (m *model) outputSummary(w int) string {
	th := m.th
	if m.running {
		return th.accent.Render(fit(fmt.Sprintf("running… %s  (Esc cancels)", elapsed(time.Since(m.runStarted))), w))
	}
	r := m.last
	parts := []string{fmt.Sprintf("%d result%s", r.Count, plural(r.Count))}
	if !m.set.nullInput || r.InputCount > 0 {
		parts = append(parts, fmt.Sprintf("from %d input%s", r.InputCount, plural(r.InputCount)))
	}
	parts = append(parts, formatMs(r.ElapsedMs))
	summary := th.muted.Render(strings.Join(parts, " · "))
	if m.stale() {
		summary += th.warn.Render(" · stale: Ctrl-R reruns")
	}
	if r.Error != "" {
		style := th.danger
		label := r.Kind
		switch r.Kind {
		case "cancelled":
			style, label = th.warn, "cancelled"
		case "limit":
			style = th.warn
		case "halt":
			style = th.muted
		}
		msg := firstLine(r.Error)
		if r.Kind == "cancelled" {
			msg = "the run was stopped"
		}
		summary += th.faint.Render(" · ") + style.Render(label+": "+msg)
	}
	return pad(summary, w)
}

func elapsed(d time.Duration) string {
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func formatMs(ms float64) string {
	switch {
	case ms < 1:
		return fmt.Sprintf("%.2fms", ms)
	case ms < 1000:
		return fmt.Sprintf("%.1fms", ms)
	}
	return fmt.Sprintf("%.2fs", ms/1000)
}

func (m *model) diagramContent(w, h int) []string {
	m.refreshOutline()
	th := m.th
	if m.outlineErr != "" {
		return []string{th.danger.Render(fit("The query does not parse, so it cannot be drawn:", w)), th.muted.Render(fit(firstLine(m.outlineErr), w))}
	}
	// The legend, in the diagram's colours: only the classes this query uses.
	used := map[string]bool{}
	for _, line := range m.outlineLines {
		used[line.class] = true
	}
	var legend []string
	for _, class := range classOrder() {
		if used[class.name] {
			legend = append(legend, th.class(class.name).Render("■ "+class.label))
		}
	}
	out := []string{pad(strings.Join(legend, "  "), w)}
	height := h - 1
	m.diagTop = max(0, min(m.diagTop, len(m.outlineLines)-height))
	for i := m.diagTop; i < m.diagTop+height && i < len(m.outlineLines); i++ {
		line := m.outlineLines[i]
		out = append(out, pad(th.faint.Render(line.branch)+th.class(line.class).Render(oneLine(line.label)), w))
	}
	return out
}

func (m *model) catalogContent(w, h int, focused bool) []string {
	if m.helpFor == "" {
		return m.listContent(m.catalogList, "Enter inserts · ? get_help", w, h, focused, m.catalogRow)
	}
	th := m.th
	out := []string{pad(th.key.Render(m.helpFor)+th.faint.Render("  Enter inserts it · Esc back"), w)}
	height := h - 1
	for i := m.helpTop; i < m.helpTop+height && i < len(m.helpText); i++ {
		out = append(out, fit(m.helpText[i], w))
	}
	return out
}

// listContent draws a list under a line showing its filter.
func (m *model) listContent(l *list, hint string, w, h int, focused bool, row func(item, int) string) []string {
	th := m.th
	filter := th.faint.Render("type to filter · " + hint)
	if l.filter != "" {
		filter = th.accent.Render("filter: ") + l.filter + th.faint.Render(fmt.Sprintf("  (%d)", len(l.shown)))
	}
	return append([]string{pad(filter, w)}, l.render(th, w, h-1, focused, row)...)
}

func (m *model) catalogRow(it item, w int) string {
	cmd := it.data.(ideengine.Command)
	name := m.th.styleFor(styleCmdlet).Render(fmt.Sprintf("%-22s", it.title))
	meta := m.th.faint.Render(fmt.Sprintf("%-9s %-12s ", arity(cmd), truncateRunes(cmd.Category, 12)))
	return pad(name+" "+meta+m.th.muted.Render(it.detail), w)
}

func (m *model) exampleRow(it item, w int) string {
	return pad("  "+it.title+m.th.faint.Render("  "+it.detail), w)
}

func (m *model) historyRow(it item, w int) string {
	if it.kind == "saved" {
		return pad("  "+m.th.bold.Render(it.title)+m.th.faint.Render("  "+it.detail), w)
	}
	when := m.th.faint.Render(it.detail)
	return joinEnds("  "+m.th.styleFor(stylePunct).Render(it.title), when+" ", w)
}

// ---------------------------------------------------------------------------
// query colour and error

func (m *model) queryKindsFunc() Kinds {
	if m.kindsVersion != m.query.Version || m.queryKinds == nil {
		m.kindsVersion = m.query.Version
		src := []rune(m.query.Value())
		all := perRune(src, TokenizeQuery(src, m.vocab))
		m.queryKinds = m.queryKinds[:0]
		start := 0
		for i := 0; i <= len(src); i++ {
			if i == len(src) || src[i] == '\n' {
				m.queryKinds = append(m.queryKinds, all[start:i])
				start = i + 1
			}
		}
	}
	return func(row int, _ []rune) []string {
		if row < len(m.queryKinds) {
			return m.queryKinds[row]
		}
		return nil
	}
}

// errorSpan is where the query's parse error is, in runes, while the
// validation still describes the text on screen.
func (m *model) errorSpan() *Span {
	v := m.validation
	if v.OK || v.Error == "" || v.End <= v.Start || m.validatedFor.query != m.query.Version {
		return nil
	}
	src := m.query.Value()
	if v.End > len(src) {
		return nil
	}
	return &Span{Start: utf8.RuneCountInString(src[:v.Start]), End: utf8.RuneCountInString(src[:v.End])}
}

// ---------------------------------------------------------------------------
// status

func (m *model) statusLine() string {
	th := m.th
	var left string
	v := m.validation
	current := m.validatedFor.query == m.query.Version && m.validatedFor.args == m.args.Version
	switch {
	case !current:
		left = th.faint.Render(" … checking")
	case v.Empty:
		left = th.faint.Render(" empty query")
	case v.OK:
		left = th.ok.Render(" ✓ compiles")
	default:
		where := ""
		if v.Line > 0 {
			where = fmt.Sprintf("%d:%d ", v.Line, v.Column)
		}
		left = th.danger.Render(" ✗ " + where + firstLine(v.Error))
	}

	var mid string
	switch {
	case m.flashText != "":
		style := th.accent
		if m.flashErr {
			style = th.danger
		}
		mid = style.Render(m.flashText)
	case m.running:
		mid = th.accent.Render("running " + elapsed(time.Since(m.runStarted)) + " · Esc cancels")
	}

	var flags []string
	for _, f := range []struct {
		on   bool
		flag string
	}{{m.set.compact, "-c"}, {m.set.raw, "-r"}, {m.set.slurp, "-s"}, {m.set.nullInput, "-n"}, {m.set.rawInput, "-R"}} {
		if f.on {
			flags = append(flags, f.flag)
		}
	}
	right := strings.Join(flags, " ")
	if right != "" {
		right += " · "
	}
	right += fmt.Sprintf("limit %d · timeout %s", m.set.limit, fmtDuration(m.set.timeoutMs))
	if e := m.focusedEditor(); e != nil {
		row, col := e.Cursor()
		right += fmt.Sprintf(" · %d:%d", row+1, col+1)
	}
	right = th.faint.Render(right + " ")

	if mid != "" {
		left = ansi.Truncate(left, max(10, m.width/3), "…")
		left += "  " + mid
	}
	return joinEnds(left, right, m.width)
}

func (m *model) focusedEditor() *Editor {
	switch m.focus {
	case paneQuery:
		return m.query
	case paneLeft:
		if m.leftTab == tabArgs {
			return m.args
		}
		return m.input
	}
	return nil
}

func (m *model) hintLine() string {
	th := m.th
	hint := func(k, what string) string { return th.key.Render(k) + " " + th.faint.Render(what) }
	parts := []string{
		hint("^R", "run"),
		hint("Tab", "pane"),
		hint("^P", "palette"),
		hint("F1", "keys"),
		hint("^X", "accept ("+m.emitWhat()+")"),
		hint("^C", "quit"),
	}
	if m.focus == paneQuery {
		parts = append(parts[:2], append([]string{hint("^␣", "complete")}, parts[2:]...)...)
	}
	return pad(" "+strings.Join(parts, th.faint.Render("  ")), m.width)
}

// ---------------------------------------------------------------------------
// overlays

// overlayAt draws box over base with its top-left corner at x, y.
func overlayAt(base, box []string, x, y int) {
	for i, line := range box {
		row := y + i
		if row < 0 || row >= len(base) {
			continue
		}
		bw := ansi.StringWidth(line)
		left := ansi.Truncate(base[row], x, "")
		if lw := ansi.StringWidth(left); lw < x {
			left += strings.Repeat(" ", x-lw)
		}
		right := ansi.TruncateLeft(base[row], x+bw, "")
		base[row] = left + "\x1b[0m" + line + "\x1b[0m" + right
	}
}

func (m *model) drawCompletion(lines []string, l layout) {
	c := m.complete
	e := m.query
	row, col := e.Cursor()
	gutter := len(strconv.Itoa(e.Lines())) + 1
	wordCol := col - (c.word.End - c.word.Start)
	x := 1 + gutter - e.left
	for _, r := range []rune(e.Line(row))[:max(0, wordCol)] {
		x += cellWidth(r)
	}
	y := 2 + row - e.top

	rows := min(8, len(c.matches))
	width := min(64, m.width-2)
	x = max(0, min(x, m.width-width))
	if c.sel < c.top {
		c.top = c.sel
	}
	if c.sel >= c.top+rows {
		c.top = c.sel - rows + 1
	}

	var content []string
	for i := c.top; i < c.top+rows; i++ {
		item := c.matches[i]
		name := m.th.styleFor(m.completionStyle(item.kind)).Render(item.name)
		line := pad(" "+name+m.th.faint.Render("  "+item.kind+"  "+item.detail), width-2)
		if i == c.sel {
			line = m.th.selected.Render(ansi.Strip(line))
		}
		content = append(content, line)
	}
	note := m.th.faint.Render(fmt.Sprintf(" %d/%d · Tab ", c.sel+1, len(c.matches)))
	box := m.box("", note, content, width, rows+2, true)

	top := y + 1
	if top+len(box) > len(lines)-2 {
		top = y - len(box)
	}
	overlayAt(lines, box, x, top)
}

func (m *model) completionStyle(kind string) int {
	switch kind {
	case "cmdlet", "alias":
		return styleCmdlet
	case "keyword":
		return styleKeyword
	}
	return styleBuiltin
}

func (m *model) drawPalette(lines []string) {
	width := min(96, m.width-4)
	inner := width - 2
	filter := m.th.accent.Render("› ") + m.palette.filter + m.th.styleFor(styleCursor).Render(" ")
	content := []string{pad(filter, inner), m.th.faint.Render(strings.Repeat("─", inner))}
	rows := min(paletteRows, len(lines)-8)
	content = append(content, m.palette.render(m.th, inner, rows, true, func(it item, w int) string {
		kind := m.th.faint.Render(fmt.Sprintf("%-8s ", it.kind))
		return joinEnds(" "+kind+it.title, m.th.faint.Render(it.detail)+" ", w)
	})...)
	title := m.th.titleFocused.Render(" Palette ")
	box := m.box(title, m.th.faint.Render(" Enter · Esc "), content, width, len(content)+2, true)
	overlayAt(lines, box, (m.width-width)/2, 2)
}

func (m *model) drawPrompt(lines []string) {
	p := m.prompt
	width := min(80, m.width-4)
	inner := width - 2
	content := []string{
		m.th.faint.Render(fit(p.hint, inner)),
		p.editor.Render(m.th, inner, 1, true, false, nil, nil)[0],
	}
	title := m.th.titleFocused.Render(" " + p.title + " ")
	box := m.box(title, m.th.faint.Render(" Enter · Esc "), content, width, 4, true)
	overlayAt(lines, box, (m.width-width)/2, max(2, m.height/3))
}

func (m *model) drawKeys(lines []string) {
	width := min(90, m.width-4)
	inner := width - 2
	height := max(6, m.height-4)
	sheet := m.keySheet()
	m.keysTop = max(0, min(m.keysTop, len(sheet)-(height-2)))
	var content []string
	for i := m.keysTop; i < len(sheet) && len(content) < height-2; i++ {
		keys, what := sheet[i][0], sheet[i][1]
		if keys == "" {
			content = append(content, m.th.bold.Render(fit(what, inner)))
			continue
		}
		content = append(content, pad(" "+m.th.key.Render(fmt.Sprintf("%-22s", keys))+" "+what, inner))
	}
	title := m.th.titleFocused.Render(" Keys ")
	box := m.box(title, m.th.faint.Render(" ↑↓ · Esc "), content, width, height, true)
	overlayAt(lines, box, (m.width-width)/2, 2)
}

// classOrder is the diagram legend's order, with its labels.
func classOrder() []struct{ name, label string } {
	var out []struct{ name, label string }
	for _, c := range graph.Classes() {
		out = append(out, struct{ name, label string }{c.Name, c.Label})
	}
	return out
}

// fmtDuration writes a timeout the way a person would: 60s, 5m, 1h30m.
func fmtDuration(ms int) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Second:
		return d.String()
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0 && d >= 2*time.Minute:
		return fmt.Sprintf("%dm", d/time.Minute)
	case d%time.Second == 0 && d < 10*time.Minute:
		return fmt.Sprintf("%ds", d/time.Second)
	}
	return d.Round(time.Second).String()
}

// argsNote describes the bound arguments for the Args tab's title.
func argsNote(args []ideengine.Arg) string {
	if len(args) == 0 {
		return ""
	}
	return fmt.Sprintf(" (%d)", len(args))
}
