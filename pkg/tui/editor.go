package tui

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

// Editor is a multi-line text buffer with a cursor, undo, and a view that
// scrolls to keep the cursor visible. It holds the query, the input and the
// arguments.
//
// The stock textarea cannot colour tokens or underline an error span, which
// are the two things a query editor is for, so this is its own.
//
// Lines are kept as strings and only the line being edited is turned into
// runes: the input pane can hold megabytes piped from stdin, and a buffer of
// runes would be four times the size for nothing. Columns count runes.
type Editor struct {
	lines    []string
	row, col int
	// goal is the column vertical movement aims for, so moving through a
	// short line and back does not lose the place.
	goal int
	// top is the first visible line and left the first visible cell.
	top, left int

	undo, redo []snapshot
	lastEdit   editKind
	// Version changes on every edit, so a caller can tell whether what it
	// computed from the text is stale.
	Version int
}

type snapshot struct {
	lines    []string
	row, col int
}

type editKind int

const (
	editNone editKind = iota
	editInsert
	editDelete
	editOther
)

// undoDepth bounds the history. A snapshot shares the unchanged lines'
// strings, but still copies one slot per line, so a very long buffer keeps
// fewer of them.
func (e *Editor) undoDepth() int {
	if len(e.lines) > 50000 {
		return 5
	}
	return 200
}

// NewEditor returns an editor holding text, with the cursor at the start.
func NewEditor(text string) *Editor {
	e := &Editor{}
	e.lines = splitLines(text)
	return e
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(text, "\n")
}

// Value is the buffer's text.
func (e *Editor) Value() string { return strings.Join(e.lines, "\n") }

// Empty reports whether the buffer holds only whitespace.
func (e *Editor) Empty() bool {
	for _, line := range e.lines {
		if strings.TrimSpace(line) != "" {
			return false
		}
	}
	return true
}

// Lines is the number of lines in the buffer.
func (e *Editor) Lines() int { return len(e.lines) }

// Line returns one line's text.
func (e *Editor) Line(row int) string { return e.lines[row] }

// Cursor is the cursor's line and column, both from zero.
func (e *Editor) Cursor() (int, int) { return e.row, e.col }

// SetValue replaces the text. The replacement can be undone, which is what
// makes Format and Inline safe to press.
func (e *Editor) SetValue(text string) {
	if text == e.Value() {
		return
	}
	e.checkpoint(editOther)
	e.lines = splitLines(text)
	e.row = min(e.row, len(e.lines)-1)
	e.col = min(e.col, runeLen(e.lines[e.row]))
	e.goal = e.col
	e.changed()
}

// Reset replaces the text and forgets the history, for a buffer that now
// holds something else entirely.
func (e *Editor) Reset(text string) {
	e.lines = splitLines(text)
	e.row, e.col, e.goal, e.top, e.left = 0, 0, 0, 0, 0
	e.undo, e.redo = nil, nil
	e.lastEdit = editNone
	e.Version++
}

// Offset is the cursor's position in the whole text, in runes.
func (e *Editor) Offset() int {
	off := 0
	for i := 0; i < e.row; i++ {
		off += runeLen(e.lines[i]) + 1
	}
	return off + e.col
}

// SetOffset moves the cursor to a position in the whole text.
func (e *Editor) SetOffset(off int) {
	for row, line := range e.lines {
		n := runeLen(line)
		if off <= n || row == len(e.lines)-1 {
			e.row, e.col = row, max(0, min(off, n))
			e.goal = e.col
			return
		}
		off -= n + 1
	}
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func (e *Editor) changed() {
	e.Version++
	e.redo = nil
}

// checkpoint records the state before an edit. Consecutive edits of the same
// kind - a run of typing, a run of deleting - undo together.
func (e *Editor) checkpoint(kind editKind) {
	if kind != editOther && kind == e.lastEdit {
		return
	}
	e.lastEdit = kind
	e.undo = append(e.undo, e.snapshot())
	if over := len(e.undo) - e.undoDepth(); over > 0 {
		e.undo = e.undo[over:]
	}
}

func (e *Editor) snapshot() snapshot {
	return snapshot{lines: append([]string(nil), e.lines...), row: e.row, col: e.col}
}

func (e *Editor) restore(s snapshot) {
	e.lines, e.row, e.col = s.lines, s.row, s.col
	e.goal = e.col
	e.lastEdit = editNone
	e.Version++
}

// Undo reverts the last edit, reporting whether there was one.
func (e *Editor) Undo() bool {
	if len(e.undo) == 0 {
		return false
	}
	e.redo = append(e.redo, e.snapshot())
	last := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.restore(last)
	return true
}

// Redo reapplies an undone edit, reporting whether there was one.
func (e *Editor) Redo() bool {
	if len(e.redo) == 0 {
		return false
	}
	e.undo = append(e.undo, e.snapshot())
	next := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.restore(next)
	return true
}

// InsertText types text at the cursor. Newlines split the line.
func (e *Editor) InsertText(text string) {
	if text == "" {
		return
	}
	e.checkpoint(editInsert)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	line := []rune(e.lines[e.row])
	head, tail := string(line[:e.col]), string(line[e.col:])
	parts := strings.Split(text, "\n")
	if len(parts) == 1 {
		e.lines[e.row] = head + text + tail
		e.col += runeLen(text)
	} else {
		inserted := make([]string, len(parts))
		inserted[0] = head + parts[0]
		copy(inserted[1:], parts[1:])
		last := len(parts) - 1
		inserted[last] = parts[last] + tail
		e.lines = append(e.lines[:e.row], append(inserted, e.lines[e.row+1:]...)...)
		e.row += last
		e.col = runeLen(parts[last])
	}
	e.goal = e.col
	e.changed()
}

// Newline splits the line at the cursor and carries its indentation over,
// which is what a pipeline spread across lines wants.
func (e *Editor) Newline() {
	indent := leadingSpace(e.lines[e.row])
	if runeLen(indent) > e.col {
		indent = ""
	}
	e.checkpoint(editOther)
	e.lastEdit = editInsert
	line := []rune(e.lines[e.row])
	head, tail := string(line[:e.col]), string(line[e.col:])
	e.lines[e.row] = head
	e.lines = append(e.lines[:e.row+1], append([]string{indent + tail}, e.lines[e.row+1:]...)...)
	e.row++
	e.col = runeLen(indent)
	e.goal = e.col
	e.changed()
}

func leadingSpace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

// Backspace deletes the rune before the cursor, joining lines at the start of
// one.
func (e *Editor) Backspace() {
	if e.col == 0 && e.row == 0 {
		return
	}
	e.checkpoint(editDelete)
	if e.col == 0 {
		prev := e.lines[e.row-1]
		e.col = runeLen(prev)
		e.lines[e.row-1] = prev + e.lines[e.row]
		e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
		e.row--
	} else {
		line := []rune(e.lines[e.row])
		e.lines[e.row] = string(line[:e.col-1]) + string(line[e.col:])
		e.col--
	}
	e.goal = e.col
	e.changed()
}

// Delete deletes the rune under the cursor, joining lines at the end of one.
func (e *Editor) Delete() {
	line := []rune(e.lines[e.row])
	if e.col == len(line) && e.row == len(e.lines)-1 {
		return
	}
	e.checkpoint(editDelete)
	if e.col == len(line) {
		e.lines[e.row] += e.lines[e.row+1]
		e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
	} else {
		e.lines[e.row] = string(line[:e.col]) + string(line[e.col+1:])
	}
	e.changed()
}

// DeleteWordBack deletes back to the start of the word before the cursor.
func (e *Editor) DeleteWordBack() {
	if e.col == 0 {
		e.Backspace()
		return
	}
	e.checkpoint(editOther)
	line := []rune(e.lines[e.row])
	start := wordStart(line, e.col)
	e.lines[e.row] = string(line[:start]) + string(line[e.col:])
	e.col = start
	e.goal = e.col
	e.changed()
}

// KillToEnd deletes from the cursor to the end of the line, or the line break
// when the cursor is already there.
func (e *Editor) KillToEnd() {
	line := []rune(e.lines[e.row])
	if e.col == len(line) {
		e.Delete()
		return
	}
	e.checkpoint(editOther)
	e.lines[e.row] = string(line[:e.col])
	e.changed()
}

// KillToStart deletes from the start of the line to the cursor.
func (e *Editor) KillToStart() {
	if e.col == 0 {
		return
	}
	e.checkpoint(editOther)
	line := []rune(e.lines[e.row])
	e.lines[e.row] = string(line[e.col:])
	e.col, e.goal = 0, 0
	e.changed()
}

// ToggleComment comments the cursor's line out with jq's #, or back in.
func (e *Editor) ToggleComment() {
	e.checkpoint(editOther)
	line := e.lines[e.row]
	indent := leadingSpace(line)
	body := line[len(indent):]
	shift := 0
	switch {
	case strings.HasPrefix(body, "# "):
		e.lines[e.row] = indent + body[2:]
		shift = -2
	case strings.HasPrefix(body, "#"):
		e.lines[e.row] = indent + body[1:]
		shift = -1
	default:
		e.lines[e.row] = indent + "# " + body
		shift = 2
	}
	if e.col >= runeLen(indent) {
		e.col = max(runeLen(indent), e.col+shift)
	}
	e.col = min(e.col, runeLen(e.lines[e.row]))
	e.goal = e.col
	e.changed()
}

// Replace swaps the runes between two offsets of the whole text for text,
// leaving the cursor after it. Completion uses it to replace the word being
// typed.
func (e *Editor) Replace(start, end int, text string) {
	e.SetOffset(end)
	for i := start; i < end; i++ {
		e.Backspace()
	}
	e.InsertText(text)
}

func wordStart(line []rune, col int) int {
	i := col
	for i > 0 && unicode.IsSpace(line[i-1]) {
		i--
	}
	if i > 0 && isWord(line[i-1]) {
		for i > 0 && isWord(line[i-1]) {
			i--
		}
	} else if i > 0 {
		i--
	}
	return i
}

func wordEnd(line []rune, col int) int {
	i := col
	for i < len(line) && unicode.IsSpace(line[i]) {
		i++
	}
	if i < len(line) && isWord(line[i]) {
		for i < len(line) && isWord(line[i]) {
			i++
		}
	} else if i < len(line) {
		i++
	}
	return i
}

func (e *Editor) move(row, col int, keepGoal bool) {
	e.row = max(0, min(row, len(e.lines)-1))
	e.col = max(0, min(col, runeLen(e.lines[e.row])))
	if !keepGoal {
		e.goal = e.col
	}
	e.lastEdit = editNone
}

// HandleKey applies an editing or movement key, reporting whether the key was
// one, and whether it changed the text. height is the view's height, which
// paging moves by.
func (e *Editor) HandleKey(msg tea.KeyMsg, height int) (handled, changed bool) {
	before := e.Version
	// Counted, not converted: a minified document is one line, and a line
	// of runes per keystroke would be megabytes per keystroke.
	n := runeLen(e.lines[e.row])

	switch msg.Type {
	case tea.KeyRunes:
		if msg.Alt {
			return false, false
		}
		e.InsertText(string(msg.Runes))
		return true, true
	case tea.KeySpace:
		e.InsertText(" ")
		return true, true
	}

	switch msg.String() {
	case "left":
		if e.col == 0 && e.row > 0 {
			e.move(e.row-1, runeLen(e.lines[e.row-1]), false)
		} else {
			e.move(e.row, e.col-1, false)
		}
	case "right":
		if e.col == n && e.row < len(e.lines)-1 {
			e.move(e.row+1, 0, false)
		} else {
			e.move(e.row, e.col+1, false)
		}
	case "up":
		if e.row == 0 {
			e.move(0, 0, false)
		} else {
			e.move(e.row-1, e.goal, true)
		}
	case "down":
		if e.row == len(e.lines)-1 {
			e.move(e.row, n, false)
		} else {
			e.move(e.row+1, e.goal, true)
		}
	case "home", "ctrl+a":
		// First to the text, then to the margin.
		indent := runeLen(leadingSpace(e.lines[e.row]))
		if e.col == indent {
			indent = 0
		}
		e.move(e.row, indent, false)
	case "end", "ctrl+e":
		e.move(e.row, n, false)
	case "ctrl+left", "alt+left", "alt+b":
		if e.col == 0 && e.row > 0 {
			e.move(e.row-1, runeLen(e.lines[e.row-1]), false)
		} else {
			e.move(e.row, wordStart([]rune(e.lines[e.row]), e.col), false)
		}
	case "ctrl+right", "alt+right":
		if e.col == n && e.row < len(e.lines)-1 {
			e.move(e.row+1, 0, false)
		} else {
			e.move(e.row, wordEnd([]rune(e.lines[e.row]), e.col), false)
		}
	case "pgup":
		e.move(e.row-max(1, height-1), e.goal, true)
	case "pgdown":
		e.move(e.row+max(1, height-1), e.goal, true)
	case "ctrl+home":
		e.move(0, 0, false)
	case "ctrl+end":
		e.move(len(e.lines)-1, runeLen(e.lines[len(e.lines)-1]), false)
	case "enter", "ctrl+j":
		e.Newline()
	case "backspace", "ctrl+h":
		e.Backspace()
	case "delete", "ctrl+d":
		e.Delete()
	case "ctrl+w", "alt+backspace":
		e.DeleteWordBack()
	case "ctrl+k":
		e.KillToEnd()
	case "ctrl+u":
		e.KillToStart()
	case "ctrl+z":
		e.Undo()
	case "ctrl+y":
		e.Redo()
	case "ctrl+_", "ctrl+/":
		// Terminals send Ctrl-/ as Ctrl-_.
		e.ToggleComment()
	default:
		return false, false
	}
	return true, e.Version != before
}

// cellWidth is how many cells a rune takes on screen. A tab is drawn as
// tabWidth spaces and any other control character as one placeholder cell.
func cellWidth(r rune) int {
	switch {
	case r == '\t':
		return tabWidth
	case r < 0x20 || r == 0x7f:
		return 1
	}
	if w := runewidth.RuneWidth(r); w > 0 {
		return w
	}
	return 1
}

const tabWidth = 4

// displayRune is what is drawn for a rune: tabs and control characters are
// made visible without moving anything else.
func displayRune(r rune) string {
	switch {
	case r == '\t':
		return strings.Repeat(" ", tabWidth)
	case r < 0x20 || r == 0x7f:
		return "·"
	}
	if runewidth.RuneWidth(r) == 0 {
		return " "
	}
	return string(r)
}

// scrollTo keeps the cursor inside a view of the given size.
func (e *Editor) scrollTo(width, height int) {
	if e.row < e.top {
		e.top = e.row
	}
	if e.row >= e.top+height {
		e.top = e.row - height + 1
	}
	e.top = max(0, min(e.top, max(0, len(e.lines)-1)))

	x, col := 0, 0
	for _, r := range e.lines[e.row] {
		if col == e.col {
			break
		}
		x += cellWidth(r)
		col++
	}
	if x < e.left {
		e.left = max(0, x-width/4)
	}
	if x >= e.left+width {
		e.left = x - width + width/4 + 1
	}
}

// Kinds colours a line: it returns a token kind for each rune of line row,
// or nil for none.
type Kinds func(row int, line []rune) []string

// Span marks a range of the whole text, in runes: where an error is.
type Span struct{ Start, End int }

// Render draws the visible part of the buffer: height lines, each exactly
// width cells, with a line-number gutter when numbers is set. The cursor is
// drawn only when focused.
func (e *Editor) Render(th *theme, width, height int, focused, numbers bool, kinds Kinds, mark *Span) []string {
	gutter := 0
	if numbers {
		gutter = len(strconv.Itoa(len(e.lines))) + 1
	}
	textWidth := max(1, width-gutter)
	e.scrollTo(textWidth, height)

	// Offsets of the first visible line, so the error span can be tested
	// per rune.
	offset := 0
	if mark != nil {
		for i := 0; i < e.top && i < len(e.lines); i++ {
			offset += runeLen(e.lines[i]) + 1
		}
	}

	out := make([]string, 0, height)
	for row := e.top; row < e.top+height; row++ {
		if row >= len(e.lines) {
			out = append(out, strings.Repeat(" ", width))
			continue
		}
		var b strings.Builder
		if numbers {
			num := strconv.Itoa(row + 1)
			style := th.gutter
			if row == e.row && focused {
				style = th.gutterActive
			}
			b.WriteString(style.Render(strings.Repeat(" ", gutter-1-len(num)) + num + " "))
		}

		var colour func([]rune) []string
		if kinds != nil {
			colour = func(line []rune) []string { return kinds(row, line) }
		}
		cursorCol := -1
		if focused && row == e.row {
			cursorCol = e.col
		}
		b.WriteString(renderText(th, e.lines[row], colour, e.left, textWidth, cursorCol, mark, offset))
		out = append(out, b.String())
		if mark != nil {
			offset += runeLen(e.lines[row]) + 1
		}
	}
	return out
}

// longLine is the length past which a line is drawn from the part that is
// on screen, uncoloured, rather than whole.
const longLine = 4096

// renderText draws one line of text from cell left, width cells wide. A
// line of ordinary length is coloured whole; a longer one - a minified
// document is a single line of megabytes - is decoded only as far as the
// screen reaches, and not coloured, since colouring needs the whole line and
// every frame would pay for it.
func renderText(th *theme, s string, colour func([]rune) []string, left, width, cursorCol int, mark *Span, offset int) string {
	if len(s) <= longLine {
		runes := []rune(s)
		var kinds []string
		if colour != nil {
			kinds = colour(runes)
		}
		return renderLine(th, runes, kinds, left, width, cursorCol, mark, offset)
	}

	// Skip to the first rune that reaches the left edge.
	x, col, i := 0, 0, 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		w := cellWidth(r)
		if x+w > left {
			break
		}
		x += w
		col++
		i += size
	}
	// Take what fills the width, and one more for a cursor at the end.
	var runes []rune
	for cells := x - left; i < len(s) && cells <= width; {
		r, size := utf8.DecodeRuneInString(s[i:])
		runes = append(runes, r)
		cells += cellWidth(r)
		i += size
	}
	if cursorCol >= 0 {
		cursorCol -= col
	}
	return renderLine(th, runes, nil, left-x, width, cursorCol, mark, offset+col)
}

// renderLine draws one line from cell left, textWidth cells wide, padding to
// the full width. Runes of one style are drawn together.
func renderLine(th *theme, line []rune, kinds []string, left, textWidth, cursorCol int, mark *Span, offset int) string {
	var b strings.Builder
	x := 0
	drawn := 0
	var run strings.Builder
	runStyle := -1
	flush := func() {
		if run.Len() > 0 {
			b.WriteString(th.styleFor(runStyle).Render(run.String()))
			run.Reset()
		}
	}
	emit := func(style int, text string, w int) {
		if style != runStyle {
			flush()
			runStyle = style
		}
		run.WriteString(text)
		drawn += w
	}

	for i := 0; i <= len(line); i++ {
		if i == len(line) {
			if i == cursorCol && x >= left && drawn < textWidth {
				emit(styleCursor, " ", 1)
			}
			break
		}
		r := line[i]
		w := cellWidth(r)
		if x+w <= left {
			x += w
			continue
		}
		if x < left {
			// A wide rune straddling the left edge: show its visible half as
			// blank rather than shifting the line.
			emit(styleNone, strings.Repeat(" ", x+w-left), x+w-left)
			x += w
			continue
		}
		if drawn+w > textWidth {
			break
		}
		kind := ""
		if i < len(kinds) {
			kind = kinds[i]
		}
		style := th.kindStyle(kind)
		if mark != nil && offset+i >= mark.Start && offset+i < mark.End {
			style |= styleError
		}
		if i == cursorCol {
			style = styleCursor
		}
		emit(style, displayRune(r), w)
		x += w
	}
	// The error at the end of a line is a missing token: mark the space
	// after the text so there is something to see.
	if mark != nil && mark.Start == offset+len(line) && mark.End > mark.Start && drawn < textWidth && cursorCol != len(line) {
		emit(styleError, " ", 1)
	}
	flush()
	if drawn < textWidth {
		b.WriteString(strings.Repeat(" ", textWidth-drawn))
	}
	return b.String()
}
