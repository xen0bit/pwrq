package tui

import (
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// plainTheme draws without colour, so a test can read what is on screen.
func plainTheme() *theme {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.Ascii)
	return newTheme(r)
}

func key(s string) tea.KeyMsg {
	switch s {
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "f1":
		return tea.KeyMsg{Type: tea.KeyF1}
	}
	if strings.HasPrefix(s, "ctrl+") {
		for t, name := range ctrlKeys {
			if name == s {
				return tea.KeyMsg{Type: t}
			}
		}
	}
	if strings.HasPrefix(s, "alt+") {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s[4:]), Alt: true}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

var ctrlKeys = map[tea.KeyType]string{
	tea.KeyCtrlA: "ctrl+a", tea.KeyCtrlC: "ctrl+c", tea.KeyCtrlD: "ctrl+d", tea.KeyCtrlE: "ctrl+e",
	tea.KeyCtrlK: "ctrl+k", tea.KeyCtrlL: "ctrl+l", tea.KeyCtrlP: "ctrl+p", tea.KeyCtrlR: "ctrl+r",
	tea.KeyCtrlS: "ctrl+s", tea.KeyCtrlU: "ctrl+u", tea.KeyCtrlW: "ctrl+w", tea.KeyCtrlX: "ctrl+x",
	tea.KeyCtrlY: "ctrl+y", tea.KeyCtrlZ: "ctrl+z", tea.KeyCtrlUnderscore: "ctrl+_", tea.KeyCtrlAt: "ctrl+@",
}

func typeInto(e *Editor, keys ...string) {
	for _, k := range keys {
		e.HandleKey(key(k), 10)
	}
}

func TestEditorTypesAndSplitsLines(t *testing.T) {
	e := NewEditor("")
	typeInto(e, ".a", " ", "|", "enter", ".b")
	if got := e.Value(); got != ".a |\n.b" {
		t.Errorf("value = %q", got)
	}
	if row, col := e.Cursor(); row != 1 || col != 2 {
		t.Errorf("cursor = %d:%d, want 1:2", row, col)
	}
}

func TestEditorNewlineKeepsIndentation(t *testing.T) {
	e := NewEditor("  .a")
	e.SetOffset(4)
	typeInto(e, "enter", "|")
	if got := e.Value(); got != "  .a\n  |" {
		t.Errorf("value = %q", got)
	}
}

func TestEditorBackspaceJoinsLines(t *testing.T) {
	e := NewEditor("ab\ncd")
	e.SetOffset(3)
	typeInto(e, "backspace")
	if got := e.Value(); got != "abcd" {
		t.Errorf("value = %q", got)
	}
	if e.Offset() != 2 {
		t.Errorf("offset = %d, want 2", e.Offset())
	}
}

func TestEditorOffsetsCountRunes(t *testing.T) {
	e := NewEditor("é🙂\nx")
	e.SetOffset(4)
	if row, col := e.Cursor(); row != 1 || col != 1 {
		t.Errorf("cursor = %d:%d, want 1:1", row, col)
	}
	if e.Offset() != 4 {
		t.Errorf("offset = %d", e.Offset())
	}
}

// TestEditorUndoesTypingAsOneStep is what makes undo usable: a word typed is
// one thing to take back, not one per letter.
func TestEditorUndoesTypingAsOneStep(t *testing.T) {
	e := NewEditor(".a")
	e.SetOffset(2)
	typeInto(e, " ", "|", " ", "s", "h", "a")
	typeInto(e, "backspace", "backspace")
	if e.Value() != ".a | s" {
		t.Fatalf("value = %q", e.Value())
	}
	e.Undo()
	if e.Value() != ".a | sha" {
		t.Errorf("first undo takes back the deletion, got %q", e.Value())
	}
	e.Undo()
	if e.Value() != ".a" {
		t.Errorf("second undo takes back the typing, got %q", e.Value())
	}
	e.Redo()
	if e.Value() != ".a | sha" {
		t.Errorf("redo = %q", e.Value())
	}
}

func TestEditorSetValueIsUndoable(t *testing.T) {
	e := NewEditor(".a|.b")
	e.SetValue(".a\n| .b")
	e.Undo()
	if e.Value() != ".a|.b" {
		t.Errorf("value = %q", e.Value())
	}
}

func TestEditorReplaceSwapsTheWord(t *testing.T) {
	e := NewEditor(".a | sha2")
	e.Replace(5, 9, "sha256")
	if e.Value() != ".a | sha256" || e.Offset() != 11 {
		t.Errorf("value = %q, offset = %d", e.Value(), e.Offset())
	}
}

func TestEditorCommentToggles(t *testing.T) {
	e := NewEditor("  .a")
	e.ToggleComment()
	if e.Value() != "  # .a" {
		t.Fatalf("value = %q", e.Value())
	}
	e.ToggleComment()
	if e.Value() != "  .a" {
		t.Errorf("value = %q", e.Value())
	}
}

func TestEditorWordKeys(t *testing.T) {
	e := NewEditor("get_childitem(\".\")")
	e.SetOffset(13)
	typeInto(e, "ctrl+w")
	if e.Value() != "(\".\")" {
		t.Errorf("ctrl+w = %q", e.Value())
	}
	typeInto(e, "ctrl+k")
	if e.Value() != "" {
		t.Errorf("ctrl+k = %q", e.Value())
	}
}

func TestEditorIgnoresAltKeys(t *testing.T) {
	e := NewEditor("")
	if handled, _ := e.HandleKey(key("alt+f"), 10); handled {
		t.Error("alt+f belongs to the application, not the buffer")
	}
	if e.Value() != "" {
		t.Errorf("value = %q", e.Value())
	}
}

// TestEditorRenderFillsTheView keeps the layout square: every line is exactly
// as wide as asked, whatever runes are on it.
func TestEditorRenderFillsTheView(t *testing.T) {
	th := plainTheme()
	e := NewEditor("日本語 and \t tabs 🙂\nshort")
	for _, line := range e.Render(th, 20, 4, true, true, nil, nil) {
		if w := ansi.StringWidth(line); w != 20 {
			t.Errorf("line %q is %d cells, want 20", line, w)
		}
	}
}

func TestEditorScrollsToTheCursor(t *testing.T) {
	th := plainTheme()
	e := NewEditor(strings.Repeat("x", 50) + "END")
	e.SetOffset(53)
	lines := e.Render(th, 20, 1, true, false, nil, nil)
	if !strings.Contains(lines[0], "END") {
		t.Errorf("the cursor's end of the line should be visible, got %q", lines[0])
	}

	e = NewEditor(strings.Repeat("line\n", 30) + "last")
	e.SetOffset(len(e.Value()))
	lines = e.Render(th, 20, 5, true, false, nil, nil)
	if !strings.Contains(lines[4], "last") {
		t.Errorf("the cursor's line should be visible, got %q", lines)
	}
}

func TestEditorKeepsVerticalGoal(t *testing.T) {
	e := NewEditor("long line here\nab\nanother long line")
	e.SetOffset(10)
	typeInto(e, "down", "down")
	if row, col := e.Cursor(); row != 2 || col != 10 {
		t.Errorf("cursor = %d:%d, want 2:10", row, col)
	}
}

// TestEditorDrawsAHugeLineCheaply: a minified document is one line, and
// drawing or moving along it must not cost the whole line every time.
func TestEditorDrawsAHugeLineCheaply(t *testing.T) {
	th := plainTheme()
	huge := `{"k":"` + strings.Repeat("é", 4<<20) + `","end":"END"}`
	e := NewEditor(huge)

	allocs := testing.AllocsPerRun(5, func() {
		e.HandleKey(key("right"), 10)
		e.Render(th, 80, 5, true, true, func(int, []rune) []string { return nil }, nil)
	})
	if allocs > 2000 {
		t.Errorf("%.0f allocations per keystroke and frame", allocs)
	}

	e.SetOffset(runeLen(huge))
	lines := e.Render(th, 80, 1, true, false, nil, nil)
	if !strings.Contains(lines[0], `"END"}`) {
		t.Errorf("the end of the line should be drawn at the cursor, got %q", lines[0])
	}
	if w := ansi.StringWidth(lines[0]); w != 80 {
		t.Errorf("line is %d wide", w)
	}
}
